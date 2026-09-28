package bff

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

const (
	povPerMinute = 10
	povPerDay    = 40
	povMinute    = time.Minute
	povDay       = 24 * time.Hour
)

// POVAccount is one seeded balance in integer USD cents.
type POVAccount struct {
	CustomerID string
	Acoes      int64
	ETFs       int64
	RendaFixa  int64
	Caixa      int64
}

// POVCommand is one client action. Amount is integer cents.
type POVCommand struct {
	CustomerID     string
	IdempotencyKey string
	Kind           string
	Amount         int64
	Origin         string
	Destination    string
	Channel        string
	Text           string
}

// POVResult is the account-sim outcome for one command.
type POVResult struct {
	EventID string
	Replay  bool
}

// POVSource is the account-sim port. The BFF does not publish.
type POVSource interface {
	List(ctx context.Context) ([]POVAccount, error)
	Get(ctx context.Context, customerID string) (POVAccount, error)
	Apply(ctx context.Context, cmd POVCommand) (POVResult, error)
}

type povMeta struct {
	id      string
	name    string
	segment string
	advisor string
	sla     string
}

var povCatalog = []povMeta{
	{sim.CustomerMariana, "Mariana Costa", "Singular", "Ana Paula Ribeiro", "1 h"},
	{sim.CustomerFernanda, "Fernanda Lima", "Essencial", "Ana Paula Ribeiro", "24 h"},
	{sim.CustomerThiago, "Thiago Azevedo", "Advance", "Ana Paula Ribeiro", "4 h"},
}

type emptyPOV struct{}

func (emptyPOV) List(context.Context) ([]POVAccount, error) { return nil, nil }
func (emptyPOV) Get(context.Context, string) (POVAccount, error) {
	return POVAccount{}, sim.ErrUnknownCustomer
}
func (emptyPOV) Apply(context.Context, POVCommand) (POVResult, error) {
	return POVResult{}, errors.New("bff: pov source is not configured")
}

type povLimiter struct {
	mu    sync.Mutex
	hits  map[string][]time.Time
	known map[string]struct{}
}

func newPOVLimiter() *povLimiter {
	return &povLimiter{
		hits:  map[string][]time.Time{},
		known: map[string]struct{}{},
	}
}

func (l *povLimiter) allow(customerID, key string, now time.Time) (ok bool, window string, spend func(replay, remember bool)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	id := customerID + "\x00" + key
	if _, seen := l.known[id]; seen {
		return true, "", func(bool, bool) {}
	}
	cutoff := now.Add(-povDay)
	kept := l.hits[customerID][:0]
	for _, at := range l.hits[customerID] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	minute := now.Add(-povMinute)
	inMinute := 0
	for _, at := range kept {
		if at.After(minute) {
			inMinute++
		}
	}
	if inMinute >= povPerMinute {
		l.hits[customerID] = kept
		return false, "minute", nil
	}
	if len(kept) >= povPerDay {
		l.hits[customerID] = kept
		return false, "day", nil
	}
	kept = append(kept, now)
	l.hits[customerID] = kept
	spent := true
	return true, "", func(replay, remember bool) {
		l.mu.Lock()
		defer l.mu.Unlock()
		if remember {
			l.known[id] = struct{}{}
		}
		if replay && spent {
			hits := l.hits[customerID]
			if len(hits) > 0 {
				l.hits[customerID] = hits[:len(hits)-1]
			}
			spent = false
		}
	}
}

type povCounts struct {
	mu         sync.Mutex
	actions    map[string]int
	refusals   map[string]int
	duplicates int
}

func newPOVCounts() *povCounts {
	return &povCounts{actions: map[string]int{}, refusals: map[string]int{}}
}

func (c *povCounts) addAction(kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.actions[kind]++
}

func (c *povCounts) addRefusal(rule string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refusals[rule]++
}

func (c *povCounts) addDuplicate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.duplicates++
}

