package bff

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Handler serves the queue, SSE stream, actions proxy, seed cases, and timeline.
type Handler struct {
	board    *Board
	actions  ActionsClient
	cases    *CaseBoard
	timeline TimelineClient
	now      func() time.Time
}

// NewHandler returns an HTTP handler. A nil actions client is treated as unavailable.
// A nil timeline client serves the in-process c01 seed.
func NewHandler(board *Board, actions ActionsClient, tl TimelineClient) http.Handler {
	if actions == nil {
		actions = UnavailableActions{}
	}
	if tl == nil {
		tl = NewSeedTimeline()
	}
	h := &Handler{
		board:    board,
		actions:  actions,
		cases:    NewCaseBoard(),
		timeline: tl,
		now:      time.Now,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/queue", h.queue)
	mux.HandleFunc("GET /v1/queue/stream", h.stream)
	mux.HandleFunc("GET /v1/cases", h.listCases)
	mux.HandleFunc("POST /v1/cases/{id}/advance", h.advanceCase)
	mux.HandleFunc("PUT /v1/actions/{id}", h.putAction)
	mux.HandleFunc("DELETE /v1/actions/{id}", h.deleteAction)
	mux.HandleFunc("GET /v1/actions", h.listActions)
	mux.HandleFunc("GET /v1/customers/{id}/timeline", h.customerTimeline)
	return mux
}

func (h *Handler) customerTimeline(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	id := r.PathValue("id")
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
	body := struct {
		Items []TimelineEntry `json:"items"`
	}{Items: items}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
}

func (h *Handler) queue(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	items := h.board.Items()
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
	body := struct {
		Items []Signal `json:"items"`
	}{Items: out}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
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
		if _, err := fmt.Fprintf(w, "id: %s\nevent: signal\ndata: %s\n\n", ev.ID, ev.Data); err != nil {
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
	body := struct {
		Items  []Case   `json:"items"`
		States []string `json:"states"`
	}{Items: h.cases.Items(), States: CaseStates}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
}

func (h *Handler) advanceCase(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, ok := h.cases.Advance(id)
	if !ok {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(c); err != nil {
		return
	}
}

func (h *Handler) putAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	id := r.PathValue("id")
	err := h.actions.Undo(r.Context(), id)
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
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
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
