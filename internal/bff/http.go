package bff

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/screen"
	"github.com/leohteixeira/advisor-radar/internal/telemetry"
)

// Handler serves the queue, SSE stream, actions proxy, cases, timeline,
// review queue, manager panel, and customer detail.
type Handler struct {
	board    *Board
	actions  ActionsClient
	queue    QueueSource
	cases    CaseSource
	review   ReviewSource
	timeline TimelineClient
	pov      POVSource
	screens  *screen.Engine
	now      func() time.Time
	limits   *povLimiter
	advances *advanceLimiter
	// opens bounds POST /v1/signals/{id}/case with the advance-day budget:
	// every new key writes an advisory outbox row, a broker message and a
	// cases inbox row.
	opens      *advanceLimiter
	counts     *povCounts
	bastidores *bastidoresHub
	logger     *slog.Logger
	products   productNameCache
}

// NewHandler returns an HTTP handler. Nil sources are treated as empty.
// Server is the BFF HTTP handler plus the in-memory Bastidores hub.
type Server struct {
	http.Handler
	bastidores *bastidoresHub
	h          *Handler
}

// SetLogger replaces the handler logger, slog.Default() until set. Call it
// before the server handles requests.
func (s *Server) SetLogger(logger *slog.Logger) {
	if s == nil || s.h == nil || logger == nil {
		return
	}
	s.h.logger = logger
}

// ObservePOV advances Bastidores from a broker body.
func (s *Server) ObservePOV(routingKey string, body []byte) {
	if s == nil || s.bastidores == nil {
		return
	}
	s.bastidores.observe(routingKey, body)
}

func NewHandler(board *Board, actions ActionsClient, tl TimelineClient, queue QueueSource, review ReviewSource, cases CaseSource) *Server {
	return newHandler(board, actions, tl, queue, review, cases, nil, nil)
}

// NewHandlerWithPOV is NewHandler plus the client POV command port.
// now may be nil; the rate limit then uses time.Now.
func NewHandlerWithPOV(board *Board, actions ActionsClient, tl TimelineClient, queue QueueSource, review ReviewSource, cases CaseSource, pov POVSource, now func() time.Time) *Server {
	return newHandler(board, actions, tl, queue, review, cases, pov, now)
}

func newHandler(board *Board, actions ActionsClient, tl TimelineClient, queue QueueSource, review ReviewSource, cases CaseSource, pov POVSource, now func() time.Time) *Server {
	if actions == nil {
		actions = UnavailableActions{}
	}
	if tl == nil {
		tl = EmptyTimeline{}
	}
	if queue == nil {
		queue = EmptyQueue{}
	}
	if review == nil {
		review = EmptyReview{}
	}
	if cases == nil {
		cases = EmptyCases{}
	}
	h := &Handler{
		board:      board,
		actions:    actions,
		queue:      queue,
		cases:      cases,
		review:     review,
		timeline:   tl,
		pov:        pov,
		now:        now,
		limits:     newPOVLimiter(),
		advances:   newAdvanceLimiter(),
		opens:      newAdvanceLimiter(),
		counts:     newPOVCounts(),
		bastidores: newBastidoresHub(),
		logger:     slog.Default(),
	}
	if h.now == nil {
		h.now = time.Now
	}
	if h.pov == nil {
		h.pov = emptyPOV{}
	}
	h.screens = newScreenEngine(h.pov, h.queue, h.cases, h.timeline, h.now)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/queue", h.queueHandler)
	mux.HandleFunc("GET /v1/queue/stream", h.stream)
	mux.HandleFunc("GET /v1/cases", h.listCases)
	mux.HandleFunc("POST /v1/cases/{id}/advance", h.advanceCase)
	mux.HandleFunc("POST /v1/signals/{id}/case", h.openCase)
	mux.HandleFunc("PUT /v1/actions/{id}", h.putAction)
	mux.HandleFunc("DELETE /v1/actions/{id}", h.deleteAction)
	mux.HandleFunc("GET /v1/actions", h.listActions)
	mux.HandleFunc("GET /v1/customers/{id}", h.getCustomer)
	mux.HandleFunc("GET /v1/customers/{id}/timeline", h.customerTimeline)
	mux.HandleFunc("GET /v1/review", h.listReview)
	mux.HandleFunc("PUT /v1/review/{id}", h.correctReview)
	mux.HandleFunc("GET /v1/manager", h.manager)
	mux.HandleFunc("GET /v1/client-pov/customers", h.listPOV)
	mux.HandleFunc("GET /v1/client-pov/customers/{id}", h.getPOV)
	mux.HandleFunc("POST /v1/client-pov/customers/{id}/deposits", h.postPOVDeposit)
	mux.HandleFunc("POST /v1/client-pov/customers/{id}/withdrawals", h.postPOVWithdrawal)
	mux.HandleFunc("POST /v1/client-pov/customers/{id}/messages", h.postPOVMessage)
	mux.HandleFunc("POST /v1/client-pov/customers/{id}/complaints", h.postPOVComplaint)
	mux.HandleFunc("POST /v1/client-pov/customers/{id}/purchases", h.postPOVPurchase)
	mux.HandleFunc("PUT /v1/client-pov/customers/{id}/preferences", h.putPOVPreferences)
	mux.HandleFunc("GET /v1/client-pov/counters", h.povCounters)
	mux.HandleFunc("GET /v1/client-pov/simulation", h.getSimulation)
	mux.HandleFunc("POST /v1/client-pov/simulation/advance-day", h.postAdvanceDay)
	mux.HandleFunc("GET /v1/client-pov/customers/{id}/stream", h.povStream)
	mux.HandleFunc("GET /v1/client-pov/customers/{id}/screens/{slug}", h.getScreen)
	// One server span per request, named after the matched route pattern.
	return &Server{Handler: telemetry.HTTPHandler(mux, "bff"), bastidores: h.bastidores, h: h}
}

