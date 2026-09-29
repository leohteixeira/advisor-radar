package screen

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

// Source names a Snapshot input. It is the omitted reason of a section whose
// default variant needs that source when the source fails.
type Source string

// Snapshot sources.
const (
	SourceAccount  Source = "account-sim"
	SourceAdvisory Source = "advisory"
	SourceTimeline Source = "timeline"
	// SourceMoments is the advisory moment facts.
	SourceMoments Source = "moments"
	// SourceProfile is the advisory investor profile.
	SourceProfile Source = "profile"
	// SourceCases is the customer's open cases.
	SourceCases Source = "cases"
	// SourceCatalog is the account-sim product catalog.
	SourceCatalog Source = "catalog"
	// SourceRegistration is the account-sim registration data.
	SourceRegistration Source = "registration"
	// SourcePreferences is the account-sim contact channel and beta flag.
	SourcePreferences Source = "preferences"
)

// allSources is every Snapshot source in fetch and failure-report order.
var allSources = []Source{
	SourceAccount,
	SourceAdvisory,
	SourceTimeline,
	SourceMoments,
	SourceProfile,
	SourceCases,
	SourceCatalog,
	SourceRegistration,
	SourcePreferences,
}

// errNotFetched is the error of a source the screen's plan does not read:
// the engine never calls it, and no section of that screen depends on it.
var errNotFetched = errors.New("screen: source not fetched for this screen")

// ErrPanic marks a failure that was a recovered panic, in a source adapter
// or a variant.
var ErrPanic = errors.New("screen: panic")

// ErrUnknownCustomer is what an AccountSource wraps when account-sim answers
// NotFound. It is the only source failure that fails the whole screen.
var ErrUnknownCustomer = errors.New("screen: unknown customer")

// Account is the account-sim account in integer USD cents. Acoes, ETFs, and
// RendaFixa are the class aggregates of the positions at market value;
// Patrimony is positions plus Cash. SimDay is the global simulated day the
// values are at; it stays 0 until account-sim reports the day.
type Account struct {
	Acoes     int64
	ETFs      int64
	RendaFixa int64
	Cash      int64
	Patrimony int64
	Positions []Position
	SimDay    int
}

// Position is one holding at market value, in integer USD cents.
type Position struct {
	ProductID    string
	AssetClass   string
	AppliedCents int64
	ValueCents   int64
}

// Customer is the advisory book row the screens use. Advisor is the advisor's
// display name and Since the client-since text.
type Customer struct {
	Name    string
	Segment string
	Advisor string
	Since   string
}

// Activity is one customer timeline row. Source is the routing key of the
// event it came from. OccurredAt is the event time; when it is zero, Age (how
// long ago the row was when it was indexed) is used instead. ProductID and
// AmountCents are the product and the amount in integer USD cents of an
// account event that carries them, such as an aplicacao.
type Activity struct {
	Kind        string
	Title       string
	Source      string
	OccurredAt  time.Time
	Age         time.Duration
	ProductID   string
	AmountCents int64
}

// MomentFacts are the home moment conditions advisory evaluated for the
// customer. The engine only orders them; it compares no threshold. Money is
// integer USD cents.
type MomentFacts struct {
	SegmentUpgraded    bool
	UpgradedSegment    string
	SegmentUpgradeNear bool
	UpgradeGapCents    int64
	IdleCash           bool
	CashCents          int64
	PatrimonyCents     int64
	PortfolioReview    bool
	PortfolioDrop      bool
}

// InvestorProfile is the customer's suitability profile from advisory:
// Profile is "conservador", "moderado", or "arrojado", and MaxRisk the
// highest product risk it accepts. MaxRiskTable is the whole advisory
// max-risk table, so Perfil can show every level without holding the table.
type InvestorProfile struct {
	Profile      string
	MaxRisk      int
	AssessedOn   time.Time
	MaxRiskTable []ProfileMaxRisk
}

// OpenCase is one of the customer's cases that is not resolved. Age is how
// long ago it was opened.
type OpenCase struct {
	ID    string
	State string
	Age   time.Duration
}

// Product is one entry of the account-sim product catalog. AssetClass is
// "acoes", "etfs", or "renda_fixa"; Risk runs from 1 to 5; ReturnLabel is
// account-sim display text; MinimumCents is integer USD cents.
type Product struct {
	ID           string
	Name         string
	AssetClass   string
	Risk         int
	ReturnLabel  string
	MinimumCents int64
}

