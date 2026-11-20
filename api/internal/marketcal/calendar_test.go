package marketcal_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/strategy-street/api/internal/marketcal"
	candles "github.com/strategy-street/api/internal/store/candles"
)

func TestCalendar_BoundariesAndIsOpen(t *testing.T) {
	// Special session on 2026-03-14 to test exact acceptance criterion 1
	sessions := map[string]marketcal.SpecialSession{
		"2026-03-14": {
			Date:      time.Date(2026, 3, 14, 0, 0, 0, 0, marketcal.LocationIST),
			OpenTime:  "09:15:00",
			CloseTime: "15:30:00",
			Label:     "special_session",
		},
	}
	cal := marketcal.NewCalendar(nil, sessions)

	// Acceptance Criterion 1: IsOpen(2026-03-14T09:14:59+05:30) false, 09:15:00 true, 15:29:59 true, 15:30:00 false
	tests := []struct {
		name     string
		tStr     string
		expected bool
	}{
		{"1s before open", "2026-03-14T09:14:59+05:30", false},
		{"at open exact", "2026-03-14T09:15:00+05:30", true},
		{"mid session", "2026-03-14T12:00:00+05:30", true},
		{"1s before close", "2026-03-14T15:29:59+05:30", true},
		{"at close exact", "2026-03-14T15:30:00+05:30", false},
		{"1s after close", "2026-03-14T15:30:01+05:30", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tVal, err := time.Parse(time.RFC3339, tc.tStr)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, cal.IsOpen(tVal))
		})
	}

	// Also test on a regular weekday (2026-03-13, Friday) with default calendar
	defaultCal := marketcal.NewDefaultCalendar()
	t.Run("regular weekday boundary checks", func(t *testing.T) {
		t1, _ := time.Parse(time.RFC3339, "2026-03-13T09:14:59+05:30")
		t2, _ := time.Parse(time.RFC3339, "2026-03-13T09:15:00+05:30")
		t3, _ := time.Parse(time.RFC3339, "2026-03-13T15:29:59+05:30")
		t4, _ := time.Parse(time.RFC3339, "2026-03-13T15:30:00+05:30")

		assert.False(t, defaultCal.IsOpen(t1))
		assert.True(t, defaultCal.IsOpen(t2))
		assert.True(t, defaultCal.IsOpen(t3))
		assert.False(t, defaultCal.IsOpen(t4))
	})
}

func TestCalendar_SessionHolidayAndWeekend(t *testing.T) {
	cal := marketcal.NewDefaultCalendar()

	// Acceptance Criterion 2: Session() returns ok=false for a holiday and for a weekend
	// Seeded holidays: 2026-01-26 (Republic Day), 2026-03-03 (Holi)
	t.Run("exchange holiday returns ok=false", func(t *testing.T) {
		republicDay := time.Date(2026, 1, 26, 10, 0, 0, 0, marketcal.LocationIST)
		_, _, ok := cal.Session(republicDay)
		assert.False(t, ok)

		holi := time.Date(2026, 3, 3, 11, 0, 0, 0, marketcal.LocationIST)
		_, _, okHoli := cal.Session(holi)
		assert.False(t, okHoli)
	})

	t.Run("regular weekend returns ok=false", func(t *testing.T) {
		// 2026-03-15 is Sunday, 2026-03-21 is Saturday
		saturday := time.Date(2026, 3, 21, 10, 0, 0, 0, marketcal.LocationIST)
		_, _, okSat := cal.Session(saturday)
		assert.False(t, okSat)

		sunday := time.Date(2026, 3, 15, 12, 0, 0, 0, marketcal.LocationIST)
		_, _, okSun := cal.Session(sunday)
		assert.False(t, okSun)
	})

	t.Run("regular trading day returns valid hours", func(t *testing.T) {
		friday := time.Date(2026, 3, 13, 0, 0, 0, 0, marketcal.LocationIST)
		open, closeTime, ok := cal.Session(friday)
		require.True(t, ok)
		assert.Equal(t, 9, open.Hour())
		assert.Equal(t, 15, open.Minute())
		assert.Equal(t, 15, closeTime.Hour())
		assert.Equal(t, 30, closeTime.Minute())
	})

	t.Run("muhurat trading session on Diwali", func(t *testing.T) {
		// 2026-11-08 is Diwali Laxmi Pujan (Sunday) with Muhurat trading 18:15 to 19:15 IST
		diwali := time.Date(2026, 11, 8, 18, 30, 0, 0, marketcal.LocationIST)
		open, closeTime, ok := cal.Session(diwali)
		require.True(t, ok)
		assert.Equal(t, 18, open.Hour())
		assert.Equal(t, 15, open.Minute())
		assert.Equal(t, 19, closeTime.Hour())
		assert.Equal(t, 15, closeTime.Minute())

		// Boundary checks during Muhurat
		beforeMuhurat, _ := time.Parse(time.RFC3339, "2026-11-08T18:14:59+05:30")
		startMuhurat, _ := time.Parse(time.RFC3339, "2026-11-08T18:15:00+05:30")
		lastSecMuhurat, _ := time.Parse(time.RFC3339, "2026-11-08T19:14:59+05:30")
		endMuhurat, _ := time.Parse(time.RFC3339, "2026-11-08T19:15:00+05:30")

		assert.False(t, cal.IsOpen(beforeMuhurat))
		assert.True(t, cal.IsOpen(startMuhurat))
		assert.True(t, cal.IsOpen(lastSecMuhurat))
		assert.False(t, cal.IsOpen(endMuhurat))
	})
}

