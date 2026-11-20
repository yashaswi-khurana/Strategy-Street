package marketcal

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	candles "github.com/strategy-street/api/internal/store/candles"
	metadata "github.com/strategy-street/api/internal/store/metadata"
)

// LocationIST is the standard Asia/Kolkata timezone location (+05:30).
var LocationIST = metadata.LocationIST

// SpecialSession defines non-standard trading hours for a specific date (e.g. Muhurat trading).
type SpecialSession struct {
	Date      time.Time `json:"date"`
	OpenTime  string    `json:"open_time"`  // "HH:MM:SS"
	CloseTime string    `json:"close_time"` // "HH:MM:SS"
	Label     string    `json:"label"`
}

// Calendar provides the single authoritative calendar for market sessions, exchange holidays,
// and deterministic bar timing across the Strategy Street platform.
type Calendar struct {
	mu              sync.RWMutex
	holidays        map[string]string         // "YYYY-MM-DD" -> holiday label
	specialSessions map[string]SpecialSession // "YYYY-MM-DD" -> SpecialSession
	loc             *time.Location
	db              metadata.Queryer
	lastRefresh     time.Time
	refreshInterval time.Duration
}

// NewCalendar constructs a Calendar with explicit in-memory holidays and special sessions.
func NewCalendar(holidays map[string]string, sessions map[string]SpecialSession) *Calendar {
	hCopy := make(map[string]string, len(holidays))
	for k, v := range holidays {
		hCopy[k] = v
	}
	sCopy := make(map[string]SpecialSession, len(sessions))
	for k, v := range sessions {
		sCopy[k] = v
	}
	return &Calendar{
		holidays:        hCopy,
		specialSessions: sCopy,
		loc:             LocationIST,
		refreshInterval: 1 * time.Hour,
	}
}

// NewDefaultCalendar initializes a Calendar seeded with the standard NSE holidays and Muhurat sessions (2026-2027).
func NewDefaultCalendar() *Calendar {
	holidays := map[string]string{
		// 2026
		"2026-01-26": "Republic Day",
		"2026-03-03": "Holi",
		"2026-03-20": "Id-Ul-Fitr (Ramzan Id)",
		"2026-03-27": "Shri Ram Navami",
		"2026-04-03": "Good Friday",
		"2026-04-14": "Dr. Baba Saheb Ambedkar Jayanti",
		"2026-05-01": "Maharashtra Day",
		"2026-05-27": "Bakri Id / Eid ul-Adha",
		"2026-06-26": "Muharram",
		"2026-08-15": "Independence Day",
		"2026-09-04": "Milad-un-Nabi (Id-e-Milad)",
		"2026-10-02": "Mahatma Gandhi Jayanti",
		"2026-10-20": "Dussehra",
		"2026-11-08": "Diwali Laxmi Pujan",
		"2026-11-10": "Diwali Balipratipada",
		"2026-11-24": "Gurunanak Jayanti",
		"2026-12-25": "Christmas",
		// 2027
		"2027-01-26": "Republic Day",
		"2027-03-22": "Holi",
		"2027-03-26": "Good Friday",
		"2027-04-14": "Dr. Baba Saheb Ambedkar Jayanti",
		"2027-05-01": "Maharashtra Day",
		"2027-08-15": "Independence Day",
		"2027-10-02": "Mahatma Gandhi Jayanti",
		"2027-10-09": "Dussehra",
		"2027-10-28": "Diwali Laxmi Pujan",
		"2027-10-29": "Diwali Balipratipada",
		"2027-11-14": "Gurunanak Jayanti",
		"2027-12-25": "Christmas",
	}

	specialSessions := map[string]SpecialSession{
		"2026-11-08": {
			Date:      time.Date(2026, 11, 8, 0, 0, 0, 0, LocationIST),
			OpenTime:  "18:15:00",
			CloseTime: "19:15:00",
			Label:     "muhurat",
		},
		"2027-10-28": {
			Date:      time.Date(2027, 10, 28, 0, 0, 0, 0, LocationIST),
			OpenTime:  "18:15:00",
			CloseTime: "19:15:00",
			Label:     "muhurat",
		},
	}

	return NewCalendar(holidays, specialSessions)
}