// AccountSource reads one account from account-sim. An unknown customer
// wraps ErrUnknownCustomer. Implementations must return when ctx is done.
type AccountSource interface {
	Account(ctx context.Context, customerID string) (Account, error)
}

// CustomerSource reads one customer from the advisory book. Implementations
// must return when ctx is done.
type CustomerSource interface {
	Customer(ctx context.Context, customerID string) (Customer, error)
}

// ActivitySource reads the customer timeline, most recent first or in any
// order. Implementations must return when ctx is done.
type ActivitySource interface {
	Activity(ctx context.Context, customerID string) ([]Activity, error)
}

// MomentSource reads the advisory moment facts of one customer.
// Implementations must return when ctx is done.
type MomentSource interface {
	Moments(ctx context.Context, customerID string) (MomentFacts, error)
}

// ProfileSource reads the advisory investor profile of one customer.
// Implementations must return when ctx is done.
type ProfileSource interface {
	Profile(ctx context.Context, customerID string) (InvestorProfile, error)
}

// CaseSource reads the customer's cases that are not resolved, in any
// order. Implementations must return when ctx is done.
type CaseSource interface {
	OpenCases(ctx context.Context, customerID string) ([]OpenCase, error)
}

// ProductSource reads the account-sim product catalog, in any order.
// Implementations must return when ctx is done.
type ProductSource interface {
	Products(ctx context.Context) ([]Product, error)
}

// Sources are the ports the engine reads a Snapshot from.
type Sources struct {
	Accounts  AccountSource
	Customers CustomerSource
	Activity  ActivitySource
	Moments   MomentSource
	Profiles  ProfileSource
	Cases     CaseSource
	Products  ProductSource
	// Registrations and Preferences are the account-sim reads of Perfil.
	Registrations RegistrationSource
	Preferences   PreferenceSource
}

// complete reports whether every source is set.
func (s Sources) complete() bool {
	return s.Accounts != nil && s.Customers != nil && s.Activity != nil &&
		s.Moments != nil && s.Profiles != nil && s.Cases != nil && s.Products != nil &&
		s.Registrations != nil && s.Preferences != nil
}

// Fetched is one source result. Err is set when the source failed or ran past
// the screen deadline; Value is then the zero value.
type Fetched[T any] struct {
	Value T
	Err   error
}

// OK reports whether the source answered.
func (f Fetched[T]) OK() bool { return f.Err == nil }

// Snapshot is everything a screen is built from, fetched once per request.
// Each source result carries its own error. Now is the request clock.
type Snapshot struct {
	CustomerID string
	Now        time.Time
	Account    Fetched[Account]
	Customer   Fetched[Customer]
	Activity   Fetched[[]Activity]
	Moments    Fetched[MomentFacts]
	Profile    Fetched[InvestorProfile]
	Cases      Fetched[[]OpenCase]
	Products   Fetched[[]Product]
	// Registration and Preferences are read for Perfil only.
	Registration Fetched[Registration]
	Preferences  Fetched[Preferences]
}

// failed returns the error of src, or nil when it answered.
func (s Snapshot) failed(src Source) error {
	switch src {
	case SourceAccount:
		return s.Account.Err
	case SourceAdvisory:
		return s.Customer.Err
	case SourceTimeline:
		return s.Activity.Err
	case SourceMoments:
		return s.Moments.Err
	case SourceProfile:
		return s.Profile.Err
	case SourceCases:
		return s.Cases.Err
	case SourceCatalog:
		return s.Products.Err
	case SourceRegistration:
		return s.Registration.Err
	case SourcePreferences:
		return s.Preferences.Err
	default:
		return fmt.Errorf("screen: unknown source %q", src)
	}
}

