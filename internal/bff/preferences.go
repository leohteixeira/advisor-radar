package bff

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// povPreferences is the PUT …/preferences request and response body.
type povPreferences struct {
	Channel string `json:"channel"`
	Beta    bool   `json:"beta"`
}

// povPreferencesBody decodes the PUT body; nil fields were left out.
type povPreferencesBody struct {
	Channel *string `json:"channel"`
	Beta    *bool   `json:"beta"`
}

// putPOVPreferences serves PUT /v1/client-pov/customers/{id}/preferences. It
// is an idempotent PUT, so it takes no Idempotency-Key. A bad id is 400; a
// body that does not decode, misses a field, or names a channel other than
// chat or email is 422 invalid and calls nothing. The stored preferences are
// read first: a PUT that changes nothing answers 200 and spends no budget. A
// change spends the per-customer POV budget (429 over it, and account-sim is
// not written) and goes to account-sim, which publishes no event.
func (h *Handler) putPOVPreferences(w http.ResponseWriter, r *http.Request) {
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	var body povPreferencesBody
	if r.Body == nil || json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body) != nil ||
		body.Channel == nil || body.Beta == nil ||
		(*body.Channel != sim.ChannelChat && *body.Channel != sim.ChannelEmail) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid"})
		return
	}
	want := POVPreferences{Channel: *body.Channel, Beta: *body.Beta}

	current, err := h.pov.Preferences(r.Context(), id)
	if errors.Is(err, sim.ErrUnknownCustomer) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	if err != nil {
		h.upstreamFailed(r, "pov.preferences.get", id, err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	if current == want {
		writeJSON(w, http.StatusOK, povPreferences(current))
		return
	}

	// No key is ever remembered for a PUT: each change spends the budget.
	allowed, window, spend := h.limits.allow(id, "", h.now())
	if !allowed {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": window})
		return
	}
	stored, err := h.pov.UpdatePreferences(r.Context(), id, want)
	if err != nil {
		refused := errors.Is(err, sim.ErrUnknownCustomer) || errors.Is(err, sim.ErrCommand)
		// As for commands, only Unavailable left after the client retries
		// means the update never reached account-sim, so it spends nothing.
		spend(!refused && status.Code(err) == codes.Unavailable, false)
		switch {
		case errors.Is(err, sim.ErrUnknownCustomer):
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		case refused:
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid"})
		default:
			h.upstreamFailed(r, "pov.preferences.update", id, err)
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		}
		return
	}
	spend(false, false)
	writeJSON(w, http.StatusOK, povPreferences(stored))
}
