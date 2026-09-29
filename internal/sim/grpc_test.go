package sim_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/outbox"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// startAccountServer serves store over an in-memory bufconn listener.
func startAccountServer(t *testing.T, store sim.Store) accountv1.AccountServiceClient {
	t.Helper()
	return startAccountServerWithLogger(t, store, nil)
}

func startAccountServerWithLogger(t *testing.T, store sim.Store, logger *slog.Logger) accountv1.AccountServiceClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	accountv1.RegisterAccountServiceServer(srv, sim.NewGRPCServer(store, logger))
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
	return accountv1.NewAccountServiceClient(conn)
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %s (%v), want %s", got, err, want)
	}
}

func seeded(t *testing.T, customerID string) sim.Account {
	t.Helper()
	for _, account := range sim.POVSeed() {
		if account.CustomerID == customerID {
			return account
		}
	}
	t.Fatalf("customer %s is not seeded", customerID)
	return sim.Account{}
}

func caixaOf(t *testing.T, client accountv1.AccountServiceClient, customerID string) int64 {
	t.Helper()
	account, err := client.GetAccount(t.Context(), &accountv1.GetAccountRequest{CustomerId: customerID})
	if err != nil {
		t.Fatalf("GetAccount(%s): %v", customerID, err)
	}
	return account.GetCaixaCents()
}

func TestGRPCServer_Deposit(t *testing.T) {
	t.Parallel()
	store := sim.NewMemory()
	client := startAccountServer(t, store)
	ctx := t.Context()
	before := seeded(t, sim.CustomerFernanda)

	reply, err := client.Deposit(ctx, &accountv1.DepositRequest{
		CustomerId: sim.CustomerFernanda, IdempotencyKey: "dep-1", AmountCents: 1_000_000, Origin: "Conta corrente",
	})
	if err != nil {
		t.Fatalf("Deposit: %v", err)
	}
	if !identity.IsV7(reply.GetEventId()) {
		t.Fatalf("event_id = %q, want UUIDv7", reply.GetEventId())
	}
	if reply.GetReplay() {
		t.Fatal("replay = true on a new key")
	}
	if got := caixaOf(t, client, sim.CustomerFernanda); got != before.Caixa+1_000_000 {
		t.Fatalf("caixa = %d, want %d", got, before.Caixa+1_000_000)
	}

	rows := store.PendingOutbox()
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(rows))
	}
	if rows[0].RoutingKey != event.NameAccountEventRecorded || rows[0].EventID != reply.GetEventId() {
		t.Fatalf("outbox row = %s %s", rows[0].RoutingKey, rows[0].EventID)
	}
	env, body, err := sim.DecodeOutboxAccount(rows[0].Payload)
	if err != nil {
		t.Fatalf("decode outbox: %v", err)
	}
	if env.SchemaVersion != event.SchemaVersionCents || env.CustomerID != sim.CustomerFernanda {
		t.Fatalf("envelope = v%d %s", env.SchemaVersion, env.CustomerID)
	}
	if body.Kind != "aporte" || body.Amount != 1_000_000 || body.Origin != "Conta corrente" {
		t.Fatalf("payload = %+v", body)
	}
}

func TestGRPCServer_DepositReplay(t *testing.T) {
	t.Parallel()
	store := sim.NewMemory()
	client := startAccountServer(t, store)
	ctx := t.Context()
	req := &accountv1.DepositRequest{
		CustomerId: sim.CustomerFernanda, IdempotencyKey: "dep-replay", AmountCents: 1_000_000, Origin: "Conta corrente",
	}

	first, err := client.Deposit(ctx, req)
	if err != nil {
		t.Fatalf("first Deposit: %v", err)
	}
	caixa := caixaOf(t, client, sim.CustomerFernanda)

	second, err := client.Deposit(ctx, req)
	if err != nil {
		t.Fatalf("second Deposit: %v", err)
	}
	if second.GetEventId() != first.GetEventId() || !second.GetReplay() {
		t.Fatalf("replay = %+v, want event %s with replay", second, first.GetEventId())
	}
	if got := len(store.PendingOutbox()); got != 1 {
		t.Fatalf("outbox rows = %d, want 1", got)
	}
	if got := caixaOf(t, client, sim.CustomerFernanda); got != caixa {
		t.Fatalf("caixa after replay = %d, want %d", got, caixa)
	}
}

