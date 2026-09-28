package bff

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// Handler serves the queue, SSE stream, actions proxy, cases, timeline,
// review queue, manager panel, and customer detail.
type Handler struct {
	board      *Board
	actions    ActionsClient
	queue      QueueSource
	cases      CaseSource
	review     ReviewSource
	timeline   TimelineClient
	pov        POVSource
	now        func() time.Time
	limits     *povLimiter
	counts     *povCounts
	bastidores *bastidoresHub
}

// NewHandler returns an HTTP handler. Nil sources are treated as empty.
// Server is the BFF HTTP handler plus the in-memory Bastidores hub.
type Server struct {
	http.Handler
	bastidores *bastidoresHub
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
		counts:     newPOVCounts(),
		bastidores: newBastidoresHub(),
	}
	if h.now == nil {
		h.now = time.Now
	}
	if h.pov == nil {
		h.pov = emptyPOV{}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/queue", h.queueHandler)
	mux.HandleFunc("GET /v1/queue/stream", h.stream)
	mux.HandleFunc("GET /v1/cases", h.listCases)
	mux.HandleFunc("POST /v1/cases/{id}/advance", h.advanceCase)
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
	mux.HandleFunc("GET /v1/client-pov/counters", h.povCounters)
	mux.HandleFunc("GET /v1/client-pov/customers/{id}/stream", h.povStream)
	return &Server{Handler: mux, bastidores: h.bastidores}
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
	if parseTimelineOrder(r.URL.Query().Get("order")) == "desc" {
		reverseTimeline(items)
	}
	body := struct {
		Items []TimelineEntry `json:"items"`
	}{Items: items}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (h *Handler) queueHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	filters := filtersFromRequest(r)
	items, err := h.queue.ListQueue(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
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
	actions, err := h.actions.List(r.Context())
	if err != nil && !errors.Is(err, ErrActionsUnavailable) {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	byID := make(map[string]SignalAction, len(actions))
	for _, a := range actions {
		byID[a.SignalID] = a
	}
	now := h.now().UTC()
	out := make([]Signal, 0, len(items))
	for _, item := range items {
		if a, ok := byID[item.ID]; ok {
			if a.SnoozedUntil != nil && a.SnoozedUntil.After(now) {
				continue
			}
			item.ContactedAt = a.ContactedAt
		}
		out = append(out, item)
	}
	h.enrichSignals(r.Context(), out)

	cases, _, err := h.cases.ListCases(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	if cases == nil {
		cases = []Case{}
	}
	h.enrichCases(r.Context(), cases)

	facets := buildSignalFacets(out, cases, filters)
	filtered := filterAndSortSignals(out, filters)
	body := struct {
		Items  []Signal   `json:"items"`
		Facets ListFacets `json:"facets"`
	}{Items: filtered, Facets: facets}
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
	items, states, err := h.cases.ListCases(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	if items == nil {
		items = []Case{}
	}
	if states == nil {
		states = CaseStates
	}
	h.enrichCases(r.Context(), items)

	signals, err := h.queue.ListQueue(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	seen := make(map[string]struct{}, len(signals))
	for _, it := range signals {
		seen[it.ID] = struct{}{}
	}
	for _, live := range h.board.Items() {
		if _, ok := seen[live.ID]; ok {
			continue
		}
		signals = append(signals, live)
	}
	h.enrichSignals(r.Context(), signals)

	facets := buildSignalFacets(signals, items, filters)
	filtered := filterAndSortCases(items, signals, filters)
	body := struct {
		Items  []Case     `json:"items"`
		States []string   `json:"states"`
		Facets ListFacets `json:"facets"`
	}{Items: filtered, States: states, Facets: facets}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
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
	items := []Case{c}
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
	if intents == nil {
		intents = map[string]int{}
	}
	return ManagerSnapshot{
		Backlog: backlog, AvgFirstContactMin: avgToday, AvgFirstContactYesterday: avgYest,
		ReviewPct: reviewPct, FallbackPct: fallbackPct, Intents: intents, AtRisk: atRisk,
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
