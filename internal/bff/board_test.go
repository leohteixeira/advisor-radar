package bff_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

func envelopeJSON(t *testing.T, eventID, customerID string, payload map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"event_id": eventID, "occurred_at": time.Now().UTC(),
		"customer_id": customerID, "schema_version": 1, "payload": payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestBoard_Empty(t *testing.T) {
	t.Parallel()
	if len(bff.NewBoard().Items()) != 0 {
		t.Fatal("want empty board")
	}
}

func TestBoard_Alert(t *testing.T) {
	t.Parallel()
	board := bff.NewBoard()
	eid, cust := identity.MustNewV7(), identity.MustNewV7()
	body := envelopeJSON(t, eid, cust, map[string]any{"kind": "saque", "rule": "r", "amount": 1.0})
	if err := board.ApplyDelivery(context.Background(), "alert.raised", body); err != nil {
		t.Fatal(err)
	}
	items := board.Items()
	if len(items) != 1 || items[0].ID != eid || items[0].Alert != "saque" {
		t.Fatalf("%+v", items)
	}
}

func TestBoard_Message(t *testing.T) {
	t.Parallel()
	board := bff.NewBoard()
	eid, cust := identity.MustNewV7(), identity.MustNewV7()
	body := envelopeJSON(t, eid, cust, map[string]any{
		"text": "oi", "intent": "operacional", "intent_prob": 0.9,
		"frustration": 0, "churn_risk": 0.1, "wants_human": 0.1, "channel": "chat",
	})
	if err := board.ApplyDelivery(context.Background(), "message.triaged", body); err != nil {
		t.Fatal(err)
	}
	if len(board.Items()) != 1 {
		t.Fatalf("%d", len(board.Items()))
	}
}

// A live message keeps the id of the message.received event it was triaged
// from, which is the id a case opened by the intake stores. The id stays out
// of the JSON the browser receives.
func TestBoard_MessageKeepsItsSourceID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
	}{
		{name: "triaged event names its source", source: identity.MustNewV7()},
		{name: "older triage without a source id", source: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := bff.NewBoard()
			eid, cust := identity.MustNewV7(), identity.MustNewV7()
			payload := map[string]any{"text": "oi", "intent": "operacional", "intent_prob": 0.9, "channel": "chat"}
			if tt.source != "" {
				payload["source_event_id"] = tt.source
			}
			if err := board.ApplyDelivery(t.Context(), "message.triaged", envelopeJSON(t, eid, cust, payload)); err != nil {
				t.Fatal(err)
			}
			items := board.Items()
			if len(items) != 1 || items[0].ID != eid || items[0].SourceID != tt.source {
				t.Fatalf("items = %+v, want id %s and source %q", items, eid, tt.source)
			}
			raw, err := json.Marshal(items[0])
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]any
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			for key, value := range wire {
				if key != "id" && value == tt.source && tt.source != "" {
					t.Fatalf("signal JSON carries the source id under %q", key)
				}
			}
		})
	}
}

func TestBoard_Duplicate(t *testing.T) {
	t.Parallel()
	board := bff.NewBoard()
	eid, cust := identity.MustNewV7(), identity.MustNewV7()
	body := envelopeJSON(t, eid, cust, map[string]any{"kind": "saque", "rule": "r"})
	ctx := context.Background()
	if err := board.ApplyDelivery(ctx, "alert.raised", body); err != nil {
		t.Fatal(err)
	}
	if err := board.ApplyDelivery(ctx, "alert.raised", body); err != nil {
		t.Fatal(err)
	}
	if len(board.Items()) != 1 {
		t.Fatalf("%d", len(board.Items()))
	}
}

func TestBoard_BadBody(t *testing.T) {
	t.Parallel()
	board := bff.NewBoard()
	err := board.ApplyDelivery(context.Background(), "alert.raised", []byte("{"))
	if err == nil || !bff.IsPermanent(err) {
		t.Fatalf("err = %v", err)
	}
	if len(board.Items()) != 0 {
		t.Fatal("want empty")
	}
}