// LoadCalendarFromDB queries exchange_holidays and market_sessions from the database and returns a cached Calendar.
func LoadCalendarFromDB(ctx context.Context, db metadata.Queryer) (*Calendar, error) {
	cal := &Calendar{
		holidays:        make(map[string]string),
		specialSessions: make(map[string]SpecialSession),
		loc:             LocationIST,
		db:              db,
		refreshInterval: 1 * time.Hour,
	}

	if err := cal.refresh(ctx); err != nil {
		return nil, err
	}
	return cal, nil
}

// RefreshIfNeeded checks if the 1-hour cache TTL has expired and re-fetches from DB if configured.
func (c *Calendar) RefreshIfNeeded(ctx context.Context) error {
	c.mu.RLock()
	if c.db == nil || time.Since(c.lastRefresh) < c.refreshInterval {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	// Re-check under write lock
	if c.db == nil || time.Since(c.lastRefresh) < c.refreshInterval {
		return nil
	}
	return c.refreshLocked(ctx)
}

func (c *Calendar) refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refreshLocked(ctx)
}

func (c *Calendar) refreshLocked(ctx context.Context) error {
	if c.db == nil {
		return nil
	}

	// 1. Fetch holidays
	const holidayQuery = `SELECT to_char(holiday, 'YYYY-MM-DD'), label FROM exchange_holidays WHERE exchange = 'NSE';`
	hRows, err := c.db.Query(ctx, holidayQuery)
	if err != nil {
		return fmt.Errorf("marketcal: failed querying holidays: %w", err)
	}
	defer hRows.Close()

	holidays := make(map[string]string)
	for hRows.Next() {
		var dStr, label string
		if err := hRows.Scan(&dStr, &label); err != nil {
			return fmt.Errorf("marketcal: scanning holiday row: %w", err)
		}
		holidays[dStr] = label
	}
	if err := hRows.Err(); err != nil {
		return fmt.Errorf("marketcal: reading holiday rows: %w", err)
	}

	// 2. Fetch special market sessions
	const sessionQuery = `
		SELECT to_char(session_date, 'YYYY-MM-DD'), to_char(open_time, 'HH24:MI:SS'), to_char(close_time, 'HH24:MI:SS'), coalesce(label, '')
		FROM market_sessions
		WHERE exchange = 'NSE';
	`
	sRows, err := c.db.Query(ctx, sessionQuery)
	if err != nil {
		return fmt.Errorf("marketcal: failed querying market sessions: %w", err)
	}
	defer sRows.Close()

	sessions := make(map[string]SpecialSession)
	for sRows.Next() {
		var dStr, openStr, closeStr, label string
		if err := sRows.Scan(&dStr, &openStr, &closeStr, &label); err != nil {
			return fmt.Errorf("marketcal: scanning market session row: %w", err)
		}
		d, _ := time.ParseInLocation("2006-01-02", dStr, c.loc)
		sessions[dStr] = SpecialSession{
			Date:      d,
			OpenTime:  openStr,
			CloseTime: closeStr,
			Label:     label,
		}
	}
	if err := sRows.Err(); err != nil {
		return fmt.Errorf("marketcal: reading market session rows: %w", err)
	}

	c.holidays = holidays
	c.specialSessions = sessions
	c.lastRefresh = time.Now()
	return nil
}

// Session returns the market open and close instants in Asia/Kolkata for the specified calendar day.
// Returns ok=false for holidays and weekends (unless a special session is explicitly scheduled).
// All date and time evaluations strictly convert to Asia/Kolkata at the boundary.
func (c *Calendar) Session(day time.Time) (open, close time.Time, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	istDay := day.In(c.loc)
	dateKey := istDay.Format("2006-01-02")
	year, month, d := istDay.Date()

	// 1. Check special session override first (e.g. Saturday disaster recovery or Muhurat trading)
	if spec, hasSpecial := c.specialSessions[dateKey]; hasSpecial {
		var openH, openM, openS int
		var closeH, closeM, closeS int
		_, _ = fmt.Sscanf(spec.OpenTime, "%d:%d:%d", &openH, &openM, &openS)
		_, _ = fmt.Sscanf(spec.CloseTime, "%d:%d:%d", &closeH, &closeM, &closeS)
		open = time.Date(year, month, d, openH, openM, openS, 0, c.loc)
		close = time.Date(year, month, d, closeH, closeM, closeS, 0, c.loc)
		return open, close, true
	}

	// 2. Check exchange holidays
	if _, isHoliday := c.holidays[dateKey]; isHoliday {
		return time.Time{}, time.Time{}, false
	}

	// 3. Check regular weekend (Saturday or Sunday)
	weekday := istDay.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return time.Time{}, time.Time{}, false
	}

	// 4. Regular NSE trading session: [09:15:00, 15:30:00) IST
	open = time.Date(year, month, d, 9, 15, 0, 0, c.loc)
	close = time.Date(year, month, d, 15, 30, 0, 0, c.loc)
	return open, close, true
}

