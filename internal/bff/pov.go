package bff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

const (
	povPerMinute = 10
	povPerDay    = 40
	povMinute    = time.Minute
	povDay       = 24 * time.Hour
)

// POVAccount is one account-sim account in integer USD cents. Acoes, ETFs,
// and RendaFixa are class aggregates of Positions at market value; Patrimony
// is positions plus Caixa, as account-sim reports it. SimDay is the global
// simulated day the values are at, and DayChange the patrimony change since
// the day before (0 on day 0).
type POVAccount struct {
	CustomerID string
	Acoes      int64
	ETFs       int64
	RendaFixa  int64
	Caixa      int64
	Patrimony  int64
	Positions  []POVPosition
	SimDay     int
	DayChange  int64
}

// POVPosition is one holding in integer USD cents.
type POVPosition struct {
	ProductID    string
	AssetClass   string
	AppliedCents int64
	ValueCents   int64
}

// POVProduct is one entry of the account-sim product catalog. AssetClass is
// "acoes", "etfs", or "renda_fixa"; MinimumCents is integer USD cents.
type POVProduct struct {
	ID           string
	Name         string
	AssetClass   string
	Risk         int
	ReturnLabel  string
	MinimumCents int64
}

// assets is the sum of the four classes.
func (a POVAccount) assets() int64 {
	return a.Acoes + a.ETFs + a.RendaFixa + a.Caixa
}

// The POV response DTOs keep the phase-2 JSON. Fields are declared in key
// order, so the encoding is byte-identical to the former map[string]any
// output, whose keys encoding/json sorted.

// povList is the GET /v1/client-pov/customers body.
type povList struct {
	Items []povListItem `json:"items"`
}

// povListItem is one client in the persona list. Assets is integer cents.
type povListItem struct {
	Advisor    string `json:"advisor"`
	Assets     int64  `json:"assets"`
	CustomerID string `json:"customer_id"`
	Hint       string `json:"hint"`
	Name       string `json:"name"`
	Segment    string `json:"segment"`
	Since      string `json:"since"`
	SLA        string `json:"sla"`
}

// povHome is the GET /v1/client-pov/customers/{id} body. Money is integer
// cents. Activity and Messages are always empty in phase 2 and must be
// non-nil so they encode as [] rather than null.
type povHome struct {
	Activity   []any         `json:"activity"`
	Advisor    string        `json:"advisor"`
	Allocation povAllocation `json:"allocation"`
	Assets     int64         `json:"assets"`
	Caixa      int64         `json:"caixa"`
	CustomerID string        `json:"customer_id"`
	Messages   []any         `json:"messages"`
	Name       string        `json:"name"`
	Segment    string        `json:"segment"`
	Since      string        `json:"since"`
	SLA        string        `json:"sla"`
}

// povAllocation is the balance per asset class in integer cents.
type povAllocation struct {
	Acoes     int64 `json:"acoes"`
	Caixa     int64 `json:"caixa"`
	ETFs      int64 `json:"etfs"`
	RendaFixa int64 `json:"renda_fixa"`
}

// POVCommand is one client action. Amount is integer cents. ProductID is set
// only for a purchase; CommandID is a per-request correlation id that
// account-sim logs and never uses for idempotency.
type POVCommand struct {
	CustomerID     string
	IdempotencyKey string
	CommandID      string
	Kind           string
	Amount         int64
	ProductID      string
	Origin         string
	Destination    string
	Channel        string
	Text           string
}

// POVResult is the account-sim outcome for one command. Retried reports that
// the command was sent more than once in this request, so a Replay may be the
// answer to this request's own earlier, lost attempt.
type POVResult struct {
	EventID string
	Replay  bool
	Retried bool
}

// POVRegistration is the account-sim fictional registration data. Phone is
// already masked display text.
type POVRegistration struct {
	Email         string
	Phone         string
	City          string
	AccountNumber string
}

// POVPreferences are the client's contact channel ("chat" or "email") and
// beta program flag.
type POVPreferences struct {
	Channel string
	Beta    bool
}

