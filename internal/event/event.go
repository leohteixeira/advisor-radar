// Package event defines the shared RabbitMQ event envelope.
package event

import (
	"encoding/json"
	"fmt"
	"time"
)

// Known MVP event names. The name is the routing key, not a JSON body field.
const (
	NameAccountEventRecorded = "account.event.recorded"
	NameMessageReceived      = "message.received"
	NameMessageTriaged       = "message.triaged"
	NameAlertRaised          = "alert.raised"
	NameCaseOpened           = "case.opened"
	NameCaseStatusChanged    = "case.status.changed"
	NameCaseSLABreached      = "case.sla.breached"
)

// SchemaVersionMVP is the only schema version used by the MVP body.
const SchemaVersionMVP = 1

var knownNames = map[string]struct{}{
	NameAccountEventRecorded: {},
	NameMessageReceived:      {},
	NameMessageTriaged:       {},
	NameAlertRaised:          {},
	NameCaseOpened:           {},
	NameCaseStatusChanged:    {},
	NameCaseSLABreached:      {},
}

// Envelope is the shared event body plus the routing-key name.
type Envelope struct {
	Name          string    `json:"-"`
	EventID       string    `json:"event_id"`
	OccurredAt    time.Time `json:"occurred_at"`
	CustomerID    string    `json:"customer_id"`
	SchemaVersion int       `json:"schema_version"`
}

// Validate checks the envelope against the MVP contract.
func (e Envelope) Validate() error {
	if _, ok := knownNames[e.Name]; !ok {
		return fmt.Errorf("event: unknown name %q", e.Name)
	}
	if e.EventID == "" {
		return fmt.Errorf("event: event_id is required")
	}
	if e.CustomerID == "" {
		return fmt.Errorf("event: customer_id is required")
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("event: occurred_at is required")
	}
	if e.SchemaVersion < SchemaVersionMVP {
		return fmt.Errorf("event: schema_version %d is below %d", e.SchemaVersion, SchemaVersionMVP)
	}
	return nil
}

// MarshalBody encodes the four body fields only; the name is omitted.
func (e Envelope) MarshalBody() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, fmt.Errorf("event: marshal body: %w", err)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("event: marshal body: %w", err)
	}
	return data, nil
}
