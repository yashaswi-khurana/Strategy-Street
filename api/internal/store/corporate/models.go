package corporate

import (
	"encoding/json"
	"fmt"
	"time"
)

// ActionType represents the type of corporate action.
type ActionType string

const (
	ActionSplit    ActionType = "split"
	ActionBonus    ActionType = "bonus"
	ActionDividend ActionType = "dividend"
	ActionRights   ActionType = "rights"
)

// IsValid checks if the corporate action type is supported.
func (a ActionType) IsValid() bool {
	switch a {
	case ActionSplit, ActionBonus, ActionDividend, ActionRights:
		return true
	default:
		return false
	}
}

// CorporateAction represents a row in the corporate_actions table.
type CorporateAction struct {
	ID          int64      `json:"id"`
	StockID     int        `json:"stock_id"`
	ActionType  ActionType `json:"action_type"`
	ExDate      time.Time  `json:"ex_date"`
	RatioFrom   *float64   `json:"ratio_from,omitempty"`
	RatioTo     *float64   `json:"ratio_to,omitempty"`
	DividendAmt *float64   `json:"dividend_amt,omitempty"`
	Source      string     `json:"source"`
	Verified    bool       `json:"verified"`
	CreatedAt   time.Time  `json:"created_at"`
}

// AdjustmentFactor represents a row in the adjustment_factors table.
type AdjustmentFactor struct {
	StockID int       `json:"stock_id"`
	ExDate  time.Time `json:"ex_date"`
	Factor  float64   `json:"factor"`
}

// DataQualityIssue represents a row in the data_quality_issues table.
type DataQualityIssue struct {
	ID        int64          `json:"id"`
	StockID   int            `json:"stock_id"`
	IssueType string         `json:"issue_type"`
	CheckDate time.Time      `json:"check_date"`
	Details   map[string]any `json:"details"`
	Resolved  bool           `json:"resolved"`
	CreatedAt time.Time      `json:"created_at"`
}

// AdjustedCandle represents the result of the adjusted_candles SQL function.
type AdjustedCandle struct {
	BucketTime time.Time `json:"bucket_time"`
	Open       float64   `json:"open"`
	High       float64   `json:"high"`
	Low        float64   `json:"low"`
	Close      float64   `json:"close"`
	Volume     int64     `json:"volume"`
}

// ActionWithPrevClose pairs a corporate action with the previous day's close price.
type ActionWithPrevClose struct {
	Action    CorporateAction
	PrevClose float64
}

// FormatDetails formats details map into a JSON string.
func FormatDetails(d map[string]any) string {
	b, err := json.Marshal(d)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (a CorporateAction) String() string {
	switch a.ActionType {
	case ActionSplit, ActionBonus:
		from, to := 0.0, 0.0
		if a.RatioFrom != nil {
			from = *a.RatioFrom
		}
		if a.RatioTo != nil {
			to = *a.RatioTo
		}
		return fmt.Sprintf("[%s] %s %v:%v on %s (verified=%v)", a.ActionType, a.Source, from, to, a.ExDate.Format("2006-01-02"), a.Verified)
	case ActionDividend:
		amt := 0.0
		if a.DividendAmt != nil {
			amt = *a.DividendAmt
		}
		return fmt.Sprintf("[dividend] %s Rs %.2f on %s (verified=%v)", a.Source, amt, a.ExDate.Format("2006-01-02"), a.Verified)
	default:
		return fmt.Sprintf("[%s] on %s", a.ActionType, a.ExDate.Format("2006-01-02"))
	}
}