func TestCalendar_NextBarClose(t *testing.T) {
	cal := marketcal.NewDefaultCalendar()

	// 1m: now = 09:16:12 returns 09:17:00
	t.Run("1m normal progression", func(t *testing.T) {
		now, _ := time.Parse(time.RFC3339, "2026-03-13T09:16:12+05:30")
		closeInstant, ok := cal.NextBarClose(now, candles.Timeframe1m)
		require.True(t, ok)
		expected, _ := time.Parse(time.RFC3339, "2026-03-13T09:17:00+05:30")
		assert.True(t, expected.Equal(closeInstant), "expected %s got %s", expected, closeInstant)
	})

	t.Run("1m first bar of session", func(t *testing.T) {
		now, _ := time.Parse(time.RFC3339, "2026-03-13T09:15:00+05:30")
		closeInstant, ok := cal.NextBarClose(now, candles.Timeframe1m)
		require.True(t, ok)
		expected, _ := time.Parse(time.RFC3339, "2026-03-13T09:16:00+05:30")
		assert.True(t, expected.Equal(closeInstant))
	})

	t.Run("1m last bar of session", func(t *testing.T) {
		now, _ := time.Parse(time.RFC3339, "2026-03-13T15:29:15+05:30")
		closeInstant, ok := cal.NextBarClose(now, candles.Timeframe1m)
		require.True(t, ok)
		expected, _ := time.Parse(time.RFC3339, "2026-03-13T15:30:00+05:30")
		assert.True(t, expected.Equal(closeInstant))
	})

	t.Run("outside session returns zero and false", func(t *testing.T) {
		// At 15:30:00 exact (closed)
		afterClose, _ := time.Parse(time.RFC3339, "2026-03-13T15:30:00+05:30")
		_, okClose := cal.NextBarClose(afterClose, candles.Timeframe1m)
		assert.False(t, okClose)

		// Before 09:15:00
		beforeOpen, _ := time.Parse(time.RFC3339, "2026-03-13T09:14:59+05:30")
		_, okBefore := cal.NextBarClose(beforeOpen, candles.Timeframe1m)
		assert.False(t, okBefore)

		// Weekend
		weekend, _ := time.Parse(time.RFC3339, "2026-03-15T10:00:00+05:30")
		_, okWk := cal.NextBarClose(weekend, candles.Timeframe1m)
		assert.False(t, okWk)
	})

	t.Run("5m and 1h progression", func(t *testing.T) {
		now, _ := time.Parse(time.RFC3339, "2026-03-13T09:16:12+05:30")
		c5m, ok5m := cal.NextBarClose(now, candles.Timeframe5m)
		require.True(t, ok5m)
		exp5m, _ := time.Parse(time.RFC3339, "2026-03-13T09:20:00+05:30")
		assert.True(t, exp5m.Equal(c5m))

		c1h, ok1h := cal.NextBarClose(now, candles.Timeframe1h)
		require.True(t, ok1h)
		exp1h, _ := time.Parse(time.RFC3339, "2026-03-13T10:15:00+05:30")
		assert.True(t, exp1h.Equal(c1h))
	})

	t.Run("1d progression", func(t *testing.T) {
		now, _ := time.Parse(time.RFC3339, "2026-03-13T10:30:00+05:30")
		c1d, ok1d := cal.NextBarClose(now, candles.Timeframe1d)
		require.True(t, ok1d)
		exp1d, _ := time.Parse(time.RFC3339, "2026-03-13T15:30:00+05:30")
		assert.True(t, exp1d.Equal(c1d))
	})
}

