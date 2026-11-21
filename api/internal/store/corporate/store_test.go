package corporate_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/strategy-street/api/internal/store/corporate"
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

func setupTestDB(t testing.TB) (*pgxpool.Pool, func()) {
	t.Helper()

	// Try connecting to local docker TimescaleDB if DATABASE_URL is accessible
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5433/strategystreet?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if pool, err := pgxpool.New(ctx, dbURL); err == nil {
		if err := pool.Ping(ctx); err == nil {
			// Local TimescaleDB is available!
			return pool, func() { pool.Close() }
		}
		pool.Close()
	}

	// Fallback to testcontainers
	if testing.Short() {
		t.Skip("skipping container integration test in short mode")
	}

	initDockerHost()

	cCtx, cCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cCancel()

	pgContainer, runErr := tcpostgres.Run(cCtx,
		"timescale/timescaledb:2.15.3-pg16",
		tcpostgres.WithDatabase("strategystreet_ca_test"),
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

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		cleanup()
		t.Fatalf("failed creating pool: %v", err)
	}

	repoRoot := findRepoRoot(t)
	migrations := []string{
		"0001_metadata.up.sql",
		"0002_candles.up.sql",
		"0005_is_filled.up.sql",
		"0006_corporate_actions.up.sql",
	}

	for _, m := range migrations {
		mPath := filepath.Join(repoRoot, "db", "migrations", m)
		mSQL, err := os.ReadFile(mPath)
		if err != nil {
			cleanup()
			t.Fatalf("failed reading migration %s: %v", m, err)
		}
		if _, err := pool.Exec(context.Background(), string(mSQL)); err != nil {
			cleanup()
			t.Fatalf("failed executing migration %s: %v", m, err)
		}
	}

	return pool, cleanup
}

func ptr(v float64) *float64 {
	return &v
}

func parseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func parseISTTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func TestCorporateActions_AcceptanceCriteria(t *testing.T) {
	pool, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	store := corporate.NewStore(pool)

	// Create an isolated test stock
	var stockID int
	testSymbol := fmt.Sprintf("TEST_%d", time.Now().UnixNano()%1000000)
	testKey := fmt.Sprintf("NSE_EQ|%s", testSymbol)
	err := pool.QueryRow(ctx, `
		INSERT INTO stocks (instrument_key, symbol, company_name, exchange, is_active)
		VALUES ($1, $2, 'Corporate Actions Test Stock', 'NSE', TRUE)
		RETURNING id;
	`, testKey, testSymbol).Scan(&stockID)
	if err != nil {
		t.Fatalf("failed inserting test stock: %v", err)
	}

	t.Run("Criterion_1_and_2: 1:5 Split on 2024-06-14 continuity and volume unchanged", func(t *testing.T) {
		// Acceptance Criterion 1:
		// For a stock with a 1:5 split on 2024-06-14 and pre-split close of 2500,
		// adjusted_candles returns 500 for the day before the ex-date and 500 for the ex-date itself — no visible jump.
		// Acceptance Criterion 2:
		// Volume is unchanged by adjustment.

		// Seed raw candles into candles_1m:
		// Day before ex-date (2024-06-13): Close = 2500, Volume = 100000
		// Ex-date (2024-06-14): Close = 500, Volume = 250000
		t1 := parseISTTime("2024-06-13T09:15:00+05:30")
		t2 := parseISTTime("2024-06-14T09:15:00+05:30")

		_, err := pool.Exec(ctx, `
			INSERT INTO candles_1m (stock_id, bucket_time, open, high, low, close, volume, oi, is_adjusted, is_filled)
			VALUES
				($1, $2, 2490.0, 2510.0, 2480.0, 2500.0, 100000, 0, FALSE, FALSE),
				($1, $3, 498.0, 505.0, 495.0, 500.0, 250000, 0, FALSE, FALSE)
			ON CONFLICT (stock_id, bucket_time) DO UPDATE SET
				close = EXCLUDED.close, volume = EXCLUDED.volume;
		`, stockID, t1, t2)
		if err != nil {
			t.Fatalf("failed inserting test candles: %v", err)
		}

		// Insert verified 1:5 split action on 2024-06-14
		actionID, err := store.InsertAction(ctx, corporate.CorporateAction{
			StockID:    stockID,
			ActionType: corporate.ActionSplit,
			ExDate:     parseDate("2024-06-14"),
			RatioFrom:  ptr(1.0),
			RatioTo:    ptr(5.0),
			Source:     "ACCEPTANCE_TEST",
			Verified:   true,
		})
		if err != nil {
			t.Fatalf("failed inserting corporate action: %v", err)
		}
		if actionID <= 0 {
			t.Fatalf("expected valid action ID, got %d", actionID)
		}

		// Recompute factors
		factors, err := store.RecomputeStockFactors(ctx, stockID)
		if err != nil {
			t.Fatalf("failed recomputing factors: %v", err)
		}
		if len(factors) != 1 {
			t.Fatalf("expected 1 factor, got %d", len(factors))
		}
		if math.Abs(factors[0].Factor-0.20) > 1e-9 {
			t.Fatalf("expected factor 0.20, got %f", factors[0].Factor)
		}

		// Query adjusted_candles for 1m
		adjCandles, err := store.AdjustedCandles(ctx, stockID, "1m", t1, t2)
		if err != nil {
			t.Fatalf("failed querying adjusted_candles: %v", err)
		}
		if len(adjCandles) != 2 {
			t.Fatalf("expected 2 adjusted candles, got %d", len(adjCandles))
		}

		// Day before ex-date (2024-06-13): raw close was 2500 -> adjusted must be 2500 * 0.2 = 500.0000
		dayBefore := adjCandles[0]
		if math.Abs(dayBefore.Close-500.0) > 1e-4 {
			t.Errorf("day before ex-date: expected adjusted close 500.0, got %f", dayBefore.Close)
		}
		// Volume must be strictly unchanged: 100000
		if dayBefore.Volume != 100000 {
			t.Errorf("expected day before volume 100000, got %d", dayBefore.Volume)
		}

		// Ex-date itself (2024-06-14): raw close was 500 -> adjusted must be 500 * 1.0 = 500.0000
		exDay := adjCandles[1]
		if math.Abs(exDay.Close-500.0) > 1e-4 {
			t.Errorf("ex-date: expected close 500.0, got %f", exDay.Close)
		}
		// Volume must be strictly unchanged: 250000
		if exDay.Volume != 250000 {
			t.Errorf("expected ex-date volume 250000, got %d", exDay.Volume)
		}

		t.Logf("Continuous series verified: Day before = %.4f, Ex-date = %.4f (no visible jump!)", dayBefore.Close, exDay.Close)
	})

	t.Run("Criterion_3: Recompute is completely idempotent", func(t *testing.T) {
		// Acceptance Criterion 3:
		// Running make ca-recompute twice yields identical adjustment_factors (idempotent).
		f1, err := store.RecomputeStockFactors(ctx, stockID)
		if err != nil {
			t.Fatalf("first recompute failed: %v", err)
		}

		f2, err := store.RecomputeStockFactors(ctx, stockID)
		if err != nil {
			t.Fatalf("second recompute failed: %v", err)
		}

		if len(f1) != len(f2) {
			t.Fatalf("idempotency check failed: len(f1)=%d != len(f2)=%d", len(f1), len(f2))
		}
		for i := range f1 {
			if f1[i].ExDate != f2[i].ExDate || math.Abs(f1[i].Factor-f2[i].Factor) > 1e-9 {
				t.Errorf("mismatch at factor index %d: %+v vs %+v", i, f1[i], f2[i])
			}
		}
		t.Logf("Recompute idempotency verified: %d factors identical across runs", len(f1))
	})

	t.Run("Criterion_4: Unverified action leaves prices unadjusted and is flagged in quality issues", func(t *testing.T) {
		// Acceptance Criterion 4:
		// An unverified action leaves prices unadjusted and appears in data_quality_issues/report output.

		// Create another stock for unverified action testing
		var unverifiedStockID int
		uSymbol := fmt.Sprintf("UNVER_%d", time.Now().UnixNano()%1000000)
		uKey := fmt.Sprintf("NSE_EQ|%s", uSymbol)
		err := pool.QueryRow(ctx, `
			INSERT INTO stocks (instrument_key, symbol, company_name, exchange, is_active)
			VALUES ($1, $2, 'Unverified Action Stock', 'NSE', TRUE)
			RETURNING id;
		`, uKey, uSymbol).Scan(&unverifiedStockID)
		if err != nil {
			t.Fatalf("failed inserting unverified test stock: %v", err)
		}

		t1 := parseISTTime("2024-07-01T09:15:00+05:30")
		_, err = pool.Exec(ctx, `
			INSERT INTO candles_1m (stock_id, bucket_time, open, high, low, close, volume, oi, is_adjusted, is_filled)
			VALUES ($1, $2, 1000.0, 1005.0, 995.0, 1000.0, 50000, 0, FALSE, FALSE);
		`, unverifiedStockID, t1)
		if err != nil {
			t.Fatalf("failed inserting candle: %v", err)
		}

		// Insert UNVERIFIED action
		_, err = store.InsertAction(ctx, corporate.CorporateAction{
			StockID:    unverifiedStockID,
			ActionType: corporate.ActionSplit,
			ExDate:     parseDate("2024-07-02"),
			RatioFrom:  ptr(1.0),
			RatioTo:    ptr(2.0),
			Source:     "UNVERIFIED_SOURCE",
			Verified:   false, // FALSE
		})
		if err != nil {
			t.Fatalf("failed inserting unverified action: %v", err)
		}

		// Recompute factors for this stock
		factors, err := store.RecomputeStockFactors(ctx, unverifiedStockID)
		if err != nil {
			t.Fatalf("failed recomputing: %v", err)
		}
		if len(factors) != 0 {
			t.Fatalf("expected 0 factors for unverified action stock, got %d", len(factors))
		}

		// Query adjusted candles — prices MUST remain 1000.0 (unadjusted)
		adjCandles, err := store.AdjustedCandles(ctx, unverifiedStockID, "1m", t1, t1)
		if err != nil {
			t.Fatalf("failed querying adjusted_candles: %v", err)
		}
		if len(adjCandles) != 1 {
			t.Fatalf("expected 1 candle, got %d", len(adjCandles))
		}
		if math.Abs(adjCandles[0].Close-1000.0) > 1e-4 {
			t.Fatalf("expected price to remain unadjusted at 1000.0, got %f", adjCandles[0].Close)
		}

		// Detect quality issues
		issues, err := store.DetectOvernightJumps(ctx, unverifiedStockID, 0.35)
		if err != nil {
			t.Fatalf("failed running detect quality issues: %v", err)
		}

		// Check that the unverified action is flagged
		var foundUnverifiedIssue bool
		for _, issue := range issues {
			if issue.IssueType == "unverified_action" {
				foundUnverifiedIssue = true
				break
			}
		}
		if !foundUnverifiedIssue {
			t.Errorf("expected unverified_action issue to appear in data_quality_issues")
		}
		t.Logf("Unverified action quarantine verified: price untouched (%.2f) and issue logged", adjCandles[0].Close)
	})

	t.Run("Criterion_5: 60% overnight drop with no corporate action raises exactly one data_quality_issues row", func(t *testing.T) {
		// Acceptance Criterion 5:
		// A 60% overnight drop with no corporate action raises exactly one data_quality_issues row.

		var jumpStockID int
		jSymbol := fmt.Sprintf("JUMP_%d", time.Now().UnixNano()%1000000)
		jKey := fmt.Sprintf("NSE_EQ|%s", jSymbol)
		err := pool.QueryRow(ctx, `
			INSERT INTO stocks (instrument_key, symbol, company_name, exchange, is_active)
			VALUES ($1, $2, 'Overnight Jump Stock', 'NSE', TRUE)
			RETURNING id;
		`, jKey, jSymbol).Scan(&jumpStockID)
		if err != nil {
			t.Fatalf("failed inserting jump stock: %v", err)
		}

		// Insert 2 days of candles with a 60% overnight drop:
		// Day 1: close = 1000.0
		// Day 2: close = 400.0 (drop = -60.0%)
		// Notice: NO corporate action is seeded!
		d1 := parseISTTime("2024-05-10T09:15:00+05:30")
		d2 := parseISTTime("2024-05-11T09:15:00+05:30")

		_, err = pool.Exec(ctx, `
			INSERT INTO candles_1m (stock_id, bucket_time, open, high, low, close, volume, oi, is_adjusted, is_filled)
			VALUES
				($1, $2, 990.0, 1010.0, 985.0, 1000.0, 50000, 0, FALSE, FALSE),
				($1, $3, 410.0, 420.0, 395.0, 400.0, 60000, 0, FALSE, FALSE);
		`, jumpStockID, d1, d2)
		if err != nil {
			t.Fatalf("failed inserting jump candles: %v", err)
		}

		// Run detection once
		issues1, err := store.DetectOvernightJumps(ctx, jumpStockID, 0.35)
		if err != nil {
			t.Fatalf("detection 1 failed: %v", err)
		}

		var jumpIssues []corporate.DataQualityIssue
		for _, issue := range issues1 {
			if issue.IssueType == "unexplained_jump" {
				jumpIssues = append(jumpIssues, issue)
			}
		}
		if len(jumpIssues) != 1 {
			t.Fatalf("expected exactly 1 unexplained_jump issue, got %d", len(jumpIssues))
		}
		if jumpIssues[0].CheckDate.Format("2006-01-02") != "2024-05-11" {
			t.Errorf("expected check_date 2024-05-11, got %v", jumpIssues[0].CheckDate)
		}

		// Run detection a SECOND time to prove idempotency: exactly ONE row must remain
		issues2, err := store.DetectOvernightJumps(ctx, jumpStockID, 0.35)
		if err != nil {
			t.Fatalf("detection 2 failed: %v", err)
		}

		jumpIssues2 := 0
		for _, issue := range issues2 {
			if issue.IssueType == "unexplained_jump" {
				jumpIssues2++
			}
		}
		if jumpIssues2 != 1 {
			t.Fatalf("idempotency failed: expected exactly 1 unexplained_jump row after 2 runs, got %d", jumpIssues2)
		}

		t.Logf("60%% overnight drop verified: exactly one data_quality_issues row raised idempotently: %+v", jumpIssues[0].Details)
	})
}