func (c *povCounts) snapshot() (actions, refusals map[string]int, duplicates int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	actions = map[string]int{}
	refusals = map[string]int{}
	for k, v := range c.actions {
		actions[k] = v
	}
	for k, v := range c.refusals {
		refusals[k] = v
	}
	return actions, refusals, c.duplicates
}

func (h *Handler) listPOV(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.pov.List(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	byID := map[string]POVAccount{}
	for _, account := range accounts {
		byID[account.CustomerID] = account
	}
	items := make([]map[string]any, 0, len(povCatalog))
	for _, meta := range povCatalog {
		account := byID[meta.id]
		items = append(items, map[string]any{
			"customer_id": meta.id,
			"name":        meta.name,
			"segment":     meta.segment,
			"assets":      account.Acoes + account.ETFs + account.RendaFixa + account.Caixa,
			"sla":         meta.sla,
			"advisor":     meta.advisor,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) getPOV(w http.ResponseWriter, r *http.Request) {
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	account, err := h.pov.Get(r.Context(), id)
	if errors.Is(err, sim.ErrUnknownCustomer) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	meta := catalog(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"customer_id": id,
		"name":        meta.name,
		"segment":     meta.segment,
		"advisor":     meta.advisor,
		"sla":         meta.sla,
		"assets":      account.Acoes + account.ETFs + account.RendaFixa + account.Caixa,
		"caixa":       account.Caixa,
		"allocation": map[string]int64{
			"acoes": account.Acoes, "etfs": account.ETFs, "renda_fixa": account.RendaFixa, "caixa": account.Caixa,
		},
		"activity": []any{},
		"messages": []any{},
	})
}

func (h *Handler) postPOVDeposit(w http.ResponseWriter, r *http.Request) {
	h.postPOV(w, r, sim.CmdDeposit)
}

func (h *Handler) postPOVWithdrawal(w http.ResponseWriter, r *http.Request) {
	h.postPOV(w, r, sim.CmdWithdrawal)
}

func (h *Handler) postPOVMessage(w http.ResponseWriter, r *http.Request) {
	h.postPOV(w, r, sim.CmdMessage)
}

func (h *Handler) postPOVComplaint(w http.ResponseWriter, r *http.Request) {
	h.postPOV(w, r, sim.CmdComplaint)
}

func (h *Handler) postPOV(w http.ResponseWriter, r *http.Request, kind string) {
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	var body struct {
		Amount      int64  `json:"amount"`
		Origin      string `json:"origin"`
		Destination string `json:"destination"`
		Channel     string `json:"channel"`
		Text        string `json:"text"`
	}
	if r.Body != nil {
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
		if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
	}
	ok, window, spend := h.limits.allow(id, key, h.now())
	if !ok {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": window})
		return
	}
	result, err := h.pov.Apply(r.Context(), POVCommand{
		CustomerID:     id,
		IdempotencyKey: key,
		Kind:           kind,
		Amount:         body.Amount,
		Origin:         body.Origin,
		Destination:    body.Destination,
		Channel:        body.Channel,
		Text:           body.Text,
	})
	if err != nil {
		if spend != nil {
			spend(false, false)
		}
		if errors.Is(err, sim.ErrInsufficient) {
			h.counts.addRefusal("insufficient")
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "insufficient"})
			return
		}
		if errors.Is(err, sim.ErrAmount) || errors.Is(err, sim.ErrCommand) || errors.Is(err, sim.ErrUnknownCustomer) {
			h.counts.addRefusal("invalid")
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid"})
			return
		}
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	if spend != nil {
		spend(result.Replay, true)
	}
	if result.Replay {
		h.counts.addDuplicate()
	} else {
		h.counts.addAction(kind)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"event_id": result.EventID})
}

func (h *Handler) povCounters(w http.ResponseWriter, r *http.Request) {
	actions, refusals, duplicates := h.counts.snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"actions":    actions,
		"refusals":   refusals,
		"duplicates": duplicates,
	})
}

func catalog(id string) povMeta {
	for _, meta := range povCatalog {
		if meta.id == id {
			return meta
		}
	}
	return povMeta{id: id}
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
