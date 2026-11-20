package candles_test

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strategy-street/api/internal/store/candles"
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

func init() {
	initDockerHost()
}

func findRepoRoot(t testing.TB) string {
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

func setupTestContainer(t testing.TB) (*pgxpool.Pool, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping container integration test in short mode")
	}

	initDockerHost()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var pgContainer *tcpostgres.PostgresContainer
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("skipping container integration test: docker unavailable (%v)", r)
			}
		}()
		var runErr error
		pgContainer, runErr = tcpostgres.Run(ctx,
			"timescale/timescaledb:2.15.3-pg16",
			tcpostgres.WithDatabase("strategystreet_candles_test"),
			tcpostgres.WithUsername("postgres"),
			tcpostgres.WithPassword("postgres"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second),
			),
		)
		if runErr != nil {
			t.Skipf("skipping container integration test: docker unavailable: %v", runErr)
		}
	}()
	if t.Skipped() || pgContainer == nil {
		return nil, func() {}
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

	repoRoot := findRepoRoot(t)

	// Apply migration 0001 (metadata)
	m1File := filepath.Join(repoRoot, "db", "migrations", "0001_metadata.up.sql")
	m1SQL, err := os.ReadFile(m1File)
	if err != nil {
		pool.Close()
		cleanup()
		t.Fatalf("failed reading %s: %v", m1File, err)
	}
	if _, err := pool.Exec(context.Background(), string(m1SQL)); err != nil {
		pool.Close()
		cleanup()
		t.Fatalf("failed executing migration 0001: %v", err)
	}

	// Apply seeds so stocks exist for foreign key checks
	seedsPattern := filepath.Join(repoRoot, "db", "seeds", "*.sql")
	seedFiles, err := filepath.Glob(seedsPattern)
	if err != nil || len(seedFiles) == 0 {
		pool.Close()
		cleanup()
		t.Fatalf("failed globbing seeds: %v", err)
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

	// Apply migration 0002 (candles)
	m2File := filepath.Join(repoRoot, "db", "migrations", "0002_candles.up.sql")
	m2SQL, err := os.ReadFile(m2File)
	if err != nil {
		pool.Close()
		cleanup()
		t.Fatalf("failed reading %s: %v", m2File, err)
	}
	if _, err := pool.Exec(context.Background(), string(m2SQL)); err != nil {
		pool.Close()
		cleanup()
		t.Fatalf("failed executing migration 0002: %v", err)
	}

	// Apply migration 0005 (is_filled column)
	m5File := filepath.Join(repoRoot, "db", "migrations", "0005_is_filled.up.sql")
	m5SQL, err := os.ReadFile(m5File)
	if err != nil {
		pool.Close()
		cleanup()
		t.Fatalf("failed reading %s: %v", m5File, err)
	}
	if _, err := pool.Exec(context.Background(), string(m5SQL)); err != nil {
		pool.Close()
		cleanup()
		t.Fatalf("failed executing migration 0005: %v", err)
	}

	return pool, func() {
		pool.Close()
		cleanup()
	}
}

func TestCandleStore_Integration(t *testing.T) {
	pool, cleanup := setupTestContainer(t)
	defer cleanup()

	ctx := context.Background()
	store := candles.NewStore(pool)

	t.Run("Acceptance Criterion 1: conflict-update updates in place and count(*) stays 1", func(t *testing.T) {
		tBar := time.Date(2026, time.November, 18, 9, 15, 0, 0, candles.LocationIST).UTC()

		barInitial, err := candles.NewCandleFromFloats(1, tBar, 2500.0, 2515.0, 2495.0, 2510.0, 5000, 1200)
		if err != nil {
			t.Fatalf("failed creating initial candle: %v", err)
		}

		// First upsert
		if err := store.UpsertBatch(ctx, candles.Timeframe1m, []candles.Candle{barInitial}); err != nil {
			t.Fatalf("first UpsertBatch failed: %v", err)
		}

		var count int
		err = pool.QueryRow(ctx, "SELECT count(*) FROM candles_1m WHERE stock_id = 1 AND bucket_time = $1;", tBar).Scan(&count)
		if err != nil || count != 1 {
			t.Fatalf("expected count(*) = 1 after first insert, got %d (err: %v)", count, err)
		}

		// Second upsert with updated OHLCV values for the exact same (stock_id, bucket_time)
		barUpdated, err := candles.NewCandleFromFloats(1, tBar, 2500.0, 2530.0, 2490.0, 2525.0, 10000, 2500)
		if err != nil {
			t.Fatalf("failed creating updated candle: %v", err)
		}

		if err := store.UpsertBatch(ctx, candles.Timeframe1m, []candles.Candle{barUpdated}); err != nil {
			t.Fatalf("second UpsertBatch failed: %v", err)
		}

		// Verify count remains 1
		err = pool.QueryRow(ctx, "SELECT count(*) FROM candles_1m WHERE stock_id = 1 AND bucket_time = $1;", tBar).Scan(&count)
		if err != nil || count != 1 {
			t.Fatalf("expected count(*) to stay 1 after second insert, got %d (err: %v)", count, err)
		}

		// Verify row updated in place
		latest, err := store.Latest(ctx, 1, candles.Timeframe1m)
		if err != nil {
			t.Fatalf("Latest failed: %v", err)
		}
		cPrice, _ := latest.CloseFloat()
		if cPrice != 2525.0 {
			t.Fatalf("expected close price updated to 2525.0, got %f", cPrice)
		}
		hPrice, _ := latest.HighFloat()
		if hPrice != 2530.0 {
			t.Fatalf("expected high price updated to 2530.0, got %f", hPrice)
		}
		if latest.Volume != 10000 {
			t.Fatalf("expected volume updated to 10000, got %d", latest.Volume)
		}
	})

	t.Run("Acceptance Criterion 2: Range for 5-day window returns exactly the inserted bars in ascending order", func(t *testing.T) {
		const stockID = 2
		startDate := time.Date(2026, time.November, 16, 9, 15, 0, 0, candles.LocationIST) // Monday

		// Generate 5 days of 1-minute bars (375 bars per day = 1875 bars)
		var insertedBars []candles.Candle
		cur := startDate
		for day := 0; day < 5; day++ {
			dayStart := time.Date(cur.Year(), cur.Month(), cur.Day(), 9, 15, 0, 0, candles.LocationIST)
			for m := 0; m < 375; m++ {
				barTime := dayStart.Add(time.Duration(m) * time.Minute).UTC()
				bar, err := candles.NewCandleFromFloats(stockID, barTime, 100.0+float64(m), 101.0+float64(m), 99.0+float64(m), 100.5+float64(m), int64(m*10), 0)
				if err != nil {
					t.Fatalf("error generating bar: %v", err)
				}
				insertedBars = append(insertedBars, bar)
			}
			cur = cur.Add(24 * time.Hour)
		}

		if err := store.UpsertBatch(ctx, candles.Timeframe1m, insertedBars); err != nil {
			t.Fatalf("failed inserting 5-day bars: %v", err)
		}

		rangeFrom := insertedBars[0].BucketTime
		rangeTo := insertedBars[len(insertedBars)-1].BucketTime

		gotBars, err := store.Range(ctx, stockID, candles.Timeframe1m, rangeFrom, rangeTo)
		if err != nil {
			t.Fatalf("Range query failed: %v", err)
		}

		if len(gotBars) != len(insertedBars) {
			t.Fatalf("expected %d bars, got %d", len(insertedBars), len(gotBars))
		}

		// Verify ascending order and value match
		for i := range gotBars {
			if !gotBars[i].BucketTime.Equal(insertedBars[i].BucketTime) {
				t.Fatalf("bar %d timestamp mismatch: expected %s, got %s", i, insertedBars[i].BucketTime, gotBars[i].BucketTime)
			}
			if i > 0 && !gotBars[i].BucketTime.After(gotBars[i-1].BucketTime) {
				t.Fatalf("bar %d timestamp is not strictly ascending: %s <= %s", i, gotBars[i].BucketTime, gotBars[i-1].BucketTime)
			}
		}

		// Verify Latest and LatestTime
		latest, err := store.Latest(ctx, stockID, candles.Timeframe1m)
		if err != nil {
			t.Fatalf("Latest failed: %v", err)
		}
		if !latest.BucketTime.Equal(rangeTo) {
			t.Fatalf("expected latest time %s, got %s", rangeTo, latest.BucketTime)
		}

		latestTime, err := store.LatestTime(ctx, stockID, candles.Timeframe1m)
		if err != nil {
			t.Fatalf("LatestTime failed: %v", err)
		}
		if !latestTime.Equal(rangeTo) {
			t.Fatalf("expected latestTime %s, got %s", rangeTo, latestTime)
		}
	})

	t.Run("Acceptance Criterion 3: 09:15:30 on 1m rejected before DB", func(t *testing.T) {
		misaligned := time.Date(2026, time.November, 18, 9, 15, 30, 0, candles.LocationIST).UTC()
		bar, err := candles.NewCandleFromFloats(1, misaligned, 2500, 2510, 2490, 2505, 100, 0)
		if err != nil {
			t.Fatalf("failed creating candle: %v", err)
		}

		err = store.UpsertBatch(ctx, candles.Timeframe1m, []candles.Candle{bar})
		if err == nil {
			t.Fatal("expected validation error from UpsertBatch for misaligned bar, got nil")
		}
		if !errors.Is(err, candles.ErrMisalignedBar) {
			t.Fatalf("expected ErrMisalignedBar, got %v", err)
		}

		err = store.CopyBatch(ctx, candles.Timeframe1m, []candles.Candle{bar})
		if err == nil {
			t.Fatal("expected validation error from CopyBatch for misaligned bar, got nil")
		}
		if !errors.Is(err, candles.ErrMisalignedBar) {
			t.Fatalf("expected ErrMisalignedBar, got %v", err)
		}
	})

	t.Run("Acceptance Criterion 4: CopyBatch of 100k rows completes < 5s locally and is idempotent", func(t *testing.T) {
		const totalRows = 100000
		const numStocks = 10 // stock_id 3 through 12
		const rowsPerStock = totalRows / numStocks

		bars := make([]candles.Candle, totalRows)
		baseDate := time.Date(2025, time.January, 1, 9, 15, 0, 0, candles.LocationIST)

		idx := 0
		for s := 3; s < 3+numStocks; s++ {
			for m := 0; m < rowsPerStock; m++ {
				// Each bar is separated by 1 minute
				bt := baseDate.Add(time.Duration(m) * time.Minute).UTC()
				bar, err := candles.NewCandleFromFloats(s, bt, 1000.0, 1010.0, 995.0, 1005.0, 500, 50)
				if err != nil {
					t.Fatalf("error generating bar: %v", err)
				}
				bars[idx] = bar
				idx++
			}
		}

		// Measure CopyBatch duration
		start := time.Now()
		if err := store.CopyBatch(ctx, candles.Timeframe1m, bars); err != nil {
			t.Fatalf("CopyBatch failed on 100k rows: %v", err)
		}
		duration := time.Since(start)
		t.Logf("CopyBatch 100,000 rows completed in %v", duration)

		if duration > 5*time.Second {
			t.Fatalf("Acceptance Criterion 4 violated: CopyBatch took %v (limit: 5s)", duration)
		}

		// Re-run CopyBatch with identical 100k rows to verify idempotency
		startReRun := time.Now()
		if err := store.CopyBatch(ctx, candles.Timeframe1m, bars); err != nil {
			t.Fatalf("re-running CopyBatch failed: %v", err)
		}
		t.Logf("Idempotent re-run completed in %v", time.Since(startReRun))

		var countAfter int
		err := pool.QueryRow(ctx, "SELECT count(*) FROM candles_1m WHERE stock_id BETWEEN 3 AND 12;").Scan(&countAfter)
		if err != nil {
			t.Fatalf("count query failed: %v", err)
		}
		if countAfter != totalRows {
			t.Fatalf("expected count(*) = %d after idempotent re-run, got %d", totalRows, countAfter)
		}
	})

	t.Run("Acceptance Criterion 5: EXPLAIN for Range uses the (stock_id, bucket_time) index", func(t *testing.T) {
		from := time.Date(2025, time.January, 1, 9, 15, 0, 0, candles.LocationIST).UTC()
		to := time.Date(2025, time.January, 5, 15, 30, 0, 0, candles.LocationIST).UTC()

		query := `
			EXPLAIN
			SELECT stock_id, bucket_time, open, high, low, close, volume, oi, is_adjusted
			FROM candles_1m
			WHERE stock_id = $1 AND bucket_time >= $2 AND bucket_time <= $3
			ORDER BY bucket_time ASC;
		`

		rows, err := pool.Query(ctx, query, 3, from, to)
		if err != nil {
			t.Fatalf("EXPLAIN failed: %v", err)
		}
		defer rows.Close()

		var explainPlan strings.Builder
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatalf("failed scanning explain line: %v", err)
			}
			explainPlan.WriteString(line)
			explainPlan.WriteString("\n")
		}

		planStr := explainPlan.String()
		t.Logf("EXPLAIN Query Plan:\n%s", planStr)

		// Plan should utilize Index Scan, Index Only Scan, or Bitmap Index Scan on chunk tables
		hasIndexScan := strings.Contains(planStr, "Index Scan") ||
			strings.Contains(planStr, "Index Only Scan") ||
			strings.Contains(planStr, "Bitmap Index Scan")

		if !hasIndexScan {
			t.Fatalf("Acceptance Criterion 5 violated: EXPLAIN does not show index scan. Plan:\n%s", planStr)
		}
	})

	t.Run("Property test: Random bar sets round-trip identically via UpsertBatch and Range", func(t *testing.T) {
		const stockID = 15
		baseTime := time.Date(2026, time.June, 1, 9, 15, 0, 0, candles.LocationIST)
		r := rand.New(rand.NewSource(42))

		numBars := 150
		bars := make([]candles.Candle, numBars)
		for i := 0; i < numBars; i++ {
			bt := baseTime.Add(time.Duration(i) * time.Minute).UTC()
			open := 500.0 + r.Float64()*100
			high := open + r.Float64()*10
			low := open - r.Float64()*10
			closeP := low + r.Float64()*(high-low)
			vol := int64(100 + r.Intn(10000))
			oi := int64(r.Intn(5000))

			bar, err := candles.NewCandleFromFloats(stockID, bt, open, high, low, closeP, vol, oi)
			if err != nil {
				t.Fatalf("error generating property test bar %d: %v", i, err)
			}
			bars[i] = bar
		}

		if err := store.UpsertBatch(ctx, candles.Timeframe1m, bars); err != nil {
			t.Fatalf("property test UpsertBatch failed: %v", err)
		}

		got, err := store.Range(ctx, stockID, candles.Timeframe1m, bars[0].BucketTime, bars[len(bars)-1].BucketTime)
		if err != nil {
			t.Fatalf("property test Range failed: %v", err)
		}

		if len(got) != len(bars) {
			t.Fatalf("property test bar count mismatch: expected %d, got %d", len(bars), len(got))
		}

		for i := range bars {
			if !got[i].BucketTime.Equal(bars[i].BucketTime) {
				t.Fatalf("timestamp mismatch at %d: %s vs %s", i, got[i].BucketTime, bars[i].BucketTime)
			}
			oG, _ := got[i].OpenFloat()
			oE, _ := bars[i].OpenFloat()
			if oG != oE {
				t.Fatalf("open mismatch at %d: got %f, want %f", i, oG, oE)
			}
			cG, _ := got[i].CloseFloat()
			cE, _ := bars[i].CloseFloat()
			if cG != cE {
				t.Fatalf("close mismatch at %d: got %f, want %f", i, cG, cE)
			}
			if got[i].Volume != bars[i].Volume {
				t.Fatalf("volume mismatch at %d: got %d, want %d", i, got[i].Volume, bars[i].Volume)
			}
			if got[i].OI != bars[i].OI {
				t.Fatalf("oi mismatch at %d: got %d, want %d", i, got[i].OI, bars[i].OI)
			}
		}
	})

	t.Run("candles_1s insertion and range", func(t *testing.T) {
		const stockID = 1
		t0 := time.Date(2026, time.November, 18, 9, 15, 0, 0, candles.LocationIST).UTC()

		var secBars []candles.Candle
		for s := 0; s < 120; s++ {
			bt := t0.Add(time.Duration(s) * time.Second)
			bar, err := candles.NewCandleFromFloats(stockID, bt, 2500, 2502, 2498, 2501, int64(s*5), 0)
			if err != nil {
				t.Fatalf("failed generating 1s bar: %v", err)
			}
			secBars = append(secBars, bar)
		}

		if err := store.UpsertBatch(ctx, candles.Timeframe1s, secBars); err != nil {
			t.Fatalf("UpsertBatch on 1s failed: %v", err)
		}

		fetched, err := store.Range(ctx, stockID, candles.Timeframe1s, t0, t0.Add(119*time.Second))
		if err != nil {
			t.Fatalf("Range query on 1s failed: %v", err)
		}
		if len(fetched) != 120 {
			t.Fatalf("expected 120 1s bars, got %d", len(fetched))
		}
	})
}

