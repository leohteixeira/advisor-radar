// Package advisory raises deterministic alert.raised events from the book.
package advisory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// OutboxRow is one unpublished or published outbox record.
type OutboxRow struct {
	EventID    string
	RoutingKey string
	Payload    []byte
}

// AlertRow is the local alert write that pairs with an outbox row.
type AlertRow struct {
	ID            string
	CustomerID    string
	Kind          string
	Rule          string
	SourceEventID string
	RaisedAt      time.Time
	Payload       []byte
}

// AlertPayload is the domain payload on alert.raised.
type AlertPayload struct {
	Kind   string  `json:"kind"`
	Rule   string  `json:"rule"`
	Amount float64 `json:"amount,omitempty"`
	Before float64 `json:"before,omitempty"`
	After  float64 `json:"after,omitempty"`
	From   string  `json:"from,omitempty"`
	To     string  `json:"to,omitempty"`
	Days   int     `json:"days,omitempty"`
}

// Tx is the write side of one Apply transaction.
type Tx interface {
	ClaimInbox(ctx context.Context, eventID string) (bool, error)
	InsertAlert(ctx context.Context, row AlertRow) error
	InsertOutbox(ctx context.Context, row OutboxRow) error
}

// Store persists inbox, alerts, and outbox. Declared here for Apply/Publish.
type Store interface {
	WithTx(ctx context.Context, fn func(Tx) error) error
	ListUnpublished(ctx context.Context) ([]OutboxRow, error)
	MarkPublished(ctx context.Context, eventID string, at time.Time) error
}

// Broker accepts one event body under a routing key.
type Broker interface {
	Publish(ctx context.Context, routingKey string, body []byte) error
}

// Apply evaluates one inbound envelope. Only account.event.recorded enters the
// inbox. A second delivery of the same event_id is a no-op.
func Apply(ctx context.Context, store Store, env event.Envelope) error {
	if store == nil {
		return fmt.Errorf("advisory: store is required")
	}
	if env.Name != event.NameAccountEventRecorded {
		return nil
	}

	payload, err := decodeAccountPayload(env.Payload)
	if err != nil {
		return fmt.Errorf("advisory: apply %s: %w", env.EventID, err)
	}

	dollars, err := payload.Dollars(env.SchemaVersion)
	if err != nil {
		return fmt.Errorf("advisory: apply %s: %w", env.EventID, err)
	}

	decisions := EvaluateAccount(dollars)
	return raise(ctx, store, raiseInput{
		sourceEventID: env.EventID,
		customerID:    env.CustomerID,
		occurredAt:    env.OccurredAt,
		decisions:     decisions,
		alertIDs:      nil,
	})
}

// ApplySilence raises a contato alert when days > 90. alertID may be empty for
// a live id derived from sourceEventID.
func ApplySilence(
	ctx context.Context,
	store Store,
	customerID string,
	days int,
	before, after float64,
	sourceEventID string,
	alertID string,
	occurredAt time.Time,
) error {
	if store == nil {
		return fmt.Errorf("advisory: store is required")
	}
	d, ok := EvaluateSilence(days, before, after)
	if !ok {
		return nil
	}
	ids := map[string]string{}
	if alertID != "" {
		ids[RuleKeySilence] = alertID
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return raise(ctx, store, raiseInput{
		sourceEventID: sourceEventID,
		customerID:    customerID,
		occurredAt:    occurredAt,
		decisions:     []Decision{d},
		alertIDs:      ids,
	})
}

// Publish sends each unpublished outbox row once. A broker error leaves that
// row and later rows unpublished. The row is marked published only after ack.
func Publish(ctx context.Context, store Store, broker Broker) error {
	if store == nil {
		return fmt.Errorf("advisory: store is required")
	}
	if broker == nil {
		return fmt.Errorf("advisory: broker is required")
	}

	rows, err := store.ListUnpublished(ctx)
	if err != nil {
		return fmt.Errorf("advisory: list unpublished: %w", err)
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("advisory: publish: %w", err)
		}
		if err := broker.Publish(ctx, row.RoutingKey, row.Payload); err != nil {
			return fmt.Errorf("advisory: publish %s: %w", row.EventID, err)
		}
		if err := store.MarkPublished(ctx, row.EventID, time.Now().UTC()); err != nil {
			return fmt.Errorf("advisory: mark published %s: %w", row.EventID, err)
		}
	}
	return nil
}

