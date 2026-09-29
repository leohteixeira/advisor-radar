package screen

import (
	"context"
	"errors"
	"fmt"
	"time"

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
)

// allSources is every Snapshot source in fetch and failure-report order.
var allSources = []Source{
	SourceAccount,
	SourceAdvisory,
	SourceTimeline,
	SourceMoments,
	SourceProfile,
	SourceCases,
}

// ErrPanic marks a failure that was a recovered panic, in a source adapter
// or a variant.
var ErrPanic = errors.New("screen: panic")

// ErrUnknownCustomer is what an AccountSource wraps when account-sim answers
// NotFound. It is the only source failure that fails the whole screen.
var ErrUnknownCustomer = errors.New("screen: unknown customer")

// Account is the account-sim account in integer USD cents. Acoes, ETFs, and
// RendaFixa are the class aggregates of the positions at market value;
// Patrimony is positions plus Cash.
type Account struct {
	Acoes     int64
	ETFs      int64
	RendaFixa int64
	Cash      int64
	Patrimony int64
	Positions []Position
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
// long ago the row was when it was indexed) is used instead.
type Activity struct {
	Kind       string
	Title      string
	Source     string
	OccurredAt time.Time
	Age        time.Duration
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
// highest product risk it accepts.
type InvestorProfile struct {
	Profile    string
	MaxRisk    int
	AssessedOn time.Time
}

// OpenCase is one of the customer's cases that is not resolved. Age is how
// long ago it was opened.
type OpenCase struct {
	ID    string
	State string
	Age   time.Duration
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

// Sources are the ports the engine reads a Snapshot from.
type Sources struct {
	Accounts  AccountSource
	Customers CustomerSource
	Activity  ActivitySource
	Moments   MomentSource
	Profiles  ProfileSource
	Cases     CaseSource
}

// complete reports whether every source is set.
func (s Sources) complete() bool {
	return s.Accounts != nil && s.Customers != nil && s.Activity != nil &&
		s.Moments != nil && s.Profiles != nil && s.Cases != nil
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
	default:
		return fmt.Errorf("screen: unknown source %q", src)
	}
}

// fetchSnapshot calls every source in parallel under one deadline. The group
// has no shared context, so one source failing never cancels the others; each
// goroutine writes only its own field, and Wait orders those writes before
// the return.
func fetchSnapshot(ctx context.Context, src Sources, customerID string, now time.Time, deadline time.Duration) Snapshot {
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	snap := Snapshot{CustomerID: customerID, Now: now}
	var g errgroup.Group
	g.SetLimit(len(allSources))
	g.Go(func() error {
		snap.Account = fetch(ctx, SourceAccount, func(ctx context.Context) (Account, error) {
			return src.Accounts.Account(ctx, customerID)
		})
		return nil
	})
	g.Go(func() error {
		snap.Customer = fetch(ctx, SourceAdvisory, func(ctx context.Context) (Customer, error) {
			return src.Customers.Customer(ctx, customerID)
		})
		return nil
	})
	g.Go(func() error {
		snap.Activity = fetch(ctx, SourceTimeline, func(ctx context.Context) ([]Activity, error) {
			return src.Activity.Activity(ctx, customerID)
		})
		return nil
	})
	g.Go(func() error {
		snap.Moments = fetch(ctx, SourceMoments, func(ctx context.Context) (MomentFacts, error) {
			return src.Moments.Moments(ctx, customerID)
		})
		return nil
	})
	g.Go(func() error {
		snap.Profile = fetch(ctx, SourceProfile, func(ctx context.Context) (InvestorProfile, error) {
			return src.Profiles.Profile(ctx, customerID)
		})
		return nil
	})
	g.Go(func() error {
		snap.Cases = fetch(ctx, SourceCases, func(ctx context.Context) ([]OpenCase, error) {
			return src.Cases.OpenCases(ctx, customerID)
		})
		return nil
	})
	_ = g.Wait() // every goroutine returns nil; errors live in the Snapshot
	return snap
}

// fetch runs one source call. A panic in the adapter becomes that source's
// error, wrapping ErrPanic, instead of killing the process.
func fetch[T any](ctx context.Context, src Source, call func(context.Context) (T, error)) (out Fetched[T]) {
	defer func() {
		if r := recover(); r != nil {
			out = Fetched[T]{Err: fmt.Errorf("screen: %s: %w: %v", src, ErrPanic, r)}
		}
	}()
	v, err := call(ctx)
	if err != nil {
		return Fetched[T]{Err: fmt.Errorf("screen: %s: %w", src, err)}
	}
	return Fetched[T]{Value: v}
}
