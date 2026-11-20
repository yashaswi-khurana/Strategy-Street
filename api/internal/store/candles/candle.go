package candles

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// LocationIST is the standard Indian Standard Time zone (UTC+05:30).
// IST has had a fixed offset of +05:30 without Daylight Saving Time since September 1947.
var LocationIST = time.FixedZone("IST", 5*3600+30*60)

// EpochIST is the legacy reference epoch for bar-open bucket alignments: 1970-01-01T00:00:00+05:30.
// In UTC, this instant is exactly 1969-12-31T18:30:00Z (Unix timestamp -19800 seconds).
var EpochIST = time.Date(1970, time.January, 1, 0, 0, 0, 0, LocationIST).UTC()

// Origin2015IST is the unified reference epoch for all candle bucket alignments: 2015-01-01T09:15:00+05:30.
// In UTC, this instant is exactly 2015-01-01T03:45:00Z.
// This epoch anchors intraday bars to NSE market opening (09:15 IST) and aligns daily bars
// cleanly to the 09:15-15:30 IST session while ensuring continuous aggregates across all timeframes
// share the identical origin required by TimescaleDB hierarchical continuous aggregates.
var Origin2015IST = time.Date(2015, time.January, 1, 9, 15, 0, 0, LocationIST).UTC()

// OriginIntradayIST is an alias to Origin2015IST for intraday timeframes.
// Note: 1970-01-01 09:15:00 IST and 2015-01-01 09:15:00 IST are separated by exactly 16,436 days,
// which is an exact multiple of 1s, 1m, 5m, 10m, 15m, 30m, 1h, 3h, and 1d.
var OriginIntradayIST = Origin2015IST

var (
	// ErrMisalignedBar is returned when a candle's bucket_time is not aligned to the timeframe interval.
	ErrMisalignedBar = errors.New("candle bucket_time is misaligned")

	// ErrUnsupportedTimeframe is returned when a requested timeframe is invalid or not supported for the operation.
	ErrUnsupportedTimeframe = errors.New("unsupported timeframe")

	// ErrInvalidBar is returned when candle attributes fail business validation (negative price/volume, high < low, etc.).
	ErrInvalidBar = errors.New("invalid candle data")

	// ErrNotFound is returned when no candle record matches the query criteria.
	ErrNotFound = errors.New("candle not found")
)

// NotFoundError provides structured entity details when a candle or series is missing.
type NotFoundError struct {
	Entity string
	Key    string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Entity, e.Key)
}

func (e *NotFoundError) Is(target error) bool {
	if target == ErrNotFound {
		return true
	}
	t, ok := target.(*NotFoundError)
	return ok && (t.Entity == "" || t.Entity == e.Entity)
}

// Timeframe defines the 13 supported candle resolutions across the platform.
type Timeframe string

const (
	Timeframe1s  Timeframe = "1s"
	Timeframe1m  Timeframe = "1m"
	Timeframe5m  Timeframe = "5m"
	Timeframe10m Timeframe = "10m"
	Timeframe15m Timeframe = "15m"
	Timeframe30m Timeframe = "30m"
	Timeframe1h  Timeframe = "1h"
	Timeframe3h  Timeframe = "3h"
	Timeframe1d  Timeframe = "1d"
	Timeframe15d Timeframe = "15d"
	Timeframe1mo Timeframe = "1mo"
	Timeframe6mo Timeframe = "6mo"
	Timeframe1y  Timeframe = "1y"
)

// ParseTimeframe converts a string identifier to a validated Timeframe.
// Distinguishes uppercase '1M' (month) from lowercase '1m' (minute).
func ParseTimeframe(s string) (Timeframe, error) {
	trimmed := strings.TrimSpace(s)
	switch trimmed {
	case "1M", "1mo", "1Mo":
		return Timeframe1mo, nil
	case "6M", "6mo", "6Mo":
		return Timeframe6mo, nil
	case "1Y", "1y":
		return Timeframe1y, nil
	}
	norm := strings.ToLower(trimmed)
	switch norm {
	case "1s":
		return Timeframe1s, nil
	case "1m":
		return Timeframe1m, nil
	case "5m":
		return Timeframe5m, nil
	case "10m":
		return Timeframe10m, nil
	case "15m":
		return Timeframe15m, nil
	case "30m":
		return Timeframe30m, nil
	case "1h":
		return Timeframe1h, nil
	case "3h":
		return Timeframe3h, nil
	case "1d":
		return Timeframe1d, nil
	case "15d":
		return Timeframe15d, nil
	case "1mo":
		return Timeframe1mo, nil
	case "6mo":
		return Timeframe6mo, nil
	case "1y":
		return Timeframe1y, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedTimeframe, s)
	}
}