func (h *Handler) getCustomer(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	c, err := h.queue.GetCustomer(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrCustomerNotFound) || status.Code(err) == codes.NotFound {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		if status.Code(err) == codes.InvalidArgument {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c)
}

func (h *Handler) customerTimeline(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	id := r.PathValue("id")
	if _, err := identity.ParseV7(id); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	q := r.URL.Query().Get("q")
	kind := r.URL.Query().Get("kind")
	items, err := h.timeline.Search(r.Context(), id, q, kind)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	if items == nil {
		items = []TimelineEntry{}
	}
	items = slices.DeleteFunc(items, unchangedRevaluation)
	if parseTimelineOrder(r.URL.Query().Get("order")) == "desc" {
		reverseTimeline(items)
	}
	body := struct {
		Items []TimelineEntry `json:"items"`
	}{Items: items}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// signals is every signal the queue serves: advisory's seeded cast plus the
// live board, once each.
func (h *Handler) signals(ctx context.Context) ([]Signal, error) {
	items, err := h.queue.ListQueue(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		seen[it.ID] = struct{}{}
	}
	for _, live := range h.board.Items() {
		if _, ok := seen[live.ID]; ok {
			continue
		}
		items = append(items, live)
	}
	return items, nil
}

// listing loads what both list routes filter and count: the signals with the
// advisor's actions applied, and the cases. It also returns the case state
// labels. Advisory without the actions port means no signal is contacted or
// snoozed.
func (h *Handler) listing(ctx context.Context) (listing, []string, error) {
	signals, err := h.signals(ctx)
	if err != nil {
		return listing{}, nil, err
	}
	actions, err := h.actions.List(ctx)
	if err != nil && !errors.Is(err, ErrActionsUnavailable) {
		return listing{}, nil, err
	}
	cases, states, err := h.cases.ListCases(ctx)
	if err != nil {
		return listing{}, nil, err
	}
	if states == nil {
		states = CaseStates
	}
	// The sources own their slices; enrichment writes into copies.
	signals, cases = slices.Clone(signals), slices.Clone(cases)
	h.enrichSignals(ctx, signals)
	h.enrichCases(ctx, cases)
	return newListing(signals, actions, cases, h.now().UTC()), states, nil
}

func (h *Handler) queueHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	filters := filtersFromRequest(r)
	list, _, err := h.listing(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	body := struct {
		Items  []Signal   `json:"items"`
		Facets ListFacets `json:"facets"`
	}{Items: list.filteredSignals(filters), Facets: list.facets(filters)}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	lastID := r.Header.Get("Last-Event-ID")
	_ = h.board.Stream(r.Context(), lastID, func(ev StreamEvent) error {
		data := ev.Data
		var sig Signal
		if err := json.Unmarshal(ev.Data, &sig); err == nil {
			items := []Signal{sig}
			h.enrichSignals(r.Context(), items)
			if b, err := json.Marshal(items[0]); err == nil {
				data = b
			}
		}
		if _, err := fmt.Fprintf(w, "id: %s\nevent: signal\ndata: %s\n\n", ev.ID, data); err != nil {
			return fmt.Errorf("bff: write sse: %w", err)
		}
		flusher.Flush()
		return nil
	})
}

func (h *Handler) listCases(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	filters := filtersFromRequest(r)
	list, states, err := h.listing(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	body := struct {
		Items  []Case     `json:"items"`
		States []string   `json:"states"`
		Facets ListFacets `json:"facets"`
	}{Items: list.filteredCases(filters), States: states, Facets: list.facets(filters)}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// maxIdempotencyKeyLen bounds the Idempotency-Key of an advisor command.
const maxIdempotencyKeyLen = 128

// openCase serves POST /v1/signals/{id}/case, the advisor's Abrir caso
// (ADR 0013). It only accepts the command: 202 with the request id once
// advisory has it in its outbox. cases opens the case later, or joins the
// signal to the client's open case, and the browser learns which from
// GET /v1/cases. 404 means the queue does not serve that signal.
// The signal is looked up among every signal the queue serves, including one a
// case already represents and GET /v1/queue no longer lists: a retry of the
// same command must not turn into 404 once cases has acted on it.
func (h *Handler) openCase(w http.ResponseWriter, r *http.Request) {
	id, err := identity.ParseV7(r.PathValue("id"))
	key := r.Header.Get("Idempotency-Key")
	if err != nil || key == "" || len(key) > maxIdempotencyKeyLen {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	// A key already answered 202 is free, so a replay never spends; a
	// command that was not accepted gives its slot back.
	allowed, spend := h.opens.allow(key, h.now())
	if !allowed {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": advanceWindowName})
		return
	}
	accepted := false
	defer func() { spend(!accepted, accepted) }()
	signals, err := h.signals(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	i := slices.IndexFunc(signals, func(s Signal) bool { return s.ID == id })
	if i < 0 {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	sig := signals[i]
	requestID, err := h.actions.OpenCase(r.Context(), CaseCommand{
		SignalID:       sig.ID,
		CustomerID:     sig.Client,
		IdempotencyKey: key,
		Label:          scoreSignal(sig).typeLabel,
		// The facts signalSLA halves the SLA on.
		ChurnRisk:          ptrBool(sig.Churn),
		Frustration:        ptrInt(sig.Frustration),
		HumanRequested:     ptrBool(sig.Human),
		RelevantWithdrawal: sig.Alert == "saque",
	})
	if errors.Is(err, ErrActionsUnavailable) {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	accepted = true
	writeJSON(w, http.StatusAccepted, map[string]string{"request_id": requestID})
}

func (h *Handler) advanceCase(w http.ResponseWriter, r *http.Request) {
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	c, err := h.cases.Advance(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrCaseNotFound) || status.Code(err) == codes.NotFound {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	items := []Case{withSLAState(c)}
	h.enrichCases(r.Context(), items)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items[0])
}

func (h *Handler) putAction(w http.ResponseWriter, r *http.Request) {
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	action, err := decodeProxyAction(r)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	var call error
	switch action {
	case "contact":
		call = h.actions.Contact(r.Context(), id)
	case "snooze":
		call = h.actions.Snooze(r.Context(), id)
	default:
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if errors.Is(call, ErrActionsUnavailable) {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	if call != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteAction(w http.ResponseWriter, r *http.Request) {
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	err = h.actions.Undo(r.Context(), id)
	if errors.Is(err, ErrActionsUnavailable) {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listActions(w http.ResponseWriter, r *http.Request) {
	rows, err := h.actions.List(r.Context())
	if errors.Is(err, ErrActionsUnavailable) {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	if rows == nil {
		rows = []SignalAction{}
	}
	w.Header().Set("Content-Type", "application/json")
	body := struct {
		Items []SignalAction `json:"items"`
	}{Items: rows}
	_ = json.NewEncoder(w).Encode(body)
}

func (h *Handler) listReview(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	items, err := h.review.ListReview(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	if items == nil {
		items = []ReviewRow{}
	}
	h.enrichReviews(r.Context(), items)
	body := struct {
		Items []ReviewRow `json:"items"`
	}{Items: items}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (h *Handler) correctReview(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	intent, err := decodeReviewIntent(r)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	row, err := h.review.Correct(r.Context(), id, intent)
	if err != nil {
		code := status.Code(err)
		switch {
		case errors.Is(err, ErrInvalidIntent) || code == codes.InvalidArgument:
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		case errors.Is(err, ErrReviewNotFound) || code == codes.NotFound:
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		default:
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		}
		return
	}
	rows := []ReviewRow{row}
	h.enrichReviews(r.Context(), rows)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rows[0])
}

func (h *Handler) manager(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	snap, err := h.buildManager(r)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

func (h *Handler) buildManager(r *http.Request) (ManagerSnapshot, error) {
	ops, err := h.queue.ListOperators(r.Context())
	if err != nil {
		return ManagerSnapshot{}, err
	}
	names := make(map[string]string, len(ops))
	for _, o := range ops {
		names[o.ID] = o.Name
	}
	backlogRaw, err := h.cases.Backlog(r.Context())
	if err != nil {
		return ManagerSnapshot{}, err
	}
	backlog := make([]ManagerBacklog, 0, len(backlogRaw))
	for _, b := range backlogRaw {
		name := names[b.Advisor]
		if name == "" {
			name = b.Advisor
		}
		backlog = append(backlog, ManagerBacklog{Advisor: name, Open: b.Open, Risk: b.Risk, Overdue: b.Overdue})
	}
	avgToday, avgYest, err := h.queue.ContactMetrics(r.Context())
	if err != nil {
		return ManagerSnapshot{}, err
	}
	reviewPct, fallbackPct, intents, err := h.review.IntentStats(r.Context())
	if err != nil {
		return ManagerSnapshot{}, err
	}
	atRiskRaw, err := h.cases.ListAtRisk(r.Context())
	if err != nil {
		return ManagerSnapshot{}, err
	}
	atRisk := make([]ManagerAtRisk, 0, len(atRiskRaw))
	for _, a := range atRiskRaw {
		cust, err := h.queue.GetCustomer(r.Context(), a.Client)
		name := a.Client
		seg := ""
		if err == nil {
			name = cust.Name
			seg = cust.Segment
		}
		advisor := names[a.Advisor]
		if advisor == "" {
			advisor = a.Advisor
		}
		atRisk = append(atRisk, ManagerAtRisk{
			ID: a.ID, Client: name, Advisor: advisor, Segment: seg, Remaining: a.Remaining,
		})
	}
	slices.SortFunc(atRisk, func(a, b ManagerAtRisk) int {
		return cmp.Compare(a.Remaining, b.Remaining)
	})
	return ManagerSnapshot{
		Backlog: backlog, AvgFirstContactMin: avgToday, AvgFirstContactYesterday: avgYest,
		ReviewPct: reviewPct, FallbackPct: fallbackPct, Intents: intentShares(intents), AtRisk: atRisk,
	}, nil
}

func decodeProxyAction(r *http.Request) (string, error) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	var asString string
	if err := json.Unmarshal(body, &asString); err == nil {
		return asString, nil
	}
	var asObj struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(body, &asObj); err != nil {
		return "", err
	}
	return asObj.Action, nil
}

func decodeReviewIntent(r *http.Request) (string, error) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", fmt.Errorf("bff: read review intent: %w", err)
	}
	var asString string
	if err := json.Unmarshal(body, &asString); err == nil {
		return asString, nil
	}
	var asObj struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal(body, &asObj); err != nil {
		return "", fmt.Errorf("bff: decode review intent: %w", err)
	}
	return asObj.Intent, nil
}