func TestCandlesMigration_DownAndUpCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping container integration test in short mode")
	}
	initDockerHost()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var pgContainer *tcpostgres.PostgresContainer
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("skipping container integration test: docker unavailable (%v)", r)
			}
		}()
		var runErr error
		pgContainer, runErr = tcpostgres.Run(ctx,
			"timescale/timescaledb:2.15.3-pg16",
			tcpostgres.WithDatabase("strategystreet_candles_mig_test"),
			tcpostgres.WithUsername("postgres"),
			tcpostgres.WithPassword("postgres"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second),
			),
		)
		if runErr != nil {
			t.Skipf("skipping container integration test: docker unavailable: %v", runErr)
		}
	}()
	if t.Skipped() || pgContainer == nil {
		return
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

	// Apply migration 0001
	m1Up, err := os.ReadFile(filepath.Join(repoRoot, "db", "migrations", "0001_metadata.up.sql"))
	if err != nil {
		t.Fatalf("failed reading 0001 up: %v", err)
	}
	if _, err := pool.Exec(ctx, string(m1Up)); err != nil {
		t.Fatalf("0001 up failed: %v", err)
	}

	m2Up, err := os.ReadFile(filepath.Join(repoRoot, "db", "migrations", "0002_candles.up.sql"))
	if err != nil {
		t.Fatalf("failed reading 0002 up: %v", err)
	}
	m2Down, err := os.ReadFile(filepath.Join(repoRoot, "db", "migrations", "0002_candles.down.sql"))
	if err != nil {
		t.Fatalf("failed reading 0002 down: %v", err)
	}

	// 1. Initial 0002 Up
	if _, err := pool.Exec(ctx, string(m2Up)); err != nil {
		t.Fatalf("0002 up failed: %v", err)
	}

	var tableCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('candles_1s', 'candles_1m');").Scan(&tableCount)
	if err != nil || tableCount != 2 {
		t.Fatalf("expected 2 candle tables after up migration, got %d (err: %v)", tableCount, err)
	}

	// Verify they are hypertables
	var htCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM _timescaledb_catalog.hypertable WHERE table_name IN ('candles_1s', 'candles_1m');").Scan(&htCount)
	if err != nil || htCount != 2 {
		t.Fatalf("expected 2 hypertables registered in timescaledb catalog, got %d (err: %v)", htCount, err)
	}

	// 2. Down migration
	if _, err := pool.Exec(ctx, string(m2Down)); err != nil {
		t.Fatalf("0002 down failed: %v", err)
	}

	err = pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('candles_1s', 'candles_1m');").Scan(&tableCount)
	if err != nil || tableCount != 0 {
		t.Fatalf("expected 0 candle tables after down migration, got %d (err: %v)", tableCount, err)
	}

	// Verify metadata tables are intact
	var metaTableCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('sectors', 'stocks');").Scan(&metaTableCount)
	if err != nil || metaTableCount != 2 {
		t.Fatalf("expected metadata tables intact, got %d", metaTableCount)
	}

	// 3. Re-Up
	if _, err := pool.Exec(ctx, string(m2Up)); err != nil {
		t.Fatalf("0002 re-up failed: %v", err)
	}
	err = pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('candles_1s', 'candles_1m');").Scan(&tableCount)
	if err != nil || tableCount != 2 {
		t.Fatalf("expected 2 candle tables after re-up migration, got %d (err: %v)", tableCount, err)
	}
}

