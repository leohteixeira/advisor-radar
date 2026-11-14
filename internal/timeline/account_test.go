package timeline_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

// applyAt indexes one event of customerID that occurred at the given instant
// and returns its row.
func applyAt(t *testing.T, idx *timeline.Index, routingKey, customerID string, at time.Time, version int, payload map[string]any) timeline.Entry {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"event_id": identity.MustNewV7(), "occurred_at": at,
		"customer_id": customerID, "schema_version": version, "payload": payload,
	})
	if err != nil {
		t.Fatalf("marshal %s: %v", routingKey, err)
	}
	applied, entry, err := idx.ApplyDelivery(context.Background(), routingKey, body)
	if err != nil || !applied {
		t.Fatalf("apply %s = %v, %v", routingKey, applied, err)
	}
	return entry
}

// titles returns the titles of a search, in order.
func titles(t *testing.T, idx *timeline.Index, customerID, query, kind string) []string {
	t.Helper()
	rows, err := idx.Search(context.Background(), customerID, query, kind)
	if err != nil {
		t.Fatalf("Search(%q, %q): %v", query, kind, err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Title)
	}
	return out
}

// An age is measured when the row is read, not when it was indexed: the same
// rows read later are older by the time that passed.
func TestIndex_SearchMeasuresAgoAtReadTime(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 13, 19, 13, 29, 0, time.UTC)
	now := start
	idx := timeline.NewIndex(timeline.WithClock(func() time.Time { return now }))
	cust := identity.MustNewV7()

	applyAt(t, idx, event.NameAccountEventRecorded, cust, start, 3, map[string]any{"kind": "aplicacao", "amount": 100000})
	applyAt(t, idx, event.NameAccountEventRecorded, cust, start.Add(-24*time.Minute), 2, map[string]any{"kind": "aporte", "amount": 1000000})
	applyAt(t, idx, event.NameMessageReceived, cust, start.Add(2*time.Minute), 1, map[string]any{"text": "relógio adiantado"})

	steps := []struct {
		name    string
		advance time.Duration
		// want is the age of each row, most recent first: the message from a
		// clock two minutes ahead, the purchase, the deposit.
		want []int
	}{
		{name: "at index time", advance: 0, want: []int{0, 0, 24}},
		{name: "59 seconds later", advance: 59 * time.Second, want: []int{0, 0, 24}},
		{name: "5 minutes later", advance: 5 * time.Minute, want: []int{3, 5, 29}},
		{name: "a day later", advance: 24 * time.Hour, want: []int{1438, 1440, 1464}},
	}
	for _, step := range steps {
		now = start.Add(step.advance)
		rows, err := idx.Search(context.Background(), cust, "", "")
		if err != nil {
			t.Fatalf("%s: Search: %v", step.name, err)
		}
		got := make([]int, 0, len(rows))
		for _, row := range rows {
			got = append(got, row.Ago)
		}
		if !slices.Equal(got, step.want) {
			t.Errorf("%s: ages = %v, want %v", step.name, got, step.want)
		}
	}
}

