package candles

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// CandleStore defines the common interface for reading and writing multi-resolution candle series.
// Both the production TimescaleDB repository and test/in-memory fakes implement this interface.
type CandleStore interface {
	// UpsertBatch inserts or updates a slice of candles using multi-row INSERT ... ON CONFLICT DO UPDATE.
	// Only raw timeframes (1s, 1m) are permitted for writes.
	UpsertBatch(ctx context.Context, tf Timeframe, bars []Candle) error

	// Range retrieves candles for the specified stockID and timeframe in the inclusive window [from, to],
	// returned in strictly ascending order by bucket_time.
	Range(ctx context.Context, stockID int, tf Timeframe, from, to time.Time) ([]Candle, error)

	// Latest returns the most recent candle for the specified stockID and timeframe.
	// Returns ErrNotFound if no bars exist.
	Latest(ctx context.Context, stockID int, tf Timeframe) (Candle, error)

	// LatestTime returns the latest bucket_time for the specified stockID and timeframe.
	// Returns ErrNotFound if no bars exist.
	LatestTime(ctx context.Context, stockID int, tf Timeframe) (time.Time, error)

	// CopyBatch imports a high-volume slice of candles into a temporary table via pgx.CopyFrom,
	// then idempotently merges them into the target hypertable.
	CopyBatch(ctx context.Context, tf Timeframe, bars []Candle) error
}

// DB abstracts the database operations required by the pgx Store implementation.
// This interface is satisfied by *pgxpool.Pool.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store is the pgx-backed implementation of CandleStore.
type Store struct {
	db DB
}

// NewStore creates a new CandleStore backed by the provided database connection/pool.
func NewStore(db DB) *Store {
	return &Store{db: db}
}

// maxBarsPerInsert caps the number of rows in a single multi-row INSERT statement.
// 2000 bars * 9 params = 18000 parameters, which minimizes Postgres query planning overhead
// while staying well under PostgreSQL's 65535 parameter limit.
const maxBarsPerInsert = 2000

var (
	queryCacheMu sync.RWMutex
	queryCache   = make(map[string]map[int]string) // tableName -> barCount -> SQL
)

func getUpsertQuery(tableName string, count int) string {
	queryCacheMu.RLock()
	if byCount, ok := queryCache[tableName]; ok {
		if q, ok := byCount[count]; ok {
			queryCacheMu.RUnlock()
			return q
		}
	}
	queryCacheMu.RUnlock()

	queryCacheMu.Lock()
	defer queryCacheMu.Unlock()

	byCount, ok := queryCache[tableName]
	if !ok {
		byCount = make(map[int]string)
		queryCache[tableName] = byCount
	}
	if q, ok := byCount[count]; ok {
		return q
	}

	var qb strings.Builder
	qb.Grow(count*60 + 256)
	qb.WriteString("INSERT INTO ")
	qb.WriteString(tableName)
	qb.WriteString(" (stock_id, bucket_time, open, high, low, close, volume, oi, is_adjusted) VALUES ")

	for i := 0; i < count; i++ {
		if i > 0 {
			qb.WriteByte(',')
		}
		base := i * 9
		qb.WriteByte('(')
		for col := 1; col <= 9; col++ {
			if col > 1 {
				qb.WriteByte(',')
			}
			qb.WriteByte('$')
			qb.WriteString(strconv.Itoa(base + col))
		}
		qb.WriteByte(')')
	}

	qb.WriteString(`
		ON CONFLICT (stock_id, bucket_time) DO UPDATE SET
			open = EXCLUDED.open,
			high = EXCLUDED.high,
			low = EXCLUDED.low,
			close = EXCLUDED.close,
			volume = EXCLUDED.volume,
			oi = EXCLUDED.oi,
			is_adjusted = EXCLUDED.is_adjusted;
	`)

	q := qb.String()
	byCount[count] = q
	return q
}

