package cases_test

import (
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// Every state change is a line of the case's history.
func TestAdvance_WritesHistory(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	_, caseID := openFor(t, store, book.SegmentAdvance, cases.ClockFactors{}, time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC))
	want := []string{"Caso passou para Em atendimento", "Caso passou para Aguardando cliente", "Caso passou para Resolvido"}
	for i, to := range []string{cases.StateEmAtendimento, cases.StateAguardandoCliente, cases.StateResolvido} {
		if err := cases.Advance(t.Context(), store, caseID, to); err != nil {
			t.Fatalf("Advance %s: %v", to, err)
		}
		hist := store.historyOf(caseID)
		if len(hist) != i+1 || hist[i].Kind != cases.HistoryKindCase || hist[i].Text != want[i] {
			t.Fatalf("history after %s = %+v, want %q last", to, hist, want[i])
		}
	}
	// A refused transition writes nothing.
	if err := cases.Advance(t.Context(), store, caseID, cases.StateResolvido); err == nil {
		t.Fatal("advancing a resolved case: error = nil")
	}
	if n := len(store.historyOf(caseID)); n != 3 {
		t.Fatalf("history rows = %d, want 3", n)
	}
}

// While a case is open, every message the client sends from the app enters
// its history with the signal it came from, whether or not the message would
// open a case on its own.
func TestIntake_LaterMessagesJoinTheOpenCase(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newMemStore()
	lookup := singular(identity.MustNewV7())
	customerID := identity.MustNewV7()
	message := func(msg cases.TriagedMessage) []byte {
		msg.Origin = cases.OriginClientApp
		msg.SourceEventID = identity.MustNewV7()
		return triagedBody(t, identity.MustNewV7(), customerID, msg)
	}

	opened, err := cases.Intake(ctx, store, lookup, message(cases.TriagedMessage{Intent: "reclamacao", Text: "Minha transferência está atrasada"}))
	if err != nil || opened.Decision != cases.DecisionOpened {
		t.Fatalf("first complaint = %+v, %v, want opened", opened, err)
	}

	second := message(cases.TriagedMessage{Intent: "reclamacao", Text: "  Continua atrasada.  "})
	question := message(cases.TriagedMessage{Intent: "cambio", Text: "Qual a cotação de hoje?"})
	silent := message(cases.TriagedMessage{Intent: "operacional"})
	for name, body := range map[string][]byte{"second complaint": second, "question": question, "message without text": silent} {
		// Delivered twice: the redelivery adds nothing.
		for range 2 {
			res, err := cases.Intake(ctx, store, lookup, body)
			if err != nil || res.CaseID != "" && res.CaseID != opened.CaseID {
				t.Fatalf("%s = %+v, %v", name, res, err)
			}
		}
	}
	if n := len(store.casesOf(customerID)); n != 1 {
		t.Fatalf("cases = %d, want 1", n)
	}
	got := map[string]bool{}
	for _, h := range store.historyOf(opened.CaseID)[1:] {
		if h.Kind != cases.HistoryKindMessage || h.SignalID == "" || !h.OccurredAt.Equal(triagedAt) {
			t.Fatalf("history line = %+v, want a message line with its signal", h)
		}
		got[h.Text] = true
	}
	want := []string{
		"Nova mensagem do cliente: reclamação — “Continua atrasada.”",
		"Nova mensagem do cliente — “Qual a cotação de hoje?”",
		"Nova mensagem do cliente",
	}
	if len(got) != len(want) {
		t.Fatalf("history texts = %v, want %v", got, want)
	}
	for _, text := range want {
		if !got[text] {
			t.Errorf("history has no line %q: %v", text, got)
		}
	}
}

func TestIntake_NonQualifyingMessageWithoutACaseOpensNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		origin string
	}{
		{name: "client app question", origin: cases.OriginClientApp},
		{name: "seeded message", origin: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			lookup := singular(identity.MustNewV7())
			customerID := identity.MustNewV7()
			body := triagedBody(t, identity.MustNewV7(), customerID,
				cases.TriagedMessage{Origin: tt.origin, SourceEventID: identity.MustNewV7(), Intent: "cambio", Text: "Qual a cotação?"})
			res, err := cases.Intake(t.Context(), store, lookup, body)
			if err != nil || res.Decision != cases.DecisionIgnored {
				t.Fatalf("Intake = %+v, %v, want ignored", res, err)
			}
			if store.caseCount() != 0 || store.inboxCount() != 1 || lookup.calls.Load() != 0 {
				t.Fatalf("cases = %d, inbox = %d, lookups = %d, want 0, 1 and 0", store.caseCount(), store.inboxCount(), lookup.calls.Load())
			}
		})
	}
}

