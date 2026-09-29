package bff_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// sinceQueue answers the book "since" for any customer so the POV JSON is
// fully populated.
type sinceQueue struct{ bff.EmptyQueue }

func (sinceQueue) GetCustomer(_ context.Context, id string) (bff.Customer, error) {
	return bff.Customer{Since: "since-" + id[len(id)-4:]}, nil
}

// startPOV serves srv over bufconn and returns the BFF handler wired to it
// through DialAccountSim and NewGRPCPOV, the production path.
func startPOV(t *testing.T, srv accountv1.AccountServiceServer) *bff.Server {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	accountv1.RegisterAccountServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := bff.DialAccountSim("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
	)
	if err != nil {
		t.Fatalf("DialAccountSim: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	pov := bff.NewGRPCPOV(accountv1.NewAccountServiceClient(conn))
	return bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, sinceQueue{}, nil, nil, pov, nil)
}

// startSim serves the real account-sim server over a fresh seeded memory store.
func startSim(t *testing.T) *bff.Server {
	t.Helper()
	return startPOV(t, sim.NewGRPCServer(sim.NewMemory(), nil))
}

func postPOV(t *testing.T, h http.Handler, ctx context.Context, customerID, route, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/client-pov/customers/"+customerID+"/"+route, strings.NewReader(body))
	req.Header.Set("Idempotency-Key", key)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func getPath(t *testing.T, h http.Handler, ctx context.Context, path string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil))
	return rr
}

type povCounters struct {
	Actions    map[string]int `json:"actions"`
	Refusals   map[string]int `json:"refusals"`
	Duplicates int            `json:"duplicates"`
}

func countersOf(t *testing.T, h http.Handler) povCounters {
	t.Helper()
	rr := getPath(t, h, t.Context(), "/v1/client-pov/counters")
	var got povCounters
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode counters %q: %v", rr.Body.String(), err)
	}
	return got
}

func eventIDOf(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	if body.EventID == "" {
		t.Fatalf("empty event_id in %q", rr.Body.String())
	}
	return body.EventID
}

