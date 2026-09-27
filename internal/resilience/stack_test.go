package resilience_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/leohteixeira/advisor-radar/internal/jev"
	"github.com/leohteixeira/advisor-radar/internal/resilience"
	"github.com/leohteixeira/advisor-radar/internal/triage"
)

func jevOK(intent string, prob float64) []byte {
	body, _ := json.Marshal(map[string]any{
		"model": "typesafe-ai/jev",
		"answers": map[string]any{
			"intent": map[string]any{
				"type":          "choice",
				"choice":        intent,
				"probabilities": map[string]float64{intent: prob},
			},
			"frustration": map[string]any{"type": "score", "score": 0.0},
			"churn_risk":  map[string]any{"type": "boolean", "probability": 0.1},
			"wants_human": map[string]any{"type": "boolean", "probability": 0.1},
		},
	})
	return body
}

func newStack(t *testing.T, url string, cfg resilience.Config) *resilience.Stack {
	t.Helper()
	if cfg.Limiter == nil {
		cfg.Limiter = rate.NewLimiter(rate.Inf, 1)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	primary := triage.NewJevClassifier(jev.New("k", jev.WithBaseURL(url), jev.WithZeroDataRetention()))
	return resilience.New(primary, triage.HeuristicClassifier{}, cfg)
}

func TestStack_RetryAfter429(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	var secondAt time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"slow","error_type":"rate_limited"}`))
			return
		}
		secondAt = time.Now()
		_, _ = w.Write(jevOK("cambio", 0.9))
	}))
	defer srv.Close()

	first := time.Now()
	stack := newStack(t, srv.URL, resilience.Config{
		MaxAttempts:    3,
		BaseBackoff:    time.Millisecond,
		AttemptTimeout: 5 * time.Second,
		OpenTimeout:    time.Minute,
	})
	r, err := stack.Classify(context.Background(), triage.Message{ID: "1", Text: "remessa de cambio"})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
	if secondAt.Sub(first) < time.Second {
		t.Fatalf("second attempt waited %v, want >= 1s", secondAt.Sub(first))
	}
	if r.Classifier != "jev" || r.Degraded {
		t.Fatalf("result = %+v", r)
	}
}

func TestStack_Quota402OpensBreaker(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"message":"budget","error_type":"quota_for_entity_exceeded"}`))
	}))
	defer srv.Close()

	stack := newStack(t, srv.URL, resilience.Config{
		MaxAttempts:    1,
		AttemptTimeout: time.Second,
		OpenTimeout:    time.Minute,
	})
	r, err := stack.Classify(context.Background(), triage.Message{ID: "1", Text: "remessa"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Degraded || r.Classifier != "heuristic" {
		t.Fatalf("result = %+v", r)
	}
	before := calls.Load()
	r2, err := stack.Classify(context.Background(), triage.Message{ID: "2", Text: "remessa"})
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Degraded {
		t.Fatalf("result = %+v", r2)
	}
	if calls.Load() != before {
		t.Fatalf("calls while open = %d, want %d", calls.Load(), before)
	}
}

func TestStack_ThreeConsecutive5xxOpensBreaker(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom","error_type":"server_error"}`))
	}))
	defer srv.Close()

	stack := newStack(t, srv.URL, resilience.Config{
		MaxAttempts:    1,
		AttemptTimeout: time.Second,
		OpenTimeout:    time.Minute,
	})
	msg := triage.Message{Text: "remessa de cambio"}
	for i := 0; i < 3; i++ {
		r, err := stack.Classify(context.Background(), msg)
		if err != nil {
			t.Fatal(err)
		}
		if !r.Degraded {
			t.Fatalf("call %d not degraded: %+v", i, r)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
	before := calls.Load()
	r, err := stack.Classify(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Degraded {
		t.Fatalf("result = %+v", r)
	}
	if calls.Load() != before {
		t.Fatalf("breaker should be open; calls = %d", calls.Load())
	}
}

func TestStack_FiveFailuresInTenOpenBreaker(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		// Pattern for 10 outcomes: fail, ok, fail, ok, fail, ok, fail, ok, fail, then 5th fail opens.
		// Simpler: fail every other until we have 5 fails with successes in between so consecutive < 3.
		if n%2 == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom","error_type":"server_error"}`))
			return
		}
		_, _ = w.Write(jevOK("cambio", 0.9))
	}))
	defer srv.Close()

	stack := newStack(t, srv.URL, resilience.Config{
		MaxAttempts:    1,
		AttemptTimeout: time.Second,
		OpenTimeout:    time.Minute,
	})
	msg := triage.Message{Text: "remessa de cambio"}
	// Outcomes: F S F S F S F S F → 5 failures in 9; consecutive never reaches 3.
	// The fifth failure trips ReadyToTrip and opens the breaker.
	for i := 0; i < 9; i++ {
		if _, err := stack.Classify(context.Background(), msg); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 9 {
		t.Fatalf("calls = %d, want 9", calls.Load())
	}
	before := calls.Load()
	r, err := stack.Classify(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Degraded {
		t.Fatalf("open breaker should degrade: %+v", r)
	}
	if calls.Load() != before {
		t.Fatalf("breaker should be open after 5 failures in last 10; calls grew to %d", calls.Load())
	}
}

func TestStack_InvalidRequestDoesNotOpenBreaker(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad","error_type":"invalid_request"}`))
	}))
	defer srv.Close()

	stack := newStack(t, srv.URL, resilience.Config{
		MaxAttempts:    1,
		AttemptTimeout: time.Second,
		OpenTimeout:    time.Minute,
	})
	msg := triage.Message{Text: "remessa"}
	for i := 0; i < 5; i++ {
		r, err := stack.Classify(context.Background(), msg)
		if err != nil {
			t.Fatal(err)
		}
		if !r.Degraded {
			t.Fatalf("call %d: %+v", i, r)
		}
	}
	if calls.Load() != 5 {
		t.Fatalf("calls = %d, want 5 (breaker stayed closed)", calls.Load())
	}
}