// fetchSnapshot calls the sources in want in parallel under one deadline,
// each in its own child span of ctx's span. A source outside want is not
// called and carries errNotFetched. The group has no shared context, so one
// source failing never cancels the others; each goroutine writes only its own
// field, and Wait orders those writes before the return.
func fetchSnapshot(ctx context.Context, tracer trace.Tracer, src Sources, want []Source, customerID string, now time.Time, deadline time.Duration) Snapshot {
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	snap := Snapshot{
		CustomerID: customerID,
		Now:        now,
		Account:    Fetched[Account]{Err: errNotFetched},
		Customer:   Fetched[Customer]{Err: errNotFetched},
		Activity:   Fetched[[]Activity]{Err: errNotFetched},
		Moments:    Fetched[MomentFacts]{Err: errNotFetched},
		Profile:    Fetched[InvestorProfile]{Err: errNotFetched},
		Cases:      Fetched[[]OpenCase]{Err: errNotFetched},
		Products:   Fetched[[]Product]{Err: errNotFetched},
		// Perfil reads.
		Registration: Fetched[Registration]{Err: errNotFetched},
		Preferences:  Fetched[Preferences]{Err: errNotFetched},
	}
	var g errgroup.Group
	g.SetLimit(len(allSources))
	for _, source := range want {
		switch source {
		case SourceAccount:
			g.Go(func() error {
				snap.Account = fetch(ctx, tracer, SourceAccount, func(ctx context.Context) (Account, error) {
					return src.Accounts.Account(ctx, customerID)
				})
				return nil
			})
		case SourceAdvisory:
			g.Go(func() error {
				snap.Customer = fetch(ctx, tracer, SourceAdvisory, func(ctx context.Context) (Customer, error) {
					return src.Customers.Customer(ctx, customerID)
				})
				return nil
			})
		case SourceTimeline:
			g.Go(func() error {
				snap.Activity = fetch(ctx, tracer, SourceTimeline, func(ctx context.Context) ([]Activity, error) {
					return src.Activity.Activity(ctx, customerID)
				})
				return nil
			})
		case SourceMoments:
			g.Go(func() error {
				snap.Moments = fetch(ctx, tracer, SourceMoments, func(ctx context.Context) (MomentFacts, error) {
					return src.Moments.Moments(ctx, customerID)
				})
				return nil
			})
		case SourceProfile:
			g.Go(func() error {
				snap.Profile = fetch(ctx, tracer, SourceProfile, func(ctx context.Context) (InvestorProfile, error) {
					return src.Profiles.Profile(ctx, customerID)
				})
				return nil
			})
		case SourceCases:
			g.Go(func() error {
				snap.Cases = fetch(ctx, tracer, SourceCases, func(ctx context.Context) ([]OpenCase, error) {
					return src.Cases.OpenCases(ctx, customerID)
				})
				return nil
			})
		case SourceCatalog:
			g.Go(func() error {
				snap.Products = fetch(ctx, tracer, SourceCatalog, func(ctx context.Context) ([]Product, error) {
					return src.Products.Products(ctx)
				})
				return nil
			})
		case SourceRegistration:
			g.Go(func() error {
				snap.Registration = fetch(ctx, tracer, SourceRegistration, func(ctx context.Context) (Registration, error) {
					return src.Registrations.Registration(ctx, customerID)
				})
				return nil
			})
		case SourcePreferences:
			g.Go(func() error {
				snap.Preferences = fetch(ctx, tracer, SourcePreferences, func(ctx context.Context) (Preferences, error) {
					return src.Preferences.Preferences(ctx, customerID)
				})
				return nil
			})
		}
	}
	_ = g.Wait() // every goroutine returns nil; errors live in the Snapshot
	return snap
}

// fetch runs one source call in a "sdui.snapshot.<source>" span, so the
// adapter's own client spans nest under it. A panic in the adapter becomes
// that source's error, wrapping ErrPanic, instead of killing the process. A
// failure sets the span status to its failure class, never the error text,
// except ErrUnknownCustomer, which is a 404 and leaves the status unset.
func fetch[T any](ctx context.Context, tracer trace.Tracer, src Source, call func(context.Context) (T, error)) (out Fetched[T]) {
	ctx, span := tracer.Start(ctx, snapshotSpanPrefix+string(src), trace.WithAttributes(attrSource.String(string(src))))
	defer func() {
		if r := recover(); r != nil {
			out = Fetched[T]{Err: fmt.Errorf("screen: %s: %w: %v", src, ErrPanic, r)}
		}
		// An unknown customer is a 404, not a source fault.
		if out.Err != nil && !errors.Is(out.Err, ErrUnknownCustomer) {
			span.SetStatus(codes.Error, failureClass(out.Err))
		}
		span.End()
	}()
	v, err := call(ctx)
	if err != nil {
		return Fetched[T]{Err: fmt.Errorf("screen: %s: %w", src, err)}
	}
	return Fetched[T]{Value: v}
}
