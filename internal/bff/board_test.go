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

func TestBoard_UnknownAlertKindIsPermanent(t *testing.T) {
	t.Parallel()
	board := bff.NewBoard()
	body := envelopeJSON(t, identity.MustNewV7(), identity.MustNewV7(), map[string]any{"kind": "suitability"})
	err := board.ApplyDelivery(context.Background(), "alert.raised", body)
	if err == nil || !bff.IsPermanent(err) {
		t.Fatalf("err = %v, want permanent", err)
	}
}