func TestStack_HalfOpenTrialCloses(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom","error_type":"server_error"}`))
			return
		}
		_, _ = w.Write(jevOK("cambio", 0.95))
	}))
	defer srv.Close()

	stack := newStack(t, srv.URL, resilience.Config{
		MaxAttempts:    1,
		AttemptTimeout: time.Second,
		OpenTimeout:    50 * time.Millisecond,
	})
	msg := triage.Message{Text: "remessa de cambio"}
	for i := 0; i < 3; i++ {
		r, err := stack.Classify(context.Background(), msg)
		if err != nil {
			t.Fatal(err)
		}
		if !r.Degraded {
			t.Fatalf("call %d: %+v", i, r)
		}
	}
	time.Sleep(60 * time.Millisecond)
	r, err := stack.Classify(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if r.Classifier != "jev" || r.Degraded {
		t.Fatalf("half-open trial = %+v", r)
	}
	// Closed: another call still hits HTTP.
	before := calls.Load()
	r2, err := stack.Classify(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Classifier != "jev" || calls.Load() != before+1 {
		t.Fatalf("breaker should be closed; result=%+v calls=%d", r2, calls.Load())
	}
}

func TestStack_CallerCancelSkipsFallback(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	stack := newStack(t, srv.URL, resilience.Config{
		MaxAttempts:    1,
		AttemptTimeout: 2 * time.Second,
		OpenTimeout:    time.Minute,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := stack.Classify(ctx, triage.Message{Text: "remessa"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestStack_DeadlineExhaustedDuringBackoff(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"slow","error_type":"rate_limited"}`))
	}))
	defer srv.Close()

	stack := newStack(t, srv.URL, resilience.Config{
		MaxAttempts:    3,
		BaseBackoff:    time.Millisecond,
		AttemptTimeout: 2 * time.Second,
		OpenTimeout:    time.Minute,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	r, err := stack.Classify(ctx, triage.Message{Text: "remessa de cambio"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Degraded || r.Classifier != "heuristic" {
		t.Fatalf("result = %+v", r)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (no further HTTP after deadline backoff)", calls.Load())
	}
}
