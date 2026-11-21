package corporate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound = errors.New("corporate: record not found")
)

// DB abstracts database operations (satisfied by *pgxpool.Pool or pgx.Tx).
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// CorporateStore defines the persistence interface for corporate actions and price adjustments.
type CorporateStore interface {
	InsertAction(ctx context.Context, action CorporateAction) (int64, error)
	ListActions(ctx context.Context, stockID int, verifiedOnly bool) ([]CorporateAction, error)
	ListUnverifiedActions(ctx context.Context) ([]CorporateAction, error)
	VerifyAction(ctx context.Context, id int64) error
	GetFactors(ctx context.Context, stockID int) ([]AdjustmentFactor, error)
	RecomputeStockFactors(ctx context.Context, stockID int) ([]AdjustmentFactor, error)
	RecomputeAllFactors(ctx context.Context) (map[int][]AdjustmentFactor, error)
	DetectOvernightJumps(ctx context.Context, stockID int, thresholdPct float64) ([]DataQualityIssue, error)
	ListDataQualityIssues(ctx context.Context, stockID int, resolvedOnly *bool) ([]DataQualityIssue, error)
	AdjustedCandles(ctx context.Context, stockID int, tf string, from, to time.Time) ([]AdjustedCandle, error)
}

// Store is the pgx-backed implementation of CorporateStore.
type Store struct {
	db DB
}

// NewStore creates a new CorporateStore backed by db.
func NewStore(db DB) *Store {
	return &Store{db: db}
}

// InsertAction inserts a new corporate action and returns its generated ID.
func (s *Store) InsertAction(ctx context.Context, a CorporateAction) (int64, error) {
	query := `
		INSERT INTO corporate_actions (stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (stock_id, action_type, ex_date)
		DO UPDATE SET
			ratio_from = EXCLUDED.ratio_from,
			ratio_to = EXCLUDED.ratio_to,
			dividend_amt = EXCLUDED.dividend_amt,
			source = EXCLUDED.source,
			verified = EXCLUDED.verified
		RETURNING id;
	`
	var id int64
	err := s.db.QueryRow(ctx, query,
		a.StockID,
		string(a.ActionType),
		a.ExDate.Format("2006-01-02"),
		a.RatioFrom,
		a.RatioTo,
		a.DividendAmt,
		a.Source,
		a.Verified,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("corporate.InsertAction: %w", err)
	}
	return id, nil
}

// ListActions returns corporate actions for a stock, optionally filtering to verified only.
func (s *Store) ListActions(ctx context.Context, stockID int, verifiedOnly bool) ([]CorporateAction, error) {
	query := `
		SELECT id, stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified, created_at
		FROM corporate_actions
		WHERE stock_id = $1 AND ($2 = FALSE OR verified = TRUE)
		ORDER BY ex_date ASC, id ASC;
	`
	rows, err := s.db.Query(ctx, query, stockID, verifiedOnly)
	if err != nil {
		return nil, fmt.Errorf("corporate.ListActions: %w", err)
	}
	defer rows.Close()

	var actions []CorporateAction
	for rows.Next() {
		var a CorporateAction
		var actType string
		var exDate time.Time
		if err := rows.Scan(
			&a.ID,
			&a.StockID,
			&actType,
			&exDate,
			&a.RatioFrom,
			&a.RatioTo,
			&a.DividendAmt,
			&a.Source,
			&a.Verified,
			&a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("corporate.ListActions scan: %w", err)
		}
		a.ActionType = ActionType(actType)
		a.ExDate = exDate
		actions = append(actions, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("corporate.ListActions rows: %w", err)
	}
	return actions, nil
}

// ListUnverifiedActions returns all pending unverified actions across all stocks.
func (s *Store) ListUnverifiedActions(ctx context.Context) ([]CorporateAction, error) {
	query := `
		SELECT id, stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified, created_at
		FROM corporate_actions
		WHERE verified = FALSE
		ORDER BY ex_date ASC, id ASC;
	`
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("corporate.ListUnverifiedActions: %w", err)
	}
	defer rows.Close()

	var actions []CorporateAction
	for rows.Next() {
		var a CorporateAction
		var actType string
		var exDate time.Time
		if err := rows.Scan(
			&a.ID,
			&a.StockID,
			&actType,
			&exDate,
			&a.RatioFrom,
			&a.RatioTo,
			&a.DividendAmt,
			&a.Source,
			&a.Verified,
			&a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("corporate.ListUnverifiedActions scan: %w", err)
		}
		a.ActionType = ActionType(actType)
		a.ExDate = exDate
		actions = append(actions, a)
	}
	return actions, rows.Err()
}

