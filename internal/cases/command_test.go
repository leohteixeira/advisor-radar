package cases_test

import (
	"errors"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

var requestedAt = time.Date(2026, 9, 29, 15, 0, 0, 0, time.UTC)

func openCommandBody(t *testing.T, eventID, customerID string, cmd cases.OpenCommand) []byte {
	t.Helper()
	body, err := event.Envelope{
		Name:          event.NameCaseOpenRequested,
		EventID:       eventID,
		OccurredAt:    requestedAt,
		CustomerID:    customerID,
		SchemaVersion: event.SchemaVersionMVP,
		Payload:       cmd,
	}.MarshalBody()
	if err != nil {
		t.Fatalf("marshal open command: %v", err)
	}
	return body
}

func TestOpenRequested_OpensWithTheSegmentSLA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		segment   string
		cmd       cases.OpenCommand
		wantTotal int
	}{
		{name: "essencial alert", segment: book.SegmentEssencial, cmd: cases.OpenCommand{Label: "Aporte grande"}, wantTotal: 1440},
		{name: "advance alert", segment: book.SegmentAdvance, cmd: cases.OpenCommand{Label: "Mudança de segmento"}, wantTotal: 240},
		{name: "singular relevant withdrawal halves", segment: book.SegmentSingular, cmd: cases.OpenCommand{Label: "Saque relevante", RelevantWithdrawal: true}, wantTotal: 30},
		{name: "advance churn risk halves", segment: book.SegmentAdvance, cmd: cases.OpenCommand{Label: "Mensagem com risco", ChurnRisk: true}, wantTotal: 120},
		{name: "advance frustration halves", segment: book.SegmentAdvance, cmd: cases.OpenCommand{Frustration: 2}, wantTotal: 120},
		{name: "advance human request halves", segment: book.SegmentAdvance, cmd: cases.OpenCommand{HumanRequested: true}, wantTotal: 120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			customerID, signalID := identity.MustNewV7(), identity.MustNewV7()
			lookup := inSegment(tt.segment)
			tt.cmd.SignalID = signalID

			res, err := cases.OpenRequested(t.Context(), store, lookup, openCommandBody(t, identity.MustNewV7(), customerID, tt.cmd))
			if err != nil {
				t.Fatalf("OpenRequested: %v", err)
			}
			if res.Decision != cases.DecisionOpened || res.CaseID == "" {
				t.Fatalf("result = %+v, want opened", res)
			}
			row := store.mustCase(t, res.CaseID)
			if row.SLATotalMinutes != tt.wantTotal || row.SignalID != signalID || row.State != cases.StateAberto ||
				row.AdvisorID != lookup.customer.AdvisorID || !row.OpenedAt.Equal(requestedAt) {
				t.Fatalf("case = %+v, want total %d for signal %s opened at the command", row, tt.wantTotal, signalID)
			}
			if arms := store.delaysOf(res.CaseID); len(arms) != 1 || arms[0].TTLMs != tt.wantTotal*60*1000 {
				t.Fatalf("delay arms = %+v", arms)
			}
			hist := store.historyOf(res.CaseID)
			want := "Caso aberto pela assessoria"
			if tt.cmd.Label != "" {
				want += " a partir do sinal: " + tt.cmd.Label
			}
			if len(hist) != 1 || hist[0].Kind != cases.HistoryKindCase || hist[0].Text != want || hist[0].SignalID != "" {
				t.Fatalf("history = %+v, want %q", hist, want)
			}
			if keys := store.routingKeys(); len(keys) != 1 || keys[0] != event.NameCaseOpened {
				t.Fatalf("outbox = %v, want one case.opened", keys)
			}
		})
	}
}

func TestOpenRequested_JoinsTheOpenCase(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newMemStore()
	customerID, caseID := openFor(t, store, book.SegmentAdvance, cases.ClockFactors{}, requestedAt.Add(-time.Hour))
	before := store.mustCase(t, caseID)
	// Advisory is down: a join must not need it.
	lookup := &fakeLookup{err: errors.New("advisory unavailable")}
	signalID := identity.MustNewV7()
	cmd := cases.OpenCommand{SignalID: signalID, Label: "Saque relevante", RelevantWithdrawal: true}

	res, err := cases.OpenRequested(ctx, store, lookup, openCommandBody(t, identity.MustNewV7(), customerID, cmd))
	if err != nil {
		t.Fatalf("OpenRequested: %v", err)
	}
	if res.Decision != cases.DecisionJoined || res.CaseID != caseID {
		t.Fatalf("result = %+v, want joined %s", res, caseID)
	}
	if n := len(store.casesOf(customerID)); n != 1 {
		t.Fatalf("cases = %d, want 1 open case per customer", n)
	}
	if after := store.mustCase(t, caseID); after != before {
		t.Fatalf("joined case changed: %+v -> %+v", before, after)
	}
	hist := store.historyOf(caseID)
	if len(hist) != 1 || hist[0].Kind != cases.HistoryKindSignal || hist[0].SignalID != signalID ||
		hist[0].Text != "Sinal anexado ao caso pela assessoria: Saque relevante" || !hist[0].OccurredAt.Equal(requestedAt) {
		t.Fatalf("history = %+v", hist)
	}

	// A second command for the same signal (another key) adds no second line.
	res, err = cases.OpenRequested(ctx, store, lookup, openCommandBody(t, identity.MustNewV7(), customerID, cmd))
	if err != nil {
		t.Fatalf("second OpenRequested: %v", err)
	}
	if res.Decision != cases.DecisionJoined {
		t.Fatalf("decision = %q, want joined", res.Decision)
	}
	if hist := store.historyOf(caseID); len(hist) != 1 {
		t.Fatalf("history rows = %d, want 1", len(hist))
	}
	if lookup.calls.Load() != 0 {
		t.Fatal("a join called advisory")
	}
}

