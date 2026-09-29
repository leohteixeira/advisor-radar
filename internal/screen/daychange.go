package screen

import (
	"fmt"
	"strconv"
)

// variantWithDayChange is the name of the wealth_summary and
// portfolio_summary variants that add the day-change pill.
const variantWithDayChange = "with_day_change"

// dayChangeVariant decorates a summary variant with the day-change pill: it
// matches when account-sim reports a patrimony change on the current
// simulated day, builds the base, and adds "−US$ 38.520,00 (−15,5%) no dia
// 3" with its tone. The percentage is of the patrimony the day before.
type dayChangeVariant struct {
	base Variant
	typ  string
}

func (dayChangeVariant) Matches(s Snapshot) bool {
	return s.Account.OK() && s.Account.Value.DayChange != 0
}

func (v dayChangeVariant) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Account.OK() {
		return Component{}, fmt.Errorf("screen: day change needs the account: %w", s.Account.Err)
	}
	comp, err := v.base.Build(s, c)
	if err != nil {
		return Component{}, err
	}
	a := s.Account.Value
	cp := copier{cat: c, typ: v.typ, variant: variantWithDayChange, fields: Fields{
		Amount:  SignedMoney(a.DayChange),
		Percent: ChangePercent(a.DayChange, a.Patrimony-a.DayChange),
		Day:     strconv.Itoa(a.SimDay),
	}}
	text, tone := cp.text("day_change"), SignTone(a.DayChange)
	if cp.err != nil {
		return Component{}, cp.err
	}
	switch props := comp.Props.(type) {
	case WealthSummary:
		props.DayChange, props.DayChangeTone = text, tone
		comp.Props = props
	case PortfolioSummary:
		props.DayChange, props.DayChangeTone = text, tone
		comp.Props = props
	default:
		return Component{}, fmt.Errorf("screen: %s has no day change pill", comp.Type)
	}
	comp.Variant = variantWithDayChange
	return comp, nil
}
