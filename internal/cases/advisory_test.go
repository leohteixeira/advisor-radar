package cases_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	advisoryv1 "github.com/leohteixeira/advisor-radar/gen/advisory/v1"
	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// stubAdvisory answers GetCustomer and ListOperators from fixed values and
// records the deadline each call carried.
type stubAdvisory struct {
	advisoryv1.UnimplementedAdvisoryServiceServer

	customer  *advisoryv1.Customer
	getErr    error
	listErr   error
	operators []*advisoryv1.Operator

	mu        sync.Mutex
	deadlines []time.Duration // remaining time at arrival; <0 means none
}

func (s *stubAdvisory) record(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := ctx.Deadline()
	if !ok {
		s.deadlines = append(s.deadlines, -1)
		return
	}
	s.deadlines = append(s.deadlines, time.Until(d))
}

func (s *stubAdvisory) GetCustomer(ctx context.Context, _ *advisoryv1.GetCustomerRequest) (*advisoryv1.Customer, error) {
	s.record(ctx)
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.customer, nil
}

func (s *stubAdvisory) ListOperators(ctx context.Context, _ *advisoryv1.ListOperatorsRequest) (*advisoryv1.ListOperatorsResponse, error) {
	s.record(ctx)
	if s.listErr != nil {
		return nil, s.listErr
	}
	return &advisoryv1.ListOperatorsResponse{Items: s.operators}, nil
}

func (s *stubAdvisory) recorded() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.deadlines...)
}

func startAdvisory(t *testing.T, stub *stubAdvisory) advisoryv1.AdvisoryServiceClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	advisoryv1.RegisterAdvisoryServiceServer(srv, stub)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return advisoryv1.NewAdvisoryServiceClient(conn)
}