// VerifyAction marks a corporate action as verified.
func (s *Store) VerifyAction(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, "UPDATE corporate_actions SET verified = TRUE WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("corporate.VerifyAction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetFactors returns all stored adjustment factors for a stock, sorted chronologically ascending.
func (s *Store) GetFactors(ctx context.Context, stockID int) ([]AdjustmentFactor, error) {
	query := `
		SELECT stock_id, ex_date, factor
		FROM adjustment_factors
		WHERE stock_id = $1
		ORDER BY ex_date ASC;
	`
	rows, err := s.db.Query(ctx, query, stockID)
	if err != nil {
		return nil, fmt.Errorf("corporate.GetFactors: %w", err)
	}
	defer rows.Close()

	var factors []AdjustmentFactor
	for rows.Next() {
		var f AdjustmentFactor
		if err := rows.Scan(&f.StockID, &f.ExDate, &f.Factor); err != nil {
			return nil, fmt.Errorf("corporate.GetFactors scan: %w", err)
		}
		factors = append(factors, f)
	}
	return factors, rows.Err()
}

// RecomputeStockFactors wipes and idempotently recomputes adjustment factors for a single stock.
func (s *Store) RecomputeStockFactors(ctx context.Context, stockID int) ([]AdjustmentFactor, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("corporate.RecomputeStockFactors begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Wipe existing factors for this stock
	if _, err := tx.Exec(ctx, "DELETE FROM adjustment_factors WHERE stock_id = $1", stockID); err != nil {
		return nil, fmt.Errorf("failed to clear existing adjustment factors: %w", err)
	}

	// 2. Fetch all verified corporate actions for this stock
	query := `
		SELECT id, stock_id, action_type, ex_date, ratio_from, ratio_to, dividend_amt, source, verified, created_at
		FROM corporate_actions
		WHERE stock_id = $1 AND verified = TRUE
		ORDER BY ex_date DESC, id DESC;
	`
	rows, err := tx.Query(ctx, query, stockID)
	if err != nil {
		return nil, fmt.Errorf("failed to query verified actions: %w", err)
	}
	defer rows.Close()

	var actions []CorporateAction
	for rows.Next() {
		var a CorporateAction
		var actType string
		var exDate time.Time
		if err := rows.Scan(
			&a.ID,
			&a.StockID,
			&actType,
			&exDate,
			&a.RatioFrom,
			&a.RatioTo,
			&a.DividendAmt,
			&a.Source,
			&a.Verified,
			&a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan action: %w", err)
		}
		a.ActionType = ActionType(actType)
		a.ExDate = exDate
		actions = append(actions, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	if len(actions) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// 3. For any dividend actions, look up previous trading day's close price
	var items []ActionWithPrevClose
	for _, a := range actions {
		prevClose := 0.0
		if a.ActionType == ActionDividend {
			exDateStr := a.ExDate.Format("2006-01-02")
			// Try candles_1d first
			err := tx.QueryRow(ctx, `
				SELECT close FROM candles_1d
				WHERE stock_id = $1 AND (bucket_time AT TIME ZONE 'Asia/Kolkata')::DATE < $2
				ORDER BY bucket_time DESC LIMIT 1
			`, stockID, exDateStr).Scan(&prevClose)
			if err != nil {
				// Fallback to candles_1m
				_ = tx.QueryRow(ctx, `
					SELECT close FROM candles_1m
					WHERE stock_id = $1 AND (bucket_time AT TIME ZONE 'Asia/Kolkata')::DATE < $2
					ORDER BY bucket_time DESC LIMIT 1
				`, stockID, exDateStr).Scan(&prevClose)
			}
		}
		items = append(items, ActionWithPrevClose{Action: a, PrevClose: prevClose})
	}

	// 4. Compute cumulative factors
	factors, err := ComputeCumulativeFactors(items)
	if err != nil {
		return nil, fmt.Errorf("failed to compute cumulative factors: %w", err)
	}

	// 5. Persist factors into adjustment_factors
	for _, f := range factors {
		_, err := tx.Exec(ctx, `
			INSERT INTO adjustment_factors (stock_id, ex_date, factor)
			VALUES ($1, $2, $3)
			ON CONFLICT (stock_id, ex_date) DO UPDATE SET factor = EXCLUDED.factor;
		`, f.StockID, f.ExDate.Format("2006-01-02"), f.Factor)
		if err != nil {
			return nil, fmt.Errorf("failed to insert adjustment factor: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit recompute tx: %w", err)
	}

	return factors, nil
}

// RecomputeAllFactors recomputes factors across all stocks that have verified actions.
func (s *Store) RecomputeAllFactors(ctx context.Context) (map[int][]AdjustmentFactor, error) {
	rows, err := s.db.Query(ctx, "SELECT DISTINCT stock_id FROM corporate_actions WHERE verified = TRUE")
	if err != nil {
		return nil, fmt.Errorf("corporate.RecomputeAllFactors: %w", err)
	}
	defer rows.Close()

	var stockIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		stockIDs = append(stockIDs, id)
	}
	rows.Close()

	result := make(map[int][]AdjustmentFactor)
	for _, stockID := range stockIDs {
		factors, err := s.RecomputeStockFactors(ctx, stockID)
		if err != nil {
			return nil, fmt.Errorf("failed recomputing stock %d: %w", stockID, err)
		}
		result[stockID] = factors
	}
	return result, nil
}

// DetectOvernightJumps inspects consecutive daily closes for unexplained moves > thresholdPct (e.g. 0.35 = 35%)
// without a verified corporate action on that ex-date, and inserts issues into data_quality_issues.
func (s *Store) DetectOvernightJumps(ctx context.Context, stockID int, thresholdPct float64) ([]DataQualityIssue, error) {
	// First, quarantine any unverified actions into data_quality_issues
	_, _ = s.db.Exec(ctx, `
		INSERT INTO data_quality_issues (stock_id, issue_type, check_date, details, resolved)
		SELECT stock_id, 'unverified_action', ex_date,
		       jsonb_build_object(
		         'action_type', action_type,
		         'source', source,
		         'ratio_from', ratio_from,
		         'ratio_to', ratio_to,
		         'dividend_amt', dividend_amt
		       ),
		       FALSE
		FROM corporate_actions
		WHERE stock_id = $1 AND verified = FALSE
		ON CONFLICT (stock_id, issue_type, check_date) DO NOTHING;
	`, stockID)

	// Fetch consecutive daily closes from candles_1d (fallback to candles_1m daily rollups)
	query := `
		WITH daily_data AS (
			SELECT bucket_time, close FROM candles_1d WHERE stock_id = $1
			UNION ALL
			SELECT time_bucket(INTERVAL '1 day', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30') AS bucket_time,
			       last(close, bucket_time) AS close
			FROM candles_1m
			WHERE stock_id = $1 AND NOT EXISTS (SELECT 1 FROM candles_1d WHERE stock_id = $1)
			GROUP BY time_bucket(INTERVAL '1 day', bucket_time, TIMESTAMPTZ '2015-01-01 09:15:00+05:30')
		),
		ordered_bars AS (
			SELECT
				bucket_time,
				(bucket_time AT TIME ZONE 'Asia/Kolkata')::DATE AS trade_date,
				close,
				LAG(close) OVER (ORDER BY bucket_time ASC) AS prev_close,
				LAG((bucket_time AT TIME ZONE 'Asia/Kolkata')::DATE) OVER (ORDER BY bucket_time ASC) AS prev_date
			FROM daily_data
		)
		SELECT trade_date, prev_date, close, prev_close
		FROM ordered_bars
		WHERE prev_close IS NOT NULL AND prev_close > 0
		ORDER BY trade_date ASC;
	`
	rows, err := s.db.Query(ctx, query, stockID)
	if err != nil {
		return nil, fmt.Errorf("corporate.DetectOvernightJumps query: %w", err)
	}
	defer rows.Close()

	type barPair struct {
		TradeDate time.Time
		PrevDate  time.Time
		Close     float64
		PrevClose float64
	}

	var pairs []barPair
	for rows.Next() {
		var p barPair
		if err := rows.Scan(&p.TradeDate, &p.PrevDate, &p.Close, &p.PrevClose); err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	rows.Close()

	for _, p := range pairs {
		pctChange := (p.Close - p.PrevClose) / p.PrevClose
		if math.Abs(pctChange) >= thresholdPct {
			// Check if a verified corporate action exists on this trade date
			var verifiedCount int
			err := s.db.QueryRow(ctx, `
				SELECT COUNT(*) FROM corporate_actions
				WHERE stock_id = $1 AND ex_date = $2 AND verified = TRUE
			`, stockID, p.TradeDate.Format("2006-01-02")).Scan(&verifiedCount)
			if err != nil {
				return nil, fmt.Errorf("check verified action: %w", err)
			}

			if verifiedCount == 0 {
				details := map[string]any{
					"prev_date":   p.PrevDate.Format("2006-01-02"),
					"prev_close":  p.PrevClose,
					"curr_date":   p.TradeDate.Format("2006-01-02"),
					"curr_close":  p.Close,
					"pct_change":  math.Round(pctChange*10000) / 100, // percentage e.g. -60.0%
					"unexplained": true,
				}
				detailsJSON, _ := json.Marshal(details)

				_, err = s.db.Exec(ctx, `
					INSERT INTO data_quality_issues (stock_id, issue_type, check_date, details, resolved)
					VALUES ($1, 'unexplained_jump', $2, $3::jsonb, FALSE)
					ON CONFLICT (stock_id, issue_type, check_date) DO NOTHING;
				`, stockID, p.TradeDate.Format("2006-01-02"), string(detailsJSON))
				if err != nil {
					return nil, fmt.Errorf("insert quality issue: %w", err)
				}
			}
		}
	}

	return s.ListDataQualityIssues(ctx, stockID, nil)
}

// ListDataQualityIssues returns data quality issues for a stock.
func (s *Store) ListDataQualityIssues(ctx context.Context, stockID int, resolvedOnly *bool) ([]DataQualityIssue, error) {
	query := `
		SELECT id, stock_id, issue_type, check_date, details, resolved, created_at
		FROM data_quality_issues
		WHERE stock_id = $1 AND ($2::BOOLEAN IS NULL OR resolved = $2)
		ORDER BY check_date ASC, id ASC;
	`
	rows, err := s.db.Query(ctx, query, stockID, resolvedOnly)
	if err != nil {
		return nil, fmt.Errorf("corporate.ListDataQualityIssues: %w", err)
	}
	defer rows.Close()

	var issues []DataQualityIssue
	for rows.Next() {
		var i DataQualityIssue
		var detailsRaw []byte
		if err := rows.Scan(
			&i.ID,
			&i.StockID,
			&i.IssueType,
			&i.CheckDate,
			&detailsRaw,
			&i.Resolved,
			&i.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("corporate.ListDataQualityIssues scan: %w", err)
		}
		if len(detailsRaw) > 0 {
			_ = json.Unmarshal(detailsRaw, &i.Details)
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

// AdjustedCandles executes the PostgreSQL function adjusted_candles(stock_id, tf, from, to)
// and returns the adjusted candle series.
func (s *Store) AdjustedCandles(ctx context.Context, stockID int, tf string, from, to time.Time) ([]AdjustedCandle, error) {
	query := `
		SELECT bucket_time, open, high, low, close, volume
		FROM adjusted_candles($1, $2, $3, $4);
	`
	rows, err := s.db.Query(ctx, query, stockID, tf, from, to)
	if err != nil {
		return nil, fmt.Errorf("corporate.AdjustedCandles: %w", err)
	}
	defer rows.Close()

	var candles []AdjustedCandle
	for rows.Next() {
		var c AdjustedCandle
		if err := rows.Scan(
			&c.BucketTime,
			&c.Open,
			&c.High,
			&c.Low,
			&c.Close,
			&c.Volume,
		); err != nil {
			return nil, fmt.Errorf("corporate.AdjustedCandles scan: %w", err)
		}
		candles = append(candles, c)
	}
	return candles, rows.Err()
}

