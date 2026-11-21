package corporate

import (
	"math"
	"testing"
	"time"
)

func floatPtr(v float64) *float64 {
	return &v
}

func parseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestComputeActionFactor(t *testing.T) {
	tests := []struct {
		name      string
		action    CorporateAction
		prevClose float64
		want      float64
		wantErr   bool
	}{
		{
			name: "1:5 split",
			action: CorporateAction{
				ActionType: ActionSplit,
				RatioFrom:  floatPtr(1.0),
				RatioTo:    floatPtr(5.0),
			},
			want:    0.20,
			wantErr: false,
		},
		{
			name: "1:10 split",
			action: CorporateAction{
				ActionType: ActionSplit,
				RatioFrom:  floatPtr(1.0),
				RatioTo:    floatPtr(10.0),
			},
			want:    0.10,
			wantErr: false,
		},
		{
			name: "1:1 bonus (from=1, to=1)",
			action: CorporateAction{
				ActionType: ActionBonus,
				RatioFrom:  floatPtr(1.0),
				RatioTo:    floatPtr(1.0),
			},
			want:    0.50,
			wantErr: false,
		},
		{
			name: "1:1 bonus (from=1, to=2 direct ratio)",
			action: CorporateAction{
				ActionType: ActionBonus,
				RatioFrom:  floatPtr(1.0),
				RatioTo:    floatPtr(2.0),
			},
			want:    0.50,
			wantErr: false,
		},
		{
			name: "1:2 bonus (1 bonus for 2 held, from=2, to=1)",
			action: CorporateAction{
				ActionType: ActionBonus,
				RatioFrom:  floatPtr(2.0),
				RatioTo:    floatPtr(1.0),
			},
			want:    2.0 / 3.0,
			wantErr: false,
		},
		{
			name: "dividend-only: Rs 50 on prev_close 1000",
			action: CorporateAction{
				ActionType:  ActionDividend,
				DividendAmt: floatPtr(50.0),
			},
			prevClose: 1000.0,
			want:      0.95,
			wantErr:   false,
		},
		{
			name: "dividend-only: Rs 28 on prev_close 3800",
			action: CorporateAction{
				ActionType:  ActionDividend,
				DividendAmt: floatPtr(28.0),
			},
			prevClose: 3800.0,
			want:      1.0 - (28.0 / 3800.0),
			wantErr:   false,
		},
		{
			name: "dividend with zero prevClose defaults to 1.0 safely",
			action: CorporateAction{
				ActionType:  ActionDividend,
				DividendAmt: floatPtr(10.0),
			},
			prevClose: 0.0,
			want:      1.0,
			wantErr:   false,
		},
		{
			name: "dividend exceeding prevClose fails",
			action: CorporateAction{
				ActionType:  ActionDividend,
				DividendAmt: floatPtr(150.0),
			},
			prevClose: 100.0,
			wantErr:   true,
		},
		{
			name: "invalid split ratio (zero ratio_to)",
			action: CorporateAction{
				ActionType: ActionSplit,
				RatioFrom:  floatPtr(1.0),
				RatioTo:    floatPtr(0.0),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeActionFactor(tt.action, tt.prevClose)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ComputeActionFactor() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if math.Abs(got-tt.want) > 1e-9 {
					t.Errorf("ComputeActionFactor() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestComputeCumulativeFactors_Chain(t *testing.T) {
	// Chain:
	// Action 1 (earlier): 1:5 Split on 2024-03-01
	// Action 2 (later): 1:1 Bonus on 2024-09-01
	//
	// For dates before 2024-03-01: affected by both split (0.2) and bonus (0.5) => 0.2 * 0.5 = 0.10
	// For dates between 2024-03-01 and 2024-09-01: affected only by bonus (0.5) => 0.50
	items := []ActionWithPrevClose{
		{
			Action: CorporateAction{
				StockID:    1,
				ActionType: ActionSplit,
				ExDate:     parseDate("2024-03-01"),
				RatioFrom:  floatPtr(1.0),
				RatioTo:    floatPtr(5.0),
				Verified:   true,
			},
		},
		{
			Action: CorporateAction{
				StockID:    1,
				ActionType: ActionBonus,
				ExDate:     parseDate("2024-09-01"),
				RatioFrom:  floatPtr(1.0),
				RatioTo:    floatPtr(1.0),
				Verified:   true,
			},
		},
	}

	factors, err := ComputeCumulativeFactors(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(factors) != 2 {
		t.Fatalf("expected 2 factors, got %d", len(factors))
	}

	// First element should be the latest action (2024-09-01) with factor 0.5
	if factors[0].ExDate != parseDate("2024-09-01") {
		t.Errorf("expected first factor to be for 2024-09-01, got %v", factors[0].ExDate)
	}
	if math.Abs(factors[0].Factor-0.50) > 1e-9 {
		t.Errorf("expected factor 0.50, got %f", factors[0].Factor)
	}

	// Second element should be earlier action (2024-03-01) with cumulative factor 0.10
	if factors[1].ExDate != parseDate("2024-03-01") {
		t.Errorf("expected second factor to be for 2024-03-01, got %v", factors[1].ExDate)
	}
	if math.Abs(factors[1].Factor-0.10) > 1e-9 {
		t.Errorf("expected cumulative factor 0.10, got %f", factors[1].Factor)
	}
}

func TestComputeCumulativeFactors_UnverifiedSkipped(t *testing.T) {
	items := []ActionWithPrevClose{
		{
			Action: CorporateAction{
				StockID:    1,
				ActionType: ActionSplit,
				ExDate:     parseDate("2024-03-01"),
				RatioFrom:  floatPtr(1.0),
				RatioTo:    floatPtr(5.0),
				Verified:   false, // unverified!
			},
		},
	}

	factors, err := ComputeCumulativeFactors(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(factors) != 0 {
		t.Errorf("expected 0 factors for unverified actions, got %d", len(factors))
	}
}
