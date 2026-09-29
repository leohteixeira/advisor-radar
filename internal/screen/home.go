package screen

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// Component types of the home screen.
const (
	typeMomentCard    = "moment_card"
	typeWealthSummary = "wealth_summary"
	typeActionGrid    = "action_grid"
	typeAdvisorCard   = "advisor_card"
	typeActivityList  = "activity_list"
)

// segmentSingular is the advisory segment with a dedicated advisor.
const segmentSingular = "Singular"

// activityLimit is how many timeline rows the home activity shows.
const activityLimit = 5

// builtinVariants maps every (type, variant) the catalog may name to its
// implementation and the sources its Build reads. The sources of a section's
// default variant decide whether the section is omitted when one fails.
// Some variants match only when cat has the copy data they show.
func builtinVariants(cat Catalog) map[variantKey]registered {
	variants := map[variantKey]registered{
		{typeWealthSummary, "default"}: {variant: defaultWealth{}, needs: []Source{SourceAccount}},
		{typeActionGrid, "default"}:    {variant: defaultActions{}},
		{typeAdvisorCard, "dedicated"}: {variant: advisorCard{name: "dedicated", singularOnly: true}, needs: []Source{SourceAdvisory}},
		{typeAdvisorCard, "default"}:   {variant: advisorCard{name: "default"}, needs: []Source{SourceAdvisory}},
		{typeActivityList, "recent"}:   {variant: recentActivity{}, needs: []Source{SourceTimeline}},
		{typeActivityList, "empty"}:    {variant: emptyActivity{}, needs: []Source{SourceTimeline}},
	}
	maps.Copy(variants, momentVariants(cat))
	return variants
}

// customerFields are the template fields that come from advisory. They are
// empty when advisory failed, which the templates use to fall back to "Olá".
func customerFields(s Snapshot) Fields {
	if !s.Customer.OK() {
		return Fields{}
	}
	c := s.Customer.Value
	return Fields{
		FirstName:   FirstName(c.Name),
		AdvisorName: c.Advisor,
		Segment:     c.Segment,
		Since:       c.Since,
	}
}

// welcomeMoment is the default moment: it always matches and needs no source.
type welcomeMoment struct{}

func (welcomeMoment) Matches(Snapshot) bool { return true }

func (welcomeMoment) Build(s Snapshot, c Catalog) (Component, error) {
	cp := copier{cat: c, typ: typeMomentCard, variant: momentWelcome, fields: customerFields(s)}
	props := MomentCard{
		Kicker: cp.text("kicker"),
		Title:  cp.text("title"),
		Body:   cp.text("body"),
		Tone:   ToneNeutral,
		Icon:   "spark",
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeMomentCard, Variant: momentWelcome, Props: props}, nil
}

// classCents is one allocation class and its value in cents. Class is the
// allocation row class and the catalog copy key of its label.
type classCents struct {
	class string
	cents int64
}

// defaultWealth shows total patrimony, cash, and the allocation of the
// non-zero classes with largest-remainder shares.
type defaultWealth struct{}

func (defaultWealth) Matches(Snapshot) bool { return true }

func (defaultWealth) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Account.OK() {
		return Component{}, fmt.Errorf("screen: wealth needs the account: %w", s.Account.Err)
	}
	a := s.Account.Value
	cp := copier{cat: c, typ: typeWealthSummary, variant: "default"}
	classes := slices.DeleteFunc([]classCents{
		{class: "stocks", cents: a.Acoes},
		{class: "etfs", cents: a.ETFs},
		{class: "fixed_income", cents: a.RendaFixa},
		{class: "cash", cents: a.Cash},
	}, func(x classCents) bool { return x.cents <= 0 })
	values := make([]int64, len(classes))
	for i, x := range classes {
		values[i] = x.cents
	}
	shares, err := Shares(values)
	if err != nil {
		return Component{}, err
	}
	rows := make([]AllocationRow, len(classes))
	for i, x := range classes {
		rows[i] = AllocationRow{
			Class:    x.class,
			Label:    cp.text(x.class),
			Share:    Percent(shares[i]),
			BarWidth: shares[i],
		}
	}
	props := WealthSummary{
		TotalLabel: cp.text("total_label"),
		Total:      Money(a.Patrimony),
		CashLabel:  cp.text("cash_label"),
		Cash:       Money(a.Cash),
		CashCents:  a.Cash,
		Allocation: rows,
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeWealthSummary, Variant: "default", Props: props}, nil
}

// defaultActions is the four phase-2 panels: deposit, withdraw, message, and
// complaint.
type defaultActions struct{}

func (defaultActions) Matches(Snapshot) bool { return true }

