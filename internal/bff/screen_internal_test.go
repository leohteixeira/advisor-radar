package bff

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/screen"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// onePOV answers Get with account or err.
type onePOV struct {
	emptyPOV
	account POVAccount
	err     error
}

func (p onePOV) Get(context.Context, string) (POVAccount, error) { return p.account, p.err }

func TestScreenAccounts_Account(t *testing.T) {
	t.Parallel()
	src := screenAccounts{pov: onePOV{account: POVAccount{
		Acoes: 1, ETFs: 2, RendaFixa: 3, Caixa: 4, Patrimony: 10,
		Positions: []POVPosition{{ProductID: "tbill", AssetClass: "renda_fixa", AppliedCents: 2, ValueCents: 3}},
	}}}
	got, err := src.Account(t.Context(), "c")
	if err != nil {
		t.Fatalf("Account error = %v", err)
	}
	if got.Acoes != 1 || got.ETFs != 2 || got.RendaFixa != 3 || got.Cash != 4 || got.Patrimony != 10 {
		t.Errorf("account = %+v", got)
	}
	want := []screen.Position{{ProductID: "tbill", AssetClass: "renda_fixa", AppliedCents: 2, ValueCents: 3}}
	if !slices.Equal(got.Positions, want) {
		t.Errorf("positions = %+v, want %+v", got.Positions, want)
	}

	_, err = screenAccounts{pov: onePOV{err: sim.ErrUnknownCustomer}}.Account(t.Context(), "c")
	if !errors.Is(err, screen.ErrUnknownCustomer) || !errors.Is(err, sim.ErrUnknownCustomer) {
		t.Errorf("unknown customer error = %v", err)
	}
	down := errors.New("down")
	_, err = screenAccounts{pov: onePOV{err: down}}.Account(t.Context(), "c")
	if !errors.Is(err, down) || errors.Is(err, screen.ErrUnknownCustomer) {
		t.Errorf("failure error = %v", err)
	}
}

type rowsTimeline struct{ rows []TimelineEntry }

func (r rowsTimeline) Search(context.Context, string, string, string) ([]TimelineEntry, error) {
	return r.rows, nil
}

func TestScreenActivity_Activity(t *testing.T) {
	t.Parallel()
	src := screenActivity{timeline: rowsTimeline{rows: []TimelineEntry{
		{Kind: "aporte", Title: "Aporte", Ago: 90, Source: "account.event.recorded", OccurredAt: time.Unix(1_700_000_000, 0)},
		{Kind: "saque", Title: "Saque", Ago: -3},
	}}}
	got, err := src.Activity(t.Context(), "c")
	if err != nil {
		t.Fatalf("Activity error = %v", err)
	}
	want := []screen.Activity{
		{Kind: "aporte", Title: "Aporte", Source: "account.event.recorded", OccurredAt: time.Unix(1_700_000_000, 0), Age: 90 * time.Minute},
		{Kind: "saque", Title: "Saque", Age: 0},
	}
	if !slices.Equal(got, want) {
		t.Errorf("activity = %+v, want %+v", got, want)
	}
}

func TestScreenAccounts_DisabledIsAFailedSource(t *testing.T) {
	t.Parallel()
	_, err := screenAccounts{pov: emptyPOV{}}.Account(t.Context(), "c")
	if !errors.Is(err, errPOVDisabled) || errors.Is(err, screen.ErrUnknownCustomer) {
		t.Errorf("disabled account error = %v", err)
	}
}

// catalogPOV answers Products with products or err.
type catalogPOV struct {
	emptyPOV
	products []POVProduct
	err      error
}

func (p catalogPOV) Products(context.Context) ([]POVProduct, error) { return p.products, p.err }

func TestScreenProducts_Products(t *testing.T) {
	t.Parallel()
	pov := catalogPOV{products: []POVProduct{
		{ID: "acoesg", Name: "Maré Ações Globais ETF", AssetClass: "etfs", Risk: 3, ReturnLabel: "+11,2% em 12 meses", MinimumCents: 5_000},
	}}
	got, err := screenProducts{pov: pov}.Products(t.Context())
	want := []screen.Product{
		{ID: "acoesg", Name: "Maré Ações Globais ETF", AssetClass: "etfs", Risk: 3, ReturnLabel: "+11,2% em 12 meses", MinimumCents: 5_000},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("Products = %+v, %v; want %+v", got, err, want)
	}

	down := errors.New("unavailable")
	if _, err := (screenProducts{pov: catalogPOV{err: down}}).Products(t.Context()); !errors.Is(err, down) {
		t.Errorf("failed catalog error = %v, want it to wrap %v", err, down)
	}
	if _, err := (screenProducts{pov: emptyPOV{}}).Products(t.Context()); !errors.Is(err, errPOVDisabled) {
		t.Errorf("disabled catalog error = %v, want %v", err, errPOVDisabled)
	}
}

