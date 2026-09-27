// Package resilience wraps a primary classifier with the decorator order from
// architecture.md: Fallback, bulkhead, circuit breaker, retry, rate limiter,
// and a per-attempt timeout.
package resilience

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sony/gobreaker/v2"
	"golang.org/x/time/rate"

	"github.com/leohteixeira/advisor-radar/internal/jev"
	"github.com/leohteixeira/advisor-radar/internal/triage"
)

const (
	defaultAttemptTimeout = 2 * time.Second
	defaultOpenTimeout    = 15 * time.Second
	defaultMaxAttempts    = 3
	defaultBaseBackoff    = 150 * time.Millisecond
	defaultBulkhead       = 10
)

// Config tunes the resilience stack. Zero values pick production defaults.
type Config struct {
	AttemptTimeout time.Duration
	OpenTimeout    time.Duration
	MaxAttempts    int
	BaseBackoff    time.Duration
	Bulkhead       int
	Limiter        *rate.Limiter
	OnDegrade      func(m triage.Message, err error)
	Logger         *slog.Logger
}

// Stack is a triage.Classifier that applies the resilience layers outside-in.
type Stack struct {
	inner triage.Classifier
}

// New builds Fallback → bulkhead → breaker → retry → limiter → timeout(primary).
func New(primary, secondary triage.Classifier, cfg Config) *Stack {
	if cfg.AttemptTimeout <= 0 {
		cfg.AttemptTimeout = defaultAttemptTimeout
	}
	if cfg.OpenTimeout <= 0 {
		cfg.OpenTimeout = defaultOpenTimeout
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultMaxAttempts
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = defaultBaseBackoff
	}
	if cfg.Bulkhead <= 0 {
		cfg.Bulkhead = defaultBulkhead
	}
	if cfg.Limiter == nil {
		cfg.Limiter = rate.NewLimiter(rate.Inf, 1)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	var forceOpen atomic.Bool
	outcomes := &outcomeWindow{}

	cb := gobreaker.NewCircuitBreaker[triage.Result](gobreaker.Settings{
		Name:        "jev",
		MaxRequests: 1,
		Timeout:     cfg.OpenTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if forceOpen.Load() {
				return true
			}
			if counts.ConsecutiveFailures >= 3 {
				return true
			}
			return outcomes.failuresInLast(10) >= 5
		},
		IsSuccessful: func(err error) bool { return err == nil },
		IsExcluded: func(err error) bool {
			if err == nil {
				return false
			}
			var apiErr *jev.APIError
			if errors.As(err, &apiErr) && apiErr.IsInvalidRequest() {
				return true
			}
			// Mapping and other non-provider errors fall back without moving the breaker.
			return !isProviderFailure(err)
		},
	})

	timed := &timeoutClassifier{inner: primary, timeout: cfg.AttemptTimeout}
	limited := &limiterClassifier{inner: timed, limiter: cfg.Limiter}
	retried := &retryClassifier{inner: limited, maxAttempts: cfg.MaxAttempts, baseBackoff: cfg.BaseBackoff}
	broken := &breakerClassifier{
		inner:     retried,
		cb:        cb,
		forceOpen: &forceOpen,
		outcomes:  outcomes,
		logger:    cfg.Logger,
	}
	bulk := &bulkheadClassifier{inner: broken, sem: make(chan struct{}, cfg.Bulkhead)}

	return &Stack{
		inner: triage.Fallback{
			Primary:   bulk,
			Secondary: secondary,
			OnDegrade: cfg.OnDegrade,
		},
	}
}

// Classify runs the full resilience stack.
func (s *Stack) Classify(ctx context.Context, m triage.Message) (triage.Result, error) {
	return s.inner.Classify(ctx, m)
}

type timeoutClassifier struct {
	inner   triage.Classifier
	timeout time.Duration
}

func (t *timeoutClassifier) Classify(ctx context.Context, m triage.Message) (triage.Result, error) {
	pctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	return t.inner.Classify(pctx, m)
}