func (defaultActions) Build(_ Snapshot, c Catalog) (Component, error) {
	cp := copier{cat: c, typ: typeActionGrid, variant: "default"}
	panels := []struct {
		target string
		icon   string
	}{
		{PanelDeposit, "deposit"},
		{PanelWithdraw, "withdraw"},
		{PanelMessage, "msg"},
		{PanelComplaint, "alert"},
	}
	items := make([]GridItem, len(panels))
	for i, p := range panels {
		label := cp.text(p.target)
		items[i] = GridItem{
			Label:  label,
			Icon:   p.icon,
			Action: Action{Type: ActionPanel, Label: label, Target: p.target},
		}
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeActionGrid, Variant: "default", Props: ActionGrid{Items: items}}, nil
}

// advisorCard is the advisor from the advisory book. The dedicated variant
// matches Singular clients only; the default always matches.
type advisorCard struct {
	name         string
	singularOnly bool
}

func (v advisorCard) Matches(s Snapshot) bool {
	if !v.singularOnly {
		return true
	}
	return s.Customer.OK() && s.Customer.Value.Segment == segmentSingular
}

func (v advisorCard) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Customer.OK() {
		return Component{}, fmt.Errorf("screen: advisor needs the customer: %w", s.Customer.Err)
	}
	f := customerFields(s)
	if f.AdvisorName == "" {
		return Component{}, errors.New("screen: customer has no advisor")
	}
	sla, err := c.SLA(f.Segment)
	if err != nil {
		return Component{}, err
	}
	f.SLA = sla
	cp := copier{cat: c, typ: typeAdvisorCard, variant: v.name, fields: f}
	props := AdvisorCard{
		Kicker:   cp.text("kicker"),
		Name:     f.AdvisorName,
		Initials: Initials(f.AdvisorName),
		Meta:     cp.text("meta"),
		Action:   &Action{Type: ActionPanel, Label: cp.text("action"), Target: PanelMessage},
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeAdvisorCard, Variant: v.name, Props: props}, nil
}

// recentActivity lists the latest client-facing timeline rows.
type recentActivity struct{}

func (recentActivity) Matches(s Snapshot) bool {
	return s.Activity.OK() && len(visibleActivity(s.Activity.Value, s.Now)) > 0
}

func (recentActivity) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Activity.OK() {
		return Component{}, fmt.Errorf("screen: activity needs the timeline: %w", s.Activity.Err)
	}
	cp := copier{cat: c, typ: typeActivityList, variant: "recent"}
	rows := visibleActivity(s.Activity.Value, s.Now)
	items := make([]ActivityItem, len(rows))
	for i, row := range rows {
		items[i] = ActivityItem{Icon: activityIcon(row.Kind), Title: row.Title, Meta: RelativeTime(row.Age)}
	}
	props := ActivityList{Title: cp.text("title"), Items: items}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeActivityList, Variant: "recent", Props: props}, nil
}

// emptyActivity is the default activity: the timeline answered with nothing
// the client sees.
type emptyActivity struct{}

func (emptyActivity) Matches(Snapshot) bool { return true }

func (emptyActivity) Build(_ Snapshot, c Catalog) (Component, error) {
	cp := copier{cat: c, typ: typeActivityList, variant: "empty"}
	props := ActivityList{
		Title:     cp.text("title"),
		Items:     []ActivityItem{},
		EmptyText: cp.text("empty_text"),
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeActivityList, Variant: "empty", Props: props}, nil
}

// activityIcon maps a client-facing timeline kind to its web icon. Any other
// kind gets no icon.
func activityIcon(kind string) string {
	switch kind {
	case "aporte":
		return "in"
	case "saque":
		return "out"
	case "mensagem":
		return "msg"
	default:
		return ""
	}
}

// isClientSource reports whether a timeline row came from an event the client
// caused and may see: their own account movements and the messages they
// sent. Alerts, cases, triage results, and advisor notes are team-side.
func isClientSource(source string) bool {
	return source == event.NameAccountEventRecorded || source == event.NameMessageReceived
}

// visibleActivity is the client-facing rows, most recent first, at most
// activityLimit. Each row's Age is measured from OccurredAt against now when
// both are known, else it keeps the indexed Age. rows is never changed.
func visibleActivity(rows []Activity, now time.Time) []Activity {
	out := make([]Activity, 0, min(len(rows), activityLimit))
	for _, row := range rows {
		if !isClientSource(row.Source) {
			continue
		}
		if !row.OccurredAt.IsZero() && !now.IsZero() {
			row.Age = now.Sub(row.OccurredAt)
		}
		out = append(out, row)
	}
	slices.SortStableFunc(out, func(a, b Activity) int { return cmp.Compare(a.Age, b.Age) })
	return out[:min(len(out), activityLimit)]
}
