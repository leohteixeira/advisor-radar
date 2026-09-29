package advisory_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	advisoryv1 "github.com/leohteixeira/advisor-radar/gen/advisory/v1"
	"github.com/leohteixeira/advisor-radar/internal/advisory"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// Seed customers of the advisory book.
const (
	fernandaID = "01a0e3a4-9a44-757a-ac8f-dab7db5eb068"
	thiagoID   = "01a0e3a4-9a44-75dd-b3a0-403a7a87836e"
	marianaID  = "01a0e3a4-9a44-7566-b5de-eb2e365799f8"
)

var grpcNow = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// fakeBook is a FactsReader over fixed rows. Customers without a segment are
// outside the book.
type fakeBook struct {
	segments map[string]string
	profiles map[string]advisory.InvestorProfile
	alerts   map[string][]advisory.SegmentAlert
	err      error

	mu    sync.Mutex
	since []time.Time
}

func (b *fakeBook) InvestorProfile(ctx context.Context, customerID string) (advisory.InvestorProfile, error) {
	if err := ctx.Err(); err != nil {
		return advisory.InvestorProfile{}, err
	}
	if b.err != nil {
		return advisory.InvestorProfile{}, b.err
	}
	p, ok := b.profiles[customerID]
	if !ok {
		return advisory.InvestorProfile{}, fmt.Errorf("fake: %w", advisory.ErrUnknownCustomer)
	}
	return p, nil
}

func (b *fakeBook) MomentBook(ctx context.Context, customerID string, since time.Time) (advisory.MomentBook, error) {
	if err := ctx.Err(); err != nil {
		return advisory.MomentBook{}, err
	}
	b.mu.Lock()
	b.since = append(b.since, since)
	b.mu.Unlock()
	if b.err != nil {
		return advisory.MomentBook{}, b.err
	}
	segment, ok := b.segments[customerID]
	if !ok {
		return advisory.MomentBook{}, fmt.Errorf("fake: %w", advisory.ErrUnknownCustomer)
	}
	return advisory.MomentBook{Segment: segment, Alerts: b.alerts[customerID]}, nil
}

func (b *fakeBook) sinceSeen() []time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]time.Time(nil), b.since...)
}

// fakeBalances is an AccountReader over fixed balances.
type fakeBalances struct {
	balances map[string]advisory.Balance
	err      error
}

func (f fakeBalances) Balance(ctx context.Context, customerID string) (advisory.Balance, error) {
	if err := ctx.Err(); err != nil {
		return advisory.Balance{}, err
	}
	if f.err != nil {
		return advisory.Balance{}, f.err
	}
	b, ok := f.balances[customerID]
	if !ok {
		return advisory.Balance{}, status.Error(codes.NotFound, "account not found")
	}
	return b, nil
}

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

// seedBook is the day-0 book of the three seed clients, with Thiago's
// seeded (schema version 1) upgrade alert.
func seedBook() *fakeBook {
	return &fakeBook{
		segments: map[string]string{fernandaID: "Essencial", thiagoID: "Advance", marianaID: "Singular"},
		profiles: map[string]advisory.InvestorProfile{
			fernandaID: {Profile: advisory.ProfileConservador, AssessedOn: day("2026-03-12")},
			thiagoID:   {Profile: advisory.ProfileArrojado, AssessedOn: day("2026-08-04")},
			marianaID:  {Profile: advisory.ProfileModerado, AssessedOn: day("2026-01-20")},
		},
		alerts: map[string][]advisory.SegmentAlert{
			thiagoID: {{From: "Essencial", To: "Advance", SchemaVersion: 1, RaisedAt: grpcNow.Add(-238 * time.Minute)}},
		},
	}
}

// seedBalances are the day-0 account-sim balances.
func seedBalances() fakeBalances {
	return fakeBalances{balances: map[string]advisory.Balance{
		fernandaID: {PatrimonyCents: 820_000, CashCents: 114_800},
		thiagoID:   {PatrimonyCents: 6_800_000, CashCents: 6_052_000},
		marianaID:  {PatrimonyCents: 24_830_000, CashCents: 6_000_000},
	}}
}

func startAdvisory(t *testing.T, opts ...advisory.ServerOption) advisoryv1.AdvisoryServiceClient {
	t.Helper()
	opts = append([]advisory.ServerOption{advisory.WithServerClock(func() time.Time { return grpcNow })}, opts...)
	srv := advisory.NewGRPCServer(nil, opts...)
	conn := dialBufconn(t, func(s *grpc.Server) { advisoryv1.RegisterAdvisoryServiceServer(s, srv) })
	return advisoryv1.NewAdvisoryServiceClient(conn)
}