func TestBoard_PerfilAlert(t *testing.T) {
	t.Parallel()
	board := bff.NewBoard()
	eid, cust := identity.MustNewV7(), identity.MustNewV7()
	body := envelopeJSON(t, eid, cust, map[string]any{
		"kind": "perfil", "rule": "Compra acima do perfil de investidor",
		"amount": 1000.0, "before": 8200.0, "after": 8200.0,
		"product_id": "cobalto", "asset_class": "acoes", "risk": 5, "profile": "conservador", "max_risk": 2,
	})
	if err := board.ApplyDelivery(context.Background(), "alert.raised", body); err != nil {
		t.Fatal(err)
	}
	items := board.Items()
	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
	got := items[0]
	if got.Kind != "alert" || got.Alert != "perfil" || got.Rule != "Compra acima do perfil de investidor" ||
		got.Amount != 1000 || got.ProductID != "cobalto" || got.Risk != 5 || got.Profile != "conservador" || got.MaxRisk != 2 {
		t.Fatalf("signal = %+v", got)
	}
}

// A live alert card carries the description advisory wrote for it, on every
// kind but perfil, whose reason the BFF writes with the product name.
func TestBoard_AlertReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		payload  map[string]any
		expected string
	}{
		{
			name:     "withdrawal",
			payload:  map[string]any{"kind": "saque", "rule": "r", "amount": 59000.0, "reason": "Saque de US$ 59.000,00, 87% do patrimônio"},
			expected: "Saque de US$ 59.000,00, 87% do patrimônio",
		},
		{
			name:     "segment downgrade",
			payload:  map[string]any{"kind": "segmento", "rule": "r", "from": "Advance", "to": "Essencial", "reason": "Desceu de Advance para Essencial"},
			expected: "Desceu de Advance para Essencial",
		},
		{
			name:     "deposit",
			payload:  map[string]any{"kind": "aporte", "rule": "r", "amount": 60000.0, "reason": "Aporte de US$ 60.000,00, 7,5× o patrimônio"},
			expected: "Aporte de US$ 60.000,00, 7,5× o patrimônio",
		},
		{
			name:     "drop",
			payload:  map[string]any{"kind": "queda", "rule": "r", "reason": "Patrimônio caiu 17%, de US$ 130.000,00 para US$ 108.000,00"},
			expected: "Patrimônio caiu 17%, de US$ 130.000,00 para US$ 108.000,00",
		},
		{
			name:     "silence",
			payload:  map[string]any{"kind": "contato", "rule": "r", "days": 94, "reason": "Último contato há 94 dias"},
			expected: "Último contato há 94 dias",
		},
		{
			name:     "perfil keeps the reason for the BFF to write",
			payload:  map[string]any{"kind": "perfil", "rule": "r", "product_id": "cobalto", "reason": "Compra de US$ 1.000,00 em produto de risco 5."},
			expected: "",
		},
		{
			name:     "alert from before advisory sent a reason",
			payload:  map[string]any{"kind": "segmento", "rule": "r", "from": "Advance", "to": "Essencial"},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := bff.NewBoard()
			body := envelopeJSON(t, identity.MustNewV7(), identity.MustNewV7(), tt.payload)
			if err := board.ApplyDelivery(context.Background(), "alert.raised", body); err != nil {
				t.Fatal(err)
			}
			items := board.Items()
			if len(items) != 1 || items[0].Reason != tt.expected {
				t.Fatalf("items = %+v, want reason %q", items, tt.expected)
			}
		})
	}
}

func TestBoard_UnknownAlertKindIsPermanent(t *testing.T) {
	t.Parallel()
	board := bff.NewBoard()
	body := envelopeJSON(t, identity.MustNewV7(), identity.MustNewV7(), map[string]any{"kind": "suitability"})
	err := board.ApplyDelivery(context.Background(), "alert.raised", body)
	if err == nil || !bff.IsPermanent(err) {
		t.Fatalf("err = %v, want permanent", err)
	}
}
