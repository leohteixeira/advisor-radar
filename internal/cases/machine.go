package cases

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// Forward states in order. Escalation is a flag, not a fifth state.
const (
	StateAberto            = "Aberto"
	StateEmAtendimento     = "Em atendimento"
	StateAguardandoCliente = "Aguardando cliente"
	StateResolvido         = "Resolvido"
)

var stateOrder = []string{
	StateAberto,
	StateEmAtendimento,
	StateAguardandoCliente,
	StateResolvido,
}

// AtRiskRemainingMinutes is the SLA remaining threshold for the manager panel.
// A non-resolved case with this many minutes or fewer (or already overdue) is at risk.
const AtRiskRemainingMinutes = 60

// CaseRow is one cases table row.
type CaseRow struct {
	ID              string
	CustomerID      string
	SignalID        string
	AdvisorID       string
	State           string
	SLATotalMinutes int
	Escalated       bool
	OpenedAt        time.Time
}

// OutboxRow is one unpublished or published outbox record.
type OutboxRow struct {
	EventID    string
	RoutingKey string
	Payload    []byte
}

// DelayArm is one SLA delay armed inside a transaction.
// Published is true after the broker accepts the delay message.
type DelayArm struct {
	CaseID    string
	TTLMs     int
	Published bool
}

// CasePayload is the domain payload on case events.
type CasePayload struct {
	CaseID          string `json:"case_id"`
	State           string `json:"state"`
	Escalated       bool   `json:"escalated"`
	SLATotalMinutes int    `json:"sla_total_minutes"`
}

// OpenInput is the data needed to open a case in Aberto.
type OpenInput struct {
	ID         string
	CustomerID string
	SignalID   string
	AdvisorID  string
	Segment    string
	Factors    ClockFactors
	OccurredAt time.Time
}

// Tx is the write side of one Open / Advance / HandleBreach transaction.
type Tx interface {
	InsertCase(ctx context.Context, row CaseRow) error
	UpdateCase(ctx context.Context, row CaseRow) error
	GetCase(ctx context.Context, id string) (CaseRow, bool, error)
	InsertOutbox(ctx context.Context, row OutboxRow) error
	ArmDelay(ctx context.Context, caseID string, ttlMs int) error
	ClaimInbox(ctx context.Context, eventID string) (bool, error)
}

// Store persists cases, inbox, outbox, and delay arms. Declared here for the machine.
type Store interface {
	WithTx(ctx context.Context, fn func(Tx) error) error
	GetCase(ctx context.Context, id string) (CaseRow, bool, error)
	ListUnpublished(ctx context.Context) ([]OutboxRow, error)
	MarkPublished(ctx context.Context, eventID string, at time.Time) error
	ListUnpublishedDelays(ctx context.Context) ([]DelayArm, error)
	MarkDelayPublished(ctx context.Context, caseID string, at time.Time) error
}

// DelayBroker publishes one SLA delay message. The queue TTL is ttlMs.
type DelayBroker interface {
	PublishDelay(ctx context.Context, caseID string, ttlMs int, body []byte) error
}

// Broker accepts one event body under a routing key.
type Broker interface {
	Publish(ctx context.Context, routingKey string, body []byte) error
}

