package advisory

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ActionsHandler serves PUT/DELETE/GET /v1/actions for Contatado and Adiar.
type ActionsHandler struct {
	actions *Actions
}

// NewActionsHandler returns an HTTP handler for signal actions.
func NewActionsHandler(actions *Actions) http.Handler {
	h := &ActionsHandler{actions: actions}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/actions/{id}", h.put)
	mux.HandleFunc("DELETE /v1/actions/{id}", h.del)
	mux.HandleFunc("GET /v1/actions", h.list)
	return mux
}

func (h *ActionsHandler) put(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	action, err := decodeActionBody(r)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	switch action {
	case "contact":
		if err := h.actions.Contact(r.Context(), id); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	case "snooze":
		if err := h.actions.Snooze(r.Context(), id); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ActionsHandler) del(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if err := h.actions.Undo(r.Context(), id); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ActionsHandler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.actions.List(r.Context())
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
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

func decodeActionBody(r *http.Request) (string, error) {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()

	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return "", fmt.Errorf("advisory: decode action: %w", err)
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString), nil
	}
	var asObj struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(raw, &asObj); err != nil {
		return "", fmt.Errorf("advisory: decode action object: %w", err)
	}
	return strings.TrimSpace(asObj.Action), nil
}
