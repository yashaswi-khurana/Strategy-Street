package metadata_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strategy-street/api/internal/store/metadata"
)

func initDockerHost() {
	if os.Getenv("DOCKER_HOST") == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			desktopSock := filepath.Join(home, ".docker", "desktop", "docker.sock")
			if _, err := os.Stat(desktopSock); err == nil {
				os.Setenv("DOCKER_HOST", "unix://"+desktopSock)
			}
		}
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "Makefile")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repository root containing Makefile")
		}
		dir = parent
	}
}

func setupTestContainer(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping container integration test in short mode")
	}

	initDockerHost()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pgContainer, err := tcpostgres.Run(ctx,
		"timescale/timescaledb:2.15.3-pg16",
		tcpostgres.WithDatabase("strategystreet_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed starting postgres testcontainer: %v", err)
	}

	cleanup := func() {
		_ = pgContainer.Terminate(context.Background())
	}

	connStr, err := pgContainer.ConnectionString(context.Background(), "sslmode=disable")
	if err != nil {
		cleanup()
		t.Fatalf("failed getting connection string: %v", err)
	}

	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		cleanup()
		t.Fatalf("failed parsing pool config: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		cleanup()
		t.Fatalf("failed creating pool: %v", err)
	}

	// Apply migrations
	repoRoot := findRepoRoot(t)
	migrationFile := filepath.Join(repoRoot, "db", "migrations", "0001_metadata.up.sql")
	migrationSQL, err := os.ReadFile(migrationFile)
	if err != nil {
		pool.Close()
		cleanup()
		t.Fatalf("failed reading migration %s: %v", migrationFile, err)
	}

	if _, err := pool.Exec(context.Background(), string(migrationSQL)); err != nil {
		pool.Close()
		cleanup()
		t.Fatalf("failed executing migration: %v", err)
	}

	// Apply seeds in order
	seedsPattern := filepath.Join(repoRoot, "db", "seeds", "*.sql")
	seedFiles, err := filepath.Glob(seedsPattern)
	if err != nil || len(seedFiles) == 0 {
		pool.Close()
		cleanup()
		t.Fatalf("failed globbing seed files %s: %v", seedsPattern, err)
	}
	sort.Strings(seedFiles)

	for _, sf := range seedFiles {
		sql, err := os.ReadFile(sf)
		if err != nil {
			pool.Close()
			cleanup()
			t.Fatalf("failed reading seed %s: %v", sf, err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			pool.Close()
			cleanup()
			t.Fatalf("failed executing seed %s: %v", sf, err)
		}
	}

	// Re-run seeds to verify idempotency (Acceptance Criterion 2)
	for _, sf := range seedFiles {
		sql, err := os.ReadFile(sf)
		if err != nil {
			pool.Close()
			cleanup()
			t.Fatalf("failed re-reading seed %s: %v", sf, err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			pool.Close()
			cleanup()
			t.Fatalf("failed re-executing seed for idempotency check %s: %v", sf, err)
		}
	}

	return pool, func() {
		pool.Close()
		cleanup()
	}
}

func TestMetadataStore_Integration(t *testing.T) {
	pool, cleanup := setupTestContainer(t)
	defer cleanup()

	ctx := context.Background()
	store := metadata.NewStore(pool)

	t.Run("Acceptance Criterion 3: count stocks with sector_id IS NULL is 0", func(t *testing.T) {
		var nullCount int
		err := pool.QueryRow(ctx, "SELECT count(*) FROM stocks WHERE sector_id IS NULL;").Scan(&nullCount)
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if nullCount != 0 {
			t.Fatalf("expected 0 stocks with NULL sector_id, got %d", nullCount)
		}

		var totalStocks int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM stocks;").Scan(&totalStocks)
		if totalStocks < 50 {
			t.Fatalf("expected at least 50 seeded stocks, got %d", totalStocks)
		}

		var totalSectors int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM sectors;").Scan(&totalSectors)
		if totalSectors != 12 {
			t.Fatalf("expected 12 sectors, got %d", totalSectors)
		}
	})

	t.Run("Acceptance Criterion 4: lookup by symbol returns correct instrument_key and typed ErrNotFound", func(t *testing.T) {
		// Existing stock: RELIANCE
		stock, err := store.GetStockBySymbol(ctx, "RELIANCE")
		if err != nil {
			t.Fatalf("unexpected error fetching RELIANCE: %v", err)
		}
		if stock.InstrumentKey != "NSE_EQ|INE002A01018" {
			t.Fatalf("expected instrument key 'NSE_EQ|INE002A01018', got %q", stock.InstrumentKey)
		}
		if stock.SectorName == nil || *stock.SectorName != "Energy" {
			t.Fatalf("expected sector 'Energy', got %v", stock.SectorName)
		}
		if stock.TickSize != 0.05 {
			t.Fatalf("expected tick size 0.05 for RELIANCE, got %f", stock.TickSize)
		}
		if stock.ListingDate == nil {
			t.Fatal("expected non-nil ListingDate for RELIANCE")
		}
		if stock.ListingDate.Location().String() != "Asia/Kolkata" {
			t.Fatalf("expected ListingDate in Asia/Kolkata, got %v", stock.ListingDate.Location())
		}
		if stock.ListingDate.Format("2006-01-02") != "1995-11-29" {
			t.Fatalf("expected ListingDate 1995-11-29, got %s", stock.ListingDate.Format("2006-01-02"))
		}
		if stock.CreatedAt.Location().String() != "Asia/Kolkata" {
			t.Fatalf("expected CreatedAt in Asia/Kolkata, got %v", stock.CreatedAt.Location())
		}

		// Sub-250 stock: TATASTEEL (tick_size 0.01)
		tatasteel, err := store.GetStockBySymbol(ctx, "TATASTEEL")
		if err != nil {
			t.Fatalf("unexpected error fetching TATASTEEL: %v", err)
		}
		if tatasteel.InstrumentKey != "NSE_EQ|INE081A01020" {
			t.Fatalf("expected instrument key 'NSE_EQ|INE081A01020', got %q", tatasteel.InstrumentKey)
		}
		if tatasteel.TickSize != 0.01 {
			t.Fatalf("expected tick size 0.01 for TATASTEEL, got %f", tatasteel.TickSize)
		}

		// Unknown symbol returns typed ErrNotFound
		missing, err := store.GetStockBySymbol(ctx, "NON_EXISTENT_SYMBOL")
		if missing != nil {
			t.Fatalf("expected nil stock for unknown symbol, got %+v", missing)
		}
		if err == nil {
			t.Fatal("expected error for unknown symbol, got nil")
		}
		if !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("expected errors.Is(err, ErrNotFound), got %v", err)
		}
		if !errors.Is(err, &metadata.NotFoundError{Entity: "stock"}) {
			t.Fatalf("expected errors.Is(err, &NotFoundError{Entity: 'stock'}), got %v", err)
		}
		var nfErr *metadata.NotFoundError
		if !errors.As(err, &nfErr) {
			t.Fatalf("expected *NotFoundError, got %T (%v)", err, err)
		}
	})

	t.Run("Lookup by instrument_key", func(t *testing.T) {
		stock, err := store.GetStockByInstrumentKey(ctx, "NSE_EQ|INE467B01029")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stock.Symbol != "TCS" {
			t.Fatalf("expected symbol TCS, got %s", stock.Symbol)
		}

		_, err = store.GetStockByInstrumentKey(ctx, "NSE_EQ|UNKNOWN")
		if !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for unknown instrument key, got %v", err)
		}
	})

	t.Run("List sectors", func(t *testing.T) {
		sectors, err := store.ListSectors(ctx)
		if err != nil {
			t.Fatalf("unexpected error listing sectors: %v", err)
		}
		if len(sectors) != 12 {
			t.Fatalf("expected 12 sectors, got %d", len(sectors))
		}
		for _, s := range sectors {
			if s.CreatedAt.Location().String() != "Asia/Kolkata" {
				t.Fatalf("expected sector CreatedAt in Asia/Kolkata, got %v", s.CreatedAt.Location())
			}
		}
	})

	t.Run("List stocks by sector", func(t *testing.T) {
		itStocks, err := store.ListStocksBySector(ctx, "IT")
		if err != nil {
			t.Fatalf("unexpected error listing IT stocks: %v", err)
		}
		if len(itStocks) < 5 {
			t.Fatalf("expected at least 5 IT stocks, got %d", len(itStocks))
		}
		for _, st := range itStocks {
			if st.SectorName == nil || *st.SectorName != "IT" {
				t.Fatalf("expected sector IT, got %v for stock %s", st.SectorName, st.Symbol)
			}
		}

		// Non-existent sector returns empty slice without error
		missing, err := store.ListStocksBySector(ctx, "NON_EXISTENT_SECTOR")
		if err != nil {
			t.Fatalf("unexpected error for non-existent sector: %v", err)
		}
		if len(missing) != 0 {
			t.Fatalf("expected 0 stocks for non-existent sector, got %d", len(missing))
		}
	})

	t.Run("Acceptance Criterion 5: IsTradingDay using DB data only", func(t *testing.T) {
		// 2027-01-26: Republic Day (Tuesday) -> false
		republicDay2027 := time.Date(2027, time.January, 26, 10, 0, 0, 0, metadata.LocationIST)
		isTD, err := store.IsTradingDay(ctx, republicDay2027)
		if err != nil {
			t.Fatalf("IsTradingDay(2027-01-26) error: %v", err)
		}
		if isTD {
			t.Fatalf("expected IsTradingDay('2027-01-26') to be false (Republic Day), got true")
		}

		// 2027-01-27: Normal Wednesday -> true
		normalDay2027 := time.Date(2027, time.January, 27, 10, 0, 0, 0, metadata.LocationIST)
		isTD, err = store.IsTradingDay(ctx, normalDay2027)
		if err != nil {
			t.Fatalf("IsTradingDay(2027-01-27) error: %v", err)
		}
		if !isTD {
			t.Fatalf("expected IsTradingDay('2027-01-27') to be true (Wednesday), got false")
		}

		// 2027-01-30: Saturday -> false
		saturday2027 := time.Date(2027, time.January, 30, 10, 0, 0, 0, metadata.LocationIST)
		isTD, err = store.IsTradingDay(ctx, saturday2027)
		if err != nil {
			t.Fatalf("IsTradingDay(2027-01-30) error: %v", err)
		}
		if isTD {
			t.Fatalf("expected IsTradingDay('2027-01-30') to be false (Saturday), got true")
		}

		// 2026-11-08: Diwali Muhurat on Sunday -> true (special session in market_sessions)
		muhurat2026 := time.Date(2026, time.November, 8, 18, 30, 0, 0, metadata.LocationIST)
		isTD, err = store.IsTradingDay(ctx, muhurat2026)
		if err != nil {
			t.Fatalf("IsTradingDay(2026-11-08) error: %v", err)
		}
		if !isTD {
			t.Fatalf("expected IsTradingDay('2026-11-08') to be true (Muhurat session), got false")
		}

		// UTC 18:00 on 2027-01-26 is 23:30 on 2027-01-26 IST (Republic Day) -> false
		utcRepublicDay := time.Date(2027, time.January, 26, 18, 0, 0, 0, time.UTC)
		isTD, err = store.IsTradingDay(ctx, utcRepublicDay)
		if err != nil {
			t.Fatalf("IsTradingDay(UTC 18:00 Jan 26) error: %v", err)
		}
		if isTD {
			t.Fatalf("expected IsTradingDay to be false at UTC 18:00 Jan 26 (Jan 26 IST), got true")
		}

		// UTC 19:00 on 2027-01-26 is 00:30 on 2027-01-27 IST (Wednesday) -> true
		utcWednesday := time.Date(2027, time.January, 26, 19, 0, 0, 0, time.UTC)
		isTD, err = store.IsTradingDay(ctx, utcWednesday)
		if err != nil {
			t.Fatalf("IsTradingDay(UTC 19:00 Jan 26) error: %v", err)
		}
		if !isTD {
			t.Fatalf("expected IsTradingDay to be true at UTC 19:00 Jan 26 (Jan 27 IST), got false")
		}
	})

	t.Run("SessionFor dates", func(t *testing.T) {
		// Muhurat session
		muhuratDate := time.Date(2026, time.November, 8, 0, 0, 0, 0, metadata.LocationIST)
		sess, err := store.SessionFor(ctx, muhuratDate)
		if err != nil {
			t.Fatalf("unexpected error for muhurat session: %v", err)
		}
		if sess.OpenTime != "18:15:00" || sess.CloseTime != "19:15:00" {
			t.Fatalf("expected 18:15:00 - 19:15:00, got %s - %s", sess.OpenTime, sess.CloseTime)
		}
		if sess.Label != "muhurat" {
			t.Fatalf("expected label 'muhurat', got %q", sess.Label)
		}

		// Regular trading day
		regularDate := time.Date(2027, time.January, 27, 0, 0, 0, 0, metadata.LocationIST)
		regSess, err := store.SessionFor(ctx, regularDate)
		if err != nil {
			t.Fatalf("unexpected error for regular session: %v", err)
		}
		if regSess.OpenTime != "09:15:00" || regSess.CloseTime != "15:30:00" {
			t.Fatalf("expected 09:15:00 - 15:30:00, got %s - %s", regSess.OpenTime, regSess.CloseTime)
		}
		if regSess.Label != "regular" {
			t.Fatalf("expected label 'regular', got %q", regSess.Label)
		}

		// Republic Day holiday -> ErrNotFound
		republicDay := time.Date(2027, time.January, 26, 0, 0, 0, 0, metadata.LocationIST)
		_, err = store.SessionFor(ctx, republicDay)
		if !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for holiday session, got %v", err)
		}
		if !errors.Is(err, &metadata.NotFoundError{Entity: "market_session"}) {
			t.Fatalf("expected *NotFoundError for holiday session, got %v", err)
		}

		// Weekend -> ErrNotFound
		weekend := time.Date(2027, time.January, 30, 0, 0, 0, 0, metadata.LocationIST)
		_, err = store.SessionFor(ctx, weekend)
		if !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for weekend session, got %v", err)
		}

		// Rollover: UTC 19:00 Jan 26 is Jan 27 IST (Wednesday regular session)
		utcWednesday := time.Date(2027, time.January, 26, 19, 0, 0, 0, time.UTC)
		utcSess, err := store.SessionFor(ctx, utcWednesday)
		if err != nil {
			t.Fatalf("unexpected error for UTC 19:00 Jan 26 session: %v", err)
		}
		if utcSess.Label != "regular" || utcSess.OpenTime != "09:15:00" {
			t.Fatalf("expected regular session, got %+v", utcSess)
		}
	})
}