// UpsertBatch writes a batch of candles via multi-row INSERT ... ON CONFLICT (stock_id, bucket_time) DO UPDATE.
// All bars are validated for strict timeframe alignment before database execution.
func (s *Store) UpsertBatch(ctx context.Context, tf Timeframe, bars []Candle) error {
	if err := tf.ValidateWriteSupport(); err != nil {
		return err
	}
	if len(bars) == 0 {
		return nil
	}

	for i, bar := range bars {
		if err := tf.ValidateBar(bar); err != nil {
			return fmt.Errorf("bar at index %d is invalid: %w", i, err)
		}
	}

	tableName, err := tf.TableName()
	if err != nil {
		return err
	}

	// In-memory deduplication keeps the latest bar for duplicate keys to prevent
	// PostgreSQL error: "ON CONFLICT DO UPDATE command cannot affect row a second time".
	deduped := deduplicateBars(bars)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed starting transaction for UpsertBatch: %w", err)
	}
	defer tx.Rollback(ctx)

	// Disable synchronous commit locally for this batch transaction to maximize throughput
	_, _ = tx.Exec(ctx, "SET LOCAL synchronous_commit = off;")

	for start := 0; start < len(deduped); start += maxBarsPerInsert {
		end := start + maxBarsPerInsert
		if end > len(deduped) {
			end = len(deduped)
		}
		if err := s.insertChunk(ctx, tx, tableName, deduped[start:end]); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (s *Store) insertChunk(ctx context.Context, ex execer, tableName string, bars []Candle) error {
	query := getUpsertQuery(tableName, len(bars))
	args := make([]any, len(bars)*9)
	for i, bar := range bars {
		offset := i * 9
		args[offset] = bar.StockID
		args[offset+1] = bar.BucketTime.UTC()
		args[offset+2] = bar.Open
		args[offset+3] = bar.High
		args[offset+4] = bar.Low
		args[offset+5] = bar.Close
		args[offset+6] = bar.Volume
		args[offset+7] = bar.OI
		args[offset+8] = bar.IsAdjusted
	}

	if _, err := ex.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("failed executing batch upsert on %s: %w", tableName, err)
	}
	return nil
}

