package candles_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strategy-street/api/internal/store/candles"
)

func TestTimeframe_Parse(t *testing.T) {
	tests := []struct {
		input   string
		want    candles.Timeframe
		wantErr bool
	}{
		{"1s", candles.Timeframe1s, false},
		{"1m", candles.Timeframe1m, false},
		{"5m", candles.Timeframe5m, false},
		{"10m", candles.Timeframe10m, false},
		{"15m", candles.Timeframe15m, false},
		{"30m", candles.Timeframe30m, false},
		{"1h", candles.Timeframe1h, false},
		{"3h", candles.Timeframe3h, false},
		{"1d", candles.Timeframe1d, false},
		{"15d", candles.Timeframe15d, false},
		{"1mo", candles.Timeframe1mo, false},
		{"6mo", candles.Timeframe6mo, false},
		{"1y", candles.Timeframe1y, false},
		{" 1M ", candles.Timeframe1mo, false},
		{"1Y", candles.Timeframe1y, false},
		{"invalid", "", true},
		{"2m", "", true},
	}

	for _, tt := range tests {
		got, err := candles.ParseTimeframe(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseTimeframe(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseTimeframe(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestTimeframe_Duration(t *testing.T) {
	if candles.Timeframe1s.Duration() != time.Second {
		t.Fatalf("unexpected duration for 1s: %v", candles.Timeframe1s.Duration())
	}
	if candles.Timeframe1m.Duration() != time.Minute {
		t.Fatalf("unexpected duration for 1m: %v", candles.Timeframe1m.Duration())
	}
	if candles.Timeframe5m.Duration() != 5*time.Minute {
		t.Fatalf("unexpected duration for 5m: %v", candles.Timeframe5m.Duration())
	}
	if candles.Timeframe1d.Duration() != 24*time.Hour {
		t.Fatalf("unexpected duration for 1d: %v", candles.Timeframe1d.Duration())
	}
}

func TestTimeframe_AlignAndValidation(t *testing.T) {
	// EpochIST is 1970-01-01 00:00:00 IST = 1969-12-31 18:30:00 UTC
	if candles.EpochIST.Format(time.RFC3339) != "1969-12-31T18:30:00Z" {
		t.Fatalf("expected EpochIST 1969-12-31T18:30:00Z, got %s", candles.EpochIST.Format(time.RFC3339))
	}

	t.Run("Acceptance Criterion 3: writing 09:15:30 into 1m returns validation error", func(t *testing.T) {
		// 09:15:30 IST on 2026-11-18
		tIST := time.Date(2026, time.November, 18, 9, 15, 30, 0, candles.LocationIST)
		tUTC := tIST.UTC()

		err := candles.Timeframe1m.ValidateBucketTime(tUTC)
		if err == nil {
			t.Fatal("expected validation error for 09:15:30 on 1m, got nil")
		}
		if !errors.Is(err, candles.ErrMisalignedBar) {
			t.Fatalf("expected ErrMisalignedBar, got %v", err)
		}

		aligned := candles.Timeframe1m.Align(tUTC)
		// Aligned should be 09:15:00 IST
		expectedAligned := time.Date(2026, time.November, 18, 9, 15, 0, 0, candles.LocationIST).UTC()
		if !aligned.Equal(expectedAligned) {
			t.Fatalf("expected aligned %s, got %s", expectedAligned.Format(time.RFC3339), aligned.Format(time.RFC3339))
		}

		// Aligned timestamp validates successfully
		if err := candles.Timeframe1m.ValidateBucketTime(aligned); err != nil {
			t.Fatalf("expected aligned timestamp to pass validation, got %v", err)
		}
	})

	t.Run("Sub-second timestamps rejected", func(t *testing.T) {
		tWithNano := time.Date(2026, time.November, 18, 9, 15, 0, 500000000, candles.LocationIST).UTC()
		err := candles.Timeframe1m.ValidateBucketTime(tWithNano)
		if err == nil {
			t.Fatal("expected validation error for sub-second timestamp, got nil")
		}
		if !errors.Is(err, candles.ErrMisalignedBar) {
			t.Fatalf("expected ErrMisalignedBar, got %v", err)
		}
	})

	t.Run("Non-UTC timezone rejected", func(t *testing.T) {
		tIST := time.Date(2026, time.November, 18, 9, 15, 0, 0, candles.LocationIST)
		err := candles.Timeframe1m.ValidateBucketTime(tIST)
		if err == nil {
			t.Fatal("expected validation error for non-UTC timestamp, got nil")
		}
		if !errors.Is(err, candles.ErrInvalidBar) {
			t.Fatalf("expected ErrInvalidBar for non-UTC timestamp, got %v", err)
		}
	})

	t.Run("1s alignment", func(t *testing.T) {
		tExact := time.Date(2026, time.November, 18, 9, 15, 30, 0, candles.LocationIST).UTC()
		if err := candles.Timeframe1s.ValidateBucketTime(tExact); err != nil {
			t.Fatalf("expected 09:15:30 to be valid 1s bar, got %v", err)
		}

		tSubSec := time.Date(2026, time.November, 18, 9, 15, 30, 250000000, candles.LocationIST).UTC()
		if err := candles.Timeframe1s.ValidateBucketTime(tSubSec); err == nil {
			t.Fatal("expected 1s validation to fail on sub-second bar")
		}
	})

	t.Run("1d alignment in IST", func(t *testing.T) {
		// Daily candle starts at 00:00:00 IST
		tDay := time.Date(2026, time.November, 18, 0, 0, 0, 0, candles.LocationIST).UTC()
		if err := candles.Timeframe1d.ValidateBucketTime(tDay); err != nil {
			t.Fatalf("expected 00:00:00 IST to be valid 1d bar, got %v", err)
		}

		// Intraday time on 1d should fail
		tIntra := time.Date(2026, time.November, 18, 9, 15, 0, 0, candles.LocationIST).UTC()
		if err := candles.Timeframe1d.ValidateBucketTime(tIntra); err == nil {
			t.Fatal("expected 09:15 on 1d to fail validation")
		}
	})

	t.Run("1mo alignment in IST", func(t *testing.T) {
		tMidMonth := time.Date(2026, time.November, 18, 14, 30, 0, 0, candles.LocationIST)
		aligned := candles.Timeframe1mo.Align(tMidMonth)
		expected := time.Date(2026, time.November, 1, 0, 0, 0, 0, candles.LocationIST).UTC()
		if !aligned.Equal(expected) {
			t.Fatalf("expected 1mo aligned %s, got %s", expected.Format(time.RFC3339), aligned.Format(time.RFC3339))
		}
	})
}

func TestTimeframe_Constraints(t *testing.T) {
	// Raw timeframes are writable
	if !candles.Timeframe1s.IsWritable() {
		t.Fatal("expected 1s to be writable")
	}
	if !candles.Timeframe1m.IsWritable() {
		t.Fatal("expected 1m to be writable")
	}

	// Higher timeframes must NOT be writable by application
	nonWritable := []candles.Timeframe{
		candles.Timeframe5m, candles.Timeframe10m, candles.Timeframe15m,
		candles.Timeframe30m, candles.Timeframe1h, candles.Timeframe3h,
		candles.Timeframe1d, candles.Timeframe15d, candles.Timeframe1mo,
		candles.Timeframe6mo, candles.Timeframe1y,
	}

	for _, tf := range nonWritable {
		if tf.IsWritable() {
			t.Errorf("expected %s NOT to be writable", tf)
		}
		if err := tf.ValidateWriteSupport(); err == nil {
			t.Errorf("expected ValidateWriteSupport to fail for %s", tf)
		}
	}
}

func TestValidateBar(t *testing.T) {
	validTime := time.Date(2026, time.November, 18, 9, 15, 0, 0, candles.LocationIST).UTC()

	t.Run("Valid bar", func(t *testing.T) {
		bar, err := candles.NewCandleFromFloats(1, validTime, 2500.0, 2510.0, 2490.0, 2505.0, 1000, 500)
		if err != nil {
			t.Fatalf("unexpected error creating candle: %v", err)
		}
		if err := candles.Timeframe1m.ValidateBar(bar); err != nil {
			t.Fatalf("expected valid bar, got %v", err)
		}
	})

	t.Run("Negative stock_id rejected", func(t *testing.T) {
		bar, _ := candles.NewCandleFromFloats(-1, validTime, 2500, 2510, 2490, 2505, 100, 0)
		if err := candles.Timeframe1m.ValidateBar(bar); err == nil {
			t.Fatal("expected error for negative stock_id")
		}
	})

	t.Run("High less than low rejected", func(t *testing.T) {
		bar, _ := candles.NewCandleFromFloats(1, validTime, 2500, 2480, 2510, 2500, 100, 0)
		if err := candles.Timeframe1m.ValidateBar(bar); err == nil {
			t.Fatal("expected error for high < low")
		}
	})

	t.Run("Open outside low-high rejected", func(t *testing.T) {
		bar, _ := candles.NewCandleFromFloats(1, validTime, 2520, 2510, 2490, 2500, 100, 0)
		if err := candles.Timeframe1m.ValidateBar(bar); err == nil {
			t.Fatal("expected error for open > high")
		}
	})

	t.Run("Close outside low-high rejected", func(t *testing.T) {
		bar, _ := candles.NewCandleFromFloats(1, validTime, 2500, 2510, 2490, 2480, 100, 0)
		if err := candles.Timeframe1m.ValidateBar(bar); err == nil {
			t.Fatal("expected error for close < low")
		}
	})

	t.Run("Negative volume rejected", func(t *testing.T) {
		bar, _ := candles.NewCandleFromFloats(1, validTime, 2500, 2510, 2490, 2500, -10, 0)
		if err := candles.Timeframe1m.ValidateBar(bar); err == nil {
			t.Fatal("expected error for negative volume")
		}
	})

	t.Run("is_adjusted = true rejected in SS-003", func(t *testing.T) {
		bar, _ := candles.NewCandleFromFloats(1, validTime, 2500, 2510, 2490, 2500, 100, 0)
		bar.IsAdjusted = true
		if err := candles.Timeframe1m.ValidateBar(bar); err == nil {
			t.Fatal("expected error for is_adjusted = true in raw-only storage")
		}
	})
}

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	store := candles.NewMemoryStore()

	t1 := time.Date(2026, time.November, 18, 9, 15, 0, 0, candles.LocationIST).UTC()
	t2 := time.Date(2026, time.November, 18, 9, 16, 0, 0, candles.LocationIST).UTC()

	bar1, _ := candles.NewCandleFromFloats(1, t1, 2500, 2510, 2495, 2505, 100, 10)
	bar2, _ := candles.NewCandleFromFloats(1, t2, 2505, 2515, 2500, 2512, 150, 15)

	if err := store.UpsertBatch(ctx, candles.Timeframe1m, []candles.Candle{bar1, bar2}); err != nil {
		t.Fatalf("unexpected error upserting: %v", err)
	}

	// Range query
	bars, err := store.Range(ctx, 1, candles.Timeframe1m, t1, t2)
	if err != nil {
		t.Fatalf("unexpected Range error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(bars))
	}
	if !bars[0].BucketTime.Equal(t1) || !bars[1].BucketTime.Equal(t2) {
		t.Fatalf("bars not in ascending order: %v, %v", bars[0].BucketTime, bars[1].BucketTime)
	}

	// Latest & LatestTime
	latest, err := store.Latest(ctx, 1, candles.Timeframe1m)
	if err != nil {
		t.Fatalf("unexpected Latest error: %v", err)
	}
	if !latest.BucketTime.Equal(t2) {
		t.Fatalf("expected latest time %s, got %s", t2, latest.BucketTime)
	}

	latestTime, err := store.LatestTime(ctx, 1, candles.Timeframe1m)
	if err != nil {
		t.Fatalf("unexpected LatestTime error: %v", err)
	}
	if !latestTime.Equal(t2) {
		t.Fatalf("expected latest time %s, got %s", t2, latestTime)
	}

	// In-place update (idempotency)
	bar2Updated, _ := candles.NewCandleFromFloats(1, t2, 2505, 2520, 2500, 2518, 200, 25)
	if err := store.UpsertBatch(ctx, candles.Timeframe1m, []candles.Candle{bar2Updated}); err != nil {
		t.Fatalf("failed re-upserting bar2: %v", err)
	}
	barsAfter, _ := store.Range(ctx, 1, candles.Timeframe1m, t1, t2)
	if len(barsAfter) != 2 {
		t.Fatalf("expected count to remain 2 after update, got %d", len(barsAfter))
	}
	latestAfter, _ := store.Latest(ctx, 1, candles.Timeframe1m)
	cPrice, _ := latestAfter.CloseFloat()
	if cPrice != 2518.0 {
		t.Fatalf("expected updated close 2518.0, got %f", cPrice)
	}

	t.Run("Pre-1970 and EpochIST timestamp handling in Latest and LatestTime", func(t *testing.T) {
		memStore := candles.NewMemoryStore()
		epochBar, err := candles.NewCandleFromFloats(42, candles.EpochIST, 100, 105, 95, 102, 1000, 0)
		if err != nil {
			t.Fatalf("failed creating epoch candle: %v", err)
		}
		if err := memStore.UpsertBatch(ctx, candles.Timeframe1m, []candles.Candle{epochBar}); err != nil {
			t.Fatalf("failed upserting epoch candle: %v", err)
		}

		gotLatest, err := memStore.Latest(ctx, 42, candles.Timeframe1m)
		if err != nil {
			t.Fatalf("Latest failed for epoch bar: %v", err)
		}
		if !gotLatest.BucketTime.Equal(candles.EpochIST) {
			t.Fatalf("expected latest time %s, got %s", candles.EpochIST, gotLatest.BucketTime)
		}

		gotTime, err := memStore.LatestTime(ctx, 42, candles.Timeframe1m)
		if err != nil {
			t.Fatalf("LatestTime failed for epoch bar: %v", err)
		}
		if !gotTime.Equal(candles.EpochIST) {
			t.Fatalf("expected latestTime %s, got %s", candles.EpochIST, gotTime)
		}
	})
}
