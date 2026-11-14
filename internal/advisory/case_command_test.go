package advisory_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/advisory"
	"github.com/leohteixeira/advisor-radar/internal/event"
)

const (
	commandSignal   = "01a0e3a4-9a44-76a2-b210-d5a1e4624b33"
	commandCustomer = "01a0e3a4-9a44-75c7-91c3-f5c3250f406c"
)

var commandAt = time.Date(2026, 9, 29, 15, 0, 0, 0, time.UTC)

func (s *memStore) outboxRows() []advisory.OutboxRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]advisory.OutboxRow, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.outbox[id].row)
	}
	return out
}

func postCase(h http.Handler, signalID, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/signals/"+signalID+"/case", strings.NewReader(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func requestID(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.RequestID == "" {
		t.Fatalf("response body = %q (%v), want a request_id", rr.Body.String(), err)
	}
	return body.RequestID
}

func TestCaseCommandHTTP_AcceptsIntoTheOutbox(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	h := advisory.NewActionsHandler(
		advisory.NewActions(advisory.NewMemoryActionStore(), nil),
		advisory.NewCaseCommands(store, func() time.Time { return commandAt }),
	)
	body := `{"customer_id":"` + commandCustomer + `","label":"Saque relevante","relevant_withdrawal":true,"frustration":1}`

	rr := postCase(h, commandSignal, "key-1", body)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (%s)", rr.Code, rr.Body.String())
	}
	id := requestID(t, rr)

	rows := store.outboxRows()
	if len(rows) != 1 || rows[0].EventID != id || rows[0].RoutingKey != event.NameCaseOpenRequested {
		t.Fatalf("outbox = %+v, want one case.open.requested row with id %s", rows, id)
	}
	// cases decodes these literal keys; keep them in sync.
	var got map[string]any
	if err := json.Unmarshal(rows[0].Payload, &got); err != nil {
		t.Fatalf("unmarshal outbox payload: %v", err)
	}
	want := map[string]any{
		"event_id":       id,
		"occurred_at":    "2026-09-29T15:00:00Z",
		"customer_id":    commandCustomer,
		"schema_version": float64(1),
		"payload": map[string]any{
			"signal_id":           commandSignal,
			"label":               "Saque relevante",
			"frustration":         float64(1),
			"relevant_withdrawal": true,
		},
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("event body = %s, want %s", gotJSON, wantJSON)
	}

	// The same request again is the same command: same id, nothing new.
	again := postCase(h, commandSignal, "key-1", body)
	if again.Code != http.StatusAccepted || requestID(t, again) != id {
		t.Fatalf("retry = %d %s, want 202 with id %s", again.Code, again.Body.String(), id)
	}
	if n := store.outboxCount(); n != 1 {
		t.Fatalf("outbox rows after a retry = %d, want 1", n)
	}

	// Another key, or another signal, is another command.
	other := postCase(h, commandSignal, "key-2", body)
	if other.Code != http.StatusAccepted || requestID(t, other) == id {
		t.Fatalf("second key = %d %s, want 202 with a new id", other.Code, other.Body.String())
	}
	if n := store.outboxCount(); n != 2 {
		t.Fatalf("outbox rows = %d, want 2", n)
	}
}

func TestCaseCommandHTTP_Refusals(t *testing.T) {
	t.Parallel()

	valid := `{"customer_id":"` + commandCustomer + `"}`
	tests := []struct {
		name   string
		signal string
		key    string
		body   string
	}{
		{name: "signal id is not a uuid", signal: "s01", key: "k", body: valid},
		{name: "idempotency key is missing", signal: commandSignal, key: "", body: valid},
		{name: "idempotency key is too long", signal: commandSignal, key: strings.Repeat("k", 129), body: valid},
		{name: "customer id is missing", signal: commandSignal, key: "k", body: `{}`},
		{name: "customer id is not a uuid", signal: commandSignal, key: "k", body: `{"customer_id":"c1"}`},
		{name: "label is too long", signal: commandSignal, key: "k", body: `{"customer_id":"` + commandCustomer + `","label":"` + strings.Repeat("á", 121) + `"}`},
		{name: "frustration is negative", signal: commandSignal, key: "k", body: `{"customer_id":"` + commandCustomer + `","frustration":-1}`},
		{name: "body is not json", signal: commandSignal, key: "k", body: `customer`},
		{name: "body is too large", signal: commandSignal, key: "k", body: `{"customer_id":"` + commandCustomer + `","label":"` + strings.Repeat("a", 5000) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			h := advisory.NewActionsHandler(
				advisory.NewActions(advisory.NewMemoryActionStore(), nil),
				advisory.NewCaseCommands(store, nil),
			)
			rr := postCase(h, tt.signal, tt.key, tt.body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rr.Code)
			}
			if n := store.outboxCount(); n != 0 {
				t.Fatalf("outbox rows = %d, want 0", n)
			}
		})
	}
}

func TestCaseCommandHTTP_WithoutAnOutbox(t *testing.T) {
	t.Parallel()

	h := advisory.NewActionsHandler(advisory.NewActions(advisory.NewMemoryActionStore(), nil), nil)
	rr := postCase(h, commandSignal, "k", `{"customer_id":"`+commandCustomer+`"}`)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}

func TestCaseCommands_StoreFailure(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	store.failIns = "outbox"
	commands := advisory.NewCaseCommands(store, nil)
	_, err := commands.Request(t.Context(), advisory.CaseCommand{
		SignalID: commandSignal, CustomerID: commandCustomer, IdempotencyKey: "k",
	})
	if err == nil || errors.Is(err, advisory.ErrInvalidCaseCommand) {
		t.Fatalf("error = %v, want a store failure", err)
	}

	h := advisory.NewActionsHandler(advisory.NewActions(advisory.NewMemoryActionStore(), nil), commands)
	rr := postCase(h, commandSignal, "k", `{"customer_id":"`+commandCustomer+`"}`)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}
