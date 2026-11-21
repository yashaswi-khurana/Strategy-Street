package corporate

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

var (
	ErrInvalidRatio    = errors.New("invalid ratio: ratio_from and ratio_to must be positive numbers")
	ErrInvalidDividend = errors.New("invalid dividend: dividend_amt must be positive and strictly less than prevClose")
	ErrMissingPrevClose = errors.New("missing prevClose: prevClose must be greater than zero for dividend adjustment")
)

// ComputeActionFactor computes the single action factor for a corporate action.
//
// Rules (Backward Adjustment):
// 1. Split: factor = ratio_from / ratio_to. E.g. split 1:5 => 1/5 = 0.20.
// 2. Bonus:
//    - If ratio_to > ratio_from (direct conversion, e.g. from=1, to=2): factor = ratio_from / ratio_to = 0.50.
//    - If ratio_to <= ratio_from (bonus per held, e.g. 1:1 or from=2, to=1): factor = ratio_from / (ratio_from + ratio_to).
//      E.g. 1:1 bonus => 1 / (1 + 1) = 0.50.
//      E.g. 1:2 bonus (1 bonus for 2 held) => 2 / (2 + 1) = 0.6666666667.
// 3. Dividend: factor = 1.0 - (dividend_amt / prevClose).
// 4. Rights: if ratio_from > 0 and ratio_to > 0: ratio_from / ratio_to, else 1.0.
func ComputeActionFactor(action CorporateAction, prevClose float64) (float64, error) {
	switch action.ActionType {
	case ActionSplit:
		if action.RatioFrom == nil || action.RatioTo == nil || *action.RatioFrom <= 0 || *action.RatioTo <= 0 {
			return 0, ErrInvalidRatio
		}
		return *action.RatioFrom / *action.RatioTo, nil

	case ActionBonus:
		if action.RatioFrom == nil || action.RatioTo == nil || *action.RatioFrom <= 0 || *action.RatioTo <= 0 {
			return 0, ErrInvalidRatio
		}
		rf, rt := *action.RatioFrom, *action.RatioTo
		if rt > rf {
			return rf / rt, nil
		}
		return rf / (rf + rt), nil

	case ActionDividend:
		if action.DividendAmt == nil || *action.DividendAmt <= 0 {
			return 1.0, nil
		}
		if prevClose <= 0 {
			// Without prior candle close, we cannot compute dividend drop ratio; leave factor at 1.0
			return 1.0, nil
		}
		if *action.DividendAmt >= prevClose {
			return 0, fmt.Errorf("%w: dividend %f >= prevClose %f", ErrInvalidDividend, *action.DividendAmt, prevClose)
		}
		return 1.0 - (*action.DividendAmt / prevClose), nil

	case ActionRights:
		if action.RatioFrom != nil && action.RatioTo != nil && *action.RatioFrom > 0 && *action.RatioTo > 0 {
			return *action.RatioFrom / *action.RatioTo, nil
		}
		return 1.0, nil

	default:
		return 1.0, nil
	}
}

// ComputeCumulativeFactors takes a list of actions with previous closes,
// filters for verified actions, sorts them by ex_date descending (latest first),
// and computes the cumulative backward adjustment factor for each ex_date.
//
// At any ex_date T_i, the factor stored in adjustment_factors applies to prices strictly before T_i:
// factor(T_i) = product of f_j for all j >= i (i.e. T_i and all later actions).
func ComputeCumulativeFactors(items []ActionWithPrevClose) ([]AdjustmentFactor, error) {
	// Filter verified actions only
	var verified []ActionWithPrevClose
	for _, item := range items {
		if item.Action.Verified {
			verified = append(verified, item)
		}
	}

	if len(verified) == 0 {
		return nil, nil
	}

	// Sort chronologically descending: most recent ex_date first
	sort.Slice(verified, func(i, j int) bool {
		return verified[i].Action.ExDate.After(verified[j].Action.ExDate)
	})

	result := make([]AdjustmentFactor, 0, len(verified))
	runningCumulative := 1.0

	for _, item := range verified {
		factor, err := ComputeActionFactor(item.Action, item.PrevClose)
		if err != nil {
			return nil, fmt.Errorf("action %v on %s: %w", item.Action.ActionType, item.Action.ExDate.Format("2006-01-02"), err)
		}

		runningCumulative *= factor
		// Round to 10 decimal places to match NUMERIC(18,10)
		roundedFactor := math.Round(runningCumulative*1e10) / 1e10

		result = append(result, AdjustmentFactor{
			StockID: item.Action.StockID,
			ExDate:  item.Action.ExDate,
			Factor:  roundedFactor,
		})
	}

	return result, nil
}