// An account row carries a Portuguese label and its facts as text: the
// amount, and the product of a purchase. The seeded market-day events spell
// their kinds in English and state whole dollars; live ones state cents.
func TestIndex_AccountRowsCarryLabelAmountAndProduct(t *testing.T) {
	t.Parallel()
	names := map[string]string{"cobalto": "Cobalto Semicondutores"}
	tests := []struct {
		name      string
		version   int
		payload   map[string]any
		wantKind  string
		wantTitle string
		wantText  string
		wantMeta  string
		// wantCents is the amount the row carries for the client screens,
		// checked when it is not zero.
		wantCents int64
	}{
		{
			name: "seeded deposit", version: 1,
			payload:  map[string]any{"kind": "deposit", "amount": 60000, "before": 8000, "after": 68000},
			wantKind: "aporte", wantTitle: "Aporte", wantText: "US$ 60.000,00", wantCents: 6_000_000,
		},
		{
			name: "seeded withdrawal", version: 1,
			payload:  map[string]any{"kind": "withdrawal", "amount": 55000, "before": 196000, "after": 141000},
			wantKind: "saque", wantTitle: "Saque", wantText: "US$ 55.000,00", wantCents: 5_500_000,
		},
		{
			name: "seeded asset drop", version: 1,
			payload:  map[string]any{"kind": "asset_drop", "amount": -12000, "before": 79000, "after": 67000},
			wantKind: "queda", wantTitle: "Queda", wantText: "−US$ 12.000,00", wantCents: -1_200_000,
		},
		{
			name: "seeded row with its own text", version: 1,
			payload: map[string]any{
				"kind": "saque", "amount": 20000, "title": "Saque",
				"text": "US$ 20.000,00 para conta nos EUA", "meta": "Sem alerta · 7% do patrimônio",
			},
			wantKind: "saque", wantTitle: "Saque", wantText: "US$ 20.000,00 para conta nos EUA", wantMeta: "Sem alerta · 7% do patrimônio",
		},
		{
			name: "live deposit", version: 2,
			payload:  map[string]any{"kind": "aporte", "amount": 1000000, "origin": "pix"},
			wantKind: "aporte", wantTitle: "Aporte", wantText: "US$ 10.000,00",
		},
		{
			name: "live withdrawal", version: 2,
			payload:  map[string]any{"kind": "saque", "amount": 5900050, "destination": "conta"},
			wantKind: "saque", wantTitle: "Saque", wantText: "US$ 59.000,50",
		},
		{
			name: "purchase", version: 3,
			payload:  map[string]any{"kind": "aplicacao", "amount": 100000, "product_id": "cobalto", "asset_class": "acoes", "risk": 5},
			wantKind: "aplicacao", wantTitle: "Aplicação", wantText: "Compra de US$ 1.000,00 em Cobalto Semicondutores",
		},
		{
			name: "purchase of a product outside the catalog", version: 3,
			payload:  map[string]any{"kind": "aplicacao", "amount": 100000, "product_id": "retirado"},
			wantKind: "aplicacao", wantTitle: "Aplicação", wantText: "Compra de US$ 1.000,00",
		},
		{
			name: "revaluation at a loss", version: 3,
			payload:  map[string]any{"kind": "reavaliacao", "amount": -3852000, "sim_day": 3, "product_id": "cobalto", "product_change_bp": -5350},
			wantKind: "reavaliacao", wantTitle: "Reavaliação", wantText: "−US$ 38.520,00 no dia simulado 3",
		},
		{
			name: "revaluation at a gain", version: 3,
			payload:  map[string]any{"kind": "reavaliacao", "amount": 1200, "sim_day": 1},
			wantKind: "reavaliacao", wantTitle: "Reavaliação", wantText: "+US$ 12,00 no dia simulado 1",
		},
		{
			name: "deposit without an amount", version: 2,
			payload:  map[string]any{"kind": "aporte"},
			wantKind: "aporte", wantTitle: "Aporte",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			idx := timeline.NewIndex(timeline.WithProductNames(names))
			cust := identity.MustNewV7()
			entry := applyAt(t, idx, event.NameAccountEventRecorded, cust, time.Now().UTC(), tt.version, tt.payload)
			if entry.Kind != tt.wantKind || entry.Title != tt.wantTitle || entry.Text != tt.wantText || entry.Meta != tt.wantMeta {
				t.Errorf("row = kind %q title %q text %q meta %q, want %q %q %q %q",
					entry.Kind, entry.Title, entry.Text, entry.Meta, tt.wantKind, tt.wantTitle, tt.wantText, tt.wantMeta)
			}
			if tt.wantCents != 0 && entry.AmountCents != tt.wantCents {
				t.Errorf("amount = %d cents, want %d", entry.AmountCents, tt.wantCents)
			}
			// Whatever the event spelled, the row is an account row.
			if got := titles(t, idx, cust, "", timeline.KindConta); !slices.Equal(got, []string{tt.wantTitle}) {
				t.Errorf("conta chip = %v, want the row", got)
			}
		})
	}
}

// The search reads the text an account row was given, so a purchase is found
// by "Compra", by its product, and by its amount.
func TestIndex_SearchMatchesAccountFacts(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex(timeline.WithProductNames(map[string]string{"cobalto": "Cobalto Semicondutores"}))
	cust := identity.MustNewV7()
	now := time.Now().UTC()
	applyAt(t, idx, event.NameAccountEventRecorded, cust, now, 3, map[string]any{"kind": "aplicacao", "amount": 100000, "product_id": "cobalto"})
	applyAt(t, idx, event.NameAccountEventRecorded, cust, now.Add(-time.Minute), 2, map[string]any{"kind": "saque", "amount": 5900000})
	applyAt(t, idx, event.NameAccountEventRecorded, cust, now.Add(-2*time.Minute), 1, map[string]any{"kind": "deposit", "amount": 60000})
	applyAt(t, idx, event.NameMessageReceived, cust, now.Add(-3*time.Minute), 1, map[string]any{"channel": "chat", "text": "Consegue pedir para o meu assessor me ligar?"})

	tests := []struct {
		query string
		kind  string
		want  []string
	}{
		{query: "Compra", want: []string{"Aplicação"}},
		{query: "cobalto", want: []string{"Aplicação"}},
		{query: "SEMICONDUTORES", want: []string{"Aplicação"}},
		{query: "aplica", want: []string{"Aplicação"}},
		{query: "1.000,00", want: []string{"Aplicação"}},
		{query: "59.000", want: []string{"Saque"}},
		{query: "60.000,00", want: []string{"Aporte"}},
		{query: "US$", want: []string{"Aplicação", "Saque", "Aporte"}},
		{query: "Compra", kind: timeline.KindMensagens, want: []string{}},
		{query: "deposit", want: []string{}},
	}
	for _, tt := range tests {
		if got := titles(t, idx, cust, tt.query, tt.kind); !slices.Equal(got, tt.want) {
			t.Errorf("search %q kind %q = %v, want %v", tt.query, tt.kind, got, tt.want)
		}
	}
}

