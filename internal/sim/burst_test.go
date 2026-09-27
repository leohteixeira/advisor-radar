package sim_test

import (
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

func TestMarketDay(t *testing.T) {
	t.Parallel()

	type want struct {
		eventID    string
		name       string
		customerID string
		payload    map[string]any
	}

	tests := []want{
		{
			eventID:    "md-n01",
			name:       event.NameMessageReceived,
			customerID: "c18",
			payload: map[string]any{
				"channel": "email",
				"text":    "Ninguém me responde há dois dias. Vou abrir reclamação no Reclame Aqui.",
			},
		},
		{
			eventID:    "md-n02",
			name:       event.NameAccountEventRecorded,
			customerID: "c19",
			payload: map[string]any{
				"kind":   "withdrawal",
				"amount": 55000.0,
				"before": 196000.0,
				"after":  141000.0,
			},
		},
		{
			eventID:    "md-n03",
			name:       event.NameMessageReceived,
			customerID: "c17",
			payload: map[string]any{
				"channel": "chat",
				"text":    "Bom dia! Como faço para ver o informe de rendimentos de 2025?",
			},
		},
		{
			eventID:    "md-n04",
			name:       event.NameMessageReceived,
			customerID: "c20",
			payload: map[string]any{
				"channel": "chat",
				"text":    "Preciso falar com uma pessoa, não com robô.",
			},
		},
		{
			eventID:    "md-n05",
			name:       event.NameAccountEventRecorded,
			customerID: "c22",
			payload: map[string]any{
				"kind":   "asset_drop",
				"amount": -12000.0,
				"before": 79000.0,
				"after":  67000.0,
			},
		},
		{
			eventID:    "md-n06",
			name:       event.NameMessageReceived,
			customerID: "c21",
			payload: map[string]any{
				"channel": "email",
				"text":    "Como declaro os dividendos recebidos em dólar?",
			},
		},
	}

	got := sim.MarketDay()
	if len(got) != len(tests) {
		t.Fatalf("MarketDay() len = %d, want %d", len(got), len(tests))
	}

	var messages, accounts int
	for i, tt := range tests {
		env := got[i]
		if env.EventID != tt.eventID {
			t.Errorf("[%d] EventID = %q, want %q", i, env.EventID, tt.eventID)
		}
		if env.Name != tt.name {
			t.Errorf("[%d] Name = %q, want %q", i, env.Name, tt.name)
		}
		if env.CustomerID != tt.customerID {
			t.Errorf("[%d] CustomerID = %q, want %q", i, env.CustomerID, tt.customerID)
		}
		if env.SchemaVersion != event.SchemaVersionMVP {
			t.Errorf("[%d] SchemaVersion = %d, want %d", i, env.SchemaVersion, event.SchemaVersionMVP)
		}
		if !env.OccurredAt.Equal(sim.MarketDayOccurredAt) {
			t.Errorf("[%d] OccurredAt = %v, want %v", i, env.OccurredAt, sim.MarketDayOccurredAt)
		}

		switch env.Name {
		case event.NameMessageReceived:
			messages++
			msg, ok := env.Payload.(sim.MessagePayload)
			if !ok {
				t.Fatalf("[%d] payload type = %T, want MessagePayload", i, env.Payload)
			}
			if msg.Channel != tt.payload["channel"] {
				t.Errorf("[%d] channel = %q, want %q", i, msg.Channel, tt.payload["channel"])
			}
			if msg.Text != tt.payload["text"] {
				t.Errorf("[%d] text = %q, want %q", i, msg.Text, tt.payload["text"])
			}
		case event.NameAccountEventRecorded:
			accounts++
			acc, ok := env.Payload.(sim.AccountPayload)
			if !ok {
				t.Fatalf("[%d] payload type = %T, want AccountPayload", i, env.Payload)
			}
			if acc.Kind != tt.payload["kind"] {
				t.Errorf("[%d] kind = %q, want %q", i, acc.Kind, tt.payload["kind"])
			}
			if acc.Amount != tt.payload["amount"].(float64) {
				t.Errorf("[%d] amount = %v, want %v", i, acc.Amount, tt.payload["amount"])
			}
			if acc.Before != tt.payload["before"].(float64) {
				t.Errorf("[%d] before = %v, want %v", i, acc.Before, tt.payload["before"])
			}
			if acc.After != tt.payload["after"].(float64) {
				t.Errorf("[%d] after = %v, want %v", i, acc.After, tt.payload["after"])
			}
		default:
			t.Fatalf("[%d] unexpected name %q", i, env.Name)
		}
	}

	if messages != 4 {
		t.Fatalf("message count = %d, want 4", messages)
	}
	if accounts != 2 {
		t.Fatalf("account event count = %d, want 2", accounts)
	}
}