// IsOpen returns true if timestamp t is within a valid trading session.
// Enforces half-open interval semantics: open <= t < close.
// A tick at exactly 15:30:00 IST belongs to NO bar.
func (c *Calendar) IsOpen(t time.Time) bool {
	open, closeTime, ok := c.Session(t)
	if !ok {
		return false
	}
	// Absolute time comparisons; open <= t < closeTime
	return !t.Before(open) && t.Before(closeTime)
}

// TradingDays returns a sorted slice of active trading dates (midnight IST) in the range [from, to] (inclusive).
// Excludes weekends and exchange holidays, but includes special weekend trading sessions.
func (c *Calendar) TradingDays(from, to time.Time) []time.Time {
	fromIST := metadata.ISTDate(from)
	toIST := metadata.ISTDate(to)
	if fromIST.After(toIST) {
		return nil
	}

	days := make([]time.Time, 0)
	for cur := fromIST; !cur.After(toIST); cur = cur.AddDate(0, 0, 1) {
		if _, _, ok := c.Session(cur); ok {
			days = append(days, cur)
		}
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i].Before(days[j])
	})
	return days
}

// BarsPerSession returns the exact number of bars expected in a standard regular trading session (09:15 - 15:30 IST).
// 375 for 1m, 22500 for 1s, 75 for 5m, 38 for 10m, 25 for 15m, 13 for 30m, 7 for 1h, 3 for 3h, 1 for 1d.
func BarsPerSession(tf candles.Timeframe) (int, error) {
	switch tf {
	case candles.Timeframe1s:
		return 22500, nil
	case candles.Timeframe1m:
		return 375, nil
	case candles.Timeframe5m:
		return 75, nil
	case candles.Timeframe10m:
		return 38, nil
	case candles.Timeframe15m:
		return 25, nil
	case candles.Timeframe30m:
		return 13, nil
	case candles.Timeframe1h:
		return 7, nil
	case candles.Timeframe3h:
		return 3, nil
	case candles.Timeframe1d:
		return 1, nil
	default:
		return 0, fmt.Errorf("%w: cannot compute bars per session for timeframe %s", candles.ErrUnsupportedTimeframe, tf)
	}
}

// MustBarsPerSession returns the number of bars per regular session, panicking on invalid timeframe.
func MustBarsPerSession(tf candles.Timeframe) int {
	n, err := BarsPerSession(tf)
	if err != nil {
		panic(err)
	}
	return n
}

// NextBarClose calculates the deterministic bar-close instant for the current timestamp now and timeframe tf.
// Returns (zero, false) if now is outside a valid session (before open, at or after close, or on a non-trading day).
// It is pure and does not depend on wall-clock seconds.
// For 1m with now = 09:16:12 IST, returns 09:17:00 IST.
// For 1m with now = 15:29:59 IST, returns 15:30:00 IST.
// For 1m with now = 15:30:00 IST, returns (zero, false).
func (c *Calendar) NextBarClose(now time.Time, tf candles.Timeframe) (time.Time, bool) {
	open, closeTime, ok := c.Session(now)
	if !ok {
		return time.Time{}, false
	}

	// now must be inside the session: [open, closeTime)
	if now.Before(open) || !now.Before(closeTime) {
		return time.Time{}, false
	}

	// For daily bars, the close instant is the session close.
	if tf == candles.Timeframe1d {
		return closeTime, true
	}

	dur := tf.Duration()
	if dur <= 0 {
		return closeTime, true
	}

	// Determine bar start boundary using timeframe alignment
	barStart := tf.Align(now)
	nextClose := barStart.Add(dur)

	// If next close would exceed session close, cap at session close
	if nextClose.After(closeTime) {
		nextClose = closeTime
	}

	return nextClose, true
}