// The row of an alert shows advisory's description of the fact (the payload's
// reason) under the rule, and the search reads it. Every live kind the rules
// raise is covered; an alert published before advisory described it has no
// text and still shows its rule.
func TestIndex_AlertReasonIsIndexedAndSearchable(t *testing.T) {
	t.Parallel()
	alerts := []struct {
		kind      string
		rule      string
		reason    string
		wantTitle string
		query     string
	}{
		{"aporte", "Aporte maior que o patrimônio anterior", "Aporte de US$ 10.000,00, 1,2× o patrimônio", "Alerta: aporte grande", "1,2×"},
		{"saque", "Saque acima de 20% do patrimônio em 24 horas", "Saque de US$ 59.000,00, 87% do patrimônio", "Alerta: saque relevante", "87% do patrimônio"},
		{"segmento", "Patrimônio cruzou a faixa de US$ 10 mil", "Desceu de Advance para Essencial", "Alerta: mudança de segmento", "desceu"},
		{"queda", "Queda acima de 15% em 5 dias úteis", "Patrimônio caiu 17% em 5 dias", "Alerta: queda de patrimônio", "caiu 17%"},
		{"contato", "Sem contato há mais de 90 dias", "Último contato há 94 dias", "Alerta: sem contato há muito tempo", "94 dias"},
		{"perfil", "Compra acima do perfil de investidor", "Compra de US$ 1.000,00 em Cobalto Semicondutores, risco 5. Perfil conservador vai até risco 2.", "Alerta: compra acima do perfil", "risco 5"},
	}
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	now := time.Now().UTC()
	for i, alert := range alerts {
		entry := applyAt(t, idx, event.NameAlertRaised, cust, now.Add(-time.Duration(i)*time.Minute), 1,
			map[string]any{"kind": alert.kind, "rule": alert.rule, "reason": alert.reason, "amount": 59000})
		if entry.Kind != alert.kind || entry.Title != alert.wantTitle || entry.Text != alert.reason || entry.Meta != alert.rule {
			t.Errorf("%s row = %+v, want title %q text %q meta %q", alert.kind, entry, alert.wantTitle, alert.reason, alert.rule)
		}
	}
	for _, alert := range alerts {
		rows, err := idx.Search(context.Background(), cust, alert.query, timeline.KindConta)
		if err != nil || len(rows) != 1 || rows[0].Text != alert.reason {
			t.Errorf("search %q = %+v, %v, want the %s alert", alert.query, rows, err, alert.kind)
		}
	}

	// Published before advisory wrote reasons: the rule alone.
	old := applyAt(t, idx, event.NameAlertRaised, cust, now.Add(-time.Hour), 1,
		map[string]any{"kind": "segmento", "rule": "Patrimônio cruzou a faixa de US$ 10 mil", "from": "Advance", "to": "Essencial"})
	if old.Text != "" || old.Meta != "Patrimônio cruzou a faixa de US$ 10 mil" {
		t.Errorf("alert without reason = text %q meta %q, want no text and the rule", old.Text, old.Meta)
	}
}

// message builds the two events that tell one message: the message.received
// and the message.triaged that classifies it.
func messageEvents(t *testing.T, customerID string, at time.Time, received map[string]any, intent string) (receivedID string, receivedBody, triagedBody []byte) {
	t.Helper()
	receivedID = identity.MustNewV7()
	var err error
	receivedBody, err = json.Marshal(map[string]any{
		"event_id": receivedID, "occurred_at": at, "customer_id": customerID, "schema_version": 1, "payload": received,
	})
	if err != nil {
		t.Fatal(err)
	}
	triagedBody, err = json.Marshal(map[string]any{
		"event_id": identity.MustNewV7(), "occurred_at": at, "customer_id": customerID, "schema_version": 1,
		"payload": map[string]any{"source_event_id": receivedID, "channel": received["channel"], "text": received["text"], "intent": intent},
	})
	if err != nil {
		t.Fatal(err)
	}
	return receivedID, receivedBody, triagedBody
}