// Duration returns the representative time.Duration for the timeframe.
// For macro calendar intervals (1mo, 6mo, 1y), it returns standard nominal durations (30d, 180d, 365d).
func (tf Timeframe) Duration() time.Duration {
	switch tf {
	case Timeframe1s:
		return 1 * time.Second
	case Timeframe1m:
		return 1 * time.Minute
	case Timeframe5m:
		return 5 * time.Minute
	case Timeframe10m:
		return 10 * time.Minute
	case Timeframe15m:
		return 15 * time.Minute
	case Timeframe30m:
		return 30 * time.Minute
	case Timeframe1h:
		return 1 * time.Hour
	case Timeframe3h:
		return 3 * time.Hour
	case Timeframe1d:
		return 24 * time.Hour
	case Timeframe15d:
		return 15 * 24 * time.Hour
	case Timeframe1mo:
		return 30 * 24 * time.Hour
	case Timeframe6mo:
		return 180 * 24 * time.Hour
	case Timeframe1y:
		return 365 * 24 * time.Hour
	default:
		return 0
	}
}

// Align calculates the bar-open timestamp in UTC, aligned to the timeframe boundary
// anchored at the unified IST-aligned epoch Origin2015IST (2015-01-01T09:15:00+05:30).
//
// For fixed intervals (1s to 15d), alignment is computed via modular offset from Origin2015IST.
// For calendar months and years (1mo, 6mo, 1y), alignment is locked to the 1st of the
// respective month/half/year in Asia/Kolkata, then returned in UTC.
func (tf Timeframe) Align(t time.Time) time.Time {
	switch tf {
	case Timeframe1mo:
		tIST := t.In(LocationIST)
		return time.Date(tIST.Year(), tIST.Month(), 1, 0, 0, 0, 0, LocationIST).UTC()
	case Timeframe6mo:
		tIST := t.In(LocationIST)
		m := ((tIST.Month()-1)/6)*6 + 1
		return time.Date(tIST.Year(), m, 1, 0, 0, 0, 0, LocationIST).UTC()
	case Timeframe1y:
		tIST := t.In(LocationIST)
		return time.Date(tIST.Year(), time.January, 1, 0, 0, 0, 0, LocationIST).UTC()
	case Timeframe1s, Timeframe1m:
		dur := tf.Duration()
		if dur <= 0 {
			return t.UTC()
		}
		diff := t.Sub(Origin2015IST)
		rem := diff % dur
		if rem < 0 {
			rem += dur
		}
		return t.Add(-rem).UTC()
	default:
		dur := tf.Duration()
		if dur <= 0 {
			return t.UTC()
		}
		diff := t.Sub(Origin2015IST)
		rem := diff % dur
		if rem < 0 {
			rem += dur
		}
		return t.Add(-rem).UTC()
	}
}

// ValidateBucketTime validates that a timestamp is non-zero, UTC, and strictly aligned to the timeframe.
func (tf Timeframe) ValidateBucketTime(t time.Time) error {
	if t.IsZero() {
		return fmt.Errorf("%w: bucket_time cannot be zero", ErrInvalidBar)
	}
	if _, offset := t.Zone(); offset != 0 {
		return fmt.Errorf("%w: bucket_time must be in UTC, got offset %d", ErrInvalidBar, offset)
	}
	aligned := tf.Align(t)
	if !t.Equal(aligned) {
		return fmt.Errorf("%w: timestamp %s is not aligned to timeframe %s (expected %s)",
			ErrMisalignedBar, t.Format(time.RFC3339Nano), tf, aligned.Format(time.RFC3339Nano))
	}
	return nil
}

// TableName returns the physical table or continuous aggregate materialized view name for this timeframe.
func (tf Timeframe) TableName() (string, error) {
	switch tf {
	case Timeframe1s:
		return "candles_1s", nil
	case Timeframe1m:
		return "candles_1m", nil
	case Timeframe5m:
		return "candles_5m", nil
	case Timeframe10m:
		return "candles_10m", nil
	case Timeframe15m:
		return "candles_15m", nil
	case Timeframe30m:
		return "candles_30m", nil
	case Timeframe1h:
		return "candles_1h", nil
	case Timeframe3h:
		return "candles_3h", nil
	case Timeframe1d:
		return "candles_1d", nil
	case Timeframe15d:
		return "candles_15d", nil
	case Timeframe1mo:
		return "candles_1mo", nil
	case Timeframe6mo:
		return "candles_6mo", nil
	case Timeframe1y:
		return "candles_1y", nil
	default:
		return "", fmt.Errorf("%w: unknown timeframe %s", ErrUnsupportedTimeframe, tf)
	}
}

