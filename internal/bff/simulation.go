package bff

import (
	"context"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// The advance-day budget is global, separate from the per-customer command
// budget: 20 advances in any 10 minutes, whoever sends them.
const (
	advancePerWindow = 20
	advanceWindow    = 10 * time.Minute
	// advanceWindowName is the error of a 429, in the phase-2 {"error":
	// window} shape.
	advanceWindowName = "ten_minutes"
)

// AdvanceResult is the account-sim reply to one advance-day command:
// the simulated day after it and the reavaliacao event ids, one per account.
// Retried reports that the command was sent more than once in this request,
// so a Replay may be the answer to this request's own earlier, lost attempt.
type AdvanceResult struct {
	SimDay   int
	EventIDs []string
	Replay   bool
	Retried  bool
}

// SimulationSource is the optional account-sim port of the global simulated
// day. The account-sim POVSource implements it; without it the simulation
// routes answer 502.
type SimulationSource interface {
	// AdvanceDay advances the global simulated day by one, idempotent by
	// key. commandID only correlates logs.
	AdvanceDay(ctx context.Context, idempotencyKey, commandID string) (AdvanceResult, error)
	// SimDay reads the current simulated day.
	SimDay(ctx context.Context) (int, error)
}

// advanceLimiter is the global advance-day budget. A key that was answered
// once is free afterwards, so a replay never spends.
type advanceLimiter struct {
	mu    sync.Mutex
	hits  []time.Time
	known map[string]struct{}
}

func newAdvanceLimiter() *advanceLimiter {
	return &advanceLimiter{known: map[string]struct{}{}}
}

// allow spends one advance at now unless the window is full. spend settles
// the attempt: refund returns the advance, and remember makes key free from
// then on.
func (l *advanceLimiter) allow(key string, now time.Time) (ok bool, spend func(refund, remember bool)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, seen := l.known[key]; seen {
		return true, func(bool, bool) {}
	}
	cutoff := now.Add(-advanceWindow)
	kept := l.hits[:0]
	for _, at := range l.hits {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	l.hits = kept
	if len(kept) >= advancePerWindow {
		return false, nil
	}
	l.hits = append(l.hits, now)
	spent := true
	return true, func(refund, remember bool) {
		l.mu.Lock()
		defer l.mu.Unlock()
		if remember {
			l.known[key] = struct{}{}
		}
		if refund && spent {
			if i := lastIndex(l.hits, now); i >= 0 {
				l.hits = append(l.hits[:i], l.hits[i+1:]...)
			}
			spent = false
		}
	}
}

// lastIndex is the index of the last hit equal to at, or −1.
func lastIndex(hits []time.Time, at time.Time) int {
	for i := len(hits) - 1; i >= 0; i-- {
		if hits[i].Equal(at) {
			return i
		}
	}
	return -1
}

// advanceDayReply is the 202 body of POST /v1/client-pov/simulation/advance-day.
// EventID is the first reavaliacao event id, empty when there is no account.
type advanceDayReply struct {
	SimDay  int    `json:"sim_day"`
	EventID string `json:"event_id"`
}

// simulationReply is the GET /v1/client-pov/simulation body.
type simulationReply struct {
	SimDay int `json:"sim_day"`
}

// postAdvanceDay serves POST /v1/client-pov/simulation/advance-day. It needs
// an Idempotency-Key (400 without), spends the global budget (429 when it is
// spent; a known key is free), and answers 202 with the new day, or 502 when
// account-sim fails. Only Unavailable, left after the client retries, means
// the command never reached account-sim, so only it refunds the budget.
func (h *Handler) postAdvanceDay(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	src, ok := h.pov.(SimulationSource)
	if !ok {
		h.upstreamFailed(r, "pov.advance_day", "", errPOVDisabled)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	allowed, spend := h.advances.allow(key, h.now())
	if !allowed {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": advanceWindowName})
		return
	}
	commandID, err := identity.NewV7()
	if err != nil {
		// The command id only correlates logs; the advance goes without one.
		commandID = ""
	}
	res, err := src.AdvanceDay(r.Context(), key, commandID)
	if err != nil {
		spend(status.Code(err) == codes.Unavailable, false)
		h.upstreamFailed(r, "pov.advance_day", "", err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	// A replay that answers this request's own retry is the first commit of
	// the advance, so it keeps the budget it spent.
	spend(res.Replay && !res.Retried, true)
	reply := advanceDayReply{SimDay: res.SimDay}
	if len(res.EventIDs) > 0 {
		reply.EventID = res.EventIDs[0]
	}
	writeJSON(w, http.StatusAccepted, reply)
}

// getSimulation serves GET /v1/client-pov/simulation: the current simulated
// day, or 502 when account-sim fails.
func (h *Handler) getSimulation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	src, ok := h.pov.(SimulationSource)
	if !ok {
		h.upstreamFailed(r, "pov.simulation", "", errPOVDisabled)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	day, err := src.SimDay(r.Context())
	if err != nil {
		h.upstreamFailed(r, "pov.simulation", "", err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, simulationReply{SimDay: day})
}