// RunPublisher polls unpublished rows until ctx is canceled.
func RunPublisher(ctx context.Context, store Store, broker Broker, every time.Duration) error {
	if every <= 0 {
		every = time.Second
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		if err := Publish(ctx, store, broker); err != nil {
			if ctx.Err() != nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

type raiseInput struct {
	sourceEventID string
	customerID    string
	occurredAt    time.Time
	decisions     []Decision
	alertIDs      map[string]string
}

func raise(ctx context.Context, store Store, in raiseInput) error {
	if len(in.decisions) == 0 {
		// Still claim the inbox so a quiet fact is not re-evaluated forever.
		return store.WithTx(ctx, func(tx Tx) error {
			claimed, err := tx.ClaimInbox(ctx, in.sourceEventID)
			if err != nil {
				return fmt.Errorf("advisory: claim inbox: %w", err)
			}
			_ = claimed
			return nil
		})
	}

	err := store.WithTx(ctx, func(tx Tx) error {
		claimed, err := tx.ClaimInbox(ctx, in.sourceEventID)
		if err != nil {
			return fmt.Errorf("advisory: claim inbox: %w", err)
		}
		if !claimed {
			return nil
		}

		for _, d := range in.decisions {
			alertID := ""
			if in.alertIDs != nil {
				if fixed, ok := in.alertIDs[d.RuleKey]; ok {
					alertID = fixed
				}
			}
			if alertID == "" {
				var err error
				alertID, err = identity.NewV7()
				if err != nil {
					return fmt.Errorf("advisory: generate alert id: %w", err)
				}
			}

			payload := AlertPayload{
				Kind:   d.Kind,
				Rule:   d.Rule,
				Amount: d.Amount,
				Before: d.Before,
				After:  d.After,
				From:   d.From,
				To:     d.To,
				Days:   d.Days,
			}
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("advisory: marshal alert payload: %w", err)
			}

			env := event.Envelope{
				Name:          event.NameAlertRaised,
				EventID:       alertID,
				OccurredAt:    in.occurredAt,
				CustomerID:    in.customerID,
				SchemaVersion: event.SchemaVersionMVP,
				Payload:       payload,
			}
			body, err := env.MarshalBody()
			if err != nil {
				return fmt.Errorf("advisory: marshal alert body: %w", err)
			}

			if err := tx.InsertAlert(ctx, AlertRow{
				ID:            alertID,
				CustomerID:    in.customerID,
				Kind:          d.Kind,
				Rule:          d.Rule,
				SourceEventID: in.sourceEventID,
				RaisedAt:      in.occurredAt,
				Payload:       payloadBytes,
			}); err != nil {
				return fmt.Errorf("advisory: insert alert %s: %w", alertID, err)
			}
			if err := tx.InsertOutbox(ctx, OutboxRow{
				EventID:    alertID,
				RoutingKey: event.NameAlertRaised,
				Payload:    body,
			}); err != nil {
				return fmt.Errorf("advisory: insert outbox %s: %w", alertID, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("advisory: raise: %w", err)
	}
	return nil
}

func decodeAccountPayload(raw any) (sim.AccountPayload, error) {
	if raw == nil {
		return sim.AccountPayload{}, fmt.Errorf("payload is required")
	}
	switch v := raw.(type) {
	case sim.AccountPayload:
		return v, nil
	case *sim.AccountPayload:
		if v == nil {
			return sim.AccountPayload{}, fmt.Errorf("payload is required")
		}
		return *v, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return sim.AccountPayload{}, fmt.Errorf("encode payload: %w", err)
		}
		var p sim.AccountPayload
		if err := json.Unmarshal(data, &p); err != nil {
			return sim.AccountPayload{}, fmt.Errorf("decode payload: %w", err)
		}
		return p, nil
	}
}
