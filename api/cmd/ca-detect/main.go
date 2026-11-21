package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strategy-street/api/internal/store/corporate"
)

func main() {
	stockFlag := flag.String("stock", "RELIANCE", "Stock symbol to run jump detection for, or ALL")
	thresholdFlag := flag.Float64("threshold", 0.35, "Overnight return threshold to flag (default 0.35 = ±35%)")
	dbURLFlag := flag.String("db", "", "Database URL (defaults to DATABASE_URL or localhost:5433)")
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

	store := corporate.NewStore(pool)
	targetSymbol := strings.ToUpper(strings.TrimSpace(*stockFlag))

	fmt.Printf("==> Corporate Actions Jump & Quality Detector (Target: %s, Threshold: ±%.1f%%)\n",
		targetSymbol, *thresholdFlag*100)

	var stockIDs []struct {
		ID     int
		Symbol string
	}

	if targetSymbol == "ALL" {
		rows, err := pool.Query(ctx, "SELECT id, symbol FROM stocks WHERE is_active = TRUE ORDER BY id ASC")
		if err != nil {
			log.Fatalf("Query stocks failed: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var s struct {
				ID     int
				Symbol string
			}
			if err := rows.Scan(&s.ID, &s.Symbol); err != nil {
				log.Fatalf("Scan stock failed: %v", err)
			}
			stockIDs = append(stockIDs, s)
		}
		rows.Close()
	} else {
		var stockID int
		err := pool.QueryRow(ctx, "SELECT id FROM stocks WHERE symbol = $1", targetSymbol).Scan(&stockID)
		if err != nil {
			log.Fatalf("Stock symbol %q not found in stocks table: %v", targetSymbol, err)
		}
		stockIDs = append(stockIDs, struct {
			ID     int
			Symbol string
		}{ID: stockID, Symbol: targetSymbol})
	}

	totalIssues := 0
	for _, s := range stockIDs {
		issues, err := store.DetectOvernightJumps(ctx, s.ID, *thresholdFlag)
		if err != nil {
			log.Printf("Warning: failed detecting jumps for %s (ID %d): %v", s.Symbol, s.ID, err)
			continue
		}
		if len(issues) > 0 {
			totalIssues += len(issues)
			fmt.Printf("\n[!] Discovered %d Data Quality Issue(s) for %s (ID: %d):\n", len(issues), s.Symbol, s.ID)
			fmt.Printf("    %-6s | %-18s | %-12s | %s\n", "ID", "Issue Type", "Check Date", "Details")
			fmt.Println("    " + strings.Repeat("-", 75))
			for _, issue := range issues {
				fmt.Printf("    %-6d | %-18s | %-12s | %s\n",
					issue.ID,
					issue.IssueType,
					issue.CheckDate.Format("2006-01-02"),
					corporate.FormatDetails(issue.Details),
				)
			}
		}
	}

	if totalIssues == 0 {
		fmt.Println("==> All checked stocks clean. Zero unexplained overnight jumps detected.")
	} else {
		fmt.Printf("\n==> Total %d issue(s) logged in data_quality_issues table.\n", totalIssues)
	}
}