func TestGRPCServer_ConcurrentSameKey(t *testing.T) {
	t.Parallel()
	store := sim.NewMemory()
	client := startAccountServer(t, store)
	ctx := t.Context()
	req := &accountv1.WithdrawRequest{
		CustomerId: sim.CustomerThiago, IdempotencyKey: "wd-race", AmountCents: 10_000, Destination: "Conta corrente",
	}

	const callers = 2
	replies := make([]*accountv1.CommandReply, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() { replies[i], errs[i] = client.Withdraw(ctx, req) })
	}
	wg.Wait()

	replays := 0
	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("Withdraw %d: %v", i, errs[i])
		}
		if replies[i].GetEventId() != replies[0].GetEventId() {
			t.Fatalf("event ids differ: %s vs %s", replies[i].GetEventId(), replies[0].GetEventId())
		}
		if replies[i].GetReplay() {
			replays++
		}
	}
	if replays != 1 {
		t.Fatalf("replays = %d, want 1", replays)
	}
	if got := len(store.PendingOutbox()); got != 1 {
		t.Fatalf("outbox rows = %d, want 1", got)
	}
}

func TestGRPCServer_OverCashWithdrawal(t *testing.T) {
	t.Parallel()
	store := sim.NewMemory()
	client := startAccountServer(t, store)
	caixa := seeded(t, sim.CustomerFernanda).Caixa

	_, err := client.Withdraw(t.Context(), &accountv1.WithdrawRequest{
		CustomerId: sim.CustomerFernanda, IdempotencyKey: "wd-over", AmountCents: caixa + 1, Destination: "Conta corrente",
	})
	wantCode(t, err, codes.FailedPrecondition)
	if got := len(store.PendingOutbox()); got != 0 {
		t.Fatalf("outbox rows = %d, want 0", got)
	}
	if got := caixaOf(t, client, sim.CustomerFernanda); got != caixa {
		t.Fatalf("caixa = %d, want %d", got, caixa)
	}
}

func TestGRPCServer_UnknownCustomer(t *testing.T) {
	t.Parallel()
	store := sim.NewMemory()
	client := startAccountServer(t, store)
	unknown := identity.MustNewV7()

	_, err := client.Deposit(t.Context(), &accountv1.DepositRequest{
		CustomerId: unknown, IdempotencyKey: "dep-unknown", AmountCents: 100, Origin: "Conta corrente",
	})
	wantCode(t, err, codes.NotFound)
	if got := len(store.PendingOutbox()); got != 0 {
		t.Fatalf("outbox rows = %d, want 0", got)
	}

	_, err = client.GetAccount(t.Context(), &accountv1.GetAccountRequest{CustomerId: unknown})
	wantCode(t, err, codes.NotFound)
}

func TestGRPCServer_BadInput(t *testing.T) {
	t.Parallel()
	v4 := uuid.NewString()
	tests := []struct {
		name string
		call func(context.Context, accountv1.AccountServiceClient) error
	}{
		{name: "zero amount", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.Deposit(ctx, &accountv1.DepositRequest{
				CustomerId: sim.CustomerFernanda, IdempotencyKey: "k", AmountCents: 0, Origin: "Conta corrente",
			})
			return err
		}},
		{name: "negative amount", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.Withdraw(ctx, &accountv1.WithdrawRequest{
				CustomerId: sim.CustomerFernanda, IdempotencyKey: "k", AmountCents: -1, Destination: "Conta corrente",
			})
			return err
		}},
		{name: "amount above max", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.Withdraw(ctx, &accountv1.WithdrawRequest{
				CustomerId: sim.CustomerMariana, IdempotencyKey: "k", AmountCents: sim.MaxAmountCents + 1, Destination: "Conta corrente",
			})
			return err
		}},
		{name: "deposit near max int64", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.Deposit(ctx, &accountv1.DepositRequest{
				CustomerId: sim.CustomerFernanda, IdempotencyKey: "k", AmountCents: math.MaxInt64, Origin: "Conta corrente",
			})
			return err
		}},
		{name: "missing key", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.Deposit(ctx, &accountv1.DepositRequest{
				CustomerId: sim.CustomerFernanda, AmountCents: 100, Origin: "Conta corrente",
			})
			return err
		}},
		{name: "missing origin", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.Deposit(ctx, &accountv1.DepositRequest{
				CustomerId: sim.CustomerFernanda, IdempotencyKey: "k", AmountCents: 100,
			})
			return err
		}},
		{name: "bad channel", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.SendMessage(ctx, &accountv1.SendMessageRequest{
				CustomerId: sim.CustomerFernanda, IdempotencyKey: "k", Channel: "sms", Text: "oi",
			})
			return err
		}},
		{name: "empty message text", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.SendMessage(ctx, &accountv1.SendMessageRequest{
				CustomerId: sim.CustomerFernanda, IdempotencyKey: "k", Channel: "chat",
			})
			return err
		}},
		{name: "empty complaint text", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.FileComplaint(ctx, &accountv1.FileComplaintRequest{
				CustomerId: sim.CustomerFernanda, IdempotencyKey: "k",
			})
			return err
		}},
		{name: "non-v7 customer on command", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.Deposit(ctx, &accountv1.DepositRequest{
				CustomerId: v4, IdempotencyKey: "k", AmountCents: 100, Origin: "Conta corrente",
			})
			return err
		}},
		{name: "malformed customer on command", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.FileComplaint(ctx, &accountv1.FileComplaintRequest{
				CustomerId: "fernanda", IdempotencyKey: "k", Text: "quero sair",
			})
			return err
		}},
		{name: "non-v7 customer on query", call: func(ctx context.Context, c accountv1.AccountServiceClient) error {
			_, err := c.GetAccount(ctx, &accountv1.GetAccountRequest{CustomerId: v4})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := sim.NewMemory()
			client := startAccountServer(t, store)
			wantCode(t, tt.call(t.Context(), client), codes.InvalidArgument)
			if got := len(store.PendingOutbox()); got != 0 {
				t.Fatalf("outbox rows = %d, want 0", got)
			}
		})
	}
}

