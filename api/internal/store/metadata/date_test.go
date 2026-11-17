package metadata

import (
	"testing"
	"time"
)

func TestISTDateHelper(t *testing.T) {
	t.Run("UTC 20:00 rolls over to next IST day", func(t *testing.T) {
		// UTC 20:00 on 2026-01-15 + 05:30 = 01:30 on 2026-01-16 IST
		utcTime := time.Date(2026, time.January, 15, 20, 0, 0, 0, time.UTC)

		istConverted := ToIST(utcTime)
		if istConverted.Day() != 16 {
			t.Fatalf("expected IST day 16, got %d (full: %s)", istConverted.Day(), istConverted)
		}
		if istConverted.Hour() != 1 || istConverted.Minute() != 30 {
			t.Fatalf("expected IST time 01:30, got %02d:%02d", istConverted.Hour(), istConverted.Minute())
		}

		istStr := ISTDateString(utcTime)
		if istStr != "2026-01-16" {
			t.Fatalf("expected IST date string '2026-01-16', got %q", istStr)
		}

		istDate := ISTDate(utcTime)
		expectedDate := time.Date(2026, time.January, 16, 0, 0, 0, 0, LocationIST)
		if !istDate.Equal(expectedDate) {
			t.Fatalf("expected IST date %v, got %v", expectedDate, istDate)
		}
	})

	t.Run("Midnight boundary at UTC 18:30:00", func(t *testing.T) {
		// UTC 18:29:59 is still the same day in IST (23:59:59)
		beforeBoundary := time.Date(2026, time.June, 10, 18, 29, 59, 0, time.UTC)
		if ISTDateString(beforeBoundary) != "2026-06-10" {
			t.Fatalf("expected same day before 18:30 UTC, got %s", ISTDateString(beforeBoundary))
		}

		// UTC 18:30:00 rolls over exactly at midnight to next day in IST (00:00:00)
		atBoundary := time.Date(2026, time.June, 10, 18, 30, 0, 0, time.UTC)
		if ISTDateString(atBoundary) != "2026-06-11" {
			t.Fatalf("expected next day at 18:30 UTC, got %s", ISTDateString(atBoundary))
		}
	})
}