func TestFailureClass(t *testing.T) {
	t.Parallel()
	if got := failureClass(errors.New("screen: no sla for segment \"X\"")); got != "build_error" {
		t.Errorf("class = %q, want build_error", got)
	}
	if got := failureClass(fmt.Errorf("screen: variant: %w: boom", screen.ErrPanic)); got != "panic" {
		t.Errorf("class = %q, want panic", got)
	}
}

// momentQueue answers the moment and profile reads with fixed values or err.
type momentQueue struct {
	EmptyQueue
	facts   MomentFacts
	profile InvestorProfile
	err     error
}

func (q momentQueue) MomentFacts(context.Context, string) (MomentFacts, error) { return q.facts, q.err }
func (q momentQueue) InvestorProfile(context.Context, string) (InvestorProfile, error) {
	return q.profile, q.err
}

func TestScreenMoments_Moments(t *testing.T) {
	t.Parallel()
	facts := MomentFacts{
		SegmentUpgraded: true, UpgradedSegment: "Advance", SegmentUpgradeNear: true, UpgradeGapCents: 1,
		IdleCash: true, CashCents: 2, PatrimonyCents: 3, PortfolioReview: true, PortfolioDrop: true,
	}
	got, err := screenMoments{queue: momentQueue{facts: facts}}.Moments(t.Context(), "c")
	if err != nil {
		t.Fatalf("Moments error = %v", err)
	}
	want := screen.MomentFacts{
		SegmentUpgraded: true, UpgradedSegment: "Advance", SegmentUpgradeNear: true, UpgradeGapCents: 1,
		IdleCash: true, CashCents: 2, PatrimonyCents: 3, PortfolioReview: true, PortfolioDrop: true,
	}
	if got != want {
		t.Errorf("moments = %+v, want %+v", got, want)
	}
	down := errors.New("down")
	if _, err := (screenMoments{queue: momentQueue{err: down}}).Moments(t.Context(), "c"); !errors.Is(err, down) {
		t.Errorf("failure error = %v", err)
	}
}

func TestScreenProfiles_Profile(t *testing.T) {
	t.Parallel()
	on := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	got, err := screenProfiles{queue: momentQueue{profile: InvestorProfile{Profile: "arrojado", MaxRisk: 5, AssessedOn: on}}}.Profile(t.Context(), "c")
	if err != nil {
		t.Fatalf("Profile error = %v", err)
	}
	if got != (screen.InvestorProfile{Profile: "arrojado", MaxRisk: 5, AssessedOn: on}) {
		t.Errorf("profile = %+v", got)
	}
	down := errors.New("down")
	if _, err := (screenProfiles{queue: momentQueue{err: down}}).Profile(t.Context(), "c"); !errors.Is(err, down) {
		t.Errorf("failure error = %v", err)
	}
}

// customerCases answers CustomerCases with fixed items or err.
type customerCases struct {
	EmptyCases
	items []Case
	err   error
}

func (c customerCases) CustomerCases(context.Context, string) ([]Case, []string, error) {
	return c.items, CaseStates, c.err
}

func TestScreenCases_OpenCases(t *testing.T) {
	t.Parallel()
	src := screenCases{cases: customerCases{items: []Case{
		{ID: "open", State: 0, OpenedAgo: 3},
		{ID: "working", State: 1, OpenedAgo: 60},
		{ID: "resolved", State: 3, OpenedAgo: 5},
		{ID: "unknown state", State: 9, OpenedAgo: -4},
	}}}
	got, err := src.OpenCases(t.Context(), "c")
	if err != nil {
		t.Fatalf("OpenCases error = %v", err)
	}
	want := []screen.OpenCase{
		{ID: "open", State: "Aberto", Age: 3 * time.Minute},
		{ID: "working", State: "Em atendimento", Age: time.Hour},
		{ID: "unknown state", State: "", Age: 0},
	}
	if !slices.Equal(got, want) {
		t.Errorf("open cases = %+v, want %+v", got, want)
	}

	none, err := screenCases{cases: EmptyCases{}}.OpenCases(t.Context(), "c")
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("no cases = %v, %v, want an empty list", none, err)
	}
	down := errors.New("down")
	if _, err := (screenCases{cases: customerCases{err: down}}).OpenCases(t.Context(), "c"); !errors.Is(err, down) {
		t.Errorf("failure error = %v", err)
	}
}
