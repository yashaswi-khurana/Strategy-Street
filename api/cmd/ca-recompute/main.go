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
	stockFlag := flag.String("stock", "RELIANCE", "Stock symbol to recompute factors for, or ALL")
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

	fmt.Printf("==> Corporate Actions Factor Recomputation (Target: %s)\n", targetSymbol)

	if targetSymbol == "ALL" {
		factorsMap, err := store.RecomputeAllFactors(ctx)
		if err != nil {
			log.Fatalf("RecomputeAllFactors failed: %v", err)
		}
		totalFactors := 0
		for stockID, factors := range factorsMap {
			totalFactors += len(factors)
			fmt.Printf("Stock ID %d: %d adjustment factors computed\n", stockID, len(factors))
		}
		fmt.Printf("==> Successfully recomputed %d factors across %d stocks.\n", totalFactors, len(factorsMap))
		return
	}

	// Look up stock ID for single symbol
	var stockID int
	var companyName string
	err = pool.QueryRow(ctx, "SELECT id, company_name FROM stocks WHERE symbol = $1", targetSymbol).Scan(&stockID, &companyName)
	if err != nil {
		log.Fatalf("Stock symbol %q not found in stocks table: %v", targetSymbol, err)
	}

	fmt.Printf("Resolved Stock: %s (ID: %d, %s)\n", targetSymbol, stockID, companyName)

	factors, err := store.RecomputeStockFactors(ctx, stockID)
	if err != nil {
		log.Fatalf("RecomputeStockFactors failed: %v", err)
	}

	fmt.Printf("==> Idempotently recomputed %d adjustment factor(s) for %s:\n", len(factors), targetSymbol)
	if len(factors) == 0 {
		fmt.Printf("    (No verified corporate actions found for %s)\n", targetSymbol)
	} else {
		fmt.Printf("    %-12s | %-16s | %-20s\n", "Ex-Date", "Cumulative Factor", "Multiplied Prior")
		fmt.Println("    " + strings.Repeat("-", 54))
		for _, f := range factors {
			fmt.Printf("    %-12s | %-16.10f | strictly < %s\n",
				f.ExDate.Format("2006-01-02"),
				f.Factor,
				f.ExDate.Format("2006-01-02"),
			)
		}
	}
}

