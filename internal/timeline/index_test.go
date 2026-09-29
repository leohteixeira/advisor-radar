package timeline_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

func seedCustomer(t *testing.T, idx *timeline.Index, customerID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	rows := []struct {
		key     string
		ago     time.Duration
		payload map[string]any
	}{
		{event.NameMessageTriaged, 12 * time.Minute, map[string]any{"text": "Se isso não for resolvido hoje", "intent": "Reclamação", "channel": "chat", "meta": "Reclamação"}},
		{event.NameMessageReceived, 1500 * time.Minute, map[string]any{"text": "transferência", "channel": "e-mail", "title": "Mensagem · e-mail", "meta": "Operacional"}},
		{"advisory.note.recorded", 2900 * time.Minute, map[string]any{"kind": "nota", "title": "Nota do assessor", "text": "Cliente pretende comprar imóvel em Orlando", "meta": "Ana"}},
		{event.NameAccountEventRecorded, 4400 * time.Minute, map[string]any{"kind": "saque", "title": "Saque", "text": "US$ 20.000", "meta": "7%"}},
		{event.NameCaseStatusChanged, 10100 * time.Minute, map[string]any{"case_id": "k1", "state": "Resolvido", "text": "DARF", "meta": "Tributação"}},
		{event.NameAccountEventRecorded, 21000 * time.Minute, map[string]any{"kind": "aporte", "title": "Aporte", "text": "US$ 45.000", "meta": "câmbio"}},
		{"advisory.note.recorded", 43000 * time.Minute, map[string]any{"kind": "telefone", "title": "Ligação", "text": "Revisão", "meta": "Ana"}},
	}
	for _, r := range rows {
		eid := identity.MustNewV7()
		body, _ := json.Marshal(map[string]any{
			"event_id": eid, "occurred_at": now.Add(-r.ago),
			"customer_id": customerID, "schema_version": 1, "payload": r.payload,
		})
		if _, _, err := idx.ApplyDelivery(ctx, r.key, body); err != nil {
			t.Fatalf("apply %s: %v", r.key, err)
		}
	}
}

func TestIndex_SearchOrdersByAgoAscending(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	idx := timeline.NewIndex()
	seedCustomer(t, idx, cust)
	items, err := idx.Search(context.Background(), cust, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 3 {
		t.Fatalf("items = %d, want at least 3", len(items))
	}
	// Seed includes ago ~12 (message), ~2900 (note), ~21000 (aporte).
	agos := make([]int, len(items))
	for i, e := range items {
		agos[i] = e.Ago
	}
	for i := 1; i < len(agos); i++ {
		if agos[i] < agos[i-1] {
			t.Fatalf("agos not ascending: %v", agos)
		}
	}
	if items[0].Ago > 20 {
		t.Fatalf("first ago = %d, want ~12 (most recent)", items[0].Ago)
	}
	found2900, found21000 := false, false
	var i12, i2900, i21000 int
	for i, e := range items {
		switch {
		case e.Ago >= 10 && e.Ago <= 20:
			i12 = i
		case e.Ago >= 2800 && e.Ago <= 3000:
			found2900 = true
			i2900 = i
		case e.Ago >= 20000 && e.Ago <= 22000:
			found21000 = true
			i21000 = i
		}
	}
	if !found2900 || !found21000 {
		t.Fatalf("missing expected ages in %v", agos)
	}
	if !(i12 < i2900 && i2900 < i21000) {
		t.Fatalf("want ago 12 before 2900 before 21000; indexes %d %d %d in %v", i12, i2900, i21000, agos)
	}
}

func TestIndex_SearchOrlando(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	idx := timeline.NewIndex()
	seedCustomer(t, idx, cust)
	items, err := idx.Search(context.Background(), cust, "Orlando", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
}

func TestIndex_ChipNotas(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	idx := timeline.NewIndex()
	seedCustomer(t, idx, cust)
	items, err := idx.Search(context.Background(), cust, "", "notas")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != "nota" {
		t.Fatalf("items = %+v, want one nota", items)
	}
}

func TestIndex_SeedSevenRows(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	idx := timeline.NewIndex()
	seedCustomer(t, idx, cust)
	items, err := idx.Search(context.Background(), cust, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 7 {
		t.Fatalf("items = %d, want 7", len(items))
	}
}

func TestIndex_TelefoneOnlyOnTodos(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	idx := timeline.NewIndex()
	seedCustomer(t, idx, cust)
	ctx := context.Background()
	all, err := idx.Search(ctx, cust, "", "todos")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range all {
		if e.Kind == "telefone" {
			found = true
		}
	}
	if !found {
		t.Fatal("telefone missing on Todos")
	}
	conta, err := idx.Search(ctx, cust, "", "conta")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range conta {
		if e.Kind == "telefone" {
			t.Fatal("telefone must not appear on conta")
		}
	}
}

func TestIndex_ChipContaMensagensCasos(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	idx := timeline.NewIndex()
	seedCustomer(t, idx, cust)
	ctx := context.Background()
	conta, err := idx.Search(ctx, cust, "", "conta")
	if err != nil {
		t.Fatal(err)
	}
	var saque, aporte bool
	for _, e := range conta {
		if e.Kind == "saque" {
			saque = true
		}
		if e.Kind == "aporte" {
			aporte = true
		}
	}
	if !saque || !aporte {
		t.Fatalf("conta missing saque/aporte: saque=%v aporte=%v", saque, aporte)
	}
	msgs, err := idx.Search(ctx, cust, "", "mensagens")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) < 1 {
		t.Fatal("want messages")
	}
	casos, err := idx.Search(ctx, cust, "", "casos")
	if err != nil {
		t.Fatal(err)
	}
	if len(casos) < 1 {
		t.Fatal("want cases")
	}
}

func TestIndex_IdempotentApply(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	eid := identity.MustNewV7()
	body, _ := json.Marshal(map[string]any{
		"event_id": eid, "occurred_at": time.Now().UTC(),
		"customer_id": cust, "schema_version": 1,
		"payload": map[string]any{"kind": "saque", "text": "x"},
	})
	ctx := context.Background()
	a1, _, err := idx.ApplyDelivery(ctx, event.NameAccountEventRecorded, body)
	if err != nil || !a1 {
		t.Fatalf("first apply: %v %v", a1, err)
	}
	a2, _, err := idx.ApplyDelivery(ctx, event.NameAccountEventRecorded, body)
	if err != nil || a2 {
		t.Fatalf("second apply: %v %v", a2, err)
	}
}

func TestIndex_ApplyDeliveryKeepsSourceAndTime(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	at := time.Date(2026, 9, 28, 12, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	body, _ := json.Marshal(map[string]any{
		"event_id": identity.MustNewV7(), "occurred_at": at,
		"customer_id": cust, "schema_version": 2,
		"payload": map[string]any{"kind": "aporte"},
	})
	applied, entry, err := idx.ApplyDelivery(context.Background(), event.NameAccountEventRecorded, body)
	if err != nil || !applied {
		t.Fatalf("apply: %v %v", applied, err)
	}
	if entry.Source != event.NameAccountEventRecorded || !entry.OccurredAt.Equal(at) || entry.OccurredAt.Location() != time.UTC {
		t.Errorf("entry source, occurred_at = %q, %v", entry.Source, entry.OccurredAt)
	}
	rows, err := idx.Search(context.Background(), cust, "", "")
	if err != nil || len(rows) != 1 || rows[0].Source != event.NameAccountEventRecorded || !rows[0].OccurredAt.Equal(at) {
		t.Errorf("search = %+v, %v", rows, err)
	}
}
