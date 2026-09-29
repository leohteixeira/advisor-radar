package advisory_test

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

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	"github.com/leohteixeira/advisor-radar/internal/advisory"
)

// stubAccountSim answers GetAccount from a fixed account or error and records
// the deadline each call carried.
type stubAccountSim struct {
	accountv1.UnimplementedAccountServiceServer

	account *accountv1.Account
	err     error

	mu        sync.Mutex
	deadlines []time.Duration // remaining time at arrival; <0 means none
}

func (s *stubAccountSim) GetAccount(ctx context.Context, _ *accountv1.GetAccountRequest) (*accountv1.Account, error) {
	s.mu.Lock()
	if d, ok := ctx.Deadline(); ok {
		s.deadlines = append(s.deadlines, time.Until(d))
	} else {
		s.deadlines = append(s.deadlines, -1)
	}
	s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return s.account, nil
}

func (s *stubAccountSim) recorded() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.deadlines...)
}

// dialBufconn serves register on an in-memory listener and returns a client
// connection to it.
func dialBufconn(t *testing.T, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	register(srv)
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
	return conn
}

func startAccountSim(t *testing.T, stub *stubAccountSim) *advisory.AccountSim {
	t.Helper()
	conn := dialBufconn(t, func(s *grpc.Server) { accountv1.RegisterAccountServiceServer(s, stub) })
	return advisory.NewAccountSim(accountv1.NewAccountServiceClient(conn))
}

func TestAccountSim_Balance(t *testing.T) {
	t.Parallel()
	stub := &stubAccountSim{account: &accountv1.Account{PatrimonyCents: 820_000, CaixaCents: 114_800, AcoesCents: 164_000}}
	got, err := startAccountSim(t, stub).Balance(t.Context(), "01a0e3a4-9a44-757a-ac8f-dab7db5eb068")
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if want := (advisory.Balance{PatrimonyCents: 820_000, CashCents: 114_800}); got != want {
		t.Errorf("Balance = %+v, want %+v", got, want)
	}
}

func TestAccountSim_BalanceErrorsKeepStatus(t *testing.T) {
	t.Parallel()
	for _, code := range []codes.Code{codes.NotFound, codes.Unavailable, codes.Internal} {
		t.Run(code.String(), func(t *testing.T) {
			t.Parallel()
			stub := &stubAccountSim{err: status.Error(code, "account-sim says no")}
			_, err := startAccountSim(t, stub).Balance(t.Context(), "id")
			if err == nil {
				t.Fatal("Balance error = nil")
			}
			if got := status.Code(errors.Unwrap(err)); got != code {
				t.Errorf("status = %v, want %v kept in %v", got, code, err)
			}
		})
	}
}

func TestAccountSim_DefaultDeadline(t *testing.T) {
	t.Parallel()
	stub := &stubAccountSim{account: &accountv1.Account{}}
	if _, err := startAccountSim(t, stub).Balance(context.Background(), "id"); err != nil {
		t.Fatalf("Balance: %v", err)
	}
	got := stub.recorded()
	if len(got) != 1 || got[0] <= 0 || got[0] > advisory.DefaultAccountTimeout {
		t.Fatalf("deadline remaining = %v, want in (0, %v]", got, advisory.DefaultAccountTimeout)
	}
}

func TestAccountSim_KeepsCallerDeadline(t *testing.T) {
	t.Parallel()
	stub := &stubAccountSim{account: &accountv1.Account{}}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := startAccountSim(t, stub).Balance(ctx, "id"); err != nil {
		t.Fatalf("Balance: %v", err)
	}
	got := stub.recorded()
	if len(got) != 1 || got[0] <= advisory.DefaultAccountTimeout {
		t.Fatalf("deadline remaining = %v, want the caller's 10s deadline", got)
	}
}
