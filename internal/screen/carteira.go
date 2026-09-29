package screen

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
)

// Component types of the Carteira screen. Its history is an activity_list.
const (
	typePortfolioSummary    = "portfolio_summary"
	typeAllocationBreakdown = "allocation_breakdown"
	typePositionList        = "position_list"
)

// historyLimit is how many timeline rows the Carteira history shows.
const historyLimit = 20

// carteiraVariants registers the Carteira variants and the sources each
// reads. The history reads the catalog only for product names, so it does
// not need it.
func carteiraVariants() map[variantKey]registered {
	account := []Source{SourceAccount}
	positions := []Source{SourceAccount, SourceCatalog}
	return map[variantKey]registered{
		{typePortfolioSummary, "default"}:    {variant: portfolioSummary{}, needs: account},
		{typeAllocationBreakdown, "default"}: {variant: allocationBreakdown{}, needs: account},
		{typePositionList, "stocks"}:         {variant: positionList{name: "stocks", class: classAcoes}, needs: positions},
		{typePositionList, "etf"}:            {variant: positionList{name: "etf", class: classETFs}, needs: positions},
		{typePositionList, "fixed_income"}:   {variant: positionList{name: "fixed_income", class: classRendaFixa}, needs: positions},
		{typeActivityList, "history"}:        {variant: historyActivity{}, needs: []Source{SourceTimeline}, uses: []Source{SourceCatalog}},
	}
}

// portfolioSummary is the patrimony at market value with what was applied,
// the return on it, the cash, and the simulated day.
type portfolioSummary struct{}

func (portfolioSummary) Matches(Snapshot) bool { return true }

func (portfolioSummary) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Account.OK() {
		return Component{}, fmt.Errorf("screen: portfolio summary needs the account: %w", s.Account.Err)
	}
	a := s.Account.Value
	var applied, value int64
	for _, p := range a.Positions {
		applied += p.AppliedCents
		value += p.ValueCents
	}
	gain := value - applied
	cp := copier{
		cat:     c,
		typ:     typePortfolioSummary,
		variant: "default",
		fields:  Fields{Amount: SignedMoney(gain), Percent: ChangePercent(gain, applied)},
	}
	props := PortfolioSummary{
		TotalLabel: cp.text("total_label"),
		Total:      Money(a.Patrimony),
		Stats: []Stat{
			{Label: cp.text("applied"), Value: Money(applied), Money: true},
			{Label: cp.text("return"), Value: cp.text("return_value"), Tone: SignTone(gain), Money: true},
			{Label: cp.text("cash"), Value: Money(a.Cash), Money: true},
			{Label: cp.text("day"), Value: strconv.Itoa(a.SimDay)},
		},
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typePortfolioSummary, Variant: "default", Props: props}, nil
}

// allocationBreakdown is every class of the account, zero included, with its
// value and a largest-remainder share of the patrimony.
type allocationBreakdown struct{}

func (allocationBreakdown) Matches(Snapshot) bool { return true }

func (allocationBreakdown) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Account.OK() {
		return Component{}, fmt.Errorf("screen: allocation needs the account: %w", s.Account.Err)
	}
	a := s.Account.Value
	classes := []classCents{
		{class: "stocks", cents: a.Acoes},
		{class: "etfs", cents: a.ETFs},
		{class: "fixed_income", cents: a.RendaFixa},
		{class: "cash", cents: a.Cash},
	}
	values := make([]int64, len(classes))
	for i, x := range classes {
		values[i] = x.cents
	}
	shares, err := Shares(values)
	if err != nil {
		return Component{}, err
	}
	cp := copier{cat: c, typ: typeAllocationBreakdown, variant: "default"}
	rows := make([]BreakdownRow, len(classes))
	for i, x := range classes {
		rows[i] = BreakdownRow{
			Class:    x.class,
			Label:    cp.text(x.class),
			Value:    Money(x.cents),
			Share:    Percent(shares[i]),
			BarWidth: shares[i],
		}
	}
	props := AllocationBreakdown{Title: cp.text("title"), Rows: rows}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeAllocationBreakdown, Variant: "default", Props: props}, nil
}

// positionList is the positions of one asset class, by value descending and
// then product id. A class with no position has no section (errAbsent). A
// position whose product the catalog does not name is a build error: the
// list never shows a bare product id.
type positionList struct {
	name  string
	class string
}

func (positionList) Matches(Snapshot) bool { return true }

func (v positionList) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Account.OK() {
		return Component{}, fmt.Errorf("screen: positions need the account: %w", s.Account.Err)
	}
	if !s.Products.OK() {
		return Component{}, fmt.Errorf("screen: positions need the catalog: %w", s.Products.Err)
	}
	positions := make([]Position, 0, len(s.Account.Value.Positions))
	for _, p := range s.Account.Value.Positions {
		if p.AssetClass == v.class {
			positions = append(positions, p)
		}
	}
	if len(positions) == 0 {
		return Component{}, errAbsent
	}
	slices.SortFunc(positions, func(a, b Position) int {
		return cmp.Or(cmp.Compare(b.ValueCents, a.ValueCents), cmp.Compare(a.ProductID, b.ProductID))
	})
	names := productNames(s)
	items := make([]PositionItem, 0, len(positions))
	var subtotal int64
	for _, p := range positions {
		name, ok := names[p.ProductID]
		if !ok || name == "" {
			return Component{}, fmt.Errorf("screen: position %q is not in the catalog", p.ProductID)
		}
		ret, tone := positionReturn(p)
		items = append(items, PositionItem{
			ProductID:  p.ProductID,
			Name:       name,
			Applied:    Money(p.AppliedCents),
			Value:      Money(p.ValueCents),
			Return:     ret,
			ReturnTone: tone,
		})
		subtotal += p.ValueCents
	}
	cp := copier{cat: c, typ: typePositionList, variant: v.name}
	props := PositionList{
		Title:        cp.text("title"),
		Subtotal:     Money(subtotal),
		AppliedLabel: cp.text("applied_label"),
		Items:        items,
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typePositionList, Variant: v.name, Props: props}, nil
}

// positionReturn is the signed return of a position on what was applied,
// with one decimal, and its tone. With nothing applied there is no return to
// measure: "0,0%", neutral.
func positionReturn(p Position) (label, tone string) {
	if p.AppliedCents <= 0 {
		return ChangePercent(0, 0), ToneNeutral
	}
	delta := p.ValueCents - p.AppliedCents
	return ChangePercent(delta, p.AppliedCents), SignTone(delta)
}

// historyActivity is the Carteira history: the latest client-facing timeline
// rows through the home activity mapping, or the empty text without one.
type historyActivity struct{}

func (historyActivity) Matches(Snapshot) bool { return true }

func (historyActivity) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Activity.OK() {
		return Component{}, fmt.Errorf("screen: history needs the timeline: %w", s.Activity.Err)
	}
	items, err := activityItems(s, c, historyLimit)
	if err != nil {
		return Component{}, err
	}
	cp := copier{cat: c, typ: typeActivityList, variant: "history"}
	props := ActivityList{Title: cp.text("title"), Items: items}
	if len(items) == 0 {
		props.EmptyText = cp.text("empty_text")
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeActivityList, Variant: "history", Props: props}, nil
}
