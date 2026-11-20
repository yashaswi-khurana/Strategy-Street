package metadata

import (
	"context"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata"
)

// ErrNotFound is the sentinel error returned when a requested metadata record is missing.
var ErrNotFound = errors.New("metadata: not found")

// NotFoundError is a typed error detailing the entity and lookup key that was not found.
type NotFoundError struct {
	Entity string
	Key    string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("metadata: %s %q not found", e.Entity, e.Key)
}

func (e *NotFoundError) Is(target error) bool {
	if target == ErrNotFound {
		return true
	}
	if t, ok := target.(*NotFoundError); ok {
		return (t.Entity == "" || t.Entity == e.Entity) && (t.Key == "" || t.Key == e.Key)
	}
	return false
}

// LocationIST is the standard Asia/Kolkata timezone location used across the platform.
var LocationIST *time.Location

func init() {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.FixedZone("IST", 5*3600+30*60)
	}
	LocationIST = loc
}

// ToIST converts any time.Time to the Asia/Kolkata time zone.
func ToIST(t time.Time) time.Time {
	return t.In(LocationIST)
}

// ISTDate converts any time.Time into a midnight (00:00:00) time.Time in the Asia/Kolkata zone.
func ISTDate(t time.Time) time.Time {
	ist := t.In(LocationIST)
	return time.Date(ist.Year(), ist.Month(), ist.Day(), 0, 0, 0, 0, LocationIST)
}

// ISTDateString formats any time.Time as "YYYY-MM-DD" according to Asia/Kolkata calendar day.
func ISTDateString(t time.Time) string {
	return t.In(LocationIST).Format("2006-01-02")
}

// Sector represents an industry classification row.
type Sector struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	BenchmarkIndex *string   `json:"benchmark_index,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// Stock represents a tradeable equity instrument on the exchange.
type Stock struct {
	ID            int64      `json:"id"`
	InstrumentKey string     `json:"instrument_key"` // Upstox format, e.g. 'NSE_EQ|INE002A01018'
	Symbol        string     `json:"symbol"`         // Human/UI ticker, e.g. 'RELIANCE'
	CompanyName   string     `json:"company_name"`
	Exchange      string     `json:"exchange"`
	SectorID      *int64     `json:"sector_id,omitempty"`
	SectorName    *string    `json:"sector_name,omitempty"`
	LotSize       int        `json:"lot_size"`
	TickSize      float64    `json:"tick_size"`
	IsActive      bool       `json:"is_active"`
	ListingDate   *time.Time `json:"listing_date,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ExchangeHoliday represents a scheduled trading holiday for an exchange.
type ExchangeHoliday struct {
	Exchange string    `json:"exchange"`
	Holiday  time.Time `json:"holiday"`
	Label    string    `json:"label"`
}

// MarketSession represents trading hours for a specific date and exchange.
type MarketSession struct {
	Exchange    string    `json:"exchange"`
	SessionDate time.Time `json:"session_date"`
	OpenTime    string    `json:"open_time"`  // "HH:MM:SS"
	CloseTime   string    `json:"close_time"` // "HH:MM:SS"
	Label       string    `json:"label,omitempty"`
}

// MetadataStore specifies the repository contract for core market metadata.
type MetadataStore interface {
	GetStockBySymbol(ctx context.Context, symbol string) (*Stock, error)
	GetStockByInstrumentKey(ctx context.Context, instrumentKey string) (*Stock, error)
	ListStocksBySector(ctx context.Context, sector string) ([]*Stock, error)
	ListSectors(ctx context.Context) ([]*Sector, error)
	IsTradingDay(ctx context.Context, date time.Time) (bool, error)
	SessionFor(ctx context.Context, date time.Time) (*MarketSession, error)
}
