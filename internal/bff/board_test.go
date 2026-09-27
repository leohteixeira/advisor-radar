package bff_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/bff"
)

func TestBoard_Seed(t *testing.T) {
	t.Parallel()
	board := bff.NewBoard()
	items := board.Items()
	if len(items) != 17 {
		t.Fatalf("items = %d, want 17", len(items))
	}
	for i := 0; i < 17; i++ {
		want := fmt.Sprintf("s%02d", i+1)
		if items[i].ID != want {
			t.Fatalf("items[%d].ID = %q, want %q", i, items[i].ID, want)
		}
	}
}

func TestBoard_Alert(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	board := bff.NewBoard()
	body := envelopeJSON(t, "al-md-n02-withdrawal", "c19", map[string]any{
		"kind":   "saque",
		"rule":   "Saque acima de 20% do patrimônio em 24 horas",
		"amount": 55000.0,
		"before": 196000.0,
		"after":  141000.0,
	})
	ch, unsub := board.Subscribe("")
	defer unsub()
	if err := board.ApplyDelivery(ctx, "alert.raised", body); err != nil {
		t.Fatalf("ApplyDelivery: %v", err)
	}
	items := board.Items()
	if len(items) != 18 {
		t.Fatalf("items = %d, want 18", len(items))
	}
	got := items[len(items)-1]
	if got.ID != "al-md-n02-withdrawal" {
		t.Fatalf("id = %q, want al-md-n02-withdrawal", got.ID)
	}
	if got.Alert != "saque" {
		t.Fatalf("alert = %q, want saque", got.Alert)
	}
	if got.Client != "c19" {
		t.Fatalf("client = %q, want c19", got.Client)
	}
	select {
	case ev := <-ch:
		if ev.ID != "al-md-n02-withdrawal" {
			t.Fatalf("sse id = %q", ev.ID)
		}
		var sig bff.Signal
		if err := json.Unmarshal(ev.Data, &sig); err != nil {
			t.Fatalf("sse data: %v", err)
		}
		if sig.Alert != "saque" {
			t.Fatalf("sse alert = %q, want saque", sig.Alert)
		}
	case <-time.After(time.Second):
		t.Fatal("expected one SSE event")
	}
}

func TestBoard_Message(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	board := bff.NewBoard()
	body := envelopeJSON(t, "tr-md-n01", "c18", map[string]any{
		"text":        "ignored in assertions",
		"intent":      "reclamacao",
		"intent_prob": 0.89,
		"frustration": 3.2,
		"churn_risk":  0.7,
		"wants_human": 0.1,
		"degraded":    false,
		"channel":     "e-mail",
	})
	ch, unsub := board.Subscribe("")
	defer unsub()
	if err := board.ApplyDelivery(ctx, "message.triaged", body); err != nil {
		t.Fatalf("ApplyDelivery: %v", err)
	}
	items := board.Items()
	if len(items) != 18 {
		t.Fatalf("items = %d, want 18", len(items))
	}
	got := items[len(items)-1]
	if got.ID != "tr-md-n01" {
		t.Fatalf("id = %q, want tr-md-n01", got.ID)
	}
	if got.Intent != "Reclamação" {
		t.Fatalf("intent = %q, want Reclamação", got.Intent)
	}
	if got.Frustration == nil || *got.Frustration != 3 {
		t.Fatalf("frustration = %v, want 3", got.Frustration)
	}
	if got.Churn == nil || !*got.Churn {
		t.Fatalf("churn = %v, want true", got.Churn)
	}
	if got.Human == nil || *got.Human {
		t.Fatalf("human = %v, want false", got.Human)
	}
	if got.Fallback {
		t.Fatalf("fallback = true, want false")
	}
	if got.Channel != "e-mail" {
		t.Fatalf("channel = %q, want e-mail", got.Channel)
	}
	if got.ChurnConf != "média" {
		t.Fatalf("churnConf = %q, want média", got.ChurnConf)
	}
	select {
	case ev := <-ch:
		if ev.ID != "tr-md-n01" {
			t.Fatalf("sse id = %q", ev.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("expected one SSE event")
	}
}

func TestBoard_Duplicate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	board := bff.NewBoard()
	body := envelopeJSON(t, "al-md-n02-withdrawal", "c19", map[string]any{
		"kind": "saque",
		"rule": "Saque acima de 20% do patrimônio em 24 horas",
	})
	ch, unsub := board.Subscribe("")
	defer unsub()
	if err := board.ApplyDelivery(ctx, "alert.raised", body); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := board.ApplyDelivery(ctx, "alert.raised", body); err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(board.Items()) != 18 {
		t.Fatalf("items = %d, want 18", len(board.Items()))
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected first SSE event")
	}
	select {
	case <-ch:
		t.Fatal("unexpected second SSE event")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBoard_BadBody(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	board := bff.NewBoard()
	err := board.ApplyDelivery(ctx, "alert.raised", []byte(`not-json`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !bff.IsPermanent(err) {
		t.Fatalf("error %v should be permanent", err)
	}
	if len(board.Items()) != 17 {
		t.Fatalf("items = %d, want 17", len(board.Items()))
	}
}

func envelopeJSON(t *testing.T, eventID, customerID string, payload map[string]any) []byte {
	t.Helper()
	env := map[string]any{
		"event_id":       eventID,
		"occurred_at":    time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
		"customer_id":    customerID,
		"schema_version": 1,
		"payload":        payload,
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
