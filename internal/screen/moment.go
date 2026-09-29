package screen

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/event"
)

// Home moment variants, in the catalog's priority order after portfolio_drop
// (story 13). Advisory evaluates every condition and cases supplies the open
// case; a variant here only checks the fact it is given and formats copy. It
// compares no threshold and computes no segmentation.
const (
	momentCaseOpen           = "case_open"
	momentSegmentUpgraded    = "segment_upgraded"
	momentSegmentUpgradeNear = "segment_upgrade_near"
	momentIdleCash           = "idle_cash"
	momentPortfolioReview    = "portfolio_review"
	momentWelcome            = "welcome"
)

// momentVariants registers the moment_card variants and the sources each
// reads. A variant whose source failed is skipped, so the section falls
// through to the next one and ends at welcome, which needs nothing. The
// variants that show a segment SLA match only when cat has one for it.
func momentVariants(cat Catalog) map[variantKey]registered {
	return map[variantKey]registered{
		{typeMomentCard, momentCaseOpen}: {
			variant: caseOpenMoment{cat: cat},
			needs:   []Source{SourceCases, SourceAdvisory},
		},
		{typeMomentCard, momentSegmentUpgraded}: {
			variant: segmentUpgradedMoment{cat: cat},
			needs:   []Source{SourceMoments},
		},
		{typeMomentCard, momentSegmentUpgradeNear}: {
			variant: segmentUpgradeNearMoment{},
			needs:   []Source{SourceMoments},
		},
		{typeMomentCard, momentIdleCash}: {
			variant: idleCashMoment{},
			needs:   []Source{SourceMoments, SourceProfile},
		},
		{typeMomentCard, momentPortfolioReview}: {
			variant: portfolioReviewMoment{},
			needs:   []Source{SourceMoments, SourceAdvisory},
		},
		{typeMomentCard, momentWelcome}: {variant: welcomeMoment{}},
	}
}

// hasAdvisor reports whether advisory answered with an advisor name, which
// the case_open and portfolio_review copy needs.
func hasAdvisor(s Snapshot) bool {
	return customerFields(s).AdvisorName != ""
}

// hasSLA reports whether the catalog has an SLA text for segment.
func hasSLA(c Catalog, segment string) bool {
	_, err := c.SLA(segment)
	return err == nil
}

// caseOpenMoment matches when the customer has a case that is not resolved
// and the book segment has a catalog SLA. It shows the most recent case.
type caseOpenMoment struct{ cat Catalog }

func (v caseOpenMoment) Matches(s Snapshot) bool {
	return s.Cases.OK() && len(s.Cases.Value) > 0 && hasAdvisor(s) && hasSLA(v.cat, customerFields(s).Segment)
}

func (caseOpenMoment) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Cases.OK() || len(s.Cases.Value) == 0 {
		return Component{}, errors.New("screen: case_open needs an open case")
	}
	f := customerFields(s)
	if f.AdvisorName == "" {
		return Component{}, errors.New("screen: case_open needs the advisor")
	}
	sla, err := c.SLA(f.Segment)
	if err != nil {
		return Component{}, err
	}
	latest := latestCase(s.Cases.Value)
	f.SLA = sla
	f.Protocol = Protocol(latest.ID)
	f.Age = RelativeTime(latest.Age)
	cp := copier{cat: c, typ: typeMomentCard, variant: momentCaseOpen, fields: f}
	props := MomentCard{
		Kicker: cp.text("kicker"),
		Title:  cp.text("title"),
		Body:   cp.text("body"),
		Meta:   cp.text("meta"),
		Tone:   ToneInfo,
		Icon:   "inbox",
		Action: &Action{Type: ActionPanel, Label: cp.text("action"), Target: PanelMessage},
	}
	return momentComponent(momentCaseOpen, props, cp.err)
}

// latestCase is the most recently opened case; cases is never empty.
func latestCase(cases []OpenCase) OpenCase {
	latest := cases[0]
	for _, oc := range cases[1:] {
		if oc.Age < latest.Age {
			latest = oc
		}
	}
	return latest
}

// segmentUpgradedMoment matches a live segment upgrade advisory raised in
// the last 24 h to a segment with a catalog SLA, and shows that SLA.
type segmentUpgradedMoment struct{ cat Catalog }

func (v segmentUpgradedMoment) Matches(s Snapshot) bool {
	return s.Moments.OK() && s.Moments.Value.SegmentUpgraded && hasSLA(v.cat, s.Moments.Value.UpgradedSegment)
}

func (segmentUpgradedMoment) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Moments.OK() {
		return Component{}, fmt.Errorf("screen: segment_upgraded needs the moment facts: %w", s.Moments.Err)
	}
	segment := s.Moments.Value.UpgradedSegment
	sla, err := c.SLA(segment)
	if err != nil {
		return Component{}, err
	}
	f := customerFields(s)
	f.Segment = segment
	f.SLA = sla
	cp := copier{cat: c, typ: typeMomentCard, variant: momentSegmentUpgraded, fields: f}
	props := MomentCard{
		Kicker: cp.text("kicker"),
		Title:  cp.text("title"),
		Body:   cp.text("body"),
		Tone:   ToneGold,
		Icon:   "segment",
		Action: &Action{Type: ActionNavigate, Label: cp.text("action"), Target: ScreenInvestir},
	}
	return momentComponent(momentSegmentUpgraded, props, cp.err)
}