func TestAdvisoryLookup_Lookup(t *testing.T) {
	t.Parallel()

	advisorID := identity.MustNewV7()
	operators := []*advisoryv1.Operator{
		{Id: identity.MustNewV7(), Name: "Carlos Mendes"},
		{Id: advisorID, Name: "Ana Paula Ribeiro"},
	}
	tests := []struct {
		name          string
		stub          *stubAdvisory
		want          cases.Customer
		wantUnknown   bool
		wantTransient bool
	}{
		{
			name: "maps segment and advisor name to operator id",
			stub: &stubAdvisory{
				customer:  &advisoryv1.Customer{Segment: book.SegmentSingular, Advisor: "Ana Paula Ribeiro"},
				operators: operators,
			},
			want: cases.Customer{Segment: book.SegmentSingular, AdvisorID: advisorID},
		},
		{
			name: "unknown advisor name leaves advisor empty",
			stub: &stubAdvisory{
				customer:  &advisoryv1.Customer{Segment: book.SegmentSingular, Advisor: "Nobody"},
				operators: operators,
			},
			want: cases.Customer{Segment: book.SegmentSingular},
		},
		{
			name: "ambiguous advisor name leaves advisor empty",
			stub: &stubAdvisory{
				customer: &advisoryv1.Customer{Segment: book.SegmentAdvance, Advisor: "Ana Paula Ribeiro"},
				operators: append([]*advisoryv1.Operator{{Id: identity.MustNewV7(), Name: "Ana Paula Ribeiro"}},
					operators...),
			},
			want: cases.Customer{Segment: book.SegmentAdvance},
		},
		{
			name: "no advisor name",
			stub: &stubAdvisory{customer: &advisoryv1.Customer{Segment: book.SegmentEssencial}},
			want: cases.Customer{Segment: book.SegmentEssencial},
		},
		{
			name:        "not found is unknown customer",
			stub:        &stubAdvisory{getErr: status.Error(codes.NotFound, "customer not found")},
			wantUnknown: true,
		},
		{
			name:        "invalid argument is unknown customer",
			stub:        &stubAdvisory{getErr: status.Error(codes.InvalidArgument, "invalid customer id")},
			wantUnknown: true,
		},
		{
			name:        "unimplemented is unknown customer",
			stub:        &stubAdvisory{getErr: status.Error(codes.Unimplemented, "no such method")},
			wantUnknown: true,
		},
		{
			name:          "unavailable is transient",
			stub:          &stubAdvisory{getErr: status.Error(codes.Unavailable, "down")},
			wantTransient: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lookup := cases.NewAdvisoryLookup(startAdvisory(t, tt.stub))
			got, err := lookup.Lookup(t.Context(), identity.MustNewV7())
			switch {
			case tt.wantUnknown:
				if !errors.Is(err, cases.ErrUnknownCustomer) {
					t.Fatalf("err = %v, want ErrUnknownCustomer", err)
				}
				return
			case tt.wantTransient:
				if err == nil || errors.Is(err, cases.ErrUnknownCustomer) {
					t.Fatalf("err = %v, want a transient error", err)
				}
				if status.Code(errors.Unwrap(err)) != codes.Unavailable {
					t.Fatalf("err = %v, want the Unavailable status kept", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Lookup: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Lookup = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAdvisoryLookup_ListOperatorsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		err           error
		wantPermanent bool
	}{
		{"not found is permanent", status.Error(codes.NotFound, "no operators"), true},
		{"invalid argument is permanent", status.Error(codes.InvalidArgument, "bad"), true},
		{"unimplemented is permanent", status.Error(codes.Unimplemented, "no such method"), true},
		{"unavailable is transient", status.Error(codes.Unavailable, "down"), false},
		{"internal is transient", status.Error(codes.Internal, "boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stub := &stubAdvisory{
				customer: &advisoryv1.Customer{Segment: book.SegmentSingular, Advisor: "Ana"},
				listErr:  tt.err,
			}
			_, err := cases.NewAdvisoryLookup(startAdvisory(t, stub)).Lookup(t.Context(), identity.MustNewV7())
			if err == nil {
				t.Fatal("Lookup error = nil, want error")
			}
			if got := errors.Is(err, cases.ErrUnusableCustomer); got != tt.wantPermanent {
				t.Fatalf("ErrUnusableCustomer = %v, want %v (%v)", got, tt.wantPermanent, err)
			}
			if errors.Is(err, cases.ErrUnknownCustomer) {
				t.Fatalf("err = %v, want no ErrUnknownCustomer from ListOperators", err)
			}
			if status.Code(errors.Unwrap(err)) != status.Code(tt.err) && !tt.wantPermanent {
				t.Fatalf("err = %v, want the status kept", err)
			}
		})
	}
}

func TestAdvisoryLookup_DefaultDeadline(t *testing.T) {
	t.Parallel()

	stub := &stubAdvisory{
		customer:  &advisoryv1.Customer{Segment: book.SegmentSingular, Advisor: "Ana"},
		operators: []*advisoryv1.Operator{{Id: identity.MustNewV7(), Name: "Ana"}},
	}
	lookup := cases.NewAdvisoryLookup(startAdvisory(t, stub))
	if _, err := lookup.Lookup(context.Background(), identity.MustNewV7()); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	got := stub.recorded()
	if len(got) != 2 {
		t.Fatalf("calls = %d, want GetCustomer and ListOperators", len(got))
	}
	for i, d := range got {
		if d <= 0 || d > cases.DefaultLookupTimeout {
			t.Fatalf("call %d deadline remaining = %v, want in (0, %v]", i, d, cases.DefaultLookupTimeout)
		}
	}
}

func TestAdvisoryLookup_KeepsCallerDeadline(t *testing.T) {
	t.Parallel()

	stub := &stubAdvisory{customer: &advisoryv1.Customer{Segment: book.SegmentSingular}}
	lookup := cases.NewAdvisoryLookup(startAdvisory(t, stub))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := lookup.Lookup(ctx, identity.MustNewV7()); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	got := stub.recorded()
	if len(got) != 1 || got[0] <= cases.DefaultLookupTimeout {
		t.Fatalf("deadline remaining = %v, want the caller's 10s deadline", got)
	}
}

func TestAdvisoryLookup_DeadlineExceededIsTransient(t *testing.T) {
	t.Parallel()

	stub := &stubAdvisory{customer: &advisoryv1.Customer{Segment: book.SegmentSingular}}
	lookup := cases.NewAdvisoryLookup(startAdvisory(t, stub))
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := lookup.Lookup(ctx, identity.MustNewV7())
	if err == nil || errors.Is(err, cases.ErrUnknownCustomer) {
		t.Fatalf("err = %v, want a transient deadline error", err)
	}
	if status.Code(errors.Unwrap(err)) != codes.DeadlineExceeded {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}