// Range retrieves candles for a stockID and timeframe within [from, to] (inclusive of both boundaries).
// Returned rows are ordered strictly ascending by bucket_time.
func (s *Store) Range(ctx context.Context, stockID int, tf Timeframe, from, to time.Time) ([]Candle, error) {
	if stockID <= 0 {
		return nil, fmt.Errorf("%w: stock_id must be a positive integer", ErrInvalidBar)
	}
	if from.After(to) {
		return nil, fmt.Errorf("%w: from time (%s) cannot be after to time (%s)", ErrInvalidBar, from.Format(time.RFC3339), to.Format(time.RFC3339))
	}

	tableName, err := tf.TableName()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT stock_id, bucket_time, open, high, low, close, volume, oi, is_adjusted
		FROM %s
		WHERE stock_id = $1 AND bucket_time >= $2 AND bucket_time <= $3
		ORDER BY bucket_time ASC;
	`, tableName)

	rows, err := s.db.Query(ctx, query, stockID, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("failed querying range on %s: %w", tableName, err)
	}
	defer rows.Close()

	candles := make([]Candle, 0)
	for rows.Next() {
		var c Candle
		if err := rows.Scan(
			&c.StockID,
			&c.BucketTime,
			&c.Open,
			&c.High,
			&c.Low,
			&c.Close,
			&c.Volume,
			&c.OI,
			&c.IsAdjusted,
		); err != nil {
			return nil, fmt.Errorf("failed scanning candle row: %w", err)
		}
		c.BucketTime = c.BucketTime.UTC()
		candles = append(candles, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading candle rows: %w", err)
	}
	return candles, nil
}

// Latest retrieves the single most recent candle for the specified stockID and timeframe.
func (s *Store) Latest(ctx context.Context, stockID int, tf Timeframe) (Candle, error) {
	if stockID <= 0 {
		return Candle{}, fmt.Errorf("%w: stock_id must be a positive integer", ErrInvalidBar)
	}
	tableName, err := tf.TableName()
	if err != nil {
		return Candle{}, err
	}

	query := fmt.Sprintf(`
		SELECT stock_id, bucket_time, open, high, low, close, volume, oi, is_adjusted
		FROM %s
		WHERE stock_id = $1
		ORDER BY bucket_time DESC
		LIMIT 1;
	`, tableName)

	var c Candle
	err = s.db.QueryRow(ctx, query, stockID).Scan(
		&c.StockID,
		&c.BucketTime,
		&c.Open,
		&c.High,
		&c.Low,
		&c.Close,
		&c.Volume,
		&c.OI,
		&c.IsAdjusted,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Candle{}, &NotFoundError{Entity: "candle", Key: fmt.Sprintf("stock:%d/tf:%s", stockID, tf)}
		}
		return Candle{}, fmt.Errorf("failed querying latest candle on %s: %w", tableName, err)
	}
	c.BucketTime = c.BucketTime.UTC()
	return c, nil
}

// LatestTime retrieves only the latest bucket_time for the specified stockID and timeframe.
func (s *Store) LatestTime(ctx context.Context, stockID int, tf Timeframe) (time.Time, error) {
	if stockID <= 0 {
		return time.Time{}, fmt.Errorf("%w: stock_id must be a positive integer", ErrInvalidBar)
	}
	tableName, err := tf.TableName()
	if err != nil {
		return time.Time{}, err
	}

	query := fmt.Sprintf(`
		SELECT bucket_time
		FROM %s
		WHERE stock_id = $1
		ORDER BY bucket_time DESC
		LIMIT 1;
	`, tableName)

	var bt time.Time
	err = s.db.QueryRow(ctx, query, stockID).Scan(&bt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, &NotFoundError{Entity: "candle_time", Key: fmt.Sprintf("stock:%d/tf:%s", stockID, tf)}
		}
		return time.Time{}, fmt.Errorf("failed querying latest candle time on %s: %w", tableName, err)
	}
	return bt.UTC(), nil
}

// candleCopySource is a zero-allocation streaming source for pgx.CopyFrom.
type candleCopySource struct {
	bars []Candle
	idx  int
	row  [9]any
}

func newCandleCopySource(bars []Candle) *candleCopySource {
	return &candleCopySource{bars: bars, idx: -1}
}

func (s *candleCopySource) Next() bool {
	s.idx++
	return s.idx < len(s.bars)
}

func (s *candleCopySource) Values() ([]any, error) {
	b := s.bars[s.idx]
	s.row[0] = b.StockID
	s.row[1] = b.BucketTime.UTC()
	s.row[2] = b.Open
	s.row[3] = b.High
	s.row[4] = b.Low
	s.row[5] = b.Close
	s.row[6] = b.Volume
	s.row[7] = b.OI
	s.row[8] = b.IsAdjusted
	return s.row[:], nil
}

func (s *candleCopySource) Err() error {
	return nil
}

// CopyBatch streams candles into a transaction-scoped temporary table via pgx.CopyFrom,
// then idempotently merges them into the hypertable.
func (s *Store) CopyBatch(ctx context.Context, tf Timeframe, bars []Candle) error {
	if err := tf.ValidateWriteSupport(); err != nil {
		return err
	}
	if len(bars) == 0 {
		return nil
	}

	for i, bar := range bars {
		if err := tf.ValidateBar(bar); err != nil {
			return fmt.Errorf("bar at index %d is invalid: %w", i, err)
		}
	}

	tableName, err := tf.TableName()
	if err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for CopyBatch: %w", err)
	}
	defer tx.Rollback(ctx)

	// Disable synchronous commit locally for high-throughput batch operations
	_, _ = tx.Exec(ctx, "SET LOCAL synchronous_commit = off;")

	tempTable := fmt.Sprintf("temp_%s_%d", tableName, time.Now().UnixNano())
	createTempSQL := fmt.Sprintf(`CREATE TEMP TABLE %s (LIKE %s INCLUDING DEFAULTS) ON COMMIT DROP;`, tempTable, tableName)
	if _, err := tx.Exec(ctx, createTempSQL); err != nil {
		return fmt.Errorf("failed creating temporary table %s: %w", tempTable, err)
	}

	deduped := deduplicateBars(bars)
	cols := []string{"stock_id", "bucket_time", "open", "high", "low", "close", "volume", "oi", "is_adjusted"}

	source := newCandleCopySource(deduped)

	if _, err := tx.CopyFrom(ctx, pgx.Identifier{tempTable}, cols, source); err != nil {
		return fmt.Errorf("CopyFrom to %s failed: %w", tempTable, err)
	}

	mergeSQL := fmt.Sprintf(`
		INSERT INTO %s (stock_id, bucket_time, open, high, low, close, volume, oi, is_adjusted)
		SELECT stock_id, bucket_time, open, high, low, close, volume, oi, is_adjusted
		FROM %s
		ORDER BY bucket_time, stock_id
		ON CONFLICT (stock_id, bucket_time) DO UPDATE SET
			open = EXCLUDED.open,
			high = EXCLUDED.high,
			low = EXCLUDED.low,
			close = EXCLUDED.close,
			volume = EXCLUDED.volume,
			oi = EXCLUDED.oi,
			is_adjusted = EXCLUDED.is_adjusted;
	`, tableName, tempTable)

	if _, err := tx.Exec(ctx, mergeSQL); err != nil {
		return fmt.Errorf("failed merging %s into %s: %w", tempTable, tableName, err)
	}

	return tx.Commit(ctx)
}

type barKey struct {
	stockID int
	t       time.Time
}

func deduplicateBars(bars []Candle) []Candle {
	if len(bars) <= 1 {
		return bars
	}
	seen := make(map[barKey]int, len(bars))
	result := make([]Candle, 0, len(bars))
	for _, bar := range bars {
		k := barKey{
			stockID: bar.StockID,
			t:       bar.BucketTime.UTC(),
		}
		if idx, exists := seen[k]; exists {
			result[idx] = bar // overwrite previous occurrence with newest
		} else {
			seen[k] = len(result)
			result = append(result, bar)
		}
	}
	return result
}