// POVSource is the account-sim port. The BFF does not publish. Refusals are
// the sim sentinels: ErrInsufficient, ErrUnknownCustomer, ErrAmount, and
// ErrCommand; any other error is an upstream failure.
type POVSource interface {
	List(ctx context.Context) ([]POVAccount, error)
	Get(ctx context.Context, customerID string) (POVAccount, error)
	Apply(ctx context.Context, cmd POVCommand) (POVResult, error)
	// Products returns the fictional product catalog.
	Products(ctx context.Context) ([]POVProduct, error)
	// Registration returns one customer's registration data.
	Registration(ctx context.Context, customerID string) (POVRegistration, error)
	// Preferences returns one customer's stored preferences.
	Preferences(ctx context.Context, customerID string) (POVPreferences, error)
	// UpdatePreferences stores them and returns what is stored. It publishes
	// no event.
	UpdatePreferences(ctx context.Context, customerID string, prefs POVPreferences) (POVPreferences, error)
}

type povMeta struct {
	id      string
	name    string
	segment string
	advisor string
	sla     string
	hint    string
}

var povCatalog = []povMeta{
	{sim.CustomerMariana, "Mariana Costa", "Singular", "Ana Paula Ribeiro", "1 h", "Já reclamou de uma transferência atrasada. Um saque grande ou uma ameaça de saída sobem a prioridade na hora."},
	{sim.CustomerFernanda, "Fernanda Lima", "Essencial", "Ana Paula Ribeiro", "24 h", "Perto do teto da faixa. Um depósito pode subir o segmento; uma reclamação mostra o SLA mais longo."},
	{sim.CustomerThiago, "Thiago Azevedo", "Advance", "Ana Paula Ribeiro", "4 h", "Acabou de subir de Essencial para Advance com um depósito grande."},
}

type emptyPOV struct{}

