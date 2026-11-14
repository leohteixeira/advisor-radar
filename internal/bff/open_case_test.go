package bff_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// caseAdvisory is advisory's action port for the Abrir caso command: it
// records each request and answers with status and, on 202, a request id.
type caseAdvisory struct {
	status int

	mu       sync.Mutex
	requests []caseRequest
}

type caseRequest struct {
	path string
	key  string
	body map[string]any
}

func (a *caseAdvisory) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	a.mu.Lock()
	a.requests = append(a.requests, caseRequest{path: r.Method + " " + r.URL.Path, key: r.Header.Get("Idempotency-Key"), body: body})
	a.mu.Unlock()
	if a.status != http.StatusAccepted {
		http.Error(w, http.StatusText(a.status), a.status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"request_id":"req-1"}`))
}

func (a *caseAdvisory) seen() []caseRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]caseRequest(nil), a.requests...)
}

func startCaseAdvisory(t *testing.T, status int) (*caseAdvisory, bff.ActionsClient) {
	t.Helper()
	advisory := &caseAdvisory{status: status}
	srv := httptest.NewServer(advisory)
	t.Cleanup(srv.Close)
	return advisory, bff.NewActionsClient(srv.URL)
}

func postOpenCase(h http.Handler, signalID, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/signals/"+signalID+"/case", nil)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// Abrir caso hands the command to advisory with the signal's client, label
// and SLA facts, and answers 202 with advisory's request id. Nothing is
// opened by the BFF.
func TestHTTP_OpenCase_SendsTheCommandToAdvisory(t *testing.T) {
	t.Parallel()

	cust := identity.MustNewV7()
	seededAlert, seededMessage, liveAlert := identity.MustNewV7(), identity.MustNewV7(), identity.MustNewV7()
	yes := true
	queue := stubQueue{queue: []bff.Signal{
		{ID: seededAlert, Kind: "alert", Client: cust, Alert: "saque", Amount: 190000},
		{ID: seededMessage, Kind: "message", Client: cust, Intent: "Reclamação", Churn: &yes, Human: &yes, Frustration: intPtr(3)},
	}}
	board := bff.NewBoard()
	live := envelopeJSON(t, liveAlert, cust, map[string]any{"kind": "segmento", "rule": "r", "from": "Advance", "to": "Essencial"})
	if err := board.ApplyDelivery(t.Context(), "alert.raised", live); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		signal   string
		wantBody map[string]any
	}{
		{
			name:     "seeded relevant withdrawal",
			signal:   seededAlert,
			wantBody: map[string]any{"customer_id": cust, "label": "Saque relevante", "relevant_withdrawal": true},
		},
		{
			name:   "seeded message with churn risk",
			signal: seededMessage,
			wantBody: map[string]any{
				"customer_id": cust, "label": "Mensagem com risco",
				"churn_risk": true, "human_requested": true, "frustration": float64(3),
			},
		},
		{
			name:     "live segment alert from the board",
			signal:   liveAlert,
			wantBody: map[string]any{"customer_id": cust, "label": "Mudança de segmento"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			advisory, actions := startCaseAdvisory(t, http.StatusAccepted)
			h := bff.NewHandler(board, actions, nil, queue, nil, nil)

			rr := postOpenCase(h, tt.signal, "key-1")
			if rr.Code != http.StatusAccepted {
				t.Fatalf("status = %d %q, want 202", rr.Code, rr.Body.String())
			}
			if got := strings.TrimSpace(rr.Body.String()); got != `{"request_id":"req-1"}` {
				t.Fatalf("body = %s", got)
			}
			seen := advisory.seen()
			if len(seen) != 1 || seen[0].path != "POST /v1/signals/"+tt.signal+"/case" || seen[0].key != "key-1" {
				t.Fatalf("advisory saw %+v, want one POST for %s with key-1", seen, tt.signal)
			}
			got, _ := json.Marshal(seen[0].body)
			want, _ := json.Marshal(tt.wantBody)
			if string(got) != string(want) {
				t.Fatalf("command = %s, want %s", got, want)
			}
		})
	}
}

func TestHTTP_OpenCase_Refusals(t *testing.T) {
	t.Parallel()

	cust, known := identity.MustNewV7(), identity.MustNewV7()
	queue := stubQueue{queue: []bff.Signal{{ID: known, Kind: "alert", Client: cust, Alert: "aporte"}}}

	tests := []struct {
		name        string
		signal      string
		key         string
		advisory    int // 0 leaves the actions port unset
		wantCode    int
		wantCommand bool
	}{
		{name: "signal id is not a uuid", signal: "s01", key: "k", advisory: http.StatusAccepted, wantCode: http.StatusBadRequest},
		{name: "idempotency key is missing", signal: known, key: "", advisory: http.StatusAccepted, wantCode: http.StatusBadRequest},
		{name: "idempotency key is too long", signal: known, key: strings.Repeat("k", 129), advisory: http.StatusAccepted, wantCode: http.StatusBadRequest},
		{name: "signal is not in the queue", signal: identity.MustNewV7(), key: "k", advisory: http.StatusAccepted, wantCode: http.StatusNotFound},
		{name: "advisory actions are not configured", signal: known, key: "k", wantCode: http.StatusServiceUnavailable},
		{name: "advisory has no outbox", signal: known, key: "k", advisory: http.StatusServiceUnavailable, wantCode: http.StatusBadGateway, wantCommand: true},
		{name: "advisory fails", signal: known, key: "k", advisory: http.StatusInternalServerError, wantCode: http.StatusBadGateway, wantCommand: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var advisory *caseAdvisory
			var actions bff.ActionsClient
			if tt.advisory != 0 {
				advisory, actions = startCaseAdvisory(t, tt.advisory)
			}
			h := bff.NewHandler(bff.NewBoard(), actions, nil, queue, nil, nil)

			rr := postOpenCase(h, tt.signal, tt.key)
			if rr.Code != tt.wantCode {
				t.Fatalf("status = %d %q, want %d", rr.Code, rr.Body.String(), tt.wantCode)
			}
			if advisory != nil {
				if got := len(advisory.seen()) == 1; got != tt.wantCommand {
					t.Fatalf("advisory saw %d commands, want command sent = %v", len(advisory.seen()), tt.wantCommand)
				}
			}
		})
	}
}

// A retry of Abrir caso for a signal that already has its case is still the
// same command: the queue no longer lists the signal, and the route must not
// answer 404 in the middle of the browser's flow.
func TestHTTP_OpenCase_SignalACaseAlreadyRepresents(t *testing.T) {
	t.Parallel()

	cust, opened, joined := identity.MustNewV7(), identity.MustNewV7(), identity.MustNewV7()
	queue := stubQueue{queue: []bff.Signal{
		{ID: opened, Kind: "alert", Client: cust, Alert: "aporte"},
		{ID: joined, Kind: "alert", Client: cust, Alert: "queda"},
	}}
	cases := stubCases{items: []bff.Case{{
		ID: identity.MustNewV7(), Client: cust, Signal: opened,
		History: []bff.CaseHistoryEntry{{Kind: "signal", Text: "joined", Signal: joined}},
	}}}

	listed, _ := queueItems(t, bff.NewHandler(bff.NewBoard(), nil, nil, queue, nil, cases))
	if len(listed) != 0 {
		t.Fatalf("queue lists %+v, want no signal: both are in the case", listed)
	}
	for _, signal := range []string{opened, joined} {
		advisory, actions := startCaseAdvisory(t, http.StatusAccepted)
		h := bff.NewHandler(bff.NewBoard(), actions, nil, queue, nil, cases)
		if rr := postOpenCase(h, signal, "key-1"); rr.Code != http.StatusAccepted {
			t.Fatalf("status = %d %q, want 202", rr.Code, rr.Body.String())
		}
		if seen := advisory.seen(); len(seen) != 1 || seen[0].key != "key-1" {
			t.Fatalf("advisory saw %+v, want the command with key-1", seen)
		}
	}
}

// An accepted command without a request id is an upstream failure, not a 202
// the browser cannot follow.
func TestHTTPActionsClient_OpenCaseNeedsARequestID(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	_, err := bff.NewActionsClient(srv.URL).OpenCase(t.Context(), bff.CaseCommand{SignalID: identity.MustNewV7(), IdempotencyKey: "k"})
	if err == nil {
		t.Fatal("OpenCase error = nil, want a failure")
	}
}

// Every new key costs advisory an outbox row, the broker a message and cases
// an inbox row, so the command has a budget: 20 new keys in ten minutes. A
// key already accepted replays for free, and a command advisory did not
// accept gives its slot back.
func TestHTTP_OpenCase_Budget(t *testing.T) {
	t.Parallel()
	cust, signal := identity.MustNewV7(), identity.MustNewV7()
	queue := stubQueue{queue: []bff.Signal{{ID: signal, Kind: "alert", Client: cust, Alert: "saque"}}}

	advisory, actions := startCaseAdvisory(t, http.StatusAccepted)
	h := bff.NewHandler(bff.NewBoard(), actions, nil, queue, nil, nil)
	for i := range 20 {
		if rr := postOpenCase(h, signal, "key-"+strconv.Itoa(i)); rr.Code != http.StatusAccepted {
			t.Fatalf("command %d = %d, want 202", i, rr.Code)
		}
	}
	rr := postOpenCase(h, signal, "key-over")
	if rr.Code != http.StatusTooManyRequests || strings.TrimSpace(rr.Body.String()) != `{"error":"ten_minutes"}` {
		t.Fatalf("command over the budget = %d %q, want 429 ten_minutes", rr.Code, rr.Body.String())
	}
	if rr := postOpenCase(h, signal, "key-3"); rr.Code != http.StatusAccepted {
		t.Fatalf("replay of an accepted key = %d, want 202", rr.Code)
	}
	if got := len(advisory.seen()); got != 21 {
		t.Fatalf("advisory saw %d commands, want the 20 accepted and the replay", got)
	}

	// Refused commands spend nothing: after 25 of them the budget is whole.
	down, downActions := startCaseAdvisory(t, http.StatusInternalServerError)
	h = bff.NewHandler(bff.NewBoard(), downActions, nil, queue, nil, nil)
	for i := range 25 {
		if rr := postOpenCase(h, signal, "key-"+strconv.Itoa(i)); rr.Code != http.StatusBadGateway {
			t.Fatalf("failed command %d = %d, want 502", i, rr.Code)
		}
	}
	if got := len(down.seen()); got != 25 {
		t.Fatalf("advisory saw %d failed commands, want 25", got)
	}
}
