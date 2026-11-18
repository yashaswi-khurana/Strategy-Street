package candles

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryStore provides a high-performance in-memory fake of CandleStore.
// Ideal for unit testing the paper trading engine, strategy evaluators, and mocks
// without requiring a live PostgreSQL/TimescaleDB container.
type MemoryStore struct {
	mu   sync.RWMutex
	bars map[Timeframe]map[int]map[time.Time]Candle // tf -> stockID -> bucketTimeUTC -> Candle
}

// NewMemoryStore constructs a new initialized in-memory CandleStore fake.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		bars: make(map[Timeframe]map[int]map[time.Time]Candle),
	}
}

// UpsertBatch writes candles into the in-memory store, enforcing domain rules and alignment.
func (m *MemoryStore) UpsertBatch(_ context.Context, tf Timeframe, bars []Candle) error {
	if err := tf.ValidateWriteSupport(); err != nil {
		return err
	}
	for i, bar := range bars {
		if err := tf.ValidateBar(bar); err != nil {
			return fmt.Errorf("bar at index %d is invalid: %w", i, err)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	stockMap, ok := m.bars[tf]
	if !ok {
		stockMap = make(map[int]map[time.Time]Candle)
		m.bars[tf] = stockMap
	}

	for _, bar := range bars {
		cMap, ok := stockMap[bar.StockID]
		if !ok {
			cMap = make(map[time.Time]Candle)
			stockMap[bar.StockID] = cMap
		}
		b := bar
		b.BucketTime = b.BucketTime.UTC()
		cMap[b.BucketTime] = b
	}

	return nil
}

// Range returns candles within [from, to] (inclusive) sorted ascending by bucket_time.
func (m *MemoryStore) Range(_ context.Context, stockID int, tf Timeframe, from, to time.Time) ([]Candle, error) {
	if stockID <= 0 {
		return nil, fmt.Errorf("%w: stock_id must be a positive integer", ErrInvalidBar)
	}
	if from.After(to) {
		return nil, fmt.Errorf("%w: from cannot be after to", ErrInvalidBar)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	stockMap, ok := m.bars[tf]
	if !ok {
		return []Candle{}, nil
	}
	cMap, ok := stockMap[stockID]
	if !ok {
		return []Candle{}, nil
	}

	fromUTC := from.UTC()
	toUTC := to.UTC()

	var result []Candle
	for _, bar := range cMap {
		if !bar.BucketTime.Before(fromUTC) && !bar.BucketTime.After(toUTC) {
			result = append(result, bar)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].BucketTime.Before(result[j].BucketTime)
	})

	return result, nil
}

// Latest returns the most recent candle for the stock and timeframe.
func (m *MemoryStore) Latest(_ context.Context, stockID int, tf Timeframe) (Candle, error) {
	if stockID <= 0 {
		return Candle{}, fmt.Errorf("%w: stock_id must be a positive integer", ErrInvalidBar)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	stockMap, ok := m.bars[tf]
	if !ok {
		return Candle{}, &NotFoundError{Entity: "candle", Key: fmt.Sprintf("stock:%d/tf:%s", stockID, tf)}
	}
	cMap, ok := stockMap[stockID]
	if !ok || len(cMap) == 0 {
		return Candle{}, &NotFoundError{Entity: "candle", Key: fmt.Sprintf("stock:%d/tf:%s", stockID, tf)}
	}

	var latest Candle
	var found bool
	for _, bar := range cMap {
		if !found || bar.BucketTime.After(latest.BucketTime) {
			latest = bar
			found = true
		}
	}

	if !found {
		return Candle{}, &NotFoundError{Entity: "candle", Key: fmt.Sprintf("stock:%d/tf:%s", stockID, tf)}
	}

	return latest, nil
}

// LatestTime returns the latest bucket_time for the stock and timeframe.
func (m *MemoryStore) LatestTime(ctx context.Context, stockID int, tf Timeframe) (time.Time, error) {
	candle, err := m.Latest(ctx, stockID, tf)
	if err != nil {
		return time.Time{}, err
	}
	return candle.BucketTime, nil
}

// CopyBatch inserts candles idempotently into the in-memory fake.
func (m *MemoryStore) CopyBatch(ctx context.Context, tf Timeframe, bars []Candle) error {
	return m.UpsertBatch(ctx, tf, bars)
}
