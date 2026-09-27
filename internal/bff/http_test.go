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
	h := bff.NewHandler(bff.NewBoard())
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
	h := bff.NewHandler(board)

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
	h := bff.NewHandler(board)
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
	h := bff.NewHandler(board)

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