func TestGRPCServer_MessageAndComplaint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		call        func(context.Context, accountv1.AccountServiceClient) (*accountv1.CommandReply, error)
		wantChannel string
	}{
		{name: "chat message", wantChannel: "chat", call: func(ctx context.Context, c accountv1.AccountServiceClient) (*accountv1.CommandReply, error) {
			return c.SendMessage(ctx, &accountv1.SendMessageRequest{
				CustomerId: sim.CustomerMariana, IdempotencyKey: "msg-1", Channel: "chat", Text: "Estou pensando em sair",
			})
		}},
		{name: "e-mail message", wantChannel: "e-mail", call: func(ctx context.Context, c accountv1.AccountServiceClient) (*accountv1.CommandReply, error) {
			return c.SendMessage(ctx, &accountv1.SendMessageRequest{
				CustomerId: sim.CustomerMariana, IdempotencyKey: "msg-2", Channel: "e-mail", Text: "Bom dia",
			})
		}},
		{name: "complaint", wantChannel: "chat", call: func(ctx context.Context, c accountv1.AccountServiceClient) (*accountv1.CommandReply, error) {
			return c.FileComplaint(ctx, &accountv1.FileComplaintRequest{
				CustomerId: sim.CustomerMariana, IdempotencyKey: "cmp-1", Text: "Cobrança indevida",
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := sim.NewMemory()
			client := startAccountServer(t, store)
			caixa := seeded(t, sim.CustomerMariana).Caixa

			reply, err := tt.call(t.Context(), client)
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			rows := store.PendingOutbox()
			if len(rows) != 1 {
				t.Fatalf("outbox rows = %d, want 1", len(rows))
			}
			if rows[0].RoutingKey != event.NameMessageReceived || rows[0].EventID != reply.GetEventId() {
				t.Fatalf("outbox row = %s %s", rows[0].RoutingKey, rows[0].EventID)
			}
			env, err := decodeEnvelope(rows[0])
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if env.SchemaVersion != event.SchemaVersionMVP {
				t.Fatalf("schema_version = %d, want %d", env.SchemaVersion, event.SchemaVersionMVP)
			}
			if !strings.Contains(string(rows[0].Payload), `"channel":"`+tt.wantChannel+`"`) {
				t.Fatalf("payload = %s, want channel %s", rows[0].Payload, tt.wantChannel)
			}
			if !strings.Contains(string(rows[0].Payload), `"origin":"`+sim.OriginClientApp+`"`) {
				t.Fatalf("payload = %s, want origin %s", rows[0].Payload, sim.OriginClientApp)
			}
			if got := caixaOf(t, client, sim.CustomerMariana); got != caixa {
				t.Fatalf("caixa = %d, want %d", got, caixa)
			}
		})
	}
}

func decodeEnvelope(row outbox.Row) (event.Envelope, error) {
	var env event.Envelope
	err := json.Unmarshal(row.Payload, &env)
	return env, err
}

func TestGRPCServer_Queries(t *testing.T) {
	t.Parallel()
	client := startAccountServer(t, sim.NewMemory())
	ctx := t.Context()
	want := seeded(t, sim.CustomerMariana)

	got, err := client.GetAccount(ctx, &accountv1.GetAccountRequest{CustomerId: sim.CustomerMariana})
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.GetCustomerId() != want.CustomerID || got.GetAcoesCents() != want.Acoes || got.GetEtfsCents() != want.ETFs ||
		got.GetRendaFixaCents() != want.RendaFixa || got.GetCaixaCents() != want.Caixa {
		t.Fatalf("GetAccount = %+v, want %+v", got, want)
	}

	list, err := client.ListAccounts(ctx, &accountv1.ListAccountsRequest{})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(list.GetAccounts()) != 3 {
		t.Fatalf("accounts = %d, want 3", len(list.GetAccounts()))
	}
	for _, account := range list.GetAccounts() {
		if seeded(t, account.GetCustomerId()).Caixa != account.GetCaixaCents() {
			t.Fatalf("account %s caixa = %d", account.GetCustomerId(), account.GetCaixaCents())
		}
	}
}

// failingStore wraps Memory and fails InsertOutbox with err (errLeaky by
// default), so the tests can prove rollback and the status mapping.
type failingStore struct {
	inner *sim.Memory
	err   error
}

func (s failingStore) WithTx(ctx context.Context, fn func(sim.Tx) error) error {
	err := s.err
	if err == nil {
		err = errLeaky
	}
	return s.inner.WithTx(ctx, func(tx sim.Tx) error { return fn(failingTx{Tx: tx, err: err}) })
}

type failingTx struct {
	sim.Tx
	err error
}

var errLeaky = errors.New(`INSERT INTO outbox (event_id) VALUES ($1::uuid): postgres://localdev:secret@127.0.0.1:5435/account_sim`)

func (t failingTx) InsertOutbox(context.Context, outbox.Row) error {
	return t.err
}

// syncBuffer is a bytes.Buffer safe for the server goroutines and the test.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestGRPCServer_ContextErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{name: "canceled", err: fmt.Errorf("sim pgx: insert outbox: %w", context.Canceled), want: codes.Canceled},
		{name: "deadline exceeded", err: fmt.Errorf("sim pgx: insert outbox: %w", context.DeadlineExceeded), want: codes.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var logs syncBuffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			memory := sim.NewMemory()
			client := startAccountServerWithLogger(t, failingStore{inner: memory, err: tt.err}, logger)

			_, err := client.Deposit(t.Context(), &accountv1.DepositRequest{
				CustomerId: sim.CustomerFernanda, IdempotencyKey: "dep-ctx", AmountCents: 100, Origin: "Conta corrente",
			})
			wantCode(t, err, tt.want)
			if strings.Contains(status.Convert(err).Message(), "outbox") {
				t.Fatalf("status message %q leaks storage text", status.Convert(err).Message())
			}
			if strings.Contains(logs.String(), `"level":"ERROR"`) {
				t.Fatalf("context error logged at error level: %s", logs.String())
			}
			if got := len(memory.PendingOutbox()); got != 0 {
				t.Fatalf("outbox rows = %d, want 0", got)
			}
		})
	}
}

