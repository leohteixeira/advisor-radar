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

// Schema versions accepted on the shared envelope. Version 3 keeps the
// integer USD cents of version 2 and adds the per-product fields of the
// positions model (ADR 0010).
const (
	SchemaVersionMVP       = 1 // whole USD dollars
	SchemaVersionCents     = 2 // integer USD cents
	SchemaVersionPositions = 3 // integer USD cents plus product fields
)

// KnownSchemaVersion reports whether v is a schema version consumers accept.
// A version outside 1–3 is rejected, never scaled.
func KnownSchemaVersion(v int) bool {
	return v >= SchemaVersionMVP && v <= SchemaVersionPositions
}

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
// Payload is optional domain facts; MarshalBody omits it when nil.
type Envelope struct {
	Name          string    `json:"-"`
	EventID       string    `json:"event_id"`
	OccurredAt    time.Time `json:"occurred_at"`
	CustomerID    string    `json:"customer_id"`
	SchemaVersion int       `json:"schema_version"`
	Payload       any       `json:"payload,omitempty"`
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
	if !KnownSchemaVersion(e.SchemaVersion) {
		return fmt.Errorf("event: unsupported schema_version %d", e.SchemaVersion)
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
