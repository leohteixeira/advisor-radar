package timeline_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

func TestIndex_SearchOrlando(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	items, err := idx.Search(context.Background(), "c01", "Orlando", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Kind != "nota" {
		t.Fatalf("kind = %q, want nota", items[0].Kind)
	}
	for _, it := range items {
		if it.Kind == "saque" {
			t.Fatal("saque row must not match Orlando")
		}
	}
}

func TestIndex_ChipNotas(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	items, err := idx.Search(context.Background(), "c01", "", "notas")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(items) != 1 || items[0].Kind != "nota" {
		t.Fatalf("items = %+v, want one nota", items)
	}
}

func TestIndex_EmptyCustomer(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	items, err := idx.Search(context.Background(), "c99", "", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if items == nil {
		t.Fatal("items must be empty slice, not nil")
	}
	if len(items) != 0 {
		t.Fatalf("items = %d, want 0", len(items))
	}
}

func TestIndex_SeedSevenRows(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	items, err := idx.Search(context.Background(), "c01", "", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(items) != 7 {
		t.Fatalf("items = %d, want 7", len(items))
	}
	var hasOrlando, hasDARF, hasSaque, hasMsg bool
	for _, it := range items {
		if strings.Contains(it.Text, "Orlando") {
			hasOrlando = true
		}
		if strings.Contains(it.Text, "DARF") {
			hasDARF = true
		}
		if it.Kind == "saque" {
			hasSaque = true
		}
		if it.Kind == "mensagem" {
			hasMsg = true
		}
	}
	if !hasOrlando || !hasDARF || !hasSaque || !hasMsg {
		t.Fatalf("seed missing expected rows: orlando=%v darf=%v saque=%v msg=%v", hasOrlando, hasDARF, hasSaque, hasMsg)
	}
}

func TestIndex_IdempotentApply(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	body, err := json.Marshal(map[string]any{
		"event_id":       "ev-dup-1",
		"occurred_at":    time.Now().UTC(),
		"customer_id":    "c50",
		"schema_version": 1,
		"payload": map[string]any{
			"kind":   "saque",
			"title":  "Saque",
			"text":   "US$ 1.000,00",
			"meta":   "teste",
			"amount": 1000,
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	ctx := context.Background()
	applied, _, err := idx.ApplyDelivery(ctx, event.NameAccountEventRecorded, body)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !applied {
		t.Fatal("first delivery should apply")
	}
	applied, _, err = idx.ApplyDelivery(ctx, event.NameAccountEventRecorded, body)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if applied {
		t.Fatal("second delivery must be a no-op")
	}
	items, err := idx.Search(ctx, "c50", "", "")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1 after duplicate delivery", len(items))
	}
}

func TestIndex_TelefoneOnlyOnTodos(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	ctx := context.Background()
	all, err := idx.Search(ctx, "c01", "", "todos")
	if err != nil {
		t.Fatalf("todos: %v", err)
	}
	found := false
	for _, it := range all {
		if it.Kind == "telefone" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("telefone missing on Todos")
	}
	conta, err := idx.Search(ctx, "c01", "", "conta")
	if err != nil {
		t.Fatalf("conta: %v", err)
	}
	for _, it := range conta {
		if it.Kind == "telefone" {
			t.Fatal("telefone must not appear on Conta chip")
		}
	}
}

func TestIndex_ChipContaMensagensCasos(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	ctx := context.Background()

	conta, err := idx.Search(ctx, "c01", "", "conta")
	if err != nil {
		t.Fatalf("conta: %v", err)
	}
	var hasSaque, hasAporte bool
	for _, it := range conta {
		if _, ok := map[string]struct{}{"saque": {}, "aporte": {}, "queda": {}, "segmento": {}, "contato": {}}[it.Kind]; !ok {
			t.Fatalf("unexpected conta kind %q", it.Kind)
		}
		if it.Kind == "saque" {
			hasSaque = true
		}
		if it.Kind == "aporte" {
			hasAporte = true
		}
	}
	if !hasSaque || !hasAporte {
		t.Fatalf("conta missing saque/aporte: saque=%v aporte=%v", hasSaque, hasAporte)
	}

	msgs, err := idx.Search(ctx, "c01", "", "mensagens")
	if err != nil {
		t.Fatalf("mensagens: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("mensagens empty")
	}
	for _, it := range msgs {
		if it.Kind != "mensagem" {
			t.Fatalf("mensagens kind = %q", it.Kind)
		}
	}

	casos, err := idx.Search(ctx, "c01", "", "casos")
	if err != nil {
		t.Fatalf("casos: %v", err)
	}
	if len(casos) == 0 {
		t.Fatal("casos empty")
	}
	for _, it := range casos {
		if it.Kind != "caso" {
			t.Fatalf("casos kind = %q", it.Kind)
		}
	}
}

func TestIndex_ApplyEventKinds(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	cases := []struct {
		name       string
		routingKey string
		payload    map[string]any
		wantKind   string
		chip       string
	}{
		{
			name:       "message.triaged",
			routingKey: event.NameMessageTriaged,
			payload:    map[string]any{"text": "olá", "intent": "Contato", "channel": "chat"},
			wantKind:   "mensagem",
			chip:       "mensagens",
		},
		{
			name:       "alert.raised",
			routingKey: event.NameAlertRaised,
			payload:    map[string]any{"kind": "saque", "reason": "grande", "rule": "r"},
			wantKind:   "saque",
			chip:       "conta",
		},
		{
			name:       "case.opened",
			routingKey: event.NameCaseOpened,
			payload:    map[string]any{"case_id": "k2001", "text": "aberto", "meta": "m"},
			wantKind:   "caso",
			chip:       "casos",
		},
		{
			name:       "case.status.changed",
			routingKey: event.NameCaseStatusChanged,
			payload:    map[string]any{"case_id": "k2002", "state": "resolvido", "text": "fechado", "meta": "m"},
			wantKind:   "caso",
			chip:       "casos",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			idx := timeline.NewIndex()
			cust := "c-apply-" + tc.wantKind + "-" + tc.name
			body, err := json.Marshal(map[string]any{
				"event_id":       "ev-" + tc.name,
				"occurred_at":    now,
				"customer_id":    cust,
				"schema_version": 1,
				"payload":        tc.payload,
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			applied, entry, err := idx.ApplyDelivery(context.Background(), tc.routingKey, body)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if !applied {
				t.Fatal("expected applied")
			}
			if entry.Kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", entry.Kind, tc.wantKind)
			}
			hit, err := idx.Search(context.Background(), cust, "", tc.chip)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(hit) != 1 || hit[0].Kind != tc.wantKind {
				t.Fatalf("chip %s hit = %+v", tc.chip, hit)
			}
		})
	}
}
