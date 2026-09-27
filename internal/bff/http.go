package bff

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Handler serves GET /v1/queue and GET /v1/queue/stream.
type Handler struct {
	board *Board
}

// NewHandler returns an HTTP handler for the queue board.
func NewHandler(board *Board) http.Handler {
	h := &Handler{board: board}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/queue", h.queue)
	mux.HandleFunc("GET /v1/queue/stream", h.stream)
	return mux
}

func (h *Handler) queue(w http.ResponseWriter, r *http.Request) {
	if err := r.Context().Err(); err != nil {
		http.Error(w, http.StatusText(http.StatusRequestTimeout), http.StatusRequestTimeout)
		return
	}
	body := struct {
		Items []Signal `json:"items"`
	}{Items: h.board.Items()}
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