// BenchmarkUpsertBatch benchmarks batch upsert via multi-row INSERT ... ON CONFLICT
// with 1m bars for 2 stocks × 60 trading days (45,000 bars total).
func BenchmarkUpsertBatch(b *testing.B) {
	pool, cleanup := setupTestContainer(b)
	defer cleanup()

	ctx := context.Background()
	store := candles.NewStore(pool)

	const numStocks = 2
	const tradingDays = 60
	const barsPerDay = 375
	totalBars := numStocks * tradingDays * barsPerDay // 45,000 bars

	bars := make([]candles.Candle, totalBars)
	baseTime := time.Date(2025, time.January, 1, 9, 15, 0, 0, candles.LocationIST)

	idx := 0
	for s := 1; s <= numStocks; s++ {
		curDay := baseTime
		for d := 0; d < tradingDays; d++ {
			dayStart := time.Date(curDay.Year(), curDay.Month(), curDay.Day(), 9, 15, 0, 0, candles.LocationIST)
			for m := 0; m < barsPerDay; m++ {
				bt := dayStart.Add(time.Duration(m) * time.Minute).UTC()
				bar, err := candles.NewCandleFromFloats(s, bt, 2500.0+float64(m%50), 2510.0+float64(m%50), 2490.0+float64(m%50), 2505.0+float64(m%50), int64(m*10), 100)
				if err != nil {
					b.Fatalf("failed generating benchmark bar: %v", err)
				}
				bars[idx] = bar
				idx++
			}
			curDay = curDay.Add(24 * time.Hour)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if err := store.UpsertBatch(ctx, candles.Timeframe1m, bars); err != nil {
			b.Fatalf("UpsertBatch benchmark iteration %d failed: %v", i, err)
		}
		elapsed := time.Since(start)
		barsPerSec := float64(totalBars) / elapsed.Seconds()
		b.ReportMetric(barsPerSec, "bars/sec")
	}
}

// BenchmarkCopyBatch benchmarks high-volume COPY into temp table + merge
// with 1m bars for 2 stocks × 60 trading days (45,000 bars total).
func BenchmarkCopyBatch(b *testing.B) {
	pool, cleanup := setupTestContainer(b)
	defer cleanup()

	ctx := context.Background()
	store := candles.NewStore(pool)

	const numStocks = 2
	const tradingDays = 60
	const barsPerDay = 375
	totalBars := numStocks * tradingDays * barsPerDay // 45,000 bars

	bars := make([]candles.Candle, totalBars)
	baseTime := time.Date(2025, time.January, 1, 9, 15, 0, 0, candles.LocationIST)

	idx := 0
	for s := 1; s <= numStocks; s++ {
		curDay := baseTime
		for d := 0; d < tradingDays; d++ {
			dayStart := time.Date(curDay.Year(), curDay.Month(), curDay.Day(), 9, 15, 0, 0, candles.LocationIST)
			for m := 0; m < barsPerDay; m++ {
				bt := dayStart.Add(time.Duration(m) * time.Minute).UTC()
				bar, err := candles.NewCandleFromFloats(s, bt, 2500.0+float64(m%50), 2510.0+float64(m%50), 2490.0+float64(m%50), 2505.0+float64(m%50), int64(m*10), 100)
				if err != nil {
					b.Fatalf("failed generating benchmark bar: %v", err)
				}
				bars[idx] = bar
				idx++
			}
			curDay = curDay.Add(24 * time.Hour)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if err := store.CopyBatch(ctx, candles.Timeframe1m, bars); err != nil {
			b.Fatalf("CopyBatch benchmark iteration %d failed: %v", i, err)
		}
		elapsed := time.Since(start)
		barsPerSec := float64(totalBars) / elapsed.Seconds()
		b.ReportMetric(barsPerSec, "bars/sec")
	}
}

// BenchmarkMemoryStore_UpsertBatch benchmarks the in-memory fake used by the deterministic paper trading engine.
func BenchmarkMemoryStore_UpsertBatch(b *testing.B) {
	ctx := context.Background()
	store := candles.NewMemoryStore()

	const numStocks = 2
	const tradingDays = 60
	const barsPerDay = 375
	totalBars := numStocks * tradingDays * barsPerDay // 45,000 bars

	bars := make([]candles.Candle, totalBars)
	baseTime := time.Date(2025, time.January, 1, 9, 15, 0, 0, candles.LocationIST)

	idx := 0
	for s := 1; s <= numStocks; s++ {
		curDay := baseTime
		for d := 0; d < tradingDays; d++ {
			dayStart := time.Date(curDay.Year(), curDay.Month(), curDay.Day(), 9, 15, 0, 0, candles.LocationIST)
			for m := 0; m < barsPerDay; m++ {
				bt := dayStart.Add(time.Duration(m) * time.Minute).UTC()
				bar, err := candles.NewCandleFromFloats(s, bt, 2500.0, 2510.0, 2490.0, 2505.0, 1000, 100)
				if err != nil {
					b.Fatalf("failed generating bar: %v", err)
				}
				bars[idx] = bar
				idx++
			}
			curDay = curDay.Add(24 * time.Hour)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if err := store.UpsertBatch(ctx, candles.Timeframe1m, bars); err != nil {
			b.Fatalf("MemoryStore upsert failed: %v", err)
		}
		elapsed := time.Since(start)
		barsPerSec := float64(totalBars) / elapsed.Seconds()
		b.ReportMetric(barsPerSec, "bars/sec")
	}
}

// BenchmarkUpsertBatch_Intraday benchmarks batch upsert of 50,000 bars within a single day/chunk
// (e.g. streaming 1s intraday bars for active trading).
func BenchmarkUpsertBatch_Intraday(b *testing.B) {
	pool, cleanup := setupTestContainer(b)
	defer cleanup()

	ctx := context.Background()
	store := candles.NewStore(pool)

	const totalBars = 50000
	bars := make([]candles.Candle, totalBars)
	baseTime := time.Date(2025, time.January, 1, 9, 15, 0, 0, candles.LocationIST)

	for i := 0; i < totalBars; i++ {
		bt := baseTime.Add(time.Duration(i) * time.Second).UTC()
		bar, err := candles.NewCandleFromFloats(1, bt, 2500.0, 2510.0, 2490.0, 2505.0, 100, 10)
		if err != nil {
			b.Fatalf("failed generating bar: %v", err)
		}
		bars[i] = bar
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if err := store.UpsertBatch(ctx, candles.Timeframe1s, bars); err != nil {
			b.Fatalf("UpsertBatch intraday failed: %v", err)
		}
		elapsed := time.Since(start)
		barsPerSec := float64(totalBars) / elapsed.Seconds()
		b.ReportMetric(barsPerSec, "bars/sec")
	}
}
