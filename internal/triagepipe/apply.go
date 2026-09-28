// Package triagepipe applies message.received events through a classifier and
// persists message.triaged via an idempotent inbox and transactional outbox.
package triagepipe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/sim"
	"github.com/leohteixeira/advisor-radar/internal/triage"
)

// OutboxRow is one unpublished or published outbox record.
type OutboxRow struct {
	EventID    string
	RoutingKey string
	Payload    []byte
}

// ResultRow is the local classification write that pairs with an outbox row.
type ResultRow struct {
	ID            string
	SourceEventID string
	CustomerID    string
	Intent        string
	IntentProb    float64
	Frustration   float64
	ChurnRisk     float64
	WantsHuman    float64
	Classifier    string
	ModelVersion  string
	Degraded      bool
	NeedsReview   bool
	CreatedAt     time.Time
	Payload       []byte
}

// TriagedPayload is the domain payload on message.triaged.
type TriagedPayload struct {
	SourceEventID string  `json:"source_event_id"`
	Channel       string  `json:"channel"`
	Text          string  `json:"text"`
	Intent        string  `json:"intent"`
	IntentProb    float64 `json:"intent_prob"`
	Frustration   float64 `json:"frustration"`
	ChurnRisk     float64 `json:"churn_risk"`
	WantsHuman    float64 `json:"wants_human"`
	Classifier    string  `json:"classifier"`
	ModelVersion  string  `json:"model_version"`
	Degraded      bool    `json:"degraded"`
	NeedsReview   bool    `json:"needs_review"`
	CostUSD       string  `json:"cost_usd,omitempty"`
}

// Tx is the write side of one Apply transaction.
type Tx interface {
	ClaimInbox(ctx context.Context, eventID string) (bool, error)
	InsertResult(ctx context.Context, row ResultRow) error
	InsertOutbox(ctx context.Context, row OutboxRow) error
}

// Store persists inbox, results, and outbox. Declared here for Apply/Publish.
type Store interface {
	HasInbox(ctx context.Context, eventID string) (bool, error)
	WithTx(ctx context.Context, fn func(Tx) error) error
	ListUnpublished(ctx context.Context) ([]OutboxRow, error)
	MarkPublished(ctx context.Context, eventID string, at time.Time) error
}

// Broker accepts one event body under a routing key.
type Broker interface {
	Publish(ctx context.Context, routingKey string, body []byte) error
}