type limiterClassifier struct {
	inner   triage.Classifier
	limiter *rate.Limiter
}

func (l *limiterClassifier) Classify(ctx context.Context, m triage.Message) (triage.Result, error) {
	if err := l.limiter.Wait(ctx); err != nil {
		return triage.Result{}, err
	}
	return l.inner.Classify(ctx, m)
}

type retryClassifier struct {
	inner       triage.Classifier
	maxAttempts int
	baseBackoff time.Duration
}

func (r *retryClassifier) Classify(ctx context.Context, m triage.Message) (triage.Result, error) {
	var lastErr error
	for attempt := 0; attempt < r.maxAttempts; attempt++ {
		res, err := r.inner.Classify(ctx, m)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return triage.Result{}, ctx.Err()
		}
		if !isRetryable(err) {
			return triage.Result{}, err
		}
		if attempt+1 >= r.maxAttempts {
			break
		}
		wait := r.waitDuration(err, attempt)
		if err := sleep(ctx, wait); err != nil {
			// Deadline would pass or caller canceled: stop and fall back (or cancel).
			if errors.Is(err, context.Canceled) {
				return triage.Result{}, err
			}
			return triage.Result{}, lastErr
		}
	}
	return triage.Result{}, lastErr
}

func (r *retryClassifier) waitDuration(err error, attemptIndex int) time.Duration {
	var apiErr *jev.APIError
	if errors.As(err, &apiErr) && apiErr.Status == 429 && apiErr.RetryAfter > 0 {
		return apiErr.RetryAfter
	}
	backoff := r.baseBackoff << attemptIndex
	jitter := time.Duration(rand.Int64N(int64(backoff)/2 + 1))
	return backoff + jitter
}

type breakerClassifier struct {
	inner     triage.Classifier
	cb        *gobreaker.CircuitBreaker[triage.Result]
	forceOpen *atomic.Bool
	outcomes  *outcomeWindow
	logger    *slog.Logger
}

func (b *breakerClassifier) Classify(ctx context.Context, m triage.Message) (triage.Result, error) {
	res, err := b.cb.Execute(func() (triage.Result, error) {
		out, callErr := b.inner.Classify(ctx, m)
		if callErr != nil {
			var apiErr *jev.APIError
			if errors.As(callErr, &apiErr) {
				if apiErr.IsQuotaExceeded() {
					b.forceOpen.Store(true)
				}
				if apiErr.IsInvalidRequest() {
					b.logger.Error("jev invalid_request", "status", apiErr.Status)
				}
			}
			// Record before ReadyToTrip runs so the failure-rate window includes this call.
			if isProviderFailure(callErr) {
				b.outcomes.record(false)
			}
			return out, callErr
		}
		b.forceOpen.Store(false)
		b.outcomes.record(true)
		return out, nil
	})
	return res, err
}

type bulkheadClassifier struct {
	inner triage.Classifier
	sem   chan struct{}
}

func (b *bulkheadClassifier) Classify(ctx context.Context, m triage.Message) (triage.Result, error) {
	select {
	case b.sem <- struct{}{}:
		defer func() { <-b.sem }()
	case <-ctx.Done():
		return triage.Result{}, ctx.Err()
	}
	return b.inner.Classify(ctx, m)
}

type outcomeWindow struct {
	mu   sync.Mutex
	ring []bool
}

func (w *outcomeWindow) record(success bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ring = append(w.ring, success)
	if len(w.ring) > 10 {
		w.ring = w.ring[len(w.ring)-10:]
	}
}

func (w *outcomeWindow) failuresInLast(n int) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if n > len(w.ring) {
		n = len(w.ring)
	}
	start := len(w.ring) - n
	fails := 0
	for _, ok := range w.ring[start:] {
		if !ok {
			fails++
		}
	}
	return fails
}

func isRetryable(err error) bool {
	var apiErr *jev.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Status == 429 || apiErr.Status >= 500 {
			return true
		}
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func isProviderFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr *jev.APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsProviderFailure()
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) < d {
			return context.DeadlineExceeded
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
