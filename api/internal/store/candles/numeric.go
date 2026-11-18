package candles

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	// ErrInvalidNumeric is returned when numeric conversion fails due to NaN, Inf, or invalid formatting.
	ErrInvalidNumeric = errors.New("invalid numeric value")
)

// Float64ToNumeric converts a float64 into pgtype.Numeric with up to 4 decimal places.
// This is the single designated helper for float64 -> NUMERIC(12,4) boundary conversion.
func Float64ToNumeric(f float64) (pgtype.Numeric, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return pgtype.Numeric{}, fmt.Errorf("%w: NaN or Inf is not supported in fixed-point numeric", ErrInvalidNumeric)
	}
	s := strconv.FormatFloat(f, 'f', 4, 64)
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("failed converting float64 %f to numeric: %w", f, err)
	}
	return n, nil
}

// NumericToFloat64 converts a pgtype.Numeric into a native float64.
// This is the single designated helper for NUMERIC(12,4) -> float64 boundary conversion.
func NumericToFloat64(n pgtype.Numeric) (float64, error) {
	if !n.Valid {
		return 0, fmt.Errorf("%w: numeric value is NULL or invalid", ErrInvalidNumeric)
	}
	f8, err := n.Float64Value()
	if err != nil {
		return 0, fmt.Errorf("failed converting numeric to float64: %w", err)
	}
	if !f8.Valid {
		return 0, fmt.Errorf("%w: converted float8 value is invalid", ErrInvalidNumeric)
	}
	return f8.Float64, nil
}

// TextToNumeric converts a decimal string representation (e.g. "1245.5000") into pgtype.Numeric.
func TextToNumeric(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("failed parsing text %q to numeric: %w", s, err)
	}
	return n, nil
}

// NumericToText converts pgtype.Numeric into a decimal string representation.
func NumericToText(n pgtype.Numeric) string {
	if !n.Valid {
		return ""
	}
	val, err := n.Value()
	if err != nil || val == nil {
		return ""
	}
	return fmt.Sprint(val)
}

// MustFloat64ToNumeric converts float64 to pgtype.Numeric, panicking if conversion fails.
func MustFloat64ToNumeric(f float64) pgtype.Numeric {
	n, err := Float64ToNumeric(f)
	if err != nil {
		panic(err)
	}
	return n
}

// MustTextToNumeric converts a string to pgtype.Numeric, panicking if conversion fails.
func MustTextToNumeric(s string) pgtype.Numeric {
	n, err := TextToNumeric(s)
	if err != nil {
		panic(err)
	}
	return n
}

// NewCandleFromFloats builds a new Candle struct using standard float64 price values.
func NewCandleFromFloats(stockID int, bucketTime time.Time, open, high, low, closePrice float64, volume, oi int64) (Candle, error) {
	o, err := Float64ToNumeric(open)
	if err != nil {
		return Candle{}, fmt.Errorf("open price: %w", err)
	}
	h, err := Float64ToNumeric(high)
	if err != nil {
		return Candle{}, fmt.Errorf("high price: %w", err)
	}
	l, err := Float64ToNumeric(low)
	if err != nil {
		return Candle{}, fmt.Errorf("low price: %w", err)
	}
	c, err := Float64ToNumeric(closePrice)
	if err != nil {
		return Candle{}, fmt.Errorf("close price: %w", err)
	}
	return Candle{
		StockID:    stockID,
		BucketTime: bucketTime.UTC(),
		Open:       o,
		High:       h,
		Low:        l,
		Close:      c,
		Volume:     volume,
		OI:         oi,
		IsAdjusted: false,
	}, nil
}

// OpenFloat returns the opening price converted to float64.
func (c Candle) OpenFloat() (float64, error) {
	return NumericToFloat64(c.Open)
}

// HighFloat returns the high price converted to float64.
func (c Candle) HighFloat() (float64, error) {
	return NumericToFloat64(c.High)
}

// LowFloat returns the low price converted to float64.
func (c Candle) LowFloat() (float64, error) {
	return NumericToFloat64(c.Low)
}

// CloseFloat returns the closing price converted to float64.
func (c Candle) CloseFloat() (float64, error) {
	return NumericToFloat64(c.Close)
}
