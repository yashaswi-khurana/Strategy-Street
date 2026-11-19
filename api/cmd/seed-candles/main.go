package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strategy-street/api/internal/store/candles"
)

func main() {
	daysFlag := flag.Int("days", 90, "Number of calendar days to seed (default ~3 months)")
	stocksFlag := flag.Int("stocks", 3, "Number of stocks to seed (stock_ids 1..N)")
	dbURLFlag := flag.String("db", "", "Database URL (defaults to DATABASE_URL or localhost:5433)")
	flag.Parse()

	dbURL := *dbURLFlag
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5433/strategystreet?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Invalid database URL %q: %v", dbURL, err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("Failed connecting to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}

	// 1. Fetch exchange holidays
	holidays := make(map[string]bool)
	rows, err := pool.Query(ctx, "SELECT holiday_date FROM exchange_holidays;")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var d time.Time
			if err := rows.Scan(&d); err == nil {
				holidays[d.Format("2006-01-02")] = true
			}
		}
	}

	store := candles.NewStore(pool)

	// End date: yesterday in IST
	nowIST := time.Now().In(candles.LocationIST)
	endDate := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 0, 0, 0, 0, candles.LocationIST).AddDate(0, 0, -1)
	startDate := endDate.AddDate(0, 0, -*daysFlag)

	fmt.Printf("==> Seeding ~%d days of synthetic 1m candles for %d stock(s) [%s to %s]...\n",
		*daysFlag, *stocksFlag, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	startTime := time.Now()
	totalBars := 0

	// Stock base price profiles
	basePrices := map[int]float64{
		1: 2500.0, // e.g. RELIANCE
		2: 3800.0, // e.g. TCS
		3: 1500.0, // e.g. INFY
	}

	for stockID := 1; stockID <= *stocksFlag; stockID++ {
		currentPrice := basePrices[stockID]
		if currentPrice == 0 {
			currentPrice = 1000.0 + float64(stockID*200)
		}

		stockBars := make([]candles.Candle, 0, *daysFlag*375)
		rng := rand.New(rand.NewSource(int64(stockID * 424242)))

		for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
			// Skip weekends
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			// Skip exchange holidays
			dateStr := d.Format("2006-01-02")
			if holidays[dateStr] {
				continue
			}

			// Generate 375 1-minute bars for NSE session (09:15 to 15:30 IST)
			for minute := 0; minute < 375; minute++ {
				barTimeIST := time.Date(d.Year(), d.Month(), d.Day(), 9, 15+minute, 0, 0, candles.LocationIST)
				barTimeUTC := barTimeIST.UTC()

				// Random walk with drift
				shock := rng.NormFloat64() * (currentPrice * 0.001) // 0.1% volatility per minute
				openPrice := currentPrice
				closePrice := math.Round((openPrice+shock)*100) / 100
				if closePrice < 1.0 {
					closePrice = 1.0
				}

				spread := math.Abs(closePrice-openPrice) + (currentPrice * 0.0005 * rng.Float64())
				highPrice := math.Max(openPrice, closePrice) + (spread * rng.Float64())
				lowPrice := math.Min(openPrice, closePrice) - (spread * rng.Float64())
				if lowPrice < 0.5 {
					lowPrice = 0.5
				}
				highPrice = math.Round(highPrice*100) / 100
				lowPrice = math.Round(lowPrice*100) / 100

				volume := int64(100 + rng.Intn(5000))
				oi := int64(50000 + rng.Intn(10000))

				bar, err := candles.NewCandleFromFloats(stockID, barTimeUTC, openPrice, highPrice, lowPrice, closePrice, volume, oi)
				if err != nil {
					log.Fatalf("Error generating synthetic bar: %v", err)
				}
				stockBars = append(stockBars, bar)
				currentPrice = closePrice
			}
		}

		fmt.Printf("    Stock ID %d: generated %d bars. Ingesting via CopyBatch...\n", stockID, len(stockBars))
		if err := store.CopyBatch(ctx, candles.Timeframe1m, stockBars); err != nil {
			log.Fatalf("Failed CopyBatch for stock %d: %v", stockID, err)
		}
		totalBars += len(stockBars)
	}

	fmt.Printf("==> Successfully ingested %d 1m bars in %s\n", totalBars, time.Since(startTime).Round(time.Millisecond))

	// Refresh continuous aggregates in hierarchical order
	caggs := []string{
		"candles_5m", "candles_10m", "candles_15m", "candles_30m",
		"candles_1h", "candles_3h", "candles_1d",
		"candles_15d", "candles_1mo", "candles_6mo", "candles_1y",
	}

	fmt.Println("==> Refreshing continuous aggregates...")
	for _, cagg := range caggs {
		refreshStart := time.Now()
		// CALL refresh_continuous_aggregate('view_name', NULL, NULL) refreshes full available range
		query := fmt.Sprintf("CALL refresh_continuous_aggregate('%s', NULL, NULL);", cagg)
		if _, err := pool.Exec(ctx, query); err != nil {
			log.Fatalf("Failed refreshing continuous aggregate %s: %v", cagg, err)
		}
		var count int
		_ = pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s;", cagg)).Scan(&count)
		fmt.Printf("    %-12s refreshed (%5d rows) [%s]\n", cagg, count, time.Since(refreshStart).Round(time.Millisecond))
	}

	fmt.Printf("==> All continuous aggregates successfully synchronized in %s!\n", time.Since(startTime).Round(time.Millisecond))
}