// A seeded or burst message never enters a case, even an open one.
func TestIntake_MessageWithoutOriginDoesNotJoin(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	customerID, caseID := openFor(t, store, book.SegmentAdvance, cases.ClockFactors{}, triagedAt.Add(-time.Hour))
	body := triagedBody(t, identity.MustNewV7(), customerID,
		cases.TriagedMessage{SourceEventID: identity.MustNewV7(), Intent: "reclamacao", Text: "Mensagem da carga"})
	res, err := cases.Intake(t.Context(), store, singular(identity.MustNewV7()), body)
	if err != nil || res.Decision != cases.DecisionIgnored {
		t.Fatalf("Intake = %+v, %v, want ignored", res, err)
	}
	if n := len(store.historyOf(caseID)); n != 0 {
		t.Fatalf("history rows = %d, want 0", n)
	}
}

func TestIntake_LongMessageIsCutInHistory(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	customerID, caseID := openFor(t, store, book.SegmentAdvance, cases.ClockFactors{}, triagedAt.Add(-time.Hour))
	long := make([]rune, 400)
	for i := range long {
		long[i] = 'ã'
	}
	body := triagedBody(t, identity.MustNewV7(), customerID,
		cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "operacional", Text: string(long)})
	if _, err := cases.Intake(t.Context(), store, singular(identity.MustNewV7()), body); err != nil {
		t.Fatalf("Intake: %v", err)
	}
	hist := store.historyOf(caseID)
	want := "Nova mensagem do cliente — “" + string(long[:160]) + "…”"
	if len(hist) != 1 || hist[0].Text != want {
		t.Fatalf("history = %+v, want the text cut at 160 characters", hist)
	}
}

// One definition of the SLA state serves the queue card, the queue filter
// and the manager panel.
func TestSLAState(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		total    int
		state    string
		elapsed  time.Duration
		expected string
		atRisk   bool
	}{
		{name: "30 minute case just opened", total: 30, elapsed: 0, expected: cases.SLAStateOnTime},
		{name: "30 minute case with 20 minutes left", total: 30, elapsed: 10 * time.Minute, expected: cases.SLAStateOnTime},
		{name: "30 minute case under 20 minutes left", total: 30, elapsed: 11 * time.Minute, expected: cases.SLAStateDueSoon, atRisk: true},
		{name: "4 hour case with an hour left", total: 240, elapsed: 3 * time.Hour, expected: cases.SLAStateDueSoon, atRisk: true},
		{name: "4 hour case with a third left", total: 240, elapsed: 161 * time.Minute, expected: cases.SLAStateOnTime},
		{name: "24 hour case with an hour left", total: 1440, elapsed: 23 * time.Hour, expected: cases.SLAStateDueSoon, atRisk: true},
		{name: "24 hour case with nine hours left", total: 1440, elapsed: 15 * time.Hour, expected: cases.SLAStateOnTime},
		{name: "at the deadline", total: 60, elapsed: time.Hour, expected: cases.SLAStateDueSoon, atRisk: true},
		{name: "past the deadline", total: 60, elapsed: 61 * time.Minute, expected: cases.SLAStateOverdue, atRisk: true},
		{name: "resolved after the deadline", total: 60, state: cases.StateResolvido, elapsed: 5 * time.Hour, expected: cases.SLAStateDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state := tt.state
			if state == "" {
				state = cases.StateAberto
			}
			row := cases.CaseRow{State: state, SLATotalMinutes: tt.total, OpenedAt: start}
			now := start.Add(tt.elapsed)
			if got := cases.SLAState(row, now); got != tt.expected {
				t.Errorf("SLAState = %q, want %q", got, tt.expected)
			}
			if got := cases.IsAtRisk(row, now); got != tt.atRisk {
				t.Errorf("IsAtRisk = %v, want %v", got, tt.atRisk)
			}
		})
	}
}
