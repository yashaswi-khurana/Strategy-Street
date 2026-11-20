package metadata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Queryer abstracts pgx query execution (satisfied by *pgxpool.Pool, *pgx.Conn, and pgx.Tx).
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store is the pgx-backed implementation of MetadataStore.
type Store struct {
	db Queryer
}

// NewStore creates a new metadata store backed by a pgx queryer.
func NewStore(db Queryer) *Store {
	return &Store{db: db}
}

// GetStockBySymbol retrieves an active or inactive stock record by its unique trading symbol.
func (s *Store) GetStockBySymbol(ctx context.Context, symbol string) (*Stock, error) {
	const query = `
		SELECT 
			s.id, s.instrument_key, s.symbol, s.company_name, s.exchange,
			s.sector_id, sec.name,
			s.lot_size, s.tick_size, s.is_active, s.listing_date, s.created_at
		FROM stocks s
		LEFT JOIN sectors sec ON s.sector_id = sec.id
		WHERE s.symbol = $1;
	`
	stock, err := s.scanStockRow(s.db.QueryRow(ctx, query, symbol))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &NotFoundError{Entity: "stock", Key: symbol}
		}
		return nil, fmt.Errorf("failed to get stock by symbol %q: %w", symbol, err)
	}
	return stock, nil
}

// GetStockByInstrumentKey retrieves a stock record by its Upstox instrument_key (e.g. 'NSE_EQ|INE002A01018').
func (s *Store) GetStockByInstrumentKey(ctx context.Context, instrumentKey string) (*Stock, error) {
	const query = `
		SELECT 
			s.id, s.instrument_key, s.symbol, s.company_name, s.exchange,
			s.sector_id, sec.name,
			s.lot_size, s.tick_size, s.is_active, s.listing_date, s.created_at
		FROM stocks s
		LEFT JOIN sectors sec ON s.sector_id = sec.id
		WHERE s.instrument_key = $1;
	`
	stock, err := s.scanStockRow(s.db.QueryRow(ctx, query, instrumentKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &NotFoundError{Entity: "stock", Key: instrumentKey}
		}
		return nil, fmt.Errorf("failed to get stock by instrument key %q: %w", instrumentKey, err)
	}
	return stock, nil
}

// ListStocksBySector retrieves all active stocks belonging to a specific sector name.
func (s *Store) ListStocksBySector(ctx context.Context, sector string) ([]*Stock, error) {
	const query = `
		SELECT 
			s.id, s.instrument_key, s.symbol, s.company_name, s.exchange,
			s.sector_id, sec.name,
			s.lot_size, s.tick_size, s.is_active, s.listing_date, s.created_at
		FROM stocks s
		JOIN sectors sec ON s.sector_id = sec.id
		WHERE sec.name = $1 AND s.is_active = TRUE
		ORDER BY s.symbol ASC;
	`
	rows, err := s.db.Query(ctx, query, sector)
	if err != nil {
		return nil, fmt.Errorf("failed to list stocks for sector %q: %w", sector, err)
	}
	defer rows.Close()

	stocks := make([]*Stock, 0)
	for rows.Next() {
		stock, err := s.scanStockRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed scanning stock row: %w", err)
		}
		stocks = append(stocks, stock)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading stock rows: %w", err)
	}
	return stocks, nil
}

// ListSectors retrieves all configured sectors ordered alphabetically by name.
func (s *Store) ListSectors(ctx context.Context) ([]*Sector, error) {
	const query = `
		SELECT id, name, benchmark_index, created_at
		FROM sectors
		ORDER BY name ASC;
	`
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list sectors: %w", err)
	}
	defer rows.Close()

	sectors := make([]*Sector, 0)
	for rows.Next() {
		var sec Sector
		if err := rows.Scan(&sec.ID, &sec.Name, &sec.BenchmarkIndex, &sec.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed scanning sector row: %w", err)
		}
		sec.CreatedAt = ToIST(sec.CreatedAt)
		sectors = append(sectors, &sec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading sector rows: %w", err)
	}
	return sectors, nil
}

// IsTradingDay returns true if the specified date is a trading day on the exchange.
// Date is explicitly evaluated in the Asia/Kolkata timezone. The database holds the single
// source of truth for exchange holidays and special market sessions (e.g. muhurat trading).
func (s *Store) IsTradingDay(ctx context.Context, date time.Time) (bool, error) {
	istDateStr := ISTDateString(date)
	const exchange = "NSE"

	const query = `
		SELECT (
			EXISTS (
				SELECT 1 FROM market_sessions 
				WHERE exchange = $1 AND session_date = $2::date
			)
			OR (
				EXTRACT(ISODOW FROM $2::date) < 6
				AND NOT EXISTS (
					SELECT 1 FROM exchange_holidays 
					WHERE exchange = $1 AND holiday = $2::date
				)
			)
		);
	`
	var isTradingDay bool
	err := s.db.QueryRow(ctx, query, exchange, istDateStr).Scan(&isTradingDay)
	if err != nil {
		return false, fmt.Errorf("failed checking trading day for date %s: %w", istDateStr, err)
	}
	return isTradingDay, nil
}

// SessionFor returns the market session (hours) for the given date in Asia/Kolkata.
// If a special session (e.g. muhurat) is configured, it is returned. If it is a normal trading day,
// standard NSE hours (09:15:00 - 15:30:00) are returned. If the market is closed (holiday or weekend),
// typed ErrNotFound is returned.
func (s *Store) SessionFor(ctx context.Context, date time.Time) (*MarketSession, error) {
	istDate := ISTDate(date)
	istDateStr := ISTDateString(date)
	const exchange = "NSE"

	const specialQuery = `
		SELECT exchange, session_date, to_char(open_time, 'HH24:MI:SS'), to_char(close_time, 'HH24:MI:SS'), coalesce(label, '')
		FROM market_sessions
		WHERE exchange = $1 AND session_date = $2::date;
	`
	var ms MarketSession
	var sessionDate time.Time
	err := s.db.QueryRow(ctx, specialQuery, exchange, istDateStr).Scan(
		&ms.Exchange,
		&sessionDate,
		&ms.OpenTime,
		&ms.CloseTime,
		&ms.Label,
	)
	if err == nil {
		ms.SessionDate = ISTDate(sessionDate)
		return &ms, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed querying market session: %w", err)
	}

	// Not a special session override: verify if it is an active trading day.
	isTradingDay, err := s.IsTradingDay(ctx, date)
	if err != nil {
		return nil, err
	}
	if !isTradingDay {
		return nil, &NotFoundError{Entity: "market_session", Key: istDateStr}
	}

	// Regular NSE session
	return &MarketSession{
		Exchange:    exchange,
		SessionDate: istDate,
		OpenTime:    "09:15:00",
		CloseTime:   "15:30:00",
		Label:       "regular",
	}, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanStockRow(scanner rowScanner) (*Stock, error) {
	var st Stock
	var sectorName *string
	var listingDate *time.Time

	err := scanner.Scan(
		&st.ID,
		&st.InstrumentKey,
		&st.Symbol,
		&st.CompanyName,
		&st.Exchange,
		&st.SectorID,
		&sectorName,
		&st.LotSize,
		&st.TickSize,
		&st.IsActive,
		&listingDate,
		&st.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	st.SectorName = sectorName
	if listingDate != nil {
		ld := ISTDate(*listingDate)
		st.ListingDate = &ld
	}
	st.CreatedAt = ToIST(st.CreatedAt)
	return &st, nil
}
