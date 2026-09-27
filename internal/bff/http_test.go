package bff_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/bff"
)

// flushRecorder is an httptest recorder safe for concurrent SSE writes/reads.
type flushRecorder struct {
	mu      sync.Mutex
	headers http.Header
	code    int
	ct      string
	body    strings.Builder
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{headers: make(http.Header), code: 200}
}

func (r *flushRecorder) Header() http.Header { return r.headers }

func (r *flushRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(p)
}

func (r *flushRecorder) WriteHeader(statusCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.code = statusCode
	r.ct = r.headers.Get("Content-Type")
}

func (r *flushRecorder) Flush() {}

func (r *flushRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.String()
}

func (r *flushRecorder) contentType() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ct
}

func TestHTTP_QueueJSON(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/queue", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type = %q", ct)
	}
	var body struct {
		Items []bff.Signal `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 17 {
		t.Fatalf("items = %d, want 17", len(body.Items))
	}
	if body.Items[0].ID != "s01" || body.Items[16].ID != "s17" {
		t.Fatalf("ids = %q..%q, want s01..s17", body.Items[0].ID, body.Items[16].ID)
	}
}

func TestHTTP_SSEEventNameAndCatchUp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	board := bff.NewBoard()
	h := bff.NewHandler(board, nil)

	first := envelopeJSON(t, "ev-first", "c19", map[string]any{"kind": "saque", "rule": "r"})
	second := envelopeJSON(t, "ev-second", "c19", map[string]any{"kind": "queda", "rule": "r"})
	if err := board.ApplyDelivery(ctx, "alert.raised", first); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := board.ApplyDelivery(ctx, "alert.raised", second); err != nil {
		t.Fatalf("second: %v", err)
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(reqCtx, http.MethodGet, "/v1/queue/stream", nil)
	req.Header.Set("Last-Event-ID", "ev-first")
	rr := newFlushRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(rr, req)
	}()

	deadline := time.Now().Add(2 * time.Second)
	var got string
	for {
		got = rr.String()
		if strings.Contains(got, "event: signal") && strings.Contains(got, "ev-second") {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("stream body missing catch-up; got %q", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Contains(got, "id: ev-first") {
		t.Fatalf("stream resent cursor event: %q", got)
	}
	if !strings.Contains(got, "id: ev-second") {
		t.Fatalf("missing later event: %q", got)
	}
	if ct := rr.contentType(); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	cancel()
	<-done
}

func TestHTTP_UnknownCursor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	board := bff.NewBoard()
	h := bff.NewHandler(board, nil)
	body := envelopeJSON(t, "ev-live", "c19", map[string]any{"kind": "saque", "rule": "r"})
	if err := board.ApplyDelivery(ctx, "alert.raised", body); err != nil {
		t.Fatalf("apply: %v", err)
	}

	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(reqCtx, http.MethodGet, "/v1/queue/stream", nil)
	req.Header.Set("Last-Event-ID", "not-in-ring")
	rr := newFlushRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(rr, req)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		got := rr.String()
		if strings.Contains(got, "id: ev-live") && strings.Contains(got, "event: signal") {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("unknown cursor did not replay ring; body=%q", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ct := rr.contentType(); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	cancel()
	<-done
}

func TestHTTP_SubscriberCancel(t *testing.T) {
	t.Parallel()
	board := bff.NewBoard()
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		errCh <- board.Stream(ctx, "", func(bff.StreamEvent) error { return nil })
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not end")
	}

	ch, unsub := board.Subscribe("")
	defer unsub()
	body := envelopeJSON(t, "ev-after-cancel", "c19", map[string]any{"kind": "saque", "rule": "r"})
	if err := board.ApplyDelivery(context.Background(), "alert.raised", body); err != nil {
		t.Fatalf("apply: %v", err)
	}
	select {
	case ev := <-ch:
		if ev.ID != "ev-after-cancel" {
			t.Fatalf("id = %q", ev.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("other subscriber got nothing")
	}
}

func TestHTTP_LiveSSEThenQueue(t *testing.T) {
	t.Parallel()
	board := bff.NewBoard()
	h := bff.NewHandler(board, nil)

	reqCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(reqCtx, http.MethodGet, "/v1/queue/stream", nil)
	rr := newFlushRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(rr, req)
	}()
	time.Sleep(20 * time.Millisecond)

	body := envelopeJSON(t, "al-md-n02-withdrawal", "c19", map[string]any{
		"kind": "saque",
		"rule": "Saque acima de 20% do patrimônio em 24 horas",
	})
	if err := board.ApplyDelivery(context.Background(), "alert.raised", body); err != nil {
		t.Fatalf("apply: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		got := rr.String()
		if strings.Contains(got, "event: signal") && strings.Contains(got, "al-md-n02-withdrawal") {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("no live sse; body=%q", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ct := rr.contentType(); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	cancel()
	<-done

	qreq := httptest.NewRequest(http.MethodGet, "/v1/queue", nil)
	qrr := httptest.NewRecorder()
	h.ServeHTTP(qrr, qreq)
	raw, _ := io.ReadAll(qrr.Body)
	if !strings.Contains(string(raw), "al-md-n02-withdrawal") {
		t.Fatalf("queue missing live signal: %s", raw)
	}
}

type fakeActions struct {
	rows     map[string]bff.SignalAction
	err      error
	contacts []string
	snoozes  []string
	undos    []string
}

func (f *fakeActions) List(context.Context) ([]bff.SignalAction, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]bff.SignalAction, 0, len(f.rows))
	for _, row := range f.rows {
		out = append(out, row)
	}
	return out, nil
}

func (f *fakeActions) Contact(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.contacts = append(f.contacts, id)
	return nil
}

func (f *fakeActions) Snooze(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.snoozes = append(f.snoozes, id)
	return nil
}

func (f *fakeActions) Undo(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	f.undos = append(f.undos, id)
	return nil
}

func TestHTTP_SnoozeHidden(t *testing.T) {
	t.Parallel()
	until := time.Now().UTC().Add(time.Hour)
	client := &fakeActions{rows: map[string]bff.SignalAction{
		"s01": {SignalID: "s01", SnoozedUntil: &until},
	}}
	handler := bff.NewHandler(bff.NewBoard(), client)
	req := httptest.NewRequest(http.MethodGet, "/v1/queue", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Items []bff.Signal `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, item := range body.Items {
		if item.ID == "s01" {
			t.Fatal("s01 should be hidden while snoozed")
		}
	}
	if len(body.Items) != 16 {
		t.Fatalf("items = %d, want 16", len(body.Items))
	}
}

