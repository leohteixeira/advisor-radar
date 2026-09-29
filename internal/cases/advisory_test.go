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

// stubAdvisory answers GetCustomer from fixed values and records the
// deadline each call carried. ListOperators records its call and fails, so a
// lookup that still resolves the advisor by name is caught.
type stubAdvisory struct {
	advisoryv1.UnimplementedAdvisoryServiceServer

	customer *advisoryv1.Customer
	getErr   error

	mu        sync.Mutex
	deadlines []time.Duration // remaining time at arrival; <0 means none
	listCalls int
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

func (s *stubAdvisory) ListOperators(context.Context, *advisoryv1.ListOperatorsRequest) (*advisoryv1.ListOperatorsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	return nil, status.Error(codes.Internal, "lookup must not list operators")
}

func (s *stubAdvisory) recorded() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.deadlines...)
}

func (s *stubAdvisory) operatorLists() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listCalls
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
	tests := []struct {
		name          string
		stub          *stubAdvisory
		want          cases.Customer
		wantUnknown   bool
		wantTransient bool
	}{
		{
			name: "segment and advisor id from get customer",
			stub: &stubAdvisory{customer: &advisoryv1.Customer{
				Segment: book.SegmentSingular, Advisor: "Ana Paula Ribeiro", AdvisorId: advisorID,
			}},
			want: cases.Customer{Segment: book.SegmentSingular, AdvisorID: advisorID},
		},
		{
			name: "advisor name is not used to find the id",
			stub: &stubAdvisory{customer: &advisoryv1.Customer{Segment: book.SegmentAdvance, Advisor: "Ana Paula Ribeiro"}},
			want: cases.Customer{Segment: book.SegmentAdvance},
		},
		{
			name: "no advisor",
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
			if n := tt.stub.operatorLists(); n != 0 {
				t.Errorf("ListOperators calls = %d, want 0", n)
			}
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

func TestAdvisoryLookup_DefaultDeadline(t *testing.T) {
	t.Parallel()

	stub := &stubAdvisory{
		customer: &advisoryv1.Customer{Segment: book.SegmentSingular, Advisor: "Ana", AdvisorId: identity.MustNewV7()},
	}
	lookup := cases.NewAdvisoryLookup(startAdvisory(t, stub))
	if _, err := lookup.Lookup(context.Background(), identity.MustNewV7()); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	got := stub.recorded()
	if len(got) != 1 {
		t.Fatalf("calls = %d, want GetCustomer only", len(got))
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