func TestGRPCServer_StoreFailure(t *testing.T) {
	t.Parallel()
	memory := sim.NewMemory()
	client := startAccountServer(t, failingStore{inner: memory})
	caixa := seeded(t, sim.CustomerFernanda).Caixa

	_, err := client.Deposit(t.Context(), &accountv1.DepositRequest{
		CustomerId: sim.CustomerFernanda, IdempotencyKey: "dep-fail", AmountCents: 1_000_000, Origin: "Conta corrente",
	})
	wantCode(t, err, codes.Internal)
	msg := status.Convert(err).Message()
	for _, leak := range []string{"INSERT", "outbox", "postgres://", "secret", "5435"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("status message %q leaks %q", msg, leak)
		}
	}
	if got := len(memory.PendingOutbox()); got != 0 {
		t.Fatalf("outbox rows = %d, want 0", got)
	}
	if got := caixaOf(t, client, sim.CustomerFernanda); got != caixa {
		t.Fatalf("caixa = %d, want %d after rollback", got, caixa)
	}

	// The key was rolled back too, so a retry on a healthy store applies once.
	healthy := startAccountServer(t, memory)
	reply, err := healthy.Deposit(t.Context(), &accountv1.DepositRequest{
		CustomerId: sim.CustomerFernanda, IdempotencyKey: "dep-fail", AmountCents: 1_000_000, Origin: "Conta corrente",
	})
	if err != nil {
		t.Fatalf("retry Deposit: %v", err)
	}
	if reply.GetReplay() {
		t.Fatal("retry replay = true, want a fresh apply after rollback")
	}
}