func TestOpenRequested_SignalThatOpenedTheCaseAddsNoLine(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newMemStore()
	customerID, signalID := identity.MustNewV7(), identity.MustNewV7()
	lookup := inSegment(book.SegmentAdvance)
	cmd := cases.OpenCommand{SignalID: signalID, Label: "Aporte grande"}

	first, err := cases.OpenRequested(ctx, store, lookup, openCommandBody(t, identity.MustNewV7(), customerID, cmd))
	if err != nil {
		t.Fatalf("OpenRequested: %v", err)
	}
	second, err := cases.OpenRequested(ctx, store, lookup, openCommandBody(t, identity.MustNewV7(), customerID, cmd))
	if err != nil {
		t.Fatalf("second OpenRequested: %v", err)
	}
	if second.Decision != cases.DecisionJoined || second.CaseID != first.CaseID {
		t.Fatalf("second = %+v, want joined %s", second, first.CaseID)
	}
	if hist := store.historyOf(first.CaseID); len(hist) != 1 {
		t.Fatalf("history rows = %d, want only the opening line", len(hist))
	}
}

func TestOpenRequested_RedeliveryIsNoop(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newMemStore()
	customerID := identity.MustNewV7()
	lookup := inSegment(book.SegmentAdvance)
	body := openCommandBody(t, identity.MustNewV7(), customerID, cases.OpenCommand{SignalID: identity.MustNewV7()})

	if _, err := cases.OpenRequested(ctx, store, lookup, body); err != nil {
		t.Fatalf("OpenRequested: %v", err)
	}
	res, err := cases.OpenRequested(ctx, store, lookup, body)
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if res.Decision != cases.DecisionDuplicate {
		t.Fatalf("decision = %q, want duplicate", res.Decision)
	}
	if n := len(store.casesOf(customerID)); n != 1 {
		t.Fatalf("cases = %d, want 1", n)
	}
	if store.outboxCount() != 1 || store.delayCount() != 1 {
		t.Fatalf("outbox = %d, delays = %d, want 1 and 1", store.outboxCount(), store.delayCount())
	}
}

func TestOpenRequested_Errors(t *testing.T) {
	t.Parallel()

	customerID := identity.MustNewV7()
	t.Run("signal id must be a uuid", func(t *testing.T) {
		t.Parallel()
		store := newMemStore()
		_, err := cases.OpenRequested(t.Context(), store, inSegment(book.SegmentAdvance),
			openCommandBody(t, identity.MustNewV7(), customerID, cases.OpenCommand{SignalID: "k-local-1"}))
		wantPermanent(t, err)
		if store.caseCount() != 0 || store.inboxCount() != 0 {
			t.Fatal("a refused command wrote to the store")
		}
	})
	t.Run("unknown customer is permanent", func(t *testing.T) {
		t.Parallel()
		_, err := cases.OpenRequested(t.Context(), newMemStore(), &fakeLookup{err: cases.ErrUnknownCustomer},
			openCommandBody(t, identity.MustNewV7(), customerID, cases.OpenCommand{SignalID: identity.MustNewV7()}))
		wantPermanent(t, err)
	})
	t.Run("advisory down is transient", func(t *testing.T) {
		t.Parallel()
		store := newMemStore()
		_, err := cases.OpenRequested(t.Context(), store, &fakeLookup{err: errors.New("advisory unavailable")},
			openCommandBody(t, identity.MustNewV7(), customerID, cases.OpenCommand{SignalID: identity.MustNewV7()}))
		var perm cases.PermanentDeliveryError
		if err == nil || errors.As(err, &perm) {
			t.Fatalf("error = %v, want transient", err)
		}
		if store.inboxCount() != 0 {
			t.Fatal("a transient failure claimed the inbox")
		}
	})
	t.Run("bad body is permanent", func(t *testing.T) {
		t.Parallel()
		_, err := cases.OpenRequested(t.Context(), newMemStore(), inSegment(book.SegmentAdvance), []byte(`not json`))
		wantPermanent(t, err)
	})
	t.Run("dependencies are required", func(t *testing.T) {
		t.Parallel()
		if _, err := cases.OpenRequested(t.Context(), nil, inSegment(book.SegmentAdvance), nil); err == nil {
			t.Fatal("nil store: error = nil")
		}
		if _, err := cases.OpenRequested(t.Context(), newMemStore(), nil, nil); err == nil {
			t.Fatal("nil lookup: error = nil")
		}
	})
}

// The body advisory publishes for the command (advisory's case command
// payload); keep these literal keys in sync with it.
func TestOpenRequested_AdvisoryBody(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	body := []byte(`{"event_id":"5b0a4c3e-8f1d-5c2a-9e7b-1a2b3c4d5e6f","occurred_at":"2026-09-29T15:00:00Z",` +
		`"customer_id":"01a0e3a4-9a44-75c7-91c3-f5c3250f406c","schema_version":1,` +
		`"payload":{"signal_id":"01a0e3a4-9a44-76a2-b210-d5a1e4624b33","label":"Saque relevante",` +
		`"frustration":1,"relevant_withdrawal":true}}`)

	res, err := cases.OpenRequested(t.Context(), store, inSegment(book.SegmentSingular), body)
	if err != nil {
		t.Fatalf("OpenRequested: %v", err)
	}
	row := store.mustCase(t, res.CaseID)
	if res.Decision != cases.DecisionOpened || row.SLATotalMinutes != 30 ||
		row.SignalID != "01a0e3a4-9a44-76a2-b210-d5a1e4624b33" || !row.OpenedAt.Equal(requestedAt) {
		t.Fatalf("result = %+v, case = %+v, want a 30-minute case for the signal", res, row)
	}
}