func (emptyPOV) List(context.Context) ([]POVAccount, error) { return nil, nil }
func (emptyPOV) Get(context.Context, string) (POVAccount, error) {
	return POVAccount{}, sim.ErrUnknownCustomer
}
func (emptyPOV) Apply(context.Context, POVCommand) (POVResult, error) {
	return POVResult{}, errors.New("bff: pov source is not configured")
}
func (emptyPOV) Products(context.Context) ([]POVProduct, error) {
	return nil, errPOVDisabled
}
func (emptyPOV) Registration(context.Context, string) (POVRegistration, error) {
	return POVRegistration{}, errPOVDisabled
}
func (emptyPOV) Preferences(context.Context, string) (POVPreferences, error) {
	return POVPreferences{}, errPOVDisabled
}
func (emptyPOV) UpdatePreferences(context.Context, string, POVPreferences) (POVPreferences, error) {
	return POVPreferences{}, errPOVDisabled
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

func (l *povLimiter) allow(customerID, key string, now time.Time) (ok bool, window string, spend func(refund, remember bool)) {
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
	return true, "", func(refund, remember bool) {
		l.mu.Lock()
		defer l.mu.Unlock()
		if remember {
			l.known[id] = struct{}{}
		}
		if refund && spent {
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
		h.upstreamFailed(r, "pov.list", "", err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	byID := map[string]POVAccount{}
	for _, account := range accounts {
		byID[account.CustomerID] = account
	}
	items := make([]povListItem, 0, len(povCatalog))
	for _, meta := range povCatalog {
		account := byID[meta.id]
		items = append(items, povListItem{
			Advisor:    meta.advisor,
			Assets:     account.assets(),
			CustomerID: meta.id,
			Hint:       meta.hint,
			Name:       meta.name,
			Segment:    meta.segment,
			Since:      h.customerSince(r.Context(), meta.id),
			SLA:        meta.sla,
		})
	}
	writeJSON(w, http.StatusOK, povList{Items: items})
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
		h.upstreamFailed(r, "pov.get", id, err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	meta := catalog(id)
	writeJSON(w, http.StatusOK, povHome{
		Activity: []any{},
		Advisor:  meta.advisor,
		Allocation: povAllocation{
			Acoes:     account.Acoes,
			Caixa:     account.Caixa,
			ETFs:      account.ETFs,
			RendaFixa: account.RendaFixa,
		},
		Assets:     account.assets(),
		Caixa:      account.Caixa,
		CustomerID: id,
		Messages:   []any{},
		Name:       meta.name,
		Segment:    meta.segment,
		Since:      h.customerSince(r.Context(), id),
		SLA:        meta.sla,
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

// postPOVPurchase accepts {product_id, amount_cents}. A body that does not
// decode is 422 invalid before any budget is spent; every other refusal comes
// from account-sim, as for the other commands.
func (h *Handler) postPOVPurchase(w http.ResponseWriter, r *http.Request) {
	id, key, ok := povTarget(w, r)
	if !ok {
		return
	}
	var body struct {
		ProductID   string `json:"product_id"`
		AmountCents int64  `json:"amount_cents"`
	}
	if r.Body == nil || json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body) != nil {
		h.counts.addRefusal("invalid")
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid"})
		return
	}
	commandID, err := identity.NewV7()
	if err != nil {
		// The command id only correlates logs; the purchase goes without one.
		commandID = ""
	}
	h.runPOV(w, r, POVCommand{
		CustomerID:     id,
		IdempotencyKey: key,
		CommandID:      commandID,
		Kind:           sim.CmdPurchase,
		Amount:         body.AmountCents,
		ProductID:      body.ProductID,
	})
}

// povTarget reads the customer id and the Idempotency-Key of a POV command.
// A bad id or a missing key answers 400 and reports false.
func povTarget(w http.ResponseWriter, r *http.Request) (id, key string, ok bool) {
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return "", "", false
	}
	key = r.Header.Get("Idempotency-Key")
	if key == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return "", "", false
	}
	return id, key, true
}

func (h *Handler) postPOV(w http.ResponseWriter, r *http.Request, kind string) {
	id, key, ok := povTarget(w, r)
	if !ok {
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
	h.runPOV(w, r, POVCommand{
		CustomerID:     id,
		IdempotencyKey: key,
		Kind:           kind,
		Amount:         body.Amount,
		Origin:         body.Origin,
		Destination:    body.Destination,
		Channel:        body.Channel,
		Text:           body.Text,
	})
}

// runPOV spends the per-customer budget, sends cmd to account-sim, and maps
// the outcome to the POV answers shared by every command.
func (h *Handler) runPOV(w http.ResponseWriter, r *http.Request, cmd POVCommand) {
	id, kind := cmd.CustomerID, cmd.Kind
	allowed, window, spend := h.limits.allow(id, cmd.IdempotencyKey, h.now())
	if !allowed {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": window})
		return
	}
	result, err := h.pov.Apply(r.Context(), cmd)
	if err != nil {
		refused := errors.Is(err, sim.ErrInsufficient) || errors.Is(err, sim.ErrAmount) ||
			errors.Is(err, sim.ErrCommand) || errors.Is(err, sim.ErrUnknownCustomer) ||
			errors.Is(err, sim.ErrProduct)
		if spend != nil {
			// A refusal spends the budget. So does any failure that may have
			// committed in account-sim (deadline, cancel, internal). Only
			// Unavailable, left after the client retries, means the command never
			// reached account-sim, so it spends nothing. No failure remembers
			// the key.
			spend(!refused && status.Code(err) == codes.Unavailable, false)
		}
		if errors.Is(err, sim.ErrInsufficient) {
			h.counts.addRefusal("insufficient")
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "insufficient"})
			return
		}
		if refused {
			h.counts.addRefusal("invalid")
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid"})
			return
		}
		h.upstreamFailed(r, "pov."+kind, id, err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	// A replay that answers this request's own retry is the first commit of
	// the command: it spends the budget and counts as an action.
	replay := result.Replay && !result.Retried
	if spend != nil {
		spend(replay, true)
	}
	if replay {
		h.counts.addDuplicate()
	} else {
		h.counts.addAction(kind)
	}
	h.bastidores.markCommand(id, result.EventID, kind)
	writeJSON(w, http.StatusAccepted, map[string]string{"event_id": result.EventID})
}

func (h *Handler) povStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	updates, cancel := h.bastidores.subscribe(id)
	defer cancel()
	for {
		select {
		case <-r.Context().Done():
			return
		case view := <-updates:
			raw, err := json.Marshal(view)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: bastidores\ndata: %s\n\n", raw); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) povCounters(w http.ResponseWriter, r *http.Request) {
	actions, refusals, duplicates := h.counts.snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"actions":    actions,
		"refusals":   refusals,
		"duplicates": duplicates,
	})
}

// upstreamFailed logs a POV call that answers 502. It records the operation,
// the gRPC code, and the customer, never the request body.
func (h *Handler) upstreamFailed(r *http.Request, op, customerID string, err error) {
	h.logger.LogAttrs(r.Context(), slog.LevelWarn, "account-sim call failed",
		slog.String("service", "bff"),
		slog.String("operation", op),
		slog.String("code", status.Code(err).String()),
		slog.String("customer_id", customerID),
	)
}

func (h *Handler) customerSince(ctx context.Context, id string) string {
	customer, err := h.queue.GetCustomer(ctx, id)
	if err != nil {
		return ""
	}
	return customer.Since
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