// Apply classifies one inbound envelope. Only message.received enters the
// inbox. A second delivery of the same event_id is a no-op and does not
// classify again.
func Apply(ctx context.Context, store Store, classifier triage.Classifier, env event.Envelope) error {
	if store == nil {
		return fmt.Errorf("triagepipe: store is required")
	}
	if classifier == nil {
		return fmt.Errorf("triagepipe: classifier is required")
	}
	if env.Name != event.NameMessageReceived {
		return nil
	}

	seen, err := store.HasInbox(ctx, env.EventID)
	if err != nil {
		return fmt.Errorf("triagepipe: has inbox %s: %w", env.EventID, err)
	}
	if seen {
		return nil
	}

	payload, err := decodeMessagePayload(env.Payload)
	if err != nil {
		return fmt.Errorf("triagepipe: apply %s: %w", env.EventID, err)
	}

	msg := triage.Message{
		ID:      env.EventID,
		Channel: payload.Channel,
		Text:    payload.Text,
	}
	result, err := classifier.Classify(ctx, msg)
	if err != nil {
		return fmt.Errorf("triagepipe: classify %s: %w", env.EventID, err)
	}

	triagedID, err := triagedEventID(env.EventID)
	if err != nil {
		return fmt.Errorf("triagepipe: triaged id %s: %w", env.EventID, err)
	}
	needsReview := result.NeedsReview(triage.ReviewIntentProb)
	domain := TriagedPayload{
		SourceEventID: env.EventID,
		Channel:       payload.Channel,
		Text:          payload.Text,
		Intent:        string(result.Intent),
		IntentProb:    result.IntentProb,
		Frustration:   result.Frustration,
		ChurnRisk:     result.ChurnRisk,
		WantsHuman:    result.WantsHuman,
		Classifier:    result.Classifier,
		ModelVersion:  result.ModelVersion,
		Degraded:      result.Degraded,
		NeedsReview:   needsReview,
		CostUSD:       result.CostUSD,
	}
	payloadBytes, err := json.Marshal(domain)
	if err != nil {
		return fmt.Errorf("triagepipe: marshal result payload: %w", err)
	}

	outEnv := event.Envelope{
		Name:          event.NameMessageTriaged,
		EventID:       triagedID,
		OccurredAt:    env.OccurredAt,
		CustomerID:    env.CustomerID,
		SchemaVersion: event.SchemaVersionMVP,
		Payload:       domain,
	}
	body, err := outEnv.MarshalBody()
	if err != nil {
		return fmt.Errorf("triagepipe: marshal triaged body: %w", err)
	}

	err = store.WithTx(ctx, func(tx Tx) error {
		claimed, err := tx.ClaimInbox(ctx, env.EventID)
		if err != nil {
			return fmt.Errorf("triagepipe: claim inbox: %w", err)
		}
		if !claimed {
			return nil
		}
		if err := tx.InsertResult(ctx, ResultRow{
			ID:            triagedID,
			SourceEventID: env.EventID,
			CustomerID:    env.CustomerID,
			Intent:        string(result.Intent),
			IntentProb:    result.IntentProb,
			Frustration:   result.Frustration,
			ChurnRisk:     result.ChurnRisk,
			WantsHuman:    result.WantsHuman,
			Classifier:    result.Classifier,
			ModelVersion:  result.ModelVersion,
			Degraded:      result.Degraded,
			NeedsReview:   needsReview,
			CreatedAt:     env.OccurredAt,
			Payload:       payloadBytes,
		}); err != nil {
			return fmt.Errorf("triagepipe: insert result %s: %w", triagedID, err)
		}
		if err := tx.InsertOutbox(ctx, OutboxRow{
			EventID:    triagedID,
			RoutingKey: event.NameMessageTriaged,
			Payload:    body,
		}); err != nil {
			return fmt.Errorf("triagepipe: insert outbox %s: %w", triagedID, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("triagepipe: apply: %w", err)
	}
	return nil
}

// Publish sends each unpublished outbox row once. A broker error leaves that
// row and later rows unpublished. The row is marked published only after ack.
func Publish(ctx context.Context, store Store, broker Broker) error {
	if store == nil {
		return fmt.Errorf("triagepipe: store is required")
	}
	if broker == nil {
		return fmt.Errorf("triagepipe: broker is required")
	}

	rows, err := store.ListUnpublished(ctx)
	if err != nil {
		return fmt.Errorf("triagepipe: list unpublished: %w", err)
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("triagepipe: publish: %w", err)
		}
		if err := broker.Publish(ctx, row.RoutingKey, row.Payload); err != nil {
			return fmt.Errorf("triagepipe: publish %s: %w", row.EventID, err)
		}
		if err := store.MarkPublished(ctx, row.EventID, time.Now().UTC()); err != nil {
			return fmt.Errorf("triagepipe: mark published %s: %w", row.EventID, err)
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

// triagedEventID keeps the historical "tr-" prefix for non-UUIDv7 fixtures.
// A UUIDv7 source needs a fresh UUIDv7 because results.id and outbox.event_id are UUID columns.
func triagedEventID(source string) (string, error) {
	if _, err := identity.ParseV7(source); err == nil {
		return identity.NewV7()
	}
	return "tr-" + source, nil
}

func decodeMessagePayload(raw any) (sim.MessagePayload, error) {
	if raw == nil {
		return sim.MessagePayload{}, fmt.Errorf("payload is required")
	}
	switch v := raw.(type) {
	case sim.MessagePayload:
		return v, nil
	case *sim.MessagePayload:
		if v == nil {
			return sim.MessagePayload{}, fmt.Errorf("payload is required")
		}
		return *v, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return sim.MessagePayload{}, fmt.Errorf("encode payload: %w", err)
		}
		var p sim.MessagePayload
		if err := json.Unmarshal(data, &p); err != nil {
			return sim.MessagePayload{}, fmt.Errorf("decode payload: %w", err)
		}
		return p, nil
	}
}