// A message is one row, though two events tell it. Whichever arrives first,
// the row is the message's (its id and source, so the client's activity
// lists it once) with the classification as meta; a redelivery changes
// nothing, and a message that brought its own meta keeps it.
func TestIndex_MessageAndItsTriageShareOneRow(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 13, 19, 34, 0, 0, time.UTC)
	tests := []struct {
		name     string
		received map[string]any
		// triagedFirst delivers the classification before the message.
		triagedFirst bool
		wantMeta     string
	}{
		{name: "message then triage", received: map[string]any{"channel": "chat", "text": "Estou pensando em sair"}, wantMeta: "Reclamação"},
		{name: "triage then message", received: map[string]any{"channel": "chat", "text": "Estou pensando em sair"}, triagedFirst: true, wantMeta: "Reclamação"},
		{
			name:     "seeded message with its own meta",
			received: map[string]any{"channel": "chat", "text": "Estou pensando em sair", "meta": "Reclamação · Frustrado"},
			wantMeta: "Reclamação · Frustrado",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			idx := timeline.NewIndex()
			cust := identity.MustNewV7()
			ctx := context.Background()
			receivedID, received, triaged := messageEvents(t, cust, at, tt.received, "reclamacao")
			order := []struct {
				key  string
				body []byte
			}{{event.NameMessageReceived, received}, {event.NameMessageTriaged, triaged}}
			if tt.triagedFirst {
				slices.Reverse(order)
			}
			var last timeline.Entry
			for _, delivery := range order {
				applied, entry, err := idx.ApplyDelivery(ctx, delivery.key, delivery.body)
				if err != nil || !applied {
					t.Fatalf("apply %s = %v, %v", delivery.key, applied, err)
				}
				// Elasticsearch gets the row under the message's id each time.
				if entry.EventID != receivedID {
					t.Fatalf("%s row id = %s, want the message's %s", delivery.key, entry.EventID, receivedID)
				}
				last = entry
			}
			for _, delivery := range order {
				if applied, _, err := idx.ApplyDelivery(ctx, delivery.key, delivery.body); err != nil || applied {
					t.Fatalf("redelivered %s = %v, %v, want a no-op", delivery.key, applied, err)
				}
			}

			rows, err := idx.Search(ctx, cust, "", timeline.KindMensagens)
			if err != nil || len(rows) != 1 {
				t.Fatalf("messages = %+v, %v, want one row", rows, err)
			}
			row := rows[0]
			if row.EventID != receivedID || row.Source != event.NameMessageReceived || row.Title != "Mensagem · chat" ||
				row.Text != "Estou pensando em sair" || row.Meta != tt.wantMeta {
				t.Errorf("row = %+v, want the message with meta %q", row, tt.wantMeta)
			}
			if last.Meta != tt.wantMeta || last.Source != event.NameMessageReceived {
				t.Errorf("last indexed row = %+v, want the merged one", last)
			}
		})
	}
}

// After a failed Elasticsearch write the row is forgotten with both events,
// so the requeued delivery indexes it again.
func TestIndex_ForgetDropsTheMessageRowAndBothEvents(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	ctx := context.Background()
	receivedID, received, triaged := messageEvents(t, cust, time.Now().UTC(), map[string]any{"channel": "chat", "text": "oi"}, "contato")
	for key, body := range map[string][]byte{event.NameMessageReceived: received, event.NameMessageTriaged: triaged} {
		if _, _, err := idx.ApplyDelivery(ctx, key, body); err != nil {
			t.Fatalf("apply %s: %v", key, err)
		}
	}
	idx.Forget(receivedID)
	if rows, err := idx.Search(ctx, cust, "", ""); err != nil || len(rows) != 0 {
		t.Fatalf("rows after Forget = %+v, %v, want none", rows, err)
	}
	if applied, entry, err := idx.ApplyDelivery(ctx, event.NameMessageTriaged, triaged); err != nil || !applied || entry.EventID != receivedID {
		t.Fatalf("requeued triage = %v, %+v, %v, want it applied again", applied, entry, err)
	}
}

