package advisory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// Bounds on what a case command may carry.
const (
	maxIdempotencyKeyLen = 128
	maxCaseLabelRunes    = 120
	maxCaseCommandBytes  = 4 << 10
)

// ErrInvalidCaseCommand marks a case command that cannot be accepted as sent.
var ErrInvalidCaseCommand = errors.New("advisory: invalid case command")

// caseCommandNamespace scopes the event ids derived from a signal id and an
// idempotency key.
var caseCommandNamespace = uuid.MustParse("7f1d0c1e-6a52-4d0b-9a5e-3c1f4b8a2d10")

// CaseCommand is an advisor's request to open a case from a queue signal
// (ADR 0013). Label is the signal's display label; the four facts are the
// ones that halve an SLA.
type CaseCommand struct {
	SignalID           string
	CustomerID         string
	IdempotencyKey     string
	Label              string
	ChurnRisk          bool
	Frustration        int
	HumanRequested     bool
	RelevantWithdrawal bool
}

// caseCommandPayload is the case.open.requested payload. cases decodes these
// literal keys (cases.OpenCommand); keep them in sync.
type caseCommandPayload struct {
	SignalID           string `json:"signal_id"`
	Label              string `json:"label,omitempty"`
	ChurnRisk          bool   `json:"churn_risk,omitempty"`
	Frustration        int    `json:"frustration,omitempty"`
	HumanRequested     bool   `json:"human_requested,omitempty"`
	RelevantWithdrawal bool   `json:"relevant_withdrawal,omitempty"`
}

// CaseCommands accepts case commands into the advisory outbox. advisory
// stores nothing else about them: cases decides whether a case opens.
type CaseCommands struct {
	store Store
	now   func() time.Time
}

// NewCaseCommands returns a CaseCommands over store. now defaults to
// time.Now when nil.
func NewCaseCommands(store Store, now func() time.Time) *CaseCommands {
	if now == nil {
		now = time.Now
	}
	return &CaseCommands{store: store, now: now}
}

// Request writes one case.open.requested outbox row and returns its event id.
// The id is derived from the signal id and the idempotency key, so a retry of
// the same request returns the same id and writes nothing new. A command that
// cannot be accepted wraps ErrInvalidCaseCommand.
func (c *CaseCommands) Request(ctx context.Context, cmd CaseCommand) (string, error) {
	signal, err := uuid.Parse(cmd.SignalID)
	if err != nil {
		return "", fmt.Errorf("%w: signal id is not a uuid", ErrInvalidCaseCommand)
	}
	customer, err := uuid.Parse(cmd.CustomerID)
	if err != nil {
		return "", fmt.Errorf("%w: customer id is not a uuid", ErrInvalidCaseCommand)
	}
	if cmd.IdempotencyKey == "" || len(cmd.IdempotencyKey) > maxIdempotencyKeyLen {
		return "", fmt.Errorf("%w: idempotency key must have 1 to %d bytes", ErrInvalidCaseCommand, maxIdempotencyKeyLen)
	}
	if utf8.RuneCountInString(cmd.Label) > maxCaseLabelRunes {
		return "", fmt.Errorf("%w: label is longer than %d characters", ErrInvalidCaseCommand, maxCaseLabelRunes)
	}
	if cmd.Frustration < 0 {
		return "", fmt.Errorf("%w: frustration is negative", ErrInvalidCaseCommand)
	}

	eventID := uuid.NewSHA1(caseCommandNamespace, []byte(signal.String()+"\x00"+cmd.IdempotencyKey)).String()
	body, err := event.Envelope{
		Name:          event.NameCaseOpenRequested,
		EventID:       eventID,
		OccurredAt:    c.now().UTC(),
		CustomerID:    customer.String(),
		SchemaVersion: event.SchemaVersionMVP,
		Payload: caseCommandPayload{
			SignalID:           signal.String(),
			Label:              cmd.Label,
			ChurnRisk:          cmd.ChurnRisk,
			Frustration:        cmd.Frustration,
			HumanRequested:     cmd.HumanRequested,
			RelevantWithdrawal: cmd.RelevantWithdrawal,
		},
	}.MarshalBody()
	if err != nil {
		return "", fmt.Errorf("advisory: marshal case command: %w", err)
	}

	err = c.store.WithTx(ctx, func(tx Tx) error {
		return tx.InsertOutbox(ctx, OutboxRow{EventID: eventID, RoutingKey: event.NameCaseOpenRequested, Payload: body})
	})
	if err != nil {
		return "", fmt.Errorf("advisory: store case command: %w", err)
	}
	return eventID, nil
}

// caseCommandBody is the JSON body of POST /v1/signals/{id}/case.
type caseCommandBody struct {
	CustomerID         string `json:"customer_id"`
	Label              string `json:"label"`
	ChurnRisk          bool   `json:"churn_risk"`
	Frustration        int    `json:"frustration"`
	HumanRequested     bool   `json:"human_requested"`
	RelevantWithdrawal bool   `json:"relevant_withdrawal"`
}

// requestCase serves POST /v1/signals/{id}/case: 202 with the request id once
// the command is in the outbox, 400 for a command that cannot be accepted,
// 503 when this process has no outbox.
func (h *ActionsHandler) requestCase(w http.ResponseWriter, r *http.Request) {
	if h.commands == nil {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	var body caseCommandBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCaseCommandBytes)).Decode(&body); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	requestID, err := h.commands.Request(r.Context(), CaseCommand{
		SignalID:           r.PathValue("id"),
		CustomerID:         body.CustomerID,
		IdempotencyKey:     r.Header.Get("Idempotency-Key"),
		Label:              body.Label,
		ChurnRisk:          body.ChurnRisk,
		Frustration:        body.Frustration,
		HumanRequested:     body.HumanRequested,
		RelevantWithdrawal: body.RelevantWithdrawal,
	})
	switch {
	case errors.Is(err, ErrInvalidCaseCommand):
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(struct {
		RequestID string `json:"request_id"`
	}{RequestID: requestID})
}