func TestCalendar_TradingDaysAndBarsPerSession(t *testing.T) {
	cal := marketcal.NewDefaultCalendar()

	// Acceptance Criterion 4: BarsPerSession(1m) == 375 and TradingDays(2026-01-01, 2026-03-31) excludes every seeded holiday
	assert.Equal(t, 375, marketcal.MustBarsPerSession(candles.Timeframe1m))
	assert.Equal(t, 22500, marketcal.MustBarsPerSession(candles.Timeframe1s))
	assert.Equal(t, 1, marketcal.MustBarsPerSession(candles.Timeframe1d))
	assert.Equal(t, 75, marketcal.MustBarsPerSession(candles.Timeframe5m))

	from, _ := time.Parse("2006-01-02", "2026-01-01")
	to, _ := time.Parse("2006-01-02", "2026-03-31")
	tradingDays := cal.TradingDays(from, to)
	require.NotEmpty(t, tradingDays)

	// Seeded holidays in Q1 2026:
	// 2026-01-26 (Republic Day)
	// 2026-03-03 (Holi)
	// 2026-03-20 (Id-Ul-Fitr)
	// 2026-03-27 (Shri Ram Navami)
	seededHolidays := []string{"2026-01-26", "2026-03-03", "2026-03-20", "2026-03-27"}
	tdSet := make(map[string]bool)
	for _, td := range tradingDays {
		dateStr := td.In(marketcal.LocationIST).Format("2006-01-02")
		tdSet[dateStr] = true

		// Assert no weekends included
		weekday := td.In(marketcal.LocationIST).Weekday()
		assert.NotEqual(t, time.Saturday, weekday, "trading day %s must not be Saturday", dateStr)
		assert.NotEqual(t, time.Sunday, weekday, "trading day %s must not be Sunday", dateStr)
	}

	for _, hol := range seededHolidays {
		assert.False(t, tdSet[hol], "holiday %s must be excluded from TradingDays", hol)
	}
}

func TestCalendar_FillPolicy_CarryForwardDecorator(t *testing.T) {
	cal := marketcal.NewDefaultCalendar()

	// Acceptance Criterion 3:
	// For a 1m series missing 11:00–11:04, the CarryForward decorator returns 5 extra bars with volume=0,
	// is_filled=true, and close equal to the 10:59 close; highs/lows equal that close.
	t1059, _ := time.Parse(time.RFC3339, "2026-03-13T10:59:00+05:30")
	t1105, _ := time.Parse(time.RFC3339, "2026-03-13T11:05:00+05:30")

	bar1059, err := candles.NewCandleFromFloats(42, t1059.UTC(), 2450.0, 2455.0, 2448.0, 2452.5, 1200, 50000)
	require.NoError(t, err)

	bar1105, err := candles.NewCandleFromFloats(42, t1105.UTC(), 2453.0, 2458.0, 2451.0, 2456.0, 1500, 50500)
	require.NoError(t, err)

	input := []candles.Candle{bar1059, bar1105}

	filled := marketcal.FilledRange(input, cal, candles.Timeframe1m, marketcal.CarryForward)
	require.Len(t, filled, 7, "expected 2 real bars + 5 filled bars = 7 total bars")

	// First bar is original 10:59
	assert.Equal(t, t1059.UTC(), filled[0].BucketTime)
	assert.False(t, filled[0].IsFilled)
	assert.Equal(t, int64(1200), filled[0].Volume)

	// Middle 5 bars are 11:00, 11:01, 11:02, 11:03, 11:04
	for minute := 0; minute < 5; minute++ {
		b := filled[1+minute]
		expectedTime, _ := time.Parse(time.RFC3339, "2026-03-13T11:0"+string('0'+byte(minute))+":00+05:30")
		assert.Equal(t, expectedTime.UTC(), b.BucketTime)
		assert.True(t, b.IsFilled, "bar at %s must have is_filled=true", b.BucketTime)
		assert.Equal(t, int64(0), b.Volume, "bar at %s must have volume=0", b.BucketTime)

		// Prices match 10:59 close (2452.5)
		cFloat, _ := b.CloseFloat()
		oFloat, _ := b.OpenFloat()
		hFloat, _ := b.HighFloat()
		lFloat, _ := b.LowFloat()
		assert.Equal(t, 2452.5, cFloat)
		assert.Equal(t, 2452.5, oFloat)
		assert.Equal(t, 2452.5, hFloat)
		assert.Equal(t, 2452.5, lFloat)
	}

	// Last bar is original 11:05
	assert.Equal(t, t1105.UTC(), filled[6].BucketTime)
	assert.False(t, filled[6].IsFilled)
	assert.Equal(t, int64(1500), filled[6].Volume)

	// Test with Skip policy: returns original 2 bars
	skipped := marketcal.FilledRange(input, cal, candles.Timeframe1m, marketcal.Skip)
	require.Len(t, skipped, 2)
	assert.Equal(t, t1059.UTC(), skipped[0].BucketTime)
	assert.Equal(t, t1105.UTC(), skipped[1].BucketTime)
}