// A deposit and the alert it raised are two facts and two rows; the deposit
// itself is listed once.
func TestIndex_DepositAndItsAlertAreTwoRows(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	now := time.Now().UTC()
	applyAt(t, idx, event.NameAccountEventRecorded, cust, now, 2, map[string]any{"kind": "aporte", "amount": 100000})
	applyAt(t, idx, event.NameAlertRaised, cust, now, 1, map[string]any{"kind": "aporte", "rule": "Aporte maior que o patrimônio anterior", "reason": "Aporte de US$ 1.000,00, 1,2× o patrimônio"})
	rows, err := idx.Search(context.Background(), cust, "", "")
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %+v, %v, want the deposit and its alert", rows, err)
	}
	sources := []string{rows[0].Source, rows[1].Source}
	slices.Sort(sources)
	if !slices.Equal(sources, []string{event.NameAccountEventRecorded, event.NameAlertRaised}) {
		t.Errorf("sources = %v", sources)
	}
}

// The search ignores case and accents on both sides.
func TestIndex_SearchFoldsAccents(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	now := time.Now().UTC()
	applyAt(t, idx, "advisory.note.recorded", cust, now, 1, map[string]any{"title": "Nota do assessor", "text": "Sérgio pretende comprar imóvel em Orlando. Precisa de liquidez em março.", "meta": "Ana Paula"})
	applyAt(t, idx, event.NameAccountEventRecorded, cust, now.Add(-time.Minute), 3, map[string]any{"kind": "aplicacao", "amount": 100000})
	applyAt(t, idx, event.NameAlertRaised, cust, now.Add(-2*time.Minute), 1, map[string]any{"kind": "saque", "rule": "Saque acima de 20% do patrimônio em 24 horas", "reason": "Saque de US$ 59.000,00, 87% do patrimônio"})

	tests := []struct {
		query string
		want  []string
	}{
		{"sergio", []string{"Nota do assessor"}},
		{"SÉRGIO", []string{"Nota do assessor"}},
		{"orlândo", []string{"Nota do assessor"}},
		{"imovel", []string{"Nota do assessor"}},
		{"marco", []string{"Nota do assessor"}},
		{"aplicacao", []string{"Aplicação"}},
		{"APLICAÇÃO", []string{"Aplicação"}},
		{"patrimonio", []string{"Alerta: saque relevante"}},
		{"sérgia", []string{}},
	}
	for _, tt := range tests {
		if got := titles(t, idx, cust, tt.query, ""); !slices.Equal(got, tt.want) {
			t.Errorf("search %q = %v, want %v", tt.query, got, tt.want)
		}
	}
}

// A withdrawal that raises an alert is one movement and one alert: the two
// rows carry different titles. A case row shows the protocol the client app
// shows, never the raw id, and says what happened.
func TestIndex_AlertAndCaseRowsSayWhatTheyAre(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	now := time.Now().UTC()

	movement := applyAt(t, idx, event.NameAccountEventRecorded, cust, now.Add(-2*time.Minute), 2,
		map[string]any{"kind": "withdrawal", "amount": 30_000_000, "before": 125_000_000, "after": 95_000_000})
	alert := applyAt(t, idx, event.NameAlertRaised, cust, now.Add(-2*time.Minute), 1,
		map[string]any{"kind": "saque", "rule": "Saque acima de 20% do patrimônio em 24 horas", "reason": "Saque de US$ 300.000,00, 24% do patrimônio"})
	if movement.Title != "Saque" || alert.Title != "Alerta: saque relevante" {
		t.Errorf("titles = %q and %q, want the movement and its alert told apart", movement.Title, alert.Title)
	}

	const caseID = "01a1d69a-adf5-7c3e-9d3c-5b1d4f0f2a11"
	opened := applyAt(t, idx, event.NameCaseOpened, cust, now.Add(-time.Minute), 1,
		map[string]any{"case_id": caseID, "state": "Aberto", "sla_total_minutes": 30})
	advanced := applyAt(t, idx, event.NameCaseStatusChanged, cust, now, 1,
		map[string]any{"case_id": caseID, "state": "Em atendimento"})
	for _, tt := range []struct {
		entry    timeline.Entry
		wantText string
	}{
		{opened, "Caso aberto"},
		{advanced, "Caso passou para Em atendimento"},
	} {
		if tt.entry.Title != "Caso 01A1D69A-ADF5" || tt.entry.Text != tt.wantText {
			t.Errorf("case row = title %q text %q, want %q and %q", tt.entry.Title, tt.entry.Text, "Caso 01A1D69A-ADF5", tt.wantText)
		}
	}
}