func caixaOf(t *testing.T, h http.Handler, customerID string) int64 {
	t.Helper()
	rr := getPath(t, h, t.Context(), "/v1/client-pov/customers/"+customerID)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET home %s = %d", customerID, rr.Code)
	}
	var body struct {
		Caixa int64 `json:"caixa"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode home: %v", err)
	}
	return body.Caixa
}

// TestGRPCPOV_Commands drives each POV command through the HTTP handler, the
// gRPC adapter, and the real account-sim server.
func TestGRPCPOV_Commands(t *testing.T) {
	t.Parallel()
	const fernandaCaixa = 114_800
	unknown := identity.MustNewV7()

	tests := []struct {
		name        string
		customerID  string
		route       string
		body        string
		wantCode    int
		wantBody    string // exact JSON for refusals; empty means an event_id
		wantCaixa   int64
		wantAction  string
		wantRefusal string
	}{
		{
			name: "deposit", customerID: sim.CustomerFernanda, route: "deposits",
			body: `{"amount":1000000,"origin":"pix"}`, wantCode: http.StatusAccepted,
			wantCaixa: fernandaCaixa + 1_000_000, wantAction: "deposit",
		},
		{
			name: "withdrawal within cash", customerID: sim.CustomerFernanda, route: "withdrawals",
			body: `{"amount":4800,"destination":"conta-eua"}`, wantCode: http.StatusAccepted,
			wantCaixa: fernandaCaixa - 4_800, wantAction: "withdrawal",
		},
		{
			name: "message", customerID: sim.CustomerFernanda, route: "messages",
			body: `{"channel":"chat","text":"Posso resgatar amanhã?"}`, wantCode: http.StatusAccepted,
			wantCaixa: fernandaCaixa, wantAction: "message",
		},
		{
			name: "complaint", customerID: sim.CustomerFernanda, route: "complaints",
			body: `{"text":"Transferência atrasada"}`, wantCode: http.StatusAccepted,
			wantCaixa: fernandaCaixa, wantAction: "complaint",
		},
		{
			name: "over-cash withdrawal is FailedPrecondition", customerID: sim.CustomerFernanda, route: "withdrawals",
			body: `{"amount":99999999,"destination":"conta-eua"}`, wantCode: http.StatusUnprocessableEntity,
			wantBody: `{"error":"insufficient"}`, wantCaixa: fernandaCaixa, wantRefusal: "insufficient",
		},
		{
			name: "zero amount is InvalidArgument", customerID: sim.CustomerFernanda, route: "deposits",
			body: `{"amount":0,"origin":"pix"}`, wantCode: http.StatusUnprocessableEntity,
			wantBody: `{"error":"invalid"}`, wantCaixa: fernandaCaixa, wantRefusal: "invalid",
		},
		{
			name: "bad channel is InvalidArgument", customerID: sim.CustomerFernanda, route: "messages",
			body: `{"channel":"sms","text":"oi"}`, wantCode: http.StatusUnprocessableEntity,
			wantBody: `{"error":"invalid"}`, wantCaixa: fernandaCaixa, wantRefusal: "invalid",
		},
		{
			name: "unknown customer is NotFound", customerID: unknown, route: "deposits",
			body: `{"amount":100,"origin":"pix"}`, wantCode: http.StatusUnprocessableEntity,
			wantBody: `{"error":"invalid"}`, wantRefusal: "invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := startSim(t)

			rr := postPOV(t, h, t.Context(), tt.customerID, tt.route, "key-1", tt.body)
			if rr.Code != tt.wantCode {
				t.Fatalf("status = %d %q, want %d", rr.Code, rr.Body.String(), tt.wantCode)
			}
			if tt.wantBody != "" {
				if got := strings.TrimSpace(rr.Body.String()); got != tt.wantBody {
					t.Fatalf("body = %s, want %s", got, tt.wantBody)
				}
			} else {
				eventIDOf(t, rr)
			}

			if tt.customerID == sim.CustomerFernanda {
				if got := caixaOf(t, h, tt.customerID); got != tt.wantCaixa {
					t.Fatalf("caixa = %d, want %d", got, tt.wantCaixa)
				}
			}
			counters := countersOf(t, h)
			if tt.wantAction != "" && counters.Actions[tt.wantAction] != 1 {
				t.Fatalf("actions = %v, want %s=1", counters.Actions, tt.wantAction)
			}
			if tt.wantRefusal != "" && counters.Refusals[tt.wantRefusal] != 1 {
				t.Fatalf("refusals = %v, want %s=1", counters.Refusals, tt.wantRefusal)
			}
		})
	}
}

// TestGRPCPOV_Replay checks that a replayed key answers the same event_id,
// counts a duplicate, and spends no budget.
func TestGRPCPOV_Replay(t *testing.T) {
	t.Parallel()
	h := startSim(t)
	const body = `{"amount":1000000,"origin":"pix"}`

	first := postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "k1", body)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first = %d %s", first.Code, first.Body.String())
	}
	second := postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "k1", body)
	if second.Code != http.StatusAccepted {
		t.Fatalf("replay = %d %s", second.Code, second.Body.String())
	}
	if a, b := eventIDOf(t, first), eventIDOf(t, second); a != b {
		t.Fatalf("replay event_id = %s, want %s", b, a)
	}
	if got := caixaOf(t, h, sim.CustomerFernanda); got != 114_800+1_000_000 {
		t.Fatalf("caixa after replay = %d, want one deposit", got)
	}
	counters := countersOf(t, h)
	if counters.Duplicates != 1 || counters.Actions["deposit"] != 1 {
		t.Fatalf("counters = %+v, want 1 deposit and 1 duplicate", counters)
	}

	// Nine more new commands fill the per-minute budget of ten only if the
	// replay spent nothing; the eleventh new command is then refused.
	for i := range 9 {
		rr := postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "more-"+strconv.Itoa(i), `{"amount":1,"origin":"pix"}`)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("new command %d = %d %s", i, rr.Code, rr.Body.String())
		}
	}
	if rr := postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "over", `{"amount":1,"origin":"pix"}`); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("eleventh new command = %d, want 429", rr.Code)
	}
}

func TestGRPCPOV_GetUnknownIs404(t *testing.T) {
	t.Parallel()
	h := startSim(t)
	rr := getPath(t, h, t.Context(), "/v1/client-pov/customers/"+identity.MustNewV7())
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

// TestGRPCPOV_RefusalSpendsBudget checks that refused commands spend the
// per-minute budget, as in phase 2.
func TestGRPCPOV_RefusalSpendsBudget(t *testing.T) {
	t.Parallel()
	h := startSim(t)
	for i := range 10 {
		rr := postPOV(t, h, t.Context(), sim.CustomerFernanda, "withdrawals", "over-"+strconv.Itoa(i), `{"amount":99999999,"destination":"conta-eua"}`)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("withdrawal %d = %d %q, want 422", i, rr.Code, rr.Body.String())
		}
	}
	if rr := postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "next", `{"amount":100,"origin":"pix"}`); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("eleventh command = %d, want 429", rr.Code)
	}
}

// Golden bodies captured from the phase-2 map[string]any handlers. The typed
// DTOs must encode byte for byte the same.
const (
	goldenList = `{"items":[` +
		`{"advisor":"Ana Paula Ribeiro","assets":24830000,"customer_id":"01a0e3a4-9a44-7566-b5de-eb2e365799f8","hint":"Já reclamou de uma transferência atrasada. Um saque grande ou uma ameaça de saída sobem a prioridade na hora.","name":"Mariana Costa","segment":"Singular","since":"since-99f8","sla":"1 h"},` +
		`{"advisor":"Ana Paula Ribeiro","assets":820000,"customer_id":"01a0e3a4-9a44-757a-ac8f-dab7db5eb068","hint":"Perto do teto da faixa. Um depósito pode subir o segmento; uma reclamação mostra o SLA mais longo.","name":"Fernanda Lima","segment":"Essencial","since":"since-b068","sla":"24 h"},` +
		`{"advisor":"Ana Paula Ribeiro","assets":6800000,"customer_id":"01a0e3a4-9a44-75dd-b3a0-403a7a87836e","hint":"Acabou de subir de Essencial para Advance com um depósito grande.","name":"Thiago Azevedo","segment":"Advance","since":"since-836e","sla":"4 h"}` +
		`]}` + "\n"
	goldenFernanda = `{"activity":[],"advisor":"Ana Paula Ribeiro","allocation":{"acoes":164000,"caixa":114800,"etfs":369000,"renda_fixa":172200},"assets":820000,"caixa":114800,"customer_id":"01a0e3a4-9a44-757a-ac8f-dab7db5eb068","messages":[],"name":"Fernanda Lima","segment":"Essencial","since":"since-b068","sla":"24 h"}` + "\n"
	goldenMariana  = `{"activity":[],"advisor":"Ana Paula Ribeiro","allocation":{"acoes":9090000,"caixa":6000000,"etfs":6060000,"renda_fixa":3680000},"assets":24830000,"caixa":6000000,"customer_id":"01a0e3a4-9a44-7566-b5de-eb2e365799f8","messages":[],"name":"Mariana Costa","segment":"Singular","since":"since-99f8","sla":"1 h"}` + "\n"
	goldenThiago   = `{"activity":[],"advisor":"Ana Paula Ribeiro","allocation":{"acoes":204000,"caixa":6052000,"etfs":544000,"renda_fixa":0},"assets":6800000,"caixa":6052000,"customer_id":"01a0e3a4-9a44-75dd-b3a0-403a7a87836e","messages":[],"name":"Thiago Azevedo","segment":"Advance","since":"since-836e","sla":"4 h"}` + "\n"
)

func TestGRPCPOV_GoldenJSON(t *testing.T) {
	t.Parallel()
	h := startSim(t)
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "list", path: "/v1/client-pov/customers", want: goldenList},
		{name: "home Fernanda", path: "/v1/client-pov/customers/" + sim.CustomerFernanda, want: goldenFernanda},
		{name: "home Mariana", path: "/v1/client-pov/customers/" + sim.CustomerMariana, want: goldenMariana},
		{name: "home Thiago", path: "/v1/client-pov/customers/" + sim.CustomerThiago, want: goldenThiago},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rr := getPath(t, h, t.Context(), tt.path)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d", rr.Code)
			}
			if got := rr.Body.String(); got != tt.want {
				t.Fatalf("body =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// stubAccount answers Deposit and GetAccount with respond(n) for call n
// (1-based) and records every call's deadline. A successful Deposit reports
// replay(n) when replay is set.
type stubAccount struct {
	accountv1.UnimplementedAccountServiceServer
	respond func(call int) error
	replay  func(call int) bool

	mu        sync.Mutex
	calls     int
	deadlines []time.Time
}

func (s *stubAccount) record(ctx context.Context) (int, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Time{}
	}
	s.deadlines = append(s.deadlines, deadline)
	s.mu.Unlock()
	return call, s.respond(call)
}

func (s *stubAccount) Deposit(ctx context.Context, _ *accountv1.DepositRequest) (*accountv1.CommandReply, error) {
	call, err := s.record(ctx)
	if err != nil {
		return nil, err
	}
	return &accountv1.CommandReply{
		EventId: "01a0e3a4-9a44-7000-8000-000000000001",
		Replay:  s.replay != nil && s.replay(call),
	}, nil
}

func (s *stubAccount) GetAccount(ctx context.Context, req *accountv1.GetAccountRequest) (*accountv1.Account, error) {
	if _, err := s.record(ctx); err != nil {
		return nil, err
	}
	return &accountv1.Account{CustomerId: req.GetCustomerId()}, nil
}

func (s *stubAccount) snapshot() (calls int, deadlines []time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, append([]time.Time(nil), s.deadlines...)
}

func failFirst(n int, code codes.Code) func(int) error {
	return func(call int) error {
		if call <= n {
			return status.Error(code, "stub failure")
		}
		return nil
	}
}

// TestDialAccountSim_Retry covers the Unavailable retry and the codes that
// must not be retried.
func TestDialAccountSim_Retry(t *testing.T) {
	t.Parallel()
	const deposit = `{"amount":100,"origin":"pix"}`
	tests := []struct {
		name      string
		respond   func(int) error
		get       bool
		wantCode  int
		wantCalls int
	}{
		{name: "two Unavailable then OK", respond: failFirst(2, codes.Unavailable), wantCode: http.StatusAccepted, wantCalls: 3},
		{name: "always Unavailable", respond: failFirst(1<<30, codes.Unavailable), wantCode: http.StatusBadGateway, wantCalls: 3},
		{name: "Internal is not retried", respond: failFirst(1<<30, codes.Internal), wantCode: http.StatusBadGateway, wantCalls: 1},
		{name: "FailedPrecondition is not retried", respond: failFirst(1<<30, codes.FailedPrecondition), wantCode: http.StatusUnprocessableEntity, wantCalls: 1},
		{name: "InvalidArgument is not retried", respond: failFirst(1<<30, codes.InvalidArgument), wantCode: http.StatusUnprocessableEntity, wantCalls: 1},
		{name: "NotFound on GET is 404 without retry", respond: failFirst(1<<30, codes.NotFound), get: true, wantCode: http.StatusNotFound, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stub := &stubAccount{respond: tt.respond}
			h := startPOV(t, stub)

			var rr *httptest.ResponseRecorder
			if tt.get {
				rr = getPath(t, h, t.Context(), "/v1/client-pov/customers/"+sim.CustomerFernanda)
			} else {
				rr = postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "k1", deposit)
			}
			if rr.Code != tt.wantCode {
				t.Fatalf("status = %d %q, want %d", rr.Code, rr.Body.String(), tt.wantCode)
			}
			if calls, _ := stub.snapshot(); calls != tt.wantCalls {
				t.Fatalf("calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

// TestDialAccountSim_RetryStopsWithContext checks that the backoff ends when
// the request context does, before the third attempt.
func TestDialAccountSim_RetryStopsWithContext(t *testing.T) {
	t.Parallel()
	stub := &stubAccount{respond: failFirst(1<<30, codes.Unavailable)}
	h := startPOV(t, stub)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	rr := postPOV(t, h, ctx, sim.CustomerFernanda, "deposits", "k1", `{"amount":100,"origin":"pix"}`)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
	// Two full backoffs take at least 25 ms + 50 ms; a third call would need both.
	if calls, _ := stub.snapshot(); calls >= 3 {
		t.Fatalf("calls = %d after the context ended, want fewer than 3", calls)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("returned after %s, want soon after the 30 ms deadline", elapsed)
	}
}

// TestDialAccountSim_OutageSpendsNoBudget fills more than the per-minute
// budget with Unavailable left after the retries, which means the command
// never reached account-sim; a later command must still be accepted.
func TestDialAccountSim_OutageSpendsNoBudget(t *testing.T) {
	t.Parallel()
	const failures = 11 // one more than the per-minute budget
	stub := &stubAccount{respond: failFirst(failures*3, codes.Unavailable)}
	h := startPOV(t, stub)

	for i := range failures {
		rr := postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "k-"+strconv.Itoa(i), `{"amount":100,"origin":"pix"}`)
		if rr.Code != http.StatusBadGateway {
			t.Fatalf("failure %d = %d, want 502", i, rr.Code)
		}
	}
	rr := postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "k-0", `{"amount":100,"origin":"pix"}`)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("after outage = %d %q, want 202", rr.Code, rr.Body.String())
	}
	counters := countersOf(t, h)
	if counters.Actions["deposit"] != 1 || counters.Duplicates != 0 || len(counters.Refusals) != 0 {
		t.Fatalf("counters = %+v, want one deposit and nothing else", counters)
	}
}

// TestDialAccountSim_UncertainFailureSpendsBudget checks that a 502 that may
// have committed in account-sim keeps the budget spent, as in phase 2.
func TestDialAccountSim_UncertainFailureSpendsBudget(t *testing.T) {
	t.Parallel()
	for _, code := range []codes.Code{codes.Internal, codes.DeadlineExceeded} {
		t.Run(code.String(), func(t *testing.T) {
			t.Parallel()
			stub := &stubAccount{respond: failFirst(1<<30, code)}
			h := startPOV(t, stub)

			for i := range 10 {
				rr := postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "k-"+strconv.Itoa(i), `{"amount":100,"origin":"pix"}`)
				if rr.Code != http.StatusBadGateway {
					t.Fatalf("failure %d = %d, want 502", i, rr.Code)
				}
			}
			rr := postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "next", `{"amount":100,"origin":"pix"}`)
			if rr.Code != http.StatusTooManyRequests {
				t.Fatalf("eleventh command = %d, want 429", rr.Code)
			}
			if calls, _ := stub.snapshot(); calls != 10 {
				t.Fatalf("calls = %d, want 10 without retries", calls)
			}
		})
	}
}

// TestDialAccountSim_RetryAfterLostReply covers a first attempt that commits
// but whose reply is lost: the retry answers replay=true, which is still the
// first commit of this request.
func TestDialAccountSim_RetryAfterLostReply(t *testing.T) {
	t.Parallel()
	stub := &stubAccount{
		respond: failFirst(1, codes.Unavailable),
		replay:  func(call int) bool { return call > 1 },
	}
	h := startPOV(t, stub)

	rr := postPOV(t, h, t.Context(), sim.CustomerFernanda, "deposits", "k1", `{"amount":100,"origin":"pix"}`)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d %q, want 202", rr.Code, rr.Body.String())
	}
	if calls, _ := stub.snapshot(); calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	counters := countersOf(t, h)
	if counters.Actions["deposit"] != 1 || counters.Duplicates != 0 {
		t.Fatalf("counters = %+v, want one deposit and no duplicate", counters)
	}
}

// grpcTimeoutSlack covers the transit between the client computing the
// grpc-timeout header and the server turning it back into a deadline.
const grpcTimeoutSlack = 100 * time.Millisecond

func TestDialAccountSim_Deadline(t *testing.T) {
	t.Parallel()

	t.Run("caller deadline reaches the server", func(t *testing.T) {
		t.Parallel()
		stub := &stubAccount{respond: failFirst(0, codes.OK)}
		h := startPOV(t, stub)

		callerDeadline := time.Now().Add(300 * time.Millisecond)
		ctx, cancel := context.WithDeadline(t.Context(), callerDeadline)
		defer cancel()
		if rr := postPOV(t, h, ctx, sim.CustomerFernanda, "deposits", "k1", `{"amount":100,"origin":"pix"}`); rr.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want 202", rr.Code)
		}
		_, deadlines := stub.snapshot()
		if len(deadlines) != 1 || deadlines[0].IsZero() {
			t.Fatalf("server deadlines = %v, want one", deadlines)
		}
		if got := deadlines[0]; got.After(callerDeadline.Add(grpcTimeoutSlack)) {
			t.Fatalf("server deadline %s is later than the caller's %s", got, callerDeadline)
		}
	})

	t.Run("default deadline when the caller has none", func(t *testing.T) {
		t.Parallel()
		stub := &stubAccount{respond: failFirst(0, codes.OK)}
		h := startPOV(t, stub)

		before := time.Now()
		if rr := getPath(t, h, context.WithoutCancel(t.Context()), "/v1/client-pov/customers/"+sim.CustomerFernanda); rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		_, deadlines := stub.snapshot()
		if len(deadlines) != 1 || deadlines[0].IsZero() {
			t.Fatalf("server deadlines = %v, want one", deadlines)
		}
		budget := deadlines[0].Sub(before)
		if budget < time.Second || budget > 2*time.Second+grpcTimeoutSlack {
			t.Fatalf("server deadline is %s after the call, want about 2s", budget)
		}
	})
}
