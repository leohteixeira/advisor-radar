// Package advisory raises deterministic alert.raised events from the book.
package advisory

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/book"
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
// SourceSchemaVersion is the schema version of the account event that raised
// the alert, or 0 when the alert does not come from one.
type AlertRow struct {
	ID                  string
	CustomerID          string
	Kind                string
	Rule                string
	SourceEventID       string
	RaisedAt            time.Time
	Payload             []byte
	SourceSchemaVersion int
}

// AlertPayload is the domain payload on alert.raised. Money is whole USD
// dollars on every kind. A perfil alert also names the product bought
// (ProductID, AssetClass, Risk) and the profile it exceeds (Profile, MaxRisk).
type AlertPayload struct {
	Kind          string  `json:"kind"`
	Rule          string  `json:"rule"`
	SourceEventID string  `json:"source_event_id,omitempty"`
	Amount        float64 `json:"amount,omitempty"`
	Before        float64 `json:"before,omitempty"`
	After         float64 `json:"after,omitempty"`
	From          string  `json:"from,omitempty"`
	To            string  `json:"to,omitempty"`
	Days          int     `json:"days,omitempty"`
	ProductID     string  `json:"product_id,omitempty"`
	AssetClass    string  `json:"asset_class,omitempty"`
	Risk          int     `json:"risk,omitempty"`
	Profile       string  `json:"profile,omitempty"`
	MaxRisk       int     `json:"max_risk,omitempty"`
}

// Tx is the write side of one Apply transaction. InvestorProfile reads the
// book profile the suitability rule compares against; a customer outside the
// book wraps ErrUnknownCustomer.
type Tx interface {
	ClaimInbox(ctx context.Context, eventID string) (bool, error)
	InvestorProfile(ctx context.Context, customerID string) (string, error)
	InsertAlert(ctx context.Context, row AlertRow) error
	InsertOutbox(ctx context.Context, row OutboxRow) error
	UpdateBook(ctx context.Context, customerID string, aum float64, segment string) error
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
//
// Every schema version is scaled to dollars before a rule runs. From version
// 2 on, the event is a live POV fact and the book follows it: AUM becomes
// after, and the segment is derived from it, for every kind (aporte, saque,
// aplicacao, reavaliacao). An aplicacao is also checked against the book's
// investor profile, read in the same transaction.
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
	in := raiseInput{
		sourceEventID: env.EventID,
		schemaVersion: env.SchemaVersion,
		customerID:    env.CustomerID,
		occurredAt:    env.OccurredAt,
		decisions:     decisions,
		alertIDs:      nil,
	}
	if dollars.Kind == sim.KindAplicacao {
		if env.SchemaVersion < event.SchemaVersionPositions {
			return fmt.Errorf("advisory: apply %s: %w: aplicacao at schema_version %d", env.EventID, ErrInvalidPurchase, env.SchemaVersion)
		}
		in.purchase = &dollars
	}
	if env.SchemaVersion >= event.SchemaVersionCents {
		in.updateBook = true
		in.bookAUM = dollars.After
		in.bookSegment = book.SegmentFromAssets(dollars.After)
	}
	return raise(ctx, store, in)
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
	// schemaVersion is the source account event's; 0 for a silence fact.
	schemaVersion int
	customerID    string
	occurredAt    time.Time
	decisions     []Decision
	alertIDs      map[string]string
	updateBook    bool
	bookAUM       float64
	bookSegment   string
	// purchase is the dollar-scaled aplicacao the suitability rule checks
	// against the book profile inside the transaction; nil for other facts.
	purchase *sim.AccountPayload
}

// raise claims the inbox, writes the book, and stages one alert and outbox row
// per decision, all in one transaction. A quiet fact still claims the inbox,
// so it is not re-evaluated forever.
func raise(ctx context.Context, store Store, in raiseInput) error {
	err := store.WithTx(ctx, func(tx Tx) error {
		claimed, err := tx.ClaimInbox(ctx, in.sourceEventID)
		if err != nil {
			return fmt.Errorf("advisory: claim inbox: %w", err)
		}
		if !claimed {
			return nil
		}
		if err := writeBook(ctx, tx, in); err != nil {
			return err
		}
		decisions, err := withSuitability(ctx, tx, in)
		if err != nil {
			return err
		}

		for _, d := range decisions {
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
				Kind:          d.Kind,
				Rule:          d.Rule,
				SourceEventID: in.sourceEventID,
				Amount:        d.Amount,
				Before:        d.Before,
				After:         d.After,
				From:          d.From,
				To:            d.To,
				Days:          d.Days,
				ProductID:     d.ProductID,
				AssetClass:    d.AssetClass,
				Risk:          d.Risk,
				Profile:       d.Profile,
				MaxRisk:       d.MaxRisk,
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
				ID:                  alertID,
				CustomerID:          in.customerID,
				Kind:                d.Kind,
				Rule:                d.Rule,
				SourceEventID:       in.sourceEventID,
				RaisedAt:            in.occurredAt,
				Payload:             payloadBytes,
				SourceSchemaVersion: in.schemaVersion,
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

// withSuitability returns in.decisions plus the suitability decision of a
// purchase, which needs the book profile read inside tx.
func withSuitability(ctx context.Context, tx Tx, in raiseInput) ([]Decision, error) {
	if in.purchase == nil {
		return in.decisions, nil
	}
	profile, err := tx.InvestorProfile(ctx, in.customerID)
	if err != nil {
		return nil, fmt.Errorf("advisory: suitability profile: %w", err)
	}
	d, ok, err := EvaluateSuitability(*in.purchase, profile)
	if err != nil || !ok {
		return in.decisions, err
	}
	return append(slices.Clip(in.decisions), d), nil
}

func writeBook(ctx context.Context, tx Tx, in raiseInput) error {
	if !in.updateBook {
		return nil
	}
	if err := tx.UpdateBook(ctx, in.customerID, in.bookAUM, in.bookSegment); err != nil {
		return fmt.Errorf("advisory: update book: %w", err)
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