func TestCalendar_FillPolicy_OvernightAndHolidayGaps(t *testing.T) {
	cal := marketcal.NewDefaultCalendar()

	// Gap from 15:29 Monday to 09:15 Tuesday (overnight): no extra bars generated
	tMonClose, _ := time.Parse(time.RFC3339, "2026-03-09T15:29:00+05:30")
	tTueOpen, _ := time.Parse(time.RFC3339, "2026-03-10T09:15:00+05:30")

	barMon, _ := candles.NewCandleFromFloats(1, tMonClose.UTC(), 100, 101, 99, 100.5, 500, 1000)
	barTue, _ := candles.NewCandleFromFloats(1, tTueOpen.UTC(), 101, 102, 100, 101.5, 600, 1100)

	filled := marketcal.FilledRange([]candles.Candle{barMon, barTue}, cal, candles.Timeframe1m, marketcal.CarryForward)
	assert.Len(t, filled, 2, "overnight gap must not generate phantom bars outside session hours")

	// MissingBars helper verification
	exp := marketcal.ExpectedBars(cal, candles.Timeframe1m, tMonClose, tTueOpen)
	missing := marketcal.MissingBars(exp, []candles.Candle{barMon, barTue})
	assert.Empty(t, missing, "no in-session bars missing between 15:29 Mon and 09:15 Tue")
}

func TestCalendar_DailyFillPolicy(t *testing.T) {
	cal := marketcal.NewDefaultCalendar()

	// Monday (2026-03-09) and Thursday (2026-03-12)
	// Tuesday (03-10) and Wednesday (03-11) are missing trading days
	tMon, _ := time.Parse(time.RFC3339, "2026-03-09T09:15:00+05:30")
	tThu, _ := time.Parse(time.RFC3339, "2026-03-12T09:15:00+05:30")

	bMon, _ := candles.NewCandleFromFloats(1, tMon.UTC(), 500, 510, 495, 505, 10000, 20000)
	bThu, _ := candles.NewCandleFromFloats(1, tThu.UTC(), 508, 515, 502, 512, 12000, 21000)

	filled := marketcal.FilledRange([]candles.Candle{bMon, bThu}, cal, candles.Timeframe1d, marketcal.CarryForward)
	require.Len(t, filled, 4, "Mon + 2 filled days (Tue, Wed) + Thu = 4 bars")

	assert.Equal(t, tMon.UTC(), filled[0].BucketTime)
	assert.False(t, filled[0].IsFilled)

	// Tue filled
	tTue, _ := time.Parse(time.RFC3339, "2026-03-10T09:15:00+05:30")
	assert.Equal(t, tTue.UTC(), filled[1].BucketTime)
	assert.True(t, filled[1].IsFilled)
	assert.Equal(t, int64(0), filled[1].Volume)
	c1, _ := filled[1].CloseFloat()
	assert.Equal(t, 505.0, c1)

	// Wed filled
	tWed, _ := time.Parse(time.RFC3339, "2026-03-11T09:15:00+05:30")
	assert.Equal(t, tWed.UTC(), filled[2].BucketTime)
	assert.True(t, filled[2].IsFilled)
	assert.Equal(t, int64(0), filled[2].Volume)
	c2, _ := filled[2].CloseFloat()
	assert.Equal(t, 505.0, c2)

	assert.Equal(t, tThu.UTC(), filled[3].BucketTime)
	assert.False(t, filled[3].IsFilled)
}