func TestGRPCServer_GetInvestorProfile(t *testing.T) {
	t.Parallel()
	broken := seedBook()
	broken.profiles[thiagoID] = advisory.InvestorProfile{Profile: "agressivo", AssessedOn: day("2026-08-04")}
	tests := []struct {
		name     string
		book     *fakeBook
		id       string
		expected *advisoryv1.InvestorProfile
		code     codes.Code
	}{
		{
			name: "fernanda", book: seedBook(), id: fernandaID,
			expected: &advisoryv1.InvestorProfile{Profile: "conservador", MaxRisk: 2, AssessedOn: "2026-03-12"},
		},
		{
			name: "mariana", book: seedBook(), id: marianaID,
			expected: &advisoryv1.InvestorProfile{Profile: "moderado", MaxRisk: 3, AssessedOn: "2026-01-20"},
		},
		{
			name: "thiago", book: seedBook(), id: thiagoID,
			expected: &advisoryv1.InvestorProfile{Profile: "arrojado", MaxRisk: 5, AssessedOn: "2026-08-04"},
		},
		{name: "unknown customer", book: seedBook(), id: identity.MustNewV7(), code: codes.NotFound},
		{name: "invalid id", book: seedBook(), id: "x", code: codes.InvalidArgument},
		{name: "profile outside the max-risk table", book: broken, id: thiagoID, code: codes.Internal},
		{name: "book down", book: &fakeBook{err: errors.New("pool closed")}, id: thiagoID, code: codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := startAdvisory(t, advisory.WithFactsReader(tt.book))
			got, err := client.GetInvestorProfile(t.Context(), &advisoryv1.GetInvestorProfileRequest{CustomerId: tt.id})
			if tt.code != codes.OK {
				if status.Code(err) != tt.code {
					t.Fatalf("GetInvestorProfile error = %v, want %v", err, tt.code)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetInvestorProfile: %v", err)
			}
			if !proto.Equal(got, tt.expected) {
				t.Errorf("GetInvestorProfile = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGRPCServer_GetMomentFacts(t *testing.T) {
	t.Parallel()
	afterDeposit := seedBook()
	afterDeposit.segments[fernandaID] = "Advance"
	afterDeposit.alerts[fernandaID] = []advisory.SegmentAlert{
		{From: "Essencial", To: "Advance", SchemaVersion: 2, RaisedAt: grpcNow.Add(-time.Minute)},
	}
	depositBalances := seedBalances()
	depositBalances.balances[fernandaID] = advisory.Balance{PatrimonyCents: 1_820_000, CashCents: 1_114_800}

	tests := []struct {
		name     string
		book     *fakeBook
		accounts advisory.AccountReader
		id       string
		expected *advisoryv1.MomentFacts
		code     codes.Code
	}{
		{
			name: "fernanda near advance", book: seedBook(), accounts: seedBalances(), id: fernandaID,
			expected: &advisoryv1.MomentFacts{
				SegmentUpgradeNear: true, UpgradeGapCents: 180_000, CashCents: 114_800, PatrimonyCents: 820_000,
			},
		},
		{
			name: "thiago idle cash and a seeded upgrade", book: seedBook(), accounts: seedBalances(), id: thiagoID,
			expected: &advisoryv1.MomentFacts{IdleCash: true, CashCents: 6_052_000, PatrimonyCents: 6_800_000},
		},
		{
			name: "mariana singular", book: seedBook(), accounts: seedBalances(), id: marianaID,
			expected: &advisoryv1.MomentFacts{PortfolioReview: true, CashCents: 6_000_000, PatrimonyCents: 24_830_000},
		},
		{
			name: "fernanda after a live upgrade", book: afterDeposit, accounts: depositBalances, id: fernandaID,
			expected: &advisoryv1.MomentFacts{
				SegmentUpgraded: true, UpgradedSegment: "Advance",
				IdleCash: true, CashCents: 1_114_800, PatrimonyCents: 1_820_000,
			},
		},
		{name: "unknown customer", book: seedBook(), accounts: seedBalances(), id: identity.MustNewV7(), code: codes.NotFound},
		{name: "invalid id", book: seedBook(), accounts: seedBalances(), id: "x", code: codes.InvalidArgument},
		{
			name: "account-sim down", book: seedBook(), id: thiagoID, code: codes.Unavailable,
			accounts: fakeBalances{err: status.Error(codes.Unavailable, "account-sim down")},
		},
		{
			name: "account-sim does not know the customer", book: seedBook(), id: thiagoID, code: codes.Unavailable,
			accounts: fakeBalances{},
		},
		{name: "no account-sim configured", book: seedBook(), id: thiagoID, code: codes.Unavailable},
		{name: "book down", book: &fakeBook{err: errors.New("pool closed")}, accounts: seedBalances(), id: thiagoID, code: codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := []advisory.ServerOption{advisory.WithFactsReader(tt.book)}
			if tt.accounts != nil {
				opts = append(opts, advisory.WithAccountReader(tt.accounts))
			}
			client := startAdvisory(t, opts...)
			got, err := client.GetMomentFacts(t.Context(), &advisoryv1.GetMomentFactsRequest{CustomerId: tt.id})
			if tt.code != codes.OK {
				if status.Code(err) != tt.code {
					t.Fatalf("GetMomentFacts error = %v, want %v", err, tt.code)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetMomentFacts: %v", err)
			}
			if !proto.Equal(got, tt.expected) {
				t.Errorf("GetMomentFacts = %v, want %v", got, tt.expected)
			}
			if since := tt.book.sinceSeen(); len(since) != 1 || !since[0].Equal(grpcNow.Add(-24*time.Hour)) {
				t.Errorf("alerts read since %v, want %v", since, grpcNow.Add(-24*time.Hour))
			}
		})
	}
}

func TestGRPCServer_WithoutBook(t *testing.T) {
	t.Parallel()
	client := startAdvisory(t, advisory.WithAccountReader(seedBalances()))
	if _, err := client.GetMomentFacts(t.Context(), &advisoryv1.GetMomentFactsRequest{CustomerId: thiagoID}); status.Code(err) != codes.Unavailable {
		t.Errorf("GetMomentFacts error = %v, want Unavailable", err)
	}
	if _, err := client.GetInvestorProfile(t.Context(), &advisoryv1.GetInvestorProfileRequest{CustomerId: thiagoID}); status.Code(err) != codes.Unavailable {
		t.Errorf("GetInvestorProfile error = %v, want Unavailable", err)
	}
	if _, err := client.GetCustomer(t.Context(), &advisoryv1.GetCustomerRequest{CustomerId: thiagoID}); status.Code(err) != codes.Unavailable {
		t.Errorf("GetCustomer error = %v, want Unavailable", err)
	}
	if _, err := client.ListOperators(t.Context(), &advisoryv1.ListOperatorsRequest{}); status.Code(err) != codes.Unavailable {
		t.Errorf("ListOperators error = %v, want Unavailable", err)
	}
	if _, err := client.ListQueue(t.Context(), &advisoryv1.ListQueueRequest{}); status.Code(err) != codes.Unavailable {
		t.Errorf("ListQueue error = %v, want Unavailable", err)
	}
	if _, err := client.ContactMetrics(t.Context(), &advisoryv1.ContactMetricsRequest{}); status.Code(err) != codes.Unavailable {
		t.Errorf("ContactMetrics error = %v, want Unavailable", err)
	}
}

// cancelingBalances cancels the request while account-sim is being read.
type cancelingBalances struct{ cancel context.CancelFunc }

func (c cancelingBalances) Balance(ctx context.Context, _ string) (advisory.Balance, error) {
	c.cancel()
	<-ctx.Done()
	return advisory.Balance{}, fmt.Errorf("fake account-sim: %w", ctx.Err())
}

// TestGRPCServer_CallerGone checks that a request the caller canceled or let
// expire answers with the context's code, not Unavailable or Internal.
func TestGRPCServer_CallerGone(t *testing.T) {
	t.Parallel()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	expired, cancelExpired := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancelExpired()

	srv := advisory.NewGRPCServer(nil, advisory.WithFactsReader(seedBook()), advisory.WithAccountReader(seedBalances()))
	for name, tt := range map[string]struct {
		ctx  context.Context
		code codes.Code
	}{
		"canceled": {ctx: canceled, code: codes.Canceled},
		"expired":  {ctx: expired, code: codes.DeadlineExceeded},
	} {
		if _, err := srv.GetMomentFacts(tt.ctx, &advisoryv1.GetMomentFactsRequest{CustomerId: thiagoID}); status.Code(err) != tt.code {
			t.Errorf("%s GetMomentFacts error = %v, want %v", name, err, tt.code)
		}
		if _, err := srv.GetInvestorProfile(tt.ctx, &advisoryv1.GetInvestorProfileRequest{CustomerId: thiagoID}); status.Code(err) != tt.code {
			t.Errorf("%s GetInvestorProfile error = %v, want %v", name, err, tt.code)
		}
	}

	ctx, cancelRead := context.WithCancel(t.Context())
	defer cancelRead()
	srv = advisory.NewGRPCServer(nil, advisory.WithFactsReader(seedBook()), advisory.WithAccountReader(cancelingBalances{cancel: cancelRead}))
	if _, err := srv.GetMomentFacts(ctx, &advisoryv1.GetMomentFactsRequest{CustomerId: thiagoID}); status.Code(err) != codes.Canceled {
		t.Errorf("canceled during the balance read = %v, want Canceled", err)
	}
}
