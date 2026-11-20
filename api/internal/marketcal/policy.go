package marketcal

import (
	"fmt"
	"time"

	candles "github.com/strategy-street/api/internal/store/candles"
)

// FillPolicy defines how missing bars (intervals with zero market trades) are handled.
type FillPolicy int

const (
	// CarryForward repeats the previous bar's close for open/high/low/close with volume=0 and is_filled=true.
	// This is the platform default policy.
	CarryForward FillPolicy = iota
	// Skip omits missing bars entirely (returns only real bars with trades).
	Skip
)

func (p FillPolicy) String() string {
	switch p {
	case CarryForward:
		return "CarryForward"
	case Skip:
		return "Skip"
	default:
		return fmt.Sprintf("FillPolicy(%d)", p)
	}
}

// MissingBars identifies timestamps present in expected but missing from actual candles.
func MissingBars(expected []time.Time, actual []candles.Candle) []time.Time {
	actualMap := make(map[int64]struct{}, len(actual))
	for _, bar := range actual {
		actualMap[bar.BucketTime.UTC().UnixNano()] = struct{}{}
	}

	missing := make([]time.Time, 0)
	for _, exp := range expected {
		if _, ok := actualMap[exp.UTC().UnixNano()]; !ok {
			missing = append(missing, exp.UTC())
		}
	}
	return missing
}

// MissingBarTimes identifies timestamps present in expected but missing from actualTimes.
func MissingBarTimes(expected []time.Time, actualTimes []time.Time) []time.Time {
	actualMap := make(map[int64]struct{}, len(actualTimes))
	for _, t := range actualTimes {
		actualMap[t.UTC().UnixNano()] = struct{}{}
	}

	missing := make([]time.Time, 0)
	for _, exp := range expected {
		if _, ok := actualMap[exp.UTC().UnixNano()]; !ok {
			missing = append(missing, exp.UTC())
		}
	}
	return missing
}

// ExpectedBars generates the complete sequence of expected bar timestamps within [from, to] (inclusive)
// for the given timeframe that fall within valid market sessions according to the calendar.
func ExpectedBars(cal *Calendar, tf candles.Timeframe, from, to time.Time) []time.Time {
	if cal == nil {
		cal = NewDefaultCalendar()
	}
	fromUTC := from.UTC()
	toUTC := to.UTC()
	if fromUTC.After(toUTC) {
		return nil
	}

	if tf == candles.Timeframe1d {
		tradingDays := cal.TradingDays(fromUTC, toUTC)
		expected := make([]time.Time, 0, len(tradingDays))
		for _, day := range tradingDays {
			// Daily bucket time aligned to 09:15 IST
			istDay := day.In(cal.loc)
			bt := time.Date(istDay.Year(), istDay.Month(), istDay.Day(), 9, 15, 0, 0, cal.loc).UTC()
			if !bt.Before(fromUTC) && !bt.After(toUTC) {
				expected = append(expected, bt)
			}
		}
		return expected
	}

	dur := tf.Duration()
	if dur <= 0 {
		return nil
	}

	expected := make([]time.Time, 0)
	alignedStart := tf.Align(fromUTC)
	for cur := alignedStart; !cur.After(toUTC); cur = cur.Add(dur) {
		if cur.Before(fromUTC) {
			continue
		}
		if cal.IsOpen(cur) {
			expected = append(expected, cur)
		}
	}
	return expected
}

// FilledRange decorates an input slice of candles with missing bars synthesized under the given FillPolicy.
// Filled bars are generated only for valid in-session timestamps (never overnight, across weekends, or on exchange holidays).
// Under CarryForward, missing bars repeat the prior bar's close price as open, high, low, close with volume=0, oi=prior.oi,
// and is_filled=true. Under Skip, the input slice is returned without additions.
// This function is purely evaluated at read-time; filled bars must NEVER be persisted to storage.
func FilledRange(bars []candles.Candle, cal *Calendar, tf candles.Timeframe, policy FillPolicy) []candles.Candle {
	if cal == nil {
		cal = NewDefaultCalendar()
	}
	if len(bars) <= 1 || policy == Skip {
		out := make([]candles.Candle, len(bars))
		copy(out, bars)
		return out
	}

	result := make([]candles.Candle, 0, len(bars)*2)
	result = append(result, bars[0])

	if tf == candles.Timeframe1d {
		for i := 0; i < len(bars)-1; i++ {
			prev := bars[i]
			curr := bars[i+1]

			// Find trading days strictly between prev.BucketTime and curr.BucketTime
			tradingDays := cal.TradingDays(prev.BucketTime.AddDate(0, 0, 1), curr.BucketTime.AddDate(0, 0, -1))
			for _, day := range tradingDays {
				istDay := day.In(cal.loc)
				bucketTime := time.Date(istDay.Year(), istDay.Month(), istDay.Day(), 9, 15, 0, 0, cal.loc).UTC()
				filled := candles.Candle{
					StockID:    prev.StockID,
					BucketTime: bucketTime,
					Open:       prev.Close,
					High:       prev.Close,
					Low:        prev.Close,
					Close:      prev.Close,
					Volume:     0,
					OI:         prev.OI,
					IsAdjusted: prev.IsAdjusted,
					IsFilled:   true,
				}
				result = append(result, filled)
			}
			result = append(result, curr)
		}
		return result
	}

	step := tf.Duration()
	if step <= 0 {
		out := make([]candles.Candle, len(bars))
		copy(out, bars)
		return out
	}

	for i := 0; i < len(bars)-1; i++ {
		prev := bars[i]
		curr := bars[i+1]

		for t := prev.BucketTime.Add(step); t.Before(curr.BucketTime); t = t.Add(step) {
			if cal.IsOpen(t) {
				filled := candles.Candle{
					StockID:    prev.StockID,
					BucketTime: t.UTC(),
					Open:       prev.Close,
					High:       prev.Close,
					Low:        prev.Close,
					Close:      prev.Close,
					Volume:     0,
					OI:         prev.OI,
					IsAdjusted: prev.IsAdjusted,
					IsFilled:   true,
				}
				result = append(result, filled)
			}
		}
		result = append(result, curr)
	}

	return result
}

// FilledRange is a convenience method on Calendar that delegates to package-level FilledRange.
func (c *Calendar) FilledRange(bars []candles.Candle, tf candles.Timeframe, policy FillPolicy) []candles.Candle {
	return FilledRange(bars, c, tf, policy)
}