func TestHTTP_ActionsUnavailable503(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil)
	req := httptest.NewRequest(http.MethodPut, "/v1/actions/s02", strings.NewReader(`{"action":"contact"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}

	qreq := httptest.NewRequest(http.MethodGet, "/v1/queue", nil)
	qrr := httptest.NewRecorder()
	h.ServeHTTP(qrr, qreq)
	if qrr.Code != http.StatusOK {
		t.Fatalf("queue status = %d", qrr.Code)
	}
	var body struct {
		Items []bff.Signal `json:"items"`
	}
	if err := json.Unmarshal(qrr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 17 {
		t.Fatalf("items = %d, want 17", len(body.Items))
	}
}

func TestHTTP_CasesInColumns(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/cases", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Items  []bff.Case `json:"items"`
		States []string   `json:"states"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.States) != 4 {
		t.Fatalf("states = %v", body.States)
	}
	byID := map[string]bff.Case{}
	for _, c := range body.Items {
		byID[c.ID] = c
	}
	if byID["k1042"].State != 1 {
		t.Fatalf("k1042 state = %d, want Em atendimento (1)", byID["k1042"].State)
	}
	if byID["k1038"].State != 2 {
		t.Fatalf("k1038 state = %d, want Aguardando cliente (2)", byID["k1038"].State)
	}
	if byID["k1031"].State != 3 {
		t.Fatalf("k1031 state = %d, want Resolvido (3)", byID["k1031"].State)
	}
	column := func(state int) string {
		if state < 0 || state >= len(body.States) {
			return ""
		}
		return body.States[state]
	}
	if column(byID["k1042"].State) != "Em atendimento" {
		t.Fatalf("k1042 column = %q", column(byID["k1042"].State))
	}
	if column(byID["k1038"].State) != "Aguardando cliente" {
		t.Fatalf("k1038 column = %q", column(byID["k1038"].State))
	}
	if column(byID["k1031"].State) != "Resolvido" {
		t.Fatalf("k1031 column = %q", column(byID["k1031"].State))
	}
}

func TestHTTP_ContactedAtOnQueue(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 27, 15, 0, 0, 0, time.UTC)
	client := &fakeActions{rows: map[string]bff.SignalAction{
		"s02": {SignalID: "s02", ContactedAt: &at},
	}}
	h := bff.NewHandler(bff.NewBoard(), client)
	req := httptest.NewRequest(http.MethodGet, "/v1/queue", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Items []bff.Signal `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found *bff.Signal
	for i := range body.Items {
		if body.Items[i].ID == "s02" {
			found = &body.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatal("s02 missing from queue")
	}
	if found.ContactedAt == nil || !found.ContactedAt.Equal(at) {
		t.Fatalf("contacted_at = %v, want %v", found.ContactedAt, at)
	}
}

func TestHTTP_ExpiredSnoozeVisible(t *testing.T) {
	t.Parallel()
	past := time.Now().UTC().Add(-time.Hour)
	client := &fakeActions{rows: map[string]bff.SignalAction{
		"s01": {SignalID: "s01", SnoozedUntil: &past},
	}}
	h := bff.NewHandler(bff.NewBoard(), client)
	req := httptest.NewRequest(http.MethodGet, "/v1/queue", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Items []bff.Signal `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, item := range body.Items {
		if item.ID == "s01" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("s01 should remain after expired snooze")
	}
	if len(body.Items) != 17 {
		t.Fatalf("items = %d, want 17", len(body.Items))
	}
}

func TestHTTP_ActionProxyRecordsCalls(t *testing.T) {
	t.Parallel()
	client := &fakeActions{rows: map[string]bff.SignalAction{}}
	h := bff.NewHandler(bff.NewBoard(), client)

	put := func(id, action string) int {
		req := httptest.NewRequest(http.MethodPut, "/v1/actions/"+id, strings.NewReader(`{"action":"`+action+`"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	if code := put("s02", "contact"); code != http.StatusNoContent {
		t.Fatalf("contact status = %d", code)
	}
	if len(client.contacts) != 1 || client.contacts[0] != "s02" {
		t.Fatalf("contacts = %v", client.contacts)
	}

	if code := put("s01", "snooze"); code != http.StatusNoContent {
		t.Fatalf("snooze status = %d", code)
	}
	if len(client.snoozes) != 1 || client.snoozes[0] != "s01" {
		t.Fatalf("snoozes = %v", client.snoozes)
	}

	del := httptest.NewRequest(http.MethodDelete, "/v1/actions/s02", nil)
	delRR := httptest.NewRecorder()
	h.ServeHTTP(delRR, del)
	if delRR.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", delRR.Code)
	}
	if len(client.undos) != 1 || client.undos[0] != "s02" {
		t.Fatalf("undos = %v", client.undos)
	}
}
