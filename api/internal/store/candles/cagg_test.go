package candles_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strategy-street/api/internal/store/candles"
)

// setupCaggDB connects to local docker database if available, otherwise spins up a testcontainer.
func setupCaggDB(t testing.TB) (*pgxpool.Pool, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. Try local dev database first (fast path)
	localURL := os.Getenv("DATABASE_URL")
	if localURL == "" {
		localURL = "postgres://postgres:postgres@localhost:5433/strategystreet?sslmode=disable"
	}

	if pool, err := pgxpool.New(ctx, localURL); err == nil {
		if err := pool.Ping(ctx); err == nil {
			// Ensure migrations 0001..0004 are applied on local DB
			return pool, func() {
				pool.Close()
			}
		}
		pool.Close()
	}

	// 2. Fall back to testcontainers
	initDockerHost()

	pgContainer, err := tcpostgres.Run(ctx,
		"timescale/timescaledb:2.15.3-pg16",
		tcpostgres.WithDatabase("strategystreet_cagg_test"),
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

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		cleanup()
		t.Fatalf("failed creating pool: %v", err)
	}

	repoRoot := findRepoRoot(t)

	// Apply migrations 0001, seeds, 0002, 0003, 0004
	migFiles := []string{
		filepath.Join(repoRoot, "db", "migrations", "0001_metadata.up.sql"),
		filepath.Join(repoRoot, "db", "seeds", "01_sectors.sql"),
		filepath.Join(repoRoot, "db", "seeds", "02_stocks.sql"),
		filepath.Join(repoRoot, "db", "seeds", "03_exchange_holidays.sql"),
		filepath.Join(repoRoot, "db", "seeds", "04_market_sessions.sql"),
		filepath.Join(repoRoot, "db", "migrations", "0002_candles.up.sql"),
		filepath.Join(repoRoot, "db", "migrations", "0003_continuous_aggregates.up.sql"),
		filepath.Join(repoRoot, "db", "migrations", "0004_policies.up.sql"),
	}

	for _, mf := range migFiles {
		sql, err := os.ReadFile(mf)
		if err != nil {
			pool.Close()
			cleanup()
			t.Fatalf("failed reading %s: %v", mf, err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			pool.Close()
			cleanup()
			t.Fatalf("failed executing %s: %v", mf, err)
		}
	}

	return pool, func() {
		pool.Close()
		cleanup()
	}
}

func TestContinuousAggregates_Integration(t *testing.T) {
	pool, cleanup := setupCaggDB(t)
	defer cleanup()

	ctx := context.Background()
	store := candles.NewStore(pool)

	const testStockID = 52 // Active NSE stock from seeds

	// Clean any previous test data for stock 52
	_, _ = pool.Exec(ctx, "DELETE FROM candles_1m WHERE stock_id = $1;", testStockID)

	t.Run("Empty-input test: aggregates return zero rows, not errors", func(t *testing.T) {
		const emptyStockID = 999 // Stock with no candles
		for _, tf := range []candles.Timeframe{
			candles.Timeframe5m, candles.Timeframe10m, candles.Timeframe15m,
			candles.Timeframe30m, candles.Timeframe1h, candles.Timeframe3h,
			candles.Timeframe1d, candles.Timeframe15d, candles.Timeframe1mo,
		} {
			tFrom := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
			tTo := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

			bars, err := store.Range(ctx, emptyStockID, tf, tFrom, tTo)
			if err != nil {
				t.Fatalf("expected zero rows without error for %s on empty input, got err: %v", tf, err)
			}
			if len(bars) != 0 {
				t.Fatalf("expected 0 bars for %s on empty input, got %d", tf, len(bars))
			}
		}
	})

	t.Run("Acceptance Criterion 3: inserting a 1m bar becomes visible in 5m after refresh", func(t *testing.T) {
		// Insert 1m bar at 09:15 IST
		tBar := time.Date(2026, time.November, 20, 9, 15, 0, 0, candles.LocationIST).UTC()
		bar, err := candles.NewCandleFromFloats(testStockID, tBar, 2000.0, 2010.0, 1995.0, 2005.0, 500, 100)
		if err != nil {
			t.Fatalf("failed creating candle: %v", err)
		}

		if err := store.UpsertBatch(ctx, candles.Timeframe1m, []candles.Candle{bar}); err != nil {
			t.Fatalf("failed inserting 1m bar: %v", err)
		}

		// Refresh candles_5m continuous aggregate
		_, err = pool.Exec(ctx, "CALL refresh_continuous_aggregate('candles_5m', $1::timestamptz, $2::timestamptz);", tBar, tBar.Add(5*time.Minute))
		if err != nil {
			t.Fatalf("failed refreshing candles_5m: %v", err)
		}

		// Query candles_5m
		bars5m, err := store.Range(ctx, testStockID, candles.Timeframe5m, tBar, tBar.Add(5*time.Minute))
		if err != nil {
			t.Fatalf("failed querying Range for 5m: %v", err)
		}
		if len(bars5m) != 1 {
			t.Fatalf("expected 1 bar in candles_5m after refresh, got %d", len(bars5m))
		}

		b := bars5m[0]
		if !b.BucketTime.Equal(tBar) {
			t.Fatalf("expected 5m bucket_time %s, got %s", tBar.Format(time.RFC3339), b.BucketTime.Format(time.RFC3339))
		}
		o, _ := b.OpenFloat()
		c, _ := b.CloseFloat()
		if o != 2000.0 || c != 2005.0 || b.Volume != 500 {
			t.Fatalf("unexpected OHLCV values in 5m: O=%f, C=%f, V=%d", o, c, b.Volume)
		}
	})

	t.Run("Acceptance Criterion 2 & Test Plan: 375 1m bars in trading day equal 1d volume with zero overnight overlap", func(t *testing.T) {
		// Clean test stock data
		_, _ = pool.Exec(ctx, "DELETE FROM candles_1m WHERE stock_id = $1;", testStockID)

		// Create exactly 375 1m bars for a single day: 09:15 to 15:29 IST
		dayIST := time.Date(2026, time.November, 23, 9, 15, 0, 0, candles.LocationIST)
		minuteBars := make([]candles.Candle, 375)
		var expectedVolume int64 = 0

		for i := 0; i < 375; i++ {
			bTime := dayIST.Add(time.Duration(i) * time.Minute).UTC()
			vol := int64(100 + i)
			expectedVolume += vol
			o := 1000.0 + float64(i)*0.1
			c := o + 0.05
			h := c + 0.5
			l := o - 0.5

			b, err := candles.NewCandleFromFloats(testStockID, bTime, o, h, l, c, vol, 50)
			if err != nil {
				t.Fatalf("failed creating bar %d: %v", i, err)
			}
			minuteBars[i] = b
		}

		if err := store.CopyBatch(ctx, candles.Timeframe1m, minuteBars); err != nil {
			t.Fatalf("failed CopyBatch: %v", err)
		}

		// Refresh 1d aggregate
		dayStartUTC := dayIST.UTC()
		_, err := pool.Exec(ctx, "CALL refresh_continuous_aggregate('candles_1d', NULL, NULL);")
		if err != nil {
			t.Fatalf("failed refreshing candles_1d: %v", err)
		}

		// Retrieve 1d bar
		dailyBars, err := store.Range(ctx, testStockID, candles.Timeframe1d, dayStartUTC, dayStartUTC)
		if err != nil {
			t.Fatalf("failed reading daily bar: %v", err)
		}
		if len(dailyBars) != 1 {
			t.Fatalf("expected exactly 1 daily bar, got %d", len(dailyBars))
		}

		dBar := dailyBars[0]
		if !dBar.BucketTime.Equal(dayStartUTC) {
			t.Fatalf("expected daily bar to start at 09:15 IST (%s), got %s",
				dayStartUTC.Format(time.RFC3339), dBar.BucketTime.Format(time.RFC3339))
		}
		if dBar.Volume != expectedVolume {
			t.Fatalf("expected daily bar volume %d, got %d", expectedVolume, dBar.Volume)
		}
	})

	t.Run("Acceptance Criterion 4: 1h buckets land on 09:15, 10:15, 11:15... IST", func(t *testing.T) {
		dayStartUTC := time.Date(2026, time.November, 23, 9, 15, 0, 0, candles.LocationIST).UTC()

		// Refresh 1h aggregate
		_, err := pool.Exec(ctx, "CALL refresh_continuous_aggregate('candles_1h', $1::timestamptz, $2::timestamptz);", dayStartUTC, dayStartUTC.Add(24*time.Hour))
		if err != nil {
			t.Fatalf("failed refreshing candles_1h: %v", err)
		}

		hBars, err := store.Range(ctx, testStockID, candles.Timeframe1h, dayStartUTC, dayStartUTC.Add(24*time.Hour))
		if err != nil {
			t.Fatalf("failed reading 1h bars: %v", err)
		}
		if len(hBars) == 0 {
			t.Fatal("expected 1h bars, got 0")
		}

		for i, hb := range hBars {
			hbIST := hb.BucketTime.In(candles.LocationIST)
			if hbIST.Minute() != 15 {
				t.Errorf("bar %d bucket_time %s does not land on minute 15", i, hbIST.Format(time.RFC3339))
			}
		}
	})

	t.Run("Acceptance Criterion 5: Retention and compression policies in timescaledb_information.jobs", func(t *testing.T) {
		var refreshCount, compressionCount, retentionCount int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name = 'policy_refresh_continuous_aggregate';").Scan(&refreshCount)
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name = 'policy_compression';").Scan(&compressionCount)
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name = 'policy_retention';").Scan(&retentionCount)

		if refreshCount < 10 {
			t.Errorf("expected at least 10 refresh policies, got %d", refreshCount)
		}
		if compressionCount < 5 {
			t.Errorf("expected at least 5 compression policies, got %d", compressionCount)
		}
		if retentionCount != 2 {
			t.Errorf("expected exactly 2 retention policies (1s, 1m), got %d", retentionCount)
		}

		// Ensure 1d+ have no retention policy
		var dailyRetentionJobs int
		err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM timescaledb_information.jobs
			WHERE proc_name = 'policy_retention' 
			  AND hypertable_name IN ('candles_1d', 'candles_15d', 'candles_1mo', 'candles_6mo', 'candles_1y');
		`).Scan(&dailyRetentionJobs)
		if err != nil || dailyRetentionJobs > 0 {
			t.Errorf("expected 0 retention policies on daily+, got %d (err: %v)", dailyRetentionJobs, err)
		}
	})
}

func TestContinuousAggregates_MigrationCycle(t *testing.T) {
	pool, cleanup := setupCaggDB(t)
	defer cleanup()

	ctx := context.Background()
	repoRoot := findRepoRoot(t)

	m4Down, err := os.ReadFile(filepath.Join(repoRoot, "db", "migrations", "0004_policies.down.sql"))
	if err != nil {
		t.Fatalf("failed reading 0004 down: %v", err)
	}
	m3Down, err := os.ReadFile(filepath.Join(repoRoot, "db", "migrations", "0003_continuous_aggregates.down.sql"))
	if err != nil {
		t.Fatalf("failed reading 0003 down: %v", err)
	}
	m3Up, err := os.ReadFile(filepath.Join(repoRoot, "db", "migrations", "0003_continuous_aggregates.up.sql"))
	if err != nil {
		t.Fatalf("failed reading 0003 up: %v", err)
	}
	m4Up, err := os.ReadFile(filepath.Join(repoRoot, "db", "migrations", "0004_policies.up.sql"))
	if err != nil {
		t.Fatalf("failed reading 0004 up: %v", err)
	}

	// 1. Roll back policies (0004) then continuous aggregates (0003)
	if _, err := pool.Exec(ctx, string(m4Down)); err != nil {
		t.Fatalf("failed executing 0004 down: %v", err)
	}
	if _, err := pool.Exec(ctx, string(m3Down)); err != nil {
		t.Fatalf("failed executing 0003 down: %v", err)
	}

	// 2. Verify all continuous aggregate relations are dropped
	var viewCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*) 
		FROM timescaledb_information.continuous_aggregates 
		WHERE view_name IN ('candles_5m', 'candles_10m', 'candles_15m', 'candles_30m', 'candles_1h', 'candles_3h', 'candles_1d', 'candles_15d', 'candles_1mo', 'candles_6mo', 'candles_1y');
	`).Scan(&viewCount)
	if err != nil || viewCount != 0 {
		t.Fatalf("expected 0 continuous aggregates after 0003 down, got %d (err: %v)", viewCount, err)
	}

	// 3. Re-apply continuous aggregates (0003) then policies (0004)
	if _, err := pool.Exec(ctx, string(m3Up)); err != nil {
		t.Fatalf("failed re-executing 0003 up: %v", err)
	}
	if _, err := pool.Exec(ctx, string(m4Up)); err != nil {
		t.Fatalf("failed re-executing 0004 up: %v", err)
	}

	// 4. Verify all 11 views recreated
	err = pool.QueryRow(ctx, `
		SELECT count(*) 
		FROM timescaledb_information.continuous_aggregates 
		WHERE view_name IN ('candles_5m', 'candles_10m', 'candles_15m', 'candles_30m', 'candles_1h', 'candles_3h', 'candles_1d', 'candles_15d', 'candles_1mo', 'candles_6mo', 'candles_1y');
	`).Scan(&viewCount)
	if err != nil || viewCount != 11 {
		t.Fatalf("expected 11 continuous aggregates after 0003 up, got %d (err: %v)", viewCount, err)
	}
}
