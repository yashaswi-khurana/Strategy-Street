package candles_test

import (
	"errors"
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strategy-street/api/internal/store/candles"
)

func TestNumericConversion(t *testing.T) {
	t.Run("Float64 to Numeric and back", func(t *testing.T) {
		tests := []float64{
			0.0,
			0.05,
			100.25,
			2500.5,
			2500.5432,
			99999.9999,
		}

		for _, val := range tests {
			num, err := candles.Float64ToNumeric(val)
			if err != nil {
				t.Fatalf("Float64ToNumeric(%f) failed: %v", val, err)
			}
			f, err := candles.NumericToFloat64(num)
			if err != nil {
				t.Fatalf("NumericToFloat64 failed for %f: %v", val, err)
			}
			if math.Abs(f-val) > 1e-4 {
				t.Fatalf("round-trip failed: expected %f, got %f", val, f)
			}
		}
	})

	t.Run("NaN and Inf rejected", func(t *testing.T) {
		_, err := candles.Float64ToNumeric(math.NaN())
		if err == nil {
			t.Fatal("expected error for NaN")
		}
		if !errors.Is(err, candles.ErrInvalidNumeric) {
			t.Fatalf("expected ErrInvalidNumeric, got %v", err)
		}

		_, err = candles.Float64ToNumeric(math.Inf(1))
		if err == nil {
			t.Fatal("expected error for +Inf")
		}
		if !errors.Is(err, candles.ErrInvalidNumeric) {
			t.Fatalf("expected ErrInvalidNumeric, got %v", err)
		}
	})

	t.Run("Invalid numeric to float64", func(t *testing.T) {
		var n pgtype.Numeric // Valid = false
		_, err := candles.NumericToFloat64(n)
		if err == nil {
			t.Fatal("expected error for invalid/NULL numeric")
		}
		if !errors.Is(err, candles.ErrInvalidNumeric) {
			t.Fatalf("expected ErrInvalidNumeric, got %v", err)
		}
	})

	t.Run("Text to Numeric and back", func(t *testing.T) {
		text := "3450.7500"
		n, err := candles.TextToNumeric(text)
		if err != nil {
			t.Fatalf("TextToNumeric(%q) error: %v", text, err)
		}
		f, err := candles.NumericToFloat64(n)
		if err != nil {
			t.Fatalf("NumericToFloat64 error: %v", err)
		}
		if f != 3450.75 {
			t.Fatalf("expected 3450.75, got %f", f)
		}
	})

	t.Run("Must helpers panic on invalid input", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic on NaN")
			}
		}()
		candles.MustFloat64ToNumeric(math.NaN())
	})
}