// segmentUpgradeNearMoment matches a client advisory found close to the
// Advance floor, with the gap to it.
type segmentUpgradeNearMoment struct{}

func (segmentUpgradeNearMoment) Matches(s Snapshot) bool {
	return s.Moments.OK() && s.Moments.Value.SegmentUpgradeNear
}

func (segmentUpgradeNearMoment) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Moments.OK() {
		return Component{}, fmt.Errorf("screen: segment_upgrade_near needs the moment facts: %w", s.Moments.Err)
	}
	sla, err := c.SLA(book.SegmentAdvance)
	if err != nil {
		return Component{}, err
	}
	f := customerFields(s)
	f.Gap = Money(s.Moments.Value.UpgradeGapCents)
	f.Threshold = Money(book.BoundEssencialMax * 100)
	f.SLA = sla
	cp := copier{cat: c, typ: typeMomentCard, variant: momentSegmentUpgradeNear, fields: f}
	props := MomentCard{
		Kicker: cp.text("kicker"),
		Title:  cp.text("title"),
		Body:   cp.text("body"),
		Tone:   ToneGold,
		Icon:   "segment",
		Action: &Action{Type: ActionPanel, Label: cp.text("action"), Target: PanelDeposit},
	}
	return momentComponent(momentSegmentUpgradeNear, props, cp.err)
}

// idleCashMoment matches a client advisory found with at least half of the
// patrimony in cash. Its copy names the investor profile, so it also needs
// that source. The timeline only dates the idle cash; without it the body
// leaves the days out.
type idleCashMoment struct{}

func (idleCashMoment) Matches(s Snapshot) bool {
	return s.Moments.OK() && s.Moments.Value.IdleCash && s.Profile.OK() && s.Profile.Value.Profile != ""
}

func (idleCashMoment) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Moments.OK() || !s.Profile.OK() {
		return Component{}, errors.New("screen: idle_cash needs the moment facts and the profile")
	}
	m := s.Moments.Value
	shares, err := Shares([]int64{m.CashCents, m.PatrimonyCents - m.CashCents})
	if err != nil {
		return Component{}, err
	}
	f := customerFields(s)
	f.Cash = Money(m.CashCents)
	f.CashShare = Percent(shares[0])
	f.Profile = strings.ToLower(s.Profile.Value.Profile)
	bodyKey := "body_undated"
	if days, ok := idleDays(s); ok {
		f.IdleDays = Days(days)
		bodyKey = "body"
	}
	cp := copier{cat: c, typ: typeMomentCard, variant: momentIdleCash, fields: f}
	props := MomentCard{
		Kicker: cp.text("kicker"),
		Title:  cp.text("title"),
		Body:   cp.text(bodyKey),
		Tone:   ToneInfo,
		Icon:   "cash",
		Action: &Action{Type: ActionNavigate, Label: cp.text("action"), Target: ScreenInvestir},
	}
	return momentComponent(momentIdleCash, props, cp.err)
}

// idleDays is the whole days since the customer's most recent account
// movement on the timeline, at least 1. It is false when the timeline failed
// or holds no account movement.
func idleDays(s Snapshot) (int, bool) {
	if !s.Activity.OK() {
		return 0, false
	}
	var newest time.Duration
	found := false
	for _, row := range s.Activity.Value {
		if row.Source != event.NameAccountEventRecorded {
			continue
		}
		age := row.Age
		if !row.OccurredAt.IsZero() && !s.Now.IsZero() {
			age = s.Now.Sub(row.OccurredAt)
		}
		if !found || age < newest {
			newest, found = age, true
		}
	}
	if !found {
		return 0, false
	}
	return max(int(newest/(24*time.Hour)), 1), true
}

// portfolioReviewMoment matches a Singular client, as advisory reports it,
// and offers a review with the advisor.
type portfolioReviewMoment struct{}

func (portfolioReviewMoment) Matches(s Snapshot) bool {
	return s.Moments.OK() && s.Moments.Value.PortfolioReview && hasAdvisor(s)
}

func (portfolioReviewMoment) Build(s Snapshot, c Catalog) (Component, error) {
	f := customerFields(s)
	if f.AdvisorName == "" {
		return Component{}, errors.New("screen: portfolio_review needs the advisor")
	}
	cp := copier{cat: c, typ: typeMomentCard, variant: momentPortfolioReview, fields: f}
	props := MomentCard{
		Kicker: cp.text("kicker"),
		Title:  cp.text("title"),
		Body:   cp.text("body"),
		Tone:   ToneNeutral,
		Icon:   "calendar",
		Action: &Action{Type: ActionPanel, Label: cp.text("action"), Target: PanelMessage},
	}
	return momentComponent(momentPortfolioReview, props, cp.err)
}

// momentComponent wraps moment props, or returns the first copy error.
func momentComponent(variant string, props MomentCard, copyErr error) (Component, error) {
	if copyErr != nil {
		return Component{}, copyErr
	}
	return Component{Type: typeMomentCard, Variant: variant, Props: props}, nil
}