// RelationName is an alias for TableName, providing semantic clarity when referencing relations.
func (tf Timeframe) RelationName() (string, error) {
	return tf.TableName()
}

// IsContinuousAggregate returns true if the timeframe is derived via TimescaleDB continuous aggregates.
func (tf Timeframe) IsContinuousAggregate() bool {
	switch tf {
	case Timeframe5m, Timeframe10m, Timeframe15m, Timeframe30m, Timeframe1h, Timeframe3h,
		Timeframe1d, Timeframe15d, Timeframe1mo, Timeframe6mo, Timeframe1y:
		return true
	default:
		return false
	}
}

// IsWritable returns true if the application is permitted to write candles directly for this timeframe.
// By architectural mandate, the application only ever writes raw resolutions (1s, 1m).
// Higher timeframes are derived via TimescaleDB continuous aggregates (SS-004) and never written directly.
func (tf Timeframe) IsWritable() bool {
	return tf == Timeframe1s || tf == Timeframe1m
}

// ValidateWriteSupport returns an error if the timeframe cannot be written by the application.
func (tf Timeframe) ValidateWriteSupport() error {
	if !tf.IsWritable() {
		return fmt.Errorf("%w: application writing to timeframe %q is forbidden (only 1s and 1m are writable)", ErrUnsupportedTimeframe, tf)
	}
	return nil
}

// ValidateBar checks all structural and domain rules for a candle before database interaction.
func (tf Timeframe) ValidateBar(c Candle) error {
	if c.StockID <= 0 {
		return fmt.Errorf("%w: stock_id must be a positive integer, got %d", ErrInvalidBar, c.StockID)
	}
	if err := tf.ValidateBucketTime(c.BucketTime); err != nil {
		return err
	}
	if !c.Open.Valid || !c.High.Valid || !c.Low.Valid || !c.Close.Valid {
		return fmt.Errorf("%w: open, high, low, and close must all be valid numeric values", ErrInvalidBar)
	}
	high, err := NumericToFloat64(c.High)
	if err != nil {
		return fmt.Errorf("%w: invalid high price: %v", ErrInvalidBar, err)
	}
	low, err := NumericToFloat64(c.Low)
	if err != nil {
		return fmt.Errorf("%w: invalid low price: %v", ErrInvalidBar, err)
	}
	open, err := NumericToFloat64(c.Open)
	if err != nil {
		return fmt.Errorf("%w: invalid open price: %v", ErrInvalidBar, err)
	}
	closePrice, err := NumericToFloat64(c.Close)
	if err != nil {
		return fmt.Errorf("%w: invalid close price: %v", ErrInvalidBar, err)
	}

	if open < 0 || high < 0 || low < 0 || closePrice < 0 {
		return fmt.Errorf("%w: prices cannot be negative", ErrInvalidBar)
	}
	if high < low {
		return fmt.Errorf("%w: high (%f) cannot be less than low (%f)", ErrInvalidBar, high, low)
	}
	if open > high || open < low {
		return fmt.Errorf("%w: open (%f) must be between low (%f) and high (%f)", ErrInvalidBar, open, low, high)
	}
	if closePrice > high || closePrice < low {
		return fmt.Errorf("%w: close (%f) must be between low (%f) and high (%f)", ErrInvalidBar, closePrice, low, high)
	}
	if c.Volume < 0 {
		return fmt.Errorf("%w: volume cannot be negative, got %d", ErrInvalidBar, c.Volume)
	}
	if c.OI < 0 {
		return fmt.Errorf("%w: oi cannot be negative, got %d", ErrInvalidBar, c.OI)
	}
	if c.IsAdjusted {
		return fmt.Errorf("%w: is_adjusted must be false (SS-003 stores unadjusted raw market data only)", ErrInvalidBar)
	}
	return nil
}

// Candle represents an immutable OHLCV bar for an equity asset.
// Price values are represented at storage/boundary layers as pgtype.Numeric (NUMERIC(12,4))
// to guarantee absolute decimal precision without IEEE 754 floating-point drift.
type Candle struct {
	StockID    int            `json:"stock_id"`
	BucketTime time.Time      `json:"bucket_time"`
	Open       pgtype.Numeric `json:"open"`
	High       pgtype.Numeric `json:"high"`
	Low        pgtype.Numeric `json:"low"`
	Close      pgtype.Numeric `json:"close"`
	Volume     int64          `json:"volume"`
	OI         int64          `json:"oi"`
	IsAdjusted bool           `json:"is_adjusted"`
}