func TestMigrations_DownAndUpCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container integration test in short mode")
	}
	initDockerHost()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pgContainer, err := tcpostgres.Run(ctx,
		"timescale/timescaledb:2.15.3-pg16",
		tcpostgres.WithDatabase("strategystreet_mig_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed starting postgres testcontainer: %v", err)
	}
	defer func() {
		_ = pgContainer.Terminate(context.Background())
	}()

	connStr, err := pgContainer.ConnectionString(context.Background(), "sslmode=disable")
	if err != nil {
		t.Fatalf("failed getting connection string: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("failed connecting: %v", err)
	}
	defer pool.Close()

	repoRoot := findRepoRoot(t)
	upSQL, err := os.ReadFile(filepath.Join(repoRoot, "db", "migrations", "0001_metadata.up.sql"))
	if err != nil {
		t.Fatalf("failed reading up migration: %v", err)
	}
	downSQL, err := os.ReadFile(filepath.Join(repoRoot, "db", "migrations", "0001_metadata.down.sql"))
	if err != nil {
		t.Fatalf("failed reading down migration: %v", err)
	}

	// 1. Initial Up
	if _, err := pool.Exec(context.Background(), string(upSQL)); err != nil {
		t.Fatalf("initial migration up failed: %v", err)
	}

	var tableCount int
	err = pool.QueryRow(context.Background(), "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('sectors', 'stocks', 'exchange_holidays', 'market_sessions');").Scan(&tableCount)
	if err != nil || tableCount != 4 {
		t.Fatalf("expected 4 tables after up migration, got %d (err: %v)", tableCount, err)
	}

	// 2. Down
	if _, err := pool.Exec(context.Background(), string(downSQL)); err != nil {
		t.Fatalf("migration down failed: %v", err)
	}

	err = pool.QueryRow(context.Background(), "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('sectors', 'stocks', 'exchange_holidays', 'market_sessions');").Scan(&tableCount)
	if err != nil || tableCount != 0 {
		t.Fatalf("expected 0 tables after down migration, got %d (err: %v)", tableCount, err)
	}

	// 3. Re-Up
	if _, err := pool.Exec(context.Background(), string(upSQL)); err != nil {
		t.Fatalf("re-migration up failed: %v", err)
	}
	err = pool.QueryRow(context.Background(), "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('sectors', 'stocks', 'exchange_holidays', 'market_sessions');").Scan(&tableCount)
	if err != nil || tableCount != 4 {
		t.Fatalf("expected 4 tables after re-up migration, got %d (err: %v)", tableCount, err)
	}
}