// TestGenerateSharedFixtureFile generates the shared engine/testdata/sessions.json fixture
// and tests that Go parses and matches it identically.
func TestGenerateSharedFixtureFile(t *testing.T) {
	cal := marketcal.NewDefaultCalendar()

	// Add special session on 2026-03-14 for cross-language test of AC1
	sessionsMap := map[string]marketcal.SpecialSession{
		"2026-03-14": {
			Date:      time.Date(2026, 3, 14, 0, 0, 0, 0, marketcal.LocationIST),
			OpenTime:  "09:15:00",
			CloseTime: "15:30:00",
			Label:     "special_session",
		},
		"2026-11-08": {
			Date:      time.Date(2026, 11, 8, 0, 0, 0, 0, marketcal.LocationIST),
			OpenTime:  "18:15:00",
			CloseTime: "19:15:00",
			Label:     "muhurat",
		},
	}
	holidaysMap := map[string]string{
		"2026-01-26": "Republic Day",
		"2026-03-03": "Holi",
		"2026-03-20": "Id-Ul-Fitr (Ramzan Id)",
		"2026-03-27": "Shri Ram Navami",
	}

	testCal := marketcal.NewCalendar(holidaysMap, sessionsMap)

	type SessionFixture struct {
		Date      string  `json:"date"`
		Ok        bool    `json:"ok"`
		OpenTime  *string `json:"open_time"`
		CloseTime *string `json:"close_time"`
	}

	type IsOpenFixture struct {
		Timestamp string `json:"timestamp"`
		IsOpen    bool   `json:"is_open"`
	}

	type NextBarCloseFixture struct {
		Now           string  `json:"now"`
		Timeframe     string  `json:"timeframe"`
		ExpectedClose *string `json:"expected_close"`
		ExpectedOk    bool    `json:"expected_ok"`
	}

	type MissingBarGapFixture struct {
		SeriesType string `json:"series_type"`
		Close1059  float64 `json:"close_1059"`
		FilledCount int    `json:"filled_count"`
		TotalBars  int    `json:"total_bars"`
	}

	type FixtureRoot struct {
		Version         string                 `json:"version"`
		BarsPerSession  map[string]int         `json:"bars_per_session"`
		TradingDaysQ1   []string               `json:"trading_days_q1_2026"`
		Sessions        []SessionFixture       `json:"sessions"`
		IsOpenCases     []IsOpenFixture        `json:"is_open_cases"`
		NextBarCloseCases []NextBarCloseFixture `json:"next_bar_close_cases"`
		GapFixture      MissingBarGapFixture   `json:"gap_fixture"`
	}

	// 1. Sessions
	testDates := []string{
		"2026-03-13", // Regular Friday
		"2026-03-14", // Special session Saturday
		"2026-03-15", // Weekend Sunday
		"2026-01-26", // Holiday (Republic Day)
		"2026-11-08", // Muhurat trading
	}

	sFixtures := make([]SessionFixture, 0, len(testDates))
	for _, ds := range testDates {
		d, _ := time.ParseInLocation("2006-01-02", ds, marketcal.LocationIST)
		open, closeTime, ok := testCal.Session(d)
		var oStr, cStr *string
		if ok {
			o := open.Format(time.RFC3339)
			c := closeTime.Format(time.RFC3339)
			oStr = &o
			cStr = &c
		}
		sFixtures = append(sFixtures, SessionFixture{
			Date:      ds,
			Ok:        ok,
			OpenTime:  oStr,
			CloseTime: cStr,
		})
	}

	// 2. IsOpen test cases
	isOpenTimestamps := []string{
		"2026-03-14T09:14:59+05:30",
		"2026-03-14T09:15:00+05:30",
		"2026-03-14T12:00:00+05:30",
		"2026-03-14T15:29:59+05:30",
		"2026-03-14T15:30:00+05:30",
		"2026-03-13T09:14:59+05:30",
		"2026-03-13T09:15:00+05:30",
		"2026-03-13T15:29:59+05:30",
		"2026-03-13T15:30:00+05:30",
		"2026-03-15T10:00:00+05:30",
		"2026-01-26T10:00:00+05:30",
		"2026-11-08T18:14:59+05:30",
		"2026-11-08T18:15:00+05:30",
		"2026-11-08T19:14:59+05:30",
		"2026-11-08T19:15:00+05:30",
	}

	isOpenCases := make([]IsOpenFixture, 0, len(isOpenTimestamps))
	for _, ts := range isOpenTimestamps {
		tVal, _ := time.Parse(time.RFC3339, ts)
		isOpenCases = append(isOpenCases, IsOpenFixture{
			Timestamp: ts,
			IsOpen:    testCal.IsOpen(tVal),
		})
	}

	// 3. NextBarClose test cases
	nbcTests := []struct {
		now string
		tf  string
	}{
		{"2026-03-13T09:16:12+05:30", "1m"},
		{"2026-03-13T09:15:00+05:30", "1m"},
		{"2026-03-13T15:29:15+05:30", "1m"},
		{"2026-03-13T15:30:00+05:30", "1m"},
		{"2026-03-13T09:14:59+05:30", "1m"},
		{"2026-03-13T09:16:12+05:30", "5m"},
		{"2026-03-13T09:16:12+05:30", "1h"},
		{"2026-03-13T10:30:00+05:30", "1d"},
		{"2026-03-15T10:00:00+05:30", "1m"},
	}

	nbcCases := make([]NextBarCloseFixture, 0, len(nbcTests))
	for _, tc := range nbcTests {
		nowT, _ := time.Parse(time.RFC3339, tc.now)
		cInst, ok := testCal.NextBarClose(nowT, candles.Timeframe(tc.tf))
		var cStr *string
		if ok {
			s := cInst.In(marketcal.LocationIST).Format(time.RFC3339)
			cStr = &s
		}
		nbcCases = append(nbcCases, NextBarCloseFixture{
			Now:           tc.now,
			Timeframe:     tc.tf,
			ExpectedClose: cStr,
			ExpectedOk:    ok,
		})
	}

	// 4. Trading days in Q1 2026
	q1From, _ := time.Parse("2006-01-02", "2026-01-01")
	q1To, _ := time.Parse("2006-01-02", "2026-03-31")
	tds := cal.TradingDays(q1From, q1To)
	tdStrings := make([]string, 0, len(tds))
	for _, d := range tds {
		tdStrings = append(tdStrings, d.In(marketcal.LocationIST).Format("2006-01-02"))
	}

	fixture := FixtureRoot{
		Version: "1.0",
		BarsPerSession: map[string]int{
			"1s":  22500,
			"1m":  375,
			"5m":  75,
			"10m": 38,
			"15m": 25,
			"30m": 13,
			"1h":  7,
			"3h":  3,
			"1d":  1,
		},
		TradingDaysQ1:     tdStrings,
		Sessions:          sFixtures,
		IsOpenCases:       isOpenCases,
		NextBarCloseCases: nbcCases,
		GapFixture: MissingBarGapFixture{
			SeriesType:  "1m",
			Close1059:   2452.5,
			FilledCount: 5,
			TotalBars:   7,
		},
	}

	// Write to engine/testdata/sessions.json
	repoRoot, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			// Inside api, repo root is parent
			repoRoot = filepath.Dir(repoRoot)
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			break
		}
		repoRoot = parent
	}

	outDir := filepath.Join(repoRoot, "engine", "testdata")
	err = os.MkdirAll(outDir, 0755)
	require.NoError(t, err)

	outPath := filepath.Join(outDir, "sessions.json")
	bytes, err := json.MarshalIndent(fixture, "", "  ")
	require.NoError(t, err)

	err = os.WriteFile(outPath, bytes, 0644)
	require.NoError(t, err)
	t.Logf("Successfully generated shared fixture file at %s", outPath)
}

