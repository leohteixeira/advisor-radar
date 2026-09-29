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
)

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

// Sources are the ports the engine reads a Snapshot from.
type Sources struct {
	Accounts  AccountSource
	Customers CustomerSource
	Activity  ActivitySource
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
	g.SetLimit(3)
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
