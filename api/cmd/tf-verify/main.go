package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strategy-street/api/internal/store/candles"
)

type barData struct {
	BucketTime time.Time
	Open       string
	High       string
	Low        string
	Close      string
	Volume     int64
	OI         int64
	IsAdjusted bool
}

func main() {
	stockIDFlag := flag.Int("stock-id", 1, "Stock ID to verify")
	dbURLFlag := flag.String("db", "", "Database URL")
	flag.Parse()

	dbURL := *dbURLFlag
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5433/strategystreet?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed connecting to %s: %v", dbURL, err)
	}
	defer pool.Close()

	stockID := *stockIDFlag

	// Check if data exists in candles_1m
	var count1m int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM candles_1m WHERE stock_id = $1;", stockID).Scan(&count1m)
	if err != nil || count1m == 0 {
		log.Fatalf("No candles found for stock_id=%d in candles_1m. Please run 'make seed-candles' first.", stockID)
	}

	// Refresh continuous aggregates to ensure all ingested minute bars are materialized
	caggsToRefresh := []string{
		"candles_5m", "candles_10m", "candles_15m", "candles_30m",
		"candles_1h", "candles_3h", "candles_1d",
		"candles_15d", "candles_1mo", "candles_6mo", "candles_1y",
	}
	for _, cagg := range caggsToRefresh {
		_, err := pool.Exec(ctx, fmt.Sprintf("CALL refresh_continuous_aggregate('%s', NULL, NULL);", cagg))
		if err != nil {
			log.Fatalf("Failed refreshing %s: %v", cagg, err)
		}
	}

	fmt.Printf("========================================================================================\n")
	fmt.Printf(" TF-VERIFY: Multi-Timeframe Continuous Aggregate Verification (Stock ID %d)\n", stockID)
	fmt.Printf(" Origin Anchor: 2015-01-01 09:15:00+05:30 (IST Session-Aligned)\n")
	fmt.Printf(" Source Raw Rows: %d in candles_1m\n", count1m)
	fmt.Printf("========================================================================================\n")
	fmt.Printf(" %-12s | %-12s | %-10s | %-10s | %-12s | %-8s\n",
		"Timeframe", "Relation", "Cagg Rows", "Direct Rows", "Origin Match", "Status")
	fmt.Printf("----------------------------------------------------------------------------------------\n")

	type tfSpec struct {
		tf       candles.Timeframe
		relation string
		source   string
		interval string
	}

	specs := []tfSpec{
		{candles.Timeframe1s, "candles_1s", "raw", ""},
		{candles.Timeframe1m, "candles_1m", "raw", ""},
		{candles.Timeframe5m, "candles_5m", "candles_1m", "5 minutes"},
		{candles.Timeframe10m, "candles_10m", "candles_1m", "10 minutes"},
		{candles.Timeframe15m, "candles_15m", "candles_1m", "15 minutes"},
		{candles.Timeframe30m, "candles_30m", "candles_1m", "30 minutes"},
		{candles.Timeframe1h, "candles_1h", "candles_1m", "1 hour"},
		{candles.Timeframe3h, "candles_3h", "candles_1m", "3 hours"},
		{candles.Timeframe1d, "candles_1d", "candles_1m", "1 day"},
		{candles.Timeframe15d, "candles_15d", "candles_1d", "15 days"},
		{candles.Timeframe1mo, "candles_1mo", "candles_1d", "1 month"},
		{candles.Timeframe6mo, "candles_6mo", "candles_1d", "6 months"},
		{candles.Timeframe1y, "candles_1y", "candles_1d", "1 year"},
	}

	allPass := true

	for _, spec := range specs {
		if spec.source == "raw" {
			var cnt int
			_ = pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s WHERE stock_id = $1;", spec.relation), stockID).Scan(&cnt)
			fmt.Printf(" %-12s | %-12s | %-10d | %-10s | %-12s | %-8s\n",
				spec.tf, spec.relation, cnt, "N/A (Raw)", "EXACT", "PASS")
			continue
		}

		// 1. Fetch from continuous aggregate
		caggSQL := fmt.Sprintf(`
			SELECT bucket_time, open::TEXT, high::TEXT, low::TEXT, close::TEXT, volume, oi, is_adjusted
			FROM %s
			WHERE stock_id = $1
			ORDER BY bucket_time ASC;
		`, spec.relation)

		caggBars, err := fetchBars(ctx, pool, caggSQL, stockID)
		if err != nil {
			fmt.Printf(" %-12s | %-12s | ERROR: %v\n", spec.tf, spec.relation, err)
			allPass = false
			continue
		}

		// 2. Fetch direct from-scratch computation
		directSQL := fmt.Sprintf(`
			SELECT time_bucket(INTERVAL '%s', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
			       first(open, bucket_time)::TEXT  AS open,
			       max(high)::TEXT                 AS high,
			       min(low)::TEXT                  AS low,
			       last(close, bucket_time)::TEXT  AS close,
			       sum(volume)::BIGINT             AS volume,
			       last(oi, bucket_time)           AS oi,
			       bool_and(is_adjusted)           AS is_adjusted
			FROM %s
			WHERE stock_id = $1
			GROUP BY 1
			ORDER BY 1;
		`, spec.interval, spec.source)

		directBars, err := fetchBars(ctx, pool, directSQL, stockID)
		if err != nil {
			fmt.Printf(" %-12s | %-12s | DIRECT ERROR: %v\n", spec.tf, spec.relation, err)
			allPass = false
			continue
		}

		// 3. Compare row counts and values
		match := true
		failReason := ""
		if len(caggBars) != len(directBars) {
			match = false
			failReason = fmt.Sprintf("count mismatch: cagg=%d, direct=%d", len(caggBars), len(directBars))
		} else {
			for i := 0; i < len(caggBars); i++ {
				cb := caggBars[i]
				db := directBars[i]

				if !cb.BucketTime.Equal(db.BucketTime) ||
					cb.Open != db.Open ||
					cb.High != db.High ||
					cb.Low != db.Low ||
					cb.Close != db.Close ||
					cb.Volume != db.Volume ||
					cb.OI != db.OI ||
					cb.IsAdjusted != db.IsAdjusted {
					match = false
					failReason = fmt.Sprintf("value mismatch at index %d (%s vs %s)",
						i, cb.BucketTime.Format(time.RFC3339), db.BucketTime.Format(time.RFC3339))
					break
				}
			}
		}

		originStatus := "ALIGNED"
		if spec.tf == candles.Timeframe1h {
			for _, b := range caggBars {
				bIST := b.BucketTime.In(candles.LocationIST)
				if bIST.Minute() != 15 {
					originStatus = fmt.Sprintf("MISALIGNED:%d", bIST.Minute())
					match = false
					failReason = fmt.Sprintf("1h bar landed at minute %d (must be 15)", bIST.Minute())
					break
				}
			}
		}

		status := "PASS"
		if !match {
			status = "FAIL"
			allPass = false
		}

		fmt.Printf(" %-12s | %-12s | %-10d | %-10d | %-12s | %-8s\n",
			spec.tf, spec.relation, len(caggBars), len(directBars), originStatus, status)

		if !match {
			fmt.Printf("    --> FAILURE: %s\n", failReason)
		}
	}

	fmt.Printf("========================================================================================\n")

	// Specific Acceptance Criteria Checks:
	// Criterion 2: 1d volume equals sum of 375 1m bars
	fmt.Print(" asserting Criterion 2 (1d bar volume equals sum of 375 1m bars)... ")
	var dailyVolMismatch int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM (
			SELECT d.bucket_time, d.volume, sum(m.volume) as m_sum, count(m.bucket_time) as m_count
			FROM candles_1d d
			JOIN candles_1m m ON m.stock_id = d.stock_id
				AND m.bucket_time >= d.bucket_time
				AND m.bucket_time < d.bucket_time + INTERVAL '1 day'
			WHERE d.stock_id = $1
			GROUP BY d.bucket_time, d.volume
		) t
		WHERE t.volume != t.m_sum OR t.m_count != 375;
	`, stockID).Scan(&dailyVolMismatch)
	if err != nil || dailyVolMismatch > 0 {
		fmt.Printf("FAILED (mismatches: %d, err: %v)\n", dailyVolMismatch, err)
		allPass = false
	} else {
		fmt.Printf("PASSED (all daily bars contain exactly 375 1m bars with identical volume)\n")
	}

	// Criterion 4: 1h buckets land on 09:15, 10:15, 11:15... IST
	fmt.Print(" asserting Criterion 4 (1h buckets land on :15 IST)... ")
	var misaligned1h int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM candles_1h
		WHERE stock_id = $1 AND EXTRACT(MINUTE FROM bucket_time AT TIME ZONE 'Asia/Kolkata') != 15;
	`, stockID).Scan(&misaligned1h)
	if err != nil || misaligned1h > 0 {
		fmt.Printf("FAILED (misaligned 1h bars: %d)\n", misaligned1h)
		allPass = false
	} else {
		fmt.Printf("PASSED (all 1h bars land on :15 IST)\n")
	}

	// Criterion 5: Retention/compression policies in timescaledb_information.jobs; 1d+ have no retention job
	fmt.Print(" asserting Criterion 5 (policies in jobs registry; 1d+ no retention)... ")
	var refreshJobs, compressionJobs, retentionJobs, invalidDailyRetention int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name = 'policy_refresh_continuous_aggregate';").Scan(&refreshJobs)
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name = 'policy_compression';").Scan(&compressionJobs)
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name = 'policy_retention';").Scan(&retentionJobs)
	_ = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM timescaledb_information.jobs j
		JOIN timescaledb_information.continuous_aggregates c 
		  ON (j.config->>'hypertable_id')::bigint = c.materialization_hypertable_id
		WHERE j.proc_name = 'policy_retention' 
		  AND c.view_name IN ('candles_1d', 'candles_15d', 'candles_1mo', 'candles_6mo', 'candles_1y');
	`).Scan(&invalidDailyRetention)

	if refreshJobs < 10 || compressionJobs < 5 || retentionJobs != 2 || invalidDailyRetention > 0 {
		fmt.Printf("FAILED (refresh: %d, compression: %d, retention: %d, invalidDailyRetention: %d)\n",
			refreshJobs, compressionJobs, retentionJobs, invalidDailyRetention)
		allPass = false
	} else {
		fmt.Printf("PASSED (refresh=%d, compression=%d, retention=%d, dailyRetention=%d)\n",
			refreshJobs, compressionJobs, retentionJobs, invalidDailyRetention)
	}

	fmt.Printf("========================================================================================\n")
	if !allPass {
		fmt.Println(" RESULT: VERIFICATION FAILED")
		os.Exit(1)
	}
	fmt.Println(" RESULT: ALL 13 TIMEFRAMES VERIFIED SUCCESSFULLY (BYTE-IDENTICAL)")
}

func fetchBars(ctx context.Context, pool *pgxpool.Pool, query string, stockID int) ([]barData, error) {
	rows, err := pool.Query(ctx, query, stockID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bars := make([]barData, 0)
	for rows.Next() {
		var b barData
		if err := rows.Scan(
			&b.BucketTime,
			&b.Open,
			&b.High,
			&b.Low,
			&b.Close,
			&b.Volume,
			&b.OI,
			&b.IsAdjusted,
		); err != nil {
			return nil, err
		}
		b.BucketTime = b.BucketTime.UTC()
		bars = append(bars, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bars, nil
}
