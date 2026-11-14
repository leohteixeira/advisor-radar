package cases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// OpenCommand is the case.open.requested payload: an advisor asked to open a
// case from a queue signal (ADR 0013). Label is the signal's display label.
// The four facts are the signal's SLA halving factors.
type OpenCommand struct {
	SignalID           string `json:"signal_id"`
	Label              string `json:"label,omitempty"`
	ChurnRisk          bool   `json:"churn_risk,omitempty"`
	Frustration        int    `json:"frustration,omitempty"`
	HumanRequested     bool   `json:"human_requested,omitempty"`
	RelevantWithdrawal bool   `json:"relevant_withdrawal,omitempty"`
}

// OpenRequested applies one case.open.requested delivery body. Without an
// open case it opens one for the signal, with the SLA of the client's segment
// counted from the command's occurred_at. With an open case the signal joins
// that case's history, at most once, and no case is created.
//
// It shares the intake's flow: a duplicate or a join never calls advisory; a
// PermanentDeliveryError means redelivery cannot succeed; any other error is
// transient.
func OpenRequested(ctx context.Context, store IntakeStore, lookup CustomerLookup, body []byte) (IntakeResult, error) {
	if store == nil {
		return IntakeResult{}, errors.New("cases: store is required")
	}
	if lookup == nil {
		return IntakeResult{}, errors.New("cases: customer lookup is required")
	}

	env, payload, err := decodeEnvelope(event.NameCaseOpenRequested, "open command", body)
	res := IntakeResult{EventID: env.EventID, CustomerID: env.CustomerID}
	if err != nil {
		return res, PermanentDeliveryError{Err: err}
	}
	var cmd OpenCommand
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return res, PermanentDeliveryError{Err: fmt.Errorf("cases: decode open command payload: %w", err)}
	}
	// cases.signal_id and case_history.signal_id are UUID columns.
	signal, err := uuid.Parse(cmd.SignalID)
	if err != nil {
		return res, PermanentDeliveryError{Err: errors.New("cases: validate open command: signal_id is not a uuid")}
	}

	opened, joined := "Caso aberto pela assessoria", "Sinal anexado ao caso pela assessoria"
	if cmd.Label != "" {
		opened += " a partir do sinal: " + cmd.Label
		joined += ": " + cmd.Label
	}
	return admit(ctx, store, lookup, admission{
		eventID:    env.EventID,
		customerID: env.CustomerID,
		occurredAt: env.OccurredAt,
		signalID:   signal.String(),
		factors: ClockFactors{
			ChurnRisk:          cmd.ChurnRisk,
			Frustration:        cmd.Frustration,
			HumanRequested:     cmd.HumanRequested,
			RelevantWithdrawal: cmd.RelevantWithdrawal,
		},
		openedText: opened,
		joinKind:   HistoryKindSignal,
		joinText:   joined,
		joinSignal: true,
	})
}