// Open creates a case in Aberto, writes case.opened, and arms one SLA delay.
func Open(ctx context.Context, store Store, in OpenInput) error {
	if store == nil {
		return fmt.Errorf("cases: store is required")
	}
	if in.ID == "" {
		return fmt.Errorf("cases: case id is required")
	}
	if in.CustomerID == "" {
		return fmt.Errorf("cases: customer id is required")
	}
	if in.AdvisorID == "" {
		return fmt.Errorf("cases: advisor id is required")
	}
	occurredAt := in.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	total := TotalMinutes(in.Segment, in.Factors)
	args := SLADelayArgs(total)
	row := CaseRow{
		ID:              in.ID,
		CustomerID:      in.CustomerID,
		SignalID:        in.SignalID,
		AdvisorID:       in.AdvisorID,
		State:           StateAberto,
		SLATotalMinutes: total,
		Escalated:       false,
		OpenedAt:        occurredAt,
	}
	body, err := marshalCaseEvent(event.NameCaseOpened, openedEventID(in.ID), in.CustomerID, occurredAt, row)
	if err != nil {
		return fmt.Errorf("cases: open %s: %w", in.ID, err)
	}

	err = store.WithTx(ctx, func(tx Tx) error {
		if err := tx.InsertCase(ctx, row); err != nil {
			return fmt.Errorf("cases: insert case %s: %w", in.ID, err)
		}
		if err := tx.InsertOutbox(ctx, OutboxRow{
			EventID:    openedEventID(in.ID),
			RoutingKey: event.NameCaseOpened,
			Payload:    body,
		}); err != nil {
			return fmt.Errorf("cases: insert outbox %s: %w", in.ID, err)
		}
		if err := tx.ArmDelay(ctx, in.ID, args.TTLMs); err != nil {
			return fmt.Errorf("cases: arm delay %s: %w", in.ID, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("cases: open: %w", err)
	}
	return nil
}

// Advance moves a case one forward step to the given state.
// A backward or past-end move is rejected and the state stays.
func Advance(ctx context.Context, store Store, caseID, to string) error {
	if store == nil {
		return fmt.Errorf("cases: store is required")
	}

	err := store.WithTx(ctx, func(tx Tx) error {
		row, ok, err := tx.GetCase(ctx, caseID)
		if err != nil {
			return fmt.Errorf("cases: get case %s: %w", caseID, err)
		}
		if !ok {
			return fmt.Errorf("cases: case %s not found", caseID)
		}
		next, ok := nextState(row.State)
		if !ok || to != next {
			return fmt.Errorf("cases: cannot advance %s from %s to %s", caseID, row.State, to)
		}
		row.State = to
		if err := tx.UpdateCase(ctx, row); err != nil {
			return fmt.Errorf("cases: update case %s: %w", caseID, err)
		}
		body, err := marshalCaseEvent(
			event.NameCaseStatusChanged,
			statusEventID(caseID, to),
			row.CustomerID,
			time.Now().UTC(),
			row,
		)
		if err != nil {
			return fmt.Errorf("cases: marshal status %s: %w", caseID, err)
		}
		if err := tx.InsertOutbox(ctx, OutboxRow{
			EventID:    statusEventID(caseID, to),
			RoutingKey: event.NameCaseStatusChanged,
			Payload:    body,
		}); err != nil {
			return fmt.Errorf("cases: insert outbox %s: %w", caseID, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("cases: advance: %w", err)
	}
	return nil
}

// RemainingMinutes returns SLA minutes left at now (may be negative when overdue).
func RemainingMinutes(row CaseRow, now time.Time) int {
	elapsed := int(now.Sub(row.OpenedAt).Minutes())
	return row.SLATotalMinutes - elapsed
}

// IsAtRisk reports whether a non-resolved case is at or past the at-risk threshold.
func IsAtRisk(row CaseRow, now time.Time) bool {
	if row.State == StateResolvido {
		return false
	}
	remaining := RemainingMinutes(row, now)
	return remaining <= AtRiskRemainingMinutes
}

// PermanentDeliveryError marks a dead-letter body that must not be requeued.
type PermanentDeliveryError struct {
	Err error
}

func (e PermanentDeliveryError) Error() string { return e.Err.Error() }
func (e PermanentDeliveryError) Unwrap() error { return e.Err }

// ApplyBreachDelivery decodes a dead-letter body and escalates the case.
// A plain-text body is the case id (as PublishDelays sends). A JSON envelope
// uses payload.case_id when present.
func ApplyBreachDelivery(ctx context.Context, store Store, body []byte) error {
	if store == nil {
		return fmt.Errorf("cases: store is required")
	}
	trimmed := string(body)
	if len(trimmed) > 0 && trimmed[0] != '{' {
		return HandleBreach(ctx, store, trimmed, time.Now().UTC())
	}

	var raw struct {
		EventID    string          `json:"event_id"`
		OccurredAt time.Time       `json:"occurred_at"`
		Payload    json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return PermanentDeliveryError{Err: fmt.Errorf("cases: decode delivery: %w", err)}
	}

	caseID := ""
	if len(raw.Payload) > 0 {
		var payload CasePayload
		if err := json.Unmarshal(raw.Payload, &payload); err == nil && payload.CaseID != "" {
			caseID = payload.CaseID
		}
	}
	if caseID == "" {
		caseID = raw.EventID
	}
	if caseID == "" {
		return PermanentDeliveryError{Err: fmt.Errorf("cases: case id is required")}
	}
	occurredAt := raw.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return HandleBreach(ctx, store, caseID, occurredAt)
}

// HandleBreach marks a case escalated after the SLA delay dead-letters.
// The state is unchanged. A second delivery writes no outbox row.
func HandleBreach(ctx context.Context, store Store, caseID string, occurredAt time.Time) error {
	if store == nil {
		return fmt.Errorf("cases: store is required")
	}
	if caseID == "" {
		return fmt.Errorf("cases: case id is required")
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	err := store.WithTx(ctx, func(tx Tx) error {
		claimed, err := tx.ClaimInbox(ctx, breachEventID(caseID))
		if err != nil {
			return fmt.Errorf("cases: claim inbox: %w", err)
		}
		if !claimed {
			return nil
		}
		row, ok, err := tx.GetCase(ctx, caseID)
		if err != nil {
			return fmt.Errorf("cases: get case %s: %w", caseID, err)
		}
		if !ok {
			return fmt.Errorf("cases: case %s not found", caseID)
		}
		row.Escalated = true
		if err := tx.UpdateCase(ctx, row); err != nil {
			return fmt.Errorf("cases: update case %s: %w", caseID, err)
		}
		body, err := marshalCaseEvent(
			event.NameCaseSLABreached,
			breachEventID(caseID),
			row.CustomerID,
			occurredAt,
			row,
		)
		if err != nil {
			return fmt.Errorf("cases: marshal breach %s: %w", caseID, err)
		}
		if err := tx.InsertOutbox(ctx, OutboxRow{
			EventID:    breachEventID(caseID),
			RoutingKey: event.NameCaseSLABreached,
			Payload:    body,
		}); err != nil {
			return fmt.Errorf("cases: insert outbox %s: %w", caseID, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("cases: handle breach: %w", err)
	}
	return nil
}

// Publish sends each unpublished outbox row once. A broker error leaves that
// row and later rows unpublished. The row is marked published only after ack.
func Publish(ctx context.Context, store Store, broker Broker) error {
	if store == nil {
		return fmt.Errorf("cases: store is required")
	}
	if broker == nil {
		return fmt.Errorf("cases: broker is required")
	}

	rows, err := store.ListUnpublished(ctx)
	if err != nil {
		return fmt.Errorf("cases: list unpublished: %w", err)
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("cases: publish: %w", err)
		}
		if err := broker.Publish(ctx, row.RoutingKey, row.Payload); err != nil {
			return fmt.Errorf("cases: publish %s: %w", row.EventID, err)
		}
		if err := store.MarkPublished(ctx, row.EventID, time.Now().UTC()); err != nil {
			return fmt.Errorf("cases: mark published %s: %w", row.EventID, err)
		}
	}
	return nil
}

// PublishDelays sends each armed delay once. The row is marked only after ack.
func PublishDelays(ctx context.Context, store Store, broker DelayBroker) error {
	if store == nil {
		return fmt.Errorf("cases: store is required")
	}
	if broker == nil {
		return fmt.Errorf("cases: delay broker is required")
	}
	rows, err := store.ListUnpublishedDelays(ctx)
	if err != nil {
		return fmt.Errorf("cases: list delays: %w", err)
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("cases: publish delay: %w", err)
		}
		if err := broker.PublishDelay(ctx, row.CaseID, row.TTLMs, []byte(row.CaseID)); err != nil {
			return fmt.Errorf("cases: publish delay %s: %w", row.CaseID, err)
		}
		if err := store.MarkDelayPublished(ctx, row.CaseID, time.Now().UTC()); err != nil {
			return fmt.Errorf("cases: mark delay %s: %w", row.CaseID, err)
		}
	}
	return nil
}

// RunPublisher polls unpublished rows until ctx is canceled.
// A broker that also implements DelayBroker publishes armed SLA delays.
func RunPublisher(ctx context.Context, store Store, broker Broker, every time.Duration) error {
	if every <= 0 {
		every = time.Second
	}
	delays, _ := broker.(DelayBroker)
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		if err := Publish(ctx, store, broker); err != nil && ctx.Err() != nil {
			return nil
		}
		if delays != nil {
			if err := PublishDelays(ctx, store, delays); err != nil && ctx.Err() != nil {
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

func nextState(current string) (string, bool) {
	for i, s := range stateOrder {
		if s == current {
			if i+1 >= len(stateOrder) {
				return "", false
			}
			return stateOrder[i+1], true
		}
	}
	return "", false
}

func openedEventID(caseID string) string { return "opened-" + caseID }
func statusEventID(caseID, state string) string {
	return "status-" + caseID + "-" + state
}
func breachEventID(caseID string) string { return "breach-" + caseID }

func marshalCaseEvent(
	name, eventID, customerID string,
	occurredAt time.Time,
	row CaseRow,
) ([]byte, error) {
	payload := CasePayload{
		CaseID:          row.ID,
		State:           row.State,
		Escalated:       row.Escalated,
		SLATotalMinutes: row.SLATotalMinutes,
	}
	env := event.Envelope{
		Name:          name,
		EventID:       eventID,
		OccurredAt:    occurredAt,
		CustomerID:    customerID,
		SchemaVersion: event.SchemaVersionMVP,
		Payload:       payload,
	}
	return env.MarshalBody()
}
