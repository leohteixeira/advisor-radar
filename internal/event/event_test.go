package event_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

func TestEnvelope_ValidateAndMarshalBody(t *testing.T) {
	t.Parallel()

	validTime := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

	type tc struct {
		name    string
		env     event.Envelope
		wantErr string
		wantOK  bool
	}

	tests := []tc{
		{
			name: "missing event_id",
			env: event.Envelope{
				Name:          event.NameMessageReceived,
				EventID:       "",
				OccurredAt:    validTime,
				CustomerID:    "cust-1",
				SchemaVersion: event.SchemaVersionMVP,
			},
			wantErr: "event_id",
		},
		{
			name: "missing customer_id",
			env: event.Envelope{
				Name:          event.NameMessageReceived,
				EventID:       "evt-1",
				OccurredAt:    validTime,
				CustomerID:    "",
				SchemaVersion: event.SchemaVersionMVP,
			},
			wantErr: "customer_id",
		},
		{
			name: "zero time",
			env: event.Envelope{
				Name:          event.NameAlertRaised,
				EventID:       "evt-1",
				OccurredAt:    time.Time{},
				CustomerID:    "cust-1",
				SchemaVersion: event.SchemaVersionMVP,
			},
			wantErr: "occurred_at",
		},
		{
			name: "bad schema",
			env: event.Envelope{
				Name:          event.NameCaseOpened,
				EventID:       "evt-1",
				OccurredAt:    validTime,
				CustomerID:    "cust-1",
				SchemaVersion: 0,
			},
			wantErr: "schema_version",
		},
		{
			name: "unsupported schema version 4",
			env: event.Envelope{
				Name:          event.NameAccountEventRecorded,
				EventID:       "evt-1",
				OccurredAt:    validTime,
				CustomerID:    "cust-1",
				SchemaVersion: 4,
			},
			wantErr: "schema_version",
		},
		{
			name: "valid schema version 3",
			env: event.Envelope{
				Name:          event.NameAccountEventRecorded,
				EventID:       "evt-1",
				OccurredAt:    validTime,
				CustomerID:    "cust-1",
				SchemaVersion: event.SchemaVersionPositions,
			},
			wantOK: true,
		},
		{
			name: "unknown name",
			env: event.Envelope{
				Name:          "message.reclassify.requested",
				EventID:       "evt-1",
				OccurredAt:    validTime,
				CustomerID:    "cust-1",
				SchemaVersion: event.SchemaVersionMVP,
			},
			wantErr: "unknown name",
		},
		{
			name: "valid schema version 2",
			env: event.Envelope{
				Name:          event.NameAccountEventRecorded,
				EventID:       "evt-1",
				OccurredAt:    validTime,
				CustomerID:    "cust-1",
				SchemaVersion: event.SchemaVersionCents,
			},
			wantOK: true,
		},
	}

	for _, known := range []string{
		event.NameAccountEventRecorded,
		event.NameMessageReceived,
		event.NameMessageTriaged,
		event.NameAlertRaised,
		event.NameCaseOpened,
		event.NameCaseStatusChanged,
		event.NameCaseSLABreached,
	} {
		tests = append(tests, tc{
			name: "valid " + known,
			env: event.Envelope{
				Name:          known,
				EventID:       "evt-1",
				OccurredAt:    validTime,
				CustomerID:    "cust-1",
				SchemaVersion: event.SchemaVersionMVP,
			},
			wantOK: true,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.env.Validate()
			if tt.wantOK {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				body, err := tt.env.MarshalBody()
				if err != nil {
					t.Fatalf("MarshalBody() unexpected error: %v", err)
				}
				var fields map[string]any
				if err := json.Unmarshal(body, &fields); err != nil {
					t.Fatalf("json.Unmarshal: %v", err)
				}
				if len(fields) != 4 {
					t.Fatalf("body field count = %d, want 4: %v", len(fields), fields)
				}
				for _, key := range []string{"event_id", "occurred_at", "customer_id", "schema_version"} {
					if _, ok := fields[key]; !ok {
						t.Fatalf("body missing %q", key)
					}
				}
				gotVersion, ok := fields["schema_version"].(float64)
				if !ok {
					t.Fatalf("schema_version type = %T, want number", fields["schema_version"])
				}
				if int(gotVersion) != tt.env.SchemaVersion {
					t.Fatalf("schema_version = %d, want %d", int(gotVersion), tt.env.SchemaVersion)
				}
				if _, ok := fields["name"]; ok {
					t.Fatal("body must omit name")
				}
				if _, ok := fields["payload"]; ok {
					t.Fatal("nil payload must be omitted from body")
				}
				return
			}

			if err == nil {
				t.Fatal("Validate() error = nil, want wrapped validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
			if _, marshalErr := tt.env.MarshalBody(); marshalErr == nil {
				t.Fatal("MarshalBody() error = nil, want wrapped validation error")
			}
		})
	}
}

func TestEnvelope_MarshalBody_WithPayload(t *testing.T) {
	t.Parallel()

	env := event.Envelope{
		Name:          event.NameMessageReceived,
		EventID:       "md-n01",
		OccurredAt:    time.Date(2026, 9, 27, 15, 0, 0, 0, time.UTC),
		CustomerID:    "c18",
		SchemaVersion: event.SchemaVersionMVP,
		Payload: map[string]any{
			"channel": "email",
			"text":    "hello",
		},
	}

	body, err := env.MarshalBody()
	if err != nil {
		t.Fatalf("MarshalBody: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(fields) != 5 {
		t.Fatalf("body field count = %d, want 5: %v", len(fields), fields)
	}
	for _, key := range []string{"event_id", "occurred_at", "customer_id", "schema_version", "payload"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("body missing %q", key)
		}
	}
	payload, ok := fields["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want object", fields["payload"])
	}
	if payload["channel"] != "email" {
		t.Fatalf("payload.channel = %v, want email", payload["channel"])
	}
}

func TestKnownSchemaVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version int
		want    bool
	}{
		{name: "zero", version: 0, want: false},
		{name: "dollars", version: event.SchemaVersionMVP, want: true},
		{name: "cents", version: event.SchemaVersionCents, want: true},
		{name: "positions", version: event.SchemaVersionPositions, want: true},
		{name: "future", version: 4, want: false},
		{name: "negative", version: -1, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := event.KnownSchemaVersion(tt.version); got != tt.want {
				t.Fatalf("KnownSchemaVersion(%d) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
