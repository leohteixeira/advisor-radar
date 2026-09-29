package cases_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

type fakeLookup struct {
	customer cases.Customer
	err      error
	calls    atomic.Int32
}

func (f *fakeLookup) Lookup(ctx context.Context, _ string) (cases.Customer, error) {
	f.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return cases.Customer{}, err
	}
	if f.err != nil {
		return cases.Customer{}, f.err
	}
	return f.customer, nil
}

func singular(advisorID string) *fakeLookup {
	return &fakeLookup{customer: cases.Customer{Segment: book.SegmentSingular, AdvisorID: advisorID}}
}

var triagedAt = time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)

func triagedBody(t *testing.T, eventID, customerID string, msg cases.TriagedMessage) []byte {
	t.Helper()
	body, err := event.Envelope{
		Name:          event.NameMessageTriaged,
		EventID:       eventID,
		OccurredAt:    triagedAt,
		CustomerID:    customerID,
		SchemaVersion: event.SchemaVersionMVP,
		Payload:       msg,
	}.MarshalBody()
	if err != nil {
		t.Fatalf("marshal triaged: %v", err)
	}
	return body
}

func (s *memStore) casesOf(customerID string) []cases.CaseRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]cases.CaseRow, 0)
	for _, row := range s.cases {
		if row.CustomerID == customerID {
			out = append(out, row)
		}
	}
	return out
}

func (s *memStore) historyOf(caseID string) []cases.HistoryRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]cases.HistoryRow, 0)
	for _, row := range s.history {
		if row.CaseID == caseID {
			out = append(out, row)
		}
	}
	return out
}

func (s *memStore) inboxCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.inbox)
}

func (s *memStore) seedCase(row cases.CaseRow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cases[row.ID] = row
}

func wantPermanent(t *testing.T, err error) {
	t.Helper()
	var perm cases.PermanentDeliveryError
	if !errors.As(err, &perm) {
		t.Fatalf("error = %v, want PermanentDeliveryError", err)
	}
}

func TestReason_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		reason cases.Reason
		want   string
	}{
		{cases.ReasonComplaint, "reclamação"},
		{cases.ReasonClosing, "pedido de encerramento"},
		{cases.ReasonChurnRisk, "risco de saída"},
		{cases.ReasonNone, ""},
	}
	for _, tt := range tests {
		if got := tt.reason.String(); got != tt.want {
			t.Errorf("Reason(%d).String() = %q, want %q", tt.reason, got, tt.want)
		}
	}
}

func TestQualifies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		msg        cases.TriagedMessage
		wantReason cases.Reason
		wantOK     bool
	}{
		{"complaint", cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "reclamacao"}, cases.ReasonComplaint, true},
		{"closing", cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "encerramento"}, cases.ReasonClosing, true},
		{"complaint with churn keeps the intent", cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "reclamacao", ChurnRisk: 0.9}, cases.ReasonComplaint, true},
		{"churn only", cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "investimento", ChurnRisk: 0.8}, cases.ReasonChurnRisk, true},
		{"churn at threshold", cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "cambio", ChurnRisk: cases.ChurnRiskOpen}, cases.ReasonChurnRisk, true},
		{"churn below threshold", cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "cambio", ChurnRisk: 0.49}, cases.ReasonNone, false},
		{"not qualifying", cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "cambio", ChurnRisk: 0.2}, cases.ReasonNone, false},
		{"empty", cases.TriagedMessage{}, cases.ReasonNone, false},
		{"complaint without origin", cases.TriagedMessage{Intent: "reclamacao", ChurnRisk: 0.8}, cases.ReasonNone, false},
		{"churn from another origin", cases.TriagedMessage{Origin: "burst", Intent: "investimento", ChurnRisk: 0.9}, cases.ReasonNone, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reason, ok := cases.Qualifies(tt.msg)
			if reason != tt.wantReason || ok != tt.wantOK {
				t.Fatalf("Qualifies = (%v, %v), want (%v, %v)", reason, ok, tt.wantReason, tt.wantOK)
			}
		})
	}
}

func TestIntake_Opens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		segment  string
		msg      cases.TriagedMessage
		wantSLA  int
		wantText string
	}{
		{
			name:     "Mariana complaint",
			segment:  book.SegmentSingular,
			msg:      cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "reclamacao", Frustration: 1.4, ChurnRisk: 0.1, WantsHuman: 0.2},
			wantSLA:  60,
			wantText: "Caso aberto a partir de mensagem com reclamação",
		},
		{
			name:     "complaint with frustration rounded up halves",
			segment:  book.SegmentSingular,
			msg:      cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "reclamacao", Frustration: 1.6},
			wantSLA:  30,
			wantText: "Caso aberto a partir de mensagem com reclamação",
		},
		{
			name:     "complaint asking for a human halves",
			segment:  book.SegmentSingular,
			msg:      cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "reclamacao", WantsHuman: 0.5},
			wantSLA:  30,
			wantText: "Caso aberto a partir de mensagem com reclamação",
		},
		{
			name:     "closing request",
			segment:  book.SegmentAdvance,
			msg:      cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "encerramento"},
			wantSLA:  240,
			wantText: "Caso aberto a partir de pedido de encerramento",
		},
		{
			name:     "churn only halves",
			segment:  book.SegmentSingular,
			msg:      cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "investimento", ChurnRisk: 0.8},
			wantSLA:  30,
			wantText: "Caso aberto a partir de risco de saída",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			store := newMemStore()
			advisorID := identity.MustNewV7()
			lookup := &fakeLookup{customer: cases.Customer{Segment: tt.segment, AdvisorID: advisorID}}
			customerID := identity.MustNewV7()
			eventID := identity.MustNewV7()
			sourceID := identity.MustNewV7()
			msg := tt.msg
			msg.SourceEventID = sourceID

			res, err := cases.Intake(ctx, store, lookup, triagedBody(t, eventID, customerID, msg))
			if err != nil {
				t.Fatalf("Intake: %v", err)
			}
			if res.Decision != cases.DecisionOpened || res.EventID != eventID || res.CustomerID != customerID {
				t.Fatalf("result = %+v", res)
			}
			row := store.mustCase(t, res.CaseID)
			if !identity.IsV7(row.ID) {
				t.Fatalf("case id %q is not UUIDv7", row.ID)
			}
			if row.State != cases.StateAberto || row.AdvisorID != advisorID || row.CustomerID != customerID {
				t.Fatalf("case = %+v", row)
			}
			if row.SLATotalMinutes != tt.wantSLA {
				t.Fatalf("sla = %d, want %d", row.SLATotalMinutes, tt.wantSLA)
			}
			if row.SignalID != sourceID {
				t.Fatalf("signal = %q, want source %q", row.SignalID, sourceID)
			}
			if !row.OpenedAt.Equal(triagedAt) {
				t.Fatalf("opened_at = %v, want %v", row.OpenedAt, triagedAt)
			}
			keys := store.routingKeys()
			if len(keys) != 1 || keys[0] != event.NameCaseOpened {
				t.Fatalf("outbox keys = %v, want [%s]", keys, event.NameCaseOpened)
			}
			if store.delayCount() != 1 {
				t.Fatalf("delays = %d, want 1", store.delayCount())
			}
			store.mu.Lock()
			ttl := store.delays[0].TTLMs
			store.mu.Unlock()
			if ttl != tt.wantSLA*60*1000 {
				t.Fatalf("delay ttl = %d, want %d", ttl, tt.wantSLA*60*1000)
			}
			hist := store.historyOf(row.ID)
			if len(hist) != 1 || hist[0].Kind != cases.HistoryKindCase || hist[0].Text != tt.wantText {
				t.Fatalf("history = %+v, want one caso row %q", hist, tt.wantText)
			}
			if !hist[0].OccurredAt.Equal(triagedAt) {
				t.Fatalf("history at = %v, want %v", hist[0].OccurredAt, triagedAt)
			}
		})
	}
}

func TestIntake_SignalFallsBackToTriagedEvent(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	eventID := identity.MustNewV7()
	msg := cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: "not-a-uuid", Intent: "reclamacao"}
	res, err := cases.Intake(t.Context(), store, singular(identity.MustNewV7()),
		triagedBody(t, eventID, identity.MustNewV7(), msg))
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if got := store.mustCase(t, res.CaseID).SignalID; got != eventID {
		t.Fatalf("signal = %q, want triaged event %q", got, eventID)
	}
}

func TestIntake_NotQualifyingClaimsInboxOnly(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	lookup := &fakeLookup{err: status.Error(codes.Unavailable, "advisory down")}
	msg := cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "cambio", ChurnRisk: 0.2}
	res, err := cases.Intake(t.Context(), store, lookup, triagedBody(t, identity.MustNewV7(), identity.MustNewV7(), msg))
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if res.Decision != cases.DecisionIgnored || res.CaseID != "" {
		t.Fatalf("result = %+v, want ignored", res)
	}
	if store.inboxCount() != 1 || store.caseCount() != 0 || store.outboxCount() != 0 {
		t.Fatalf("inbox=%d cases=%d outbox=%d, want 1/0/0", store.inboxCount(), store.caseCount(), store.outboxCount())
	}
	if lookup.calls.Load() != 0 {
		t.Fatalf("lookup calls = %d, want 0", lookup.calls.Load())
	}
}

func TestIntake_SeededMessageWithoutOriginOpensNothing(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	lookup := singular(identity.MustNewV7())
	msg := cases.TriagedMessage{SourceEventID: identity.MustNewV7(), Intent: "reclamacao", ChurnRisk: 0.8, Frustration: 3}
	res, err := cases.Intake(t.Context(), store, lookup, triagedBody(t, identity.MustNewV7(), identity.MustNewV7(), msg))
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if res.Decision != cases.DecisionIgnored {
		t.Fatalf("decision = %q, want ignored", res.Decision)
	}
	if store.inboxCount() != 1 || store.caseCount() != 0 || store.outboxCount() != 0 || lookup.calls.Load() != 0 {
		t.Fatalf("inbox=%d cases=%d outbox=%d lookups=%d, want 1/0/0/0",
			store.inboxCount(), store.caseCount(), store.outboxCount(), lookup.calls.Load())
	}
}

func TestIntake_ProducerPayloadFields(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	eventID := identity.MustNewV7()
	sourceID := identity.MustNewV7()
	customerID := identity.MustNewV7()
	// Field names exactly as triagepipe.TriagedPayload marshals them.
	body := []byte(`{"event_id":"` + eventID + `","occurred_at":"2026-09-29T14:00:00Z","customer_id":"` + customerID +
		`","schema_version":1,"payload":{"source_event_id":"` + sourceID + `","origin":"client_app","channel":"chat",` +
		`"text":"Quero investir","intent":"investimento","intent_prob":0.91,"frustration":0,"churn_risk":0.8,` +
		`"wants_human":0.1,"classifier":"heuristic","model_version":"keywords-v1","degraded":true,"needs_review":false}}`)
	res, err := cases.Intake(t.Context(), store, singular(identity.MustNewV7()), body)
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if res.Decision != cases.DecisionOpened {
		t.Fatalf("decision = %q, want opened", res.Decision)
	}
	row := store.mustCase(t, res.CaseID)
	if row.SLATotalMinutes != 30 {
		t.Fatalf("sla = %d, want 30 (Singular halved by churn)", row.SLATotalMinutes)
	}
	if row.SignalID != sourceID {
		t.Fatalf("signal = %q, want source %q", row.SignalID, sourceID)
	}
	hist := store.historyOf(row.ID)
	if len(hist) != 1 || hist[0].Text != "Caso aberto a partir de risco de saída" {
		t.Fatalf("history = %+v, want the churn opening row", hist)
	}
}

func TestIntake_AdvisoryDownStillAcksDuplicateAndJoin(t *testing.T) {
	t.Parallel()

	down := &fakeLookup{err: status.Error(codes.Unavailable, "advisory down")}

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()
		store := newMemStore()
		body := triagedBody(t, identity.MustNewV7(), identity.MustNewV7(),
			cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "reclamacao"})
		if _, err := cases.Intake(t.Context(), store, singular(identity.MustNewV7()), body); err != nil {
			t.Fatalf("first Intake: %v", err)
		}
		lookup := &fakeLookup{err: down.err}
		res, err := cases.Intake(t.Context(), store, lookup, body)
		if err != nil || res.Decision != cases.DecisionDuplicate {
			t.Fatalf("second Intake = %+v, %v, want duplicate", res, err)
		}
		if lookup.calls.Load() != 0 {
			t.Fatalf("lookup calls = %d, want 0", lookup.calls.Load())
		}
	})

	t.Run("join", func(t *testing.T) {
		t.Parallel()
		store := newMemStore()
		customerID := identity.MustNewV7()
		open := cases.CaseRow{
			ID: identity.MustNewV7(), CustomerID: customerID, AdvisorID: identity.MustNewV7(),
			State: cases.StateAberto, SLATotalMinutes: 60, OpenedAt: triagedAt.Add(-time.Hour),
		}
		store.seedCase(open)
		lookup := &fakeLookup{err: down.err}
		res, err := cases.Intake(t.Context(), store, lookup, triagedBody(t, identity.MustNewV7(), customerID,
			cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "encerramento"}))
		if err != nil || res.Decision != cases.DecisionJoined || res.CaseID != open.ID {
			t.Fatalf("Intake = %+v, %v, want joined %s", res, err, open.ID)
		}
		if lookup.calls.Load() != 0 {
			t.Fatalf("lookup calls = %d, want 0", lookup.calls.Load())
		}
	})
}

// staleOpenStore reports an open case before the transaction that the
// transaction no longer sees, as when the case is resolved in between.
type staleOpenStore struct {
	*memStore
}

func (s staleOpenStore) OpenCaseFor(context.Context, string) (cases.CaseRow, bool, error) {
	return cases.CaseRow{ID: identity.MustNewV7(), State: cases.StateAberto}, true, nil
}

func TestIntake_OpenCaseGoneBeforeTxIsTransient(t *testing.T) {
	t.Parallel()

	store := staleOpenStore{memStore: newMemStore()}
	lookup := singular(identity.MustNewV7())
	body := triagedBody(t, identity.MustNewV7(), identity.MustNewV7(),
		cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "reclamacao"})
	_, err := cases.Intake(t.Context(), store, lookup, body)
	if err == nil {
		t.Fatal("Intake error = nil, want a transient error")
	}
	var perm cases.PermanentDeliveryError
	if errors.As(err, &perm) {
		t.Fatalf("error = %v, want transient", err)
	}
	if store.inboxCount() != 0 || store.caseCount() != 0 || lookup.calls.Load() != 0 {
		t.Fatal("a stale pre-read committed rows or called advisory")
	}

	// The retry sees no open case, looks the customer up, and opens one.
	res, err := cases.Intake(t.Context(), store.memStore, lookup, body)
	if err != nil || res.Decision != cases.DecisionOpened {
		t.Fatalf("retry = %+v, %v, want opened", res, err)
	}
}

func TestIntake_JoinsOpenCase(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	customerID := identity.MustNewV7()
	open := cases.CaseRow{
		ID:              identity.MustNewV7(),
		CustomerID:      customerID,
		AdvisorID:       identity.MustNewV7(),
		State:           cases.StateEmAtendimento,
		SLATotalMinutes: 60,
		OpenedAt:        triagedAt.Add(-time.Hour),
	}
	store.seedCase(open)

	msg := cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "reclamacao"}
	res, err := cases.Intake(t.Context(), store, singular(identity.MustNewV7()),
		triagedBody(t, identity.MustNewV7(), customerID, msg))
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if res.Decision != cases.DecisionJoined || res.CaseID != open.ID {
		t.Fatalf("result = %+v, want joined %s", res, open.ID)
	}
	if n := len(store.casesOf(customerID)); n != 1 {
		t.Fatalf("customer cases = %d, want 1", n)
	}
	if store.outboxCount() != 0 || store.delayCount() != 0 {
		t.Fatalf("outbox=%d delays=%d, want 0/0", store.outboxCount(), store.delayCount())
	}
	hist := store.historyOf(open.ID)
	want := "Nova mensagem do cliente: reclamação"
	if len(hist) != 1 || hist[0].Kind != cases.HistoryKindMessage || hist[0].Text != want {
		t.Fatalf("history = %+v, want one mensagem row %q", hist, want)
	}
	if got := store.mustCase(t, open.ID); got.State != cases.StateEmAtendimento {
		t.Fatalf("state = %q, want unchanged", got.State)
	}
}

func TestIntake_ResolvedCaseOpensNew(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	customerID := identity.MustNewV7()
	resolved := cases.CaseRow{
		ID:              identity.MustNewV7(),
		CustomerID:      customerID,
		AdvisorID:       identity.MustNewV7(),
		State:           cases.StateResolvido,
		SLATotalMinutes: 60,
		OpenedAt:        triagedAt.Add(-24 * time.Hour),
	}
	store.seedCase(resolved)

	msg := cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "encerramento"}
	res, err := cases.Intake(t.Context(), store, singular(identity.MustNewV7()),
		triagedBody(t, identity.MustNewV7(), customerID, msg))
	if err != nil {
		t.Fatalf("Intake: %v", err)
	}
	if res.Decision != cases.DecisionOpened || res.CaseID == resolved.ID {
		t.Fatalf("result = %+v, want a new case", res)
	}
	if n := len(store.casesOf(customerID)); n != 2 {
		t.Fatalf("customer cases = %d, want 2", n)
	}
}

func TestIntake_DuplicateIsNoop(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	customerID := identity.MustNewV7()
	msg := cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "reclamacao"}
	body := triagedBody(t, identity.MustNewV7(), customerID, msg)
	lookup := singular(identity.MustNewV7())

	first, err := cases.Intake(t.Context(), store, lookup, body)
	if err != nil {
		t.Fatalf("first Intake: %v", err)
	}
	second, err := cases.Intake(t.Context(), store, lookup, body)
	if err != nil {
		t.Fatalf("second Intake: %v", err)
	}
	if second.Decision != cases.DecisionDuplicate || second.CaseID != "" {
		t.Fatalf("second = %+v, want duplicate", second)
	}
	if n := len(store.casesOf(customerID)); n != 1 {
		t.Fatalf("customer cases = %d, want 1", n)
	}
	if n := len(store.historyOf(first.CaseID)); n != 1 {
		t.Fatalf("history = %d, want 1", n)
	}
	if store.outboxCount() != 1 {
		t.Fatalf("outbox = %d, want 1", store.outboxCount())
	}
}

func TestIntake_LookupErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		lookup        *fakeLookup
		wantPermanent bool
		wantIs        error
	}{
		{
			name:   "advisory unavailable is transient",
			lookup: &fakeLookup{err: status.Error(codes.Unavailable, "down")},
		},
		{
			name:   "advisory deadline is transient",
			lookup: &fakeLookup{err: context.DeadlineExceeded},
			wantIs: context.DeadlineExceeded,
		},
		{
			name:          "unknown customer is permanent",
			lookup:        &fakeLookup{err: cases.ErrUnknownCustomer},
			wantPermanent: true,
			wantIs:        cases.ErrUnknownCustomer,
		},
		{
			name:          "advisor unresolvable in advisory is permanent",
			lookup:        &fakeLookup{err: cases.ErrUnusableCustomer},
			wantPermanent: true,
			wantIs:        cases.ErrUnusableCustomer,
		},
		{
			name:          "empty segment is permanent",
			lookup:        &fakeLookup{customer: cases.Customer{AdvisorID: identity.MustNewV7()}},
			wantPermanent: true,
			wantIs:        cases.ErrUnusableCustomer,
		},
		{
			name:          "unknown segment is permanent",
			lookup:        &fakeLookup{customer: cases.Customer{Segment: "Platinum", AdvisorID: identity.MustNewV7()}},
			wantPermanent: true,
			wantIs:        cases.ErrUnusableCustomer,
		},
		{
			name:          "empty advisor is permanent",
			lookup:        &fakeLookup{customer: cases.Customer{Segment: book.SegmentSingular}},
			wantPermanent: true,
			wantIs:        cases.ErrUnusableCustomer,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			eventID := identity.MustNewV7()
			msg := cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "reclamacao"}
			res, err := cases.Intake(t.Context(), store, tt.lookup, triagedBody(t, eventID, identity.MustNewV7(), msg))
			if err == nil {
				t.Fatal("Intake error = nil, want error")
			}
			var perm cases.PermanentDeliveryError
			if got := errors.As(err, &perm); got != tt.wantPermanent {
				t.Fatalf("permanent = %v, want %v (%v)", got, tt.wantPermanent, err)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("error = %v, want wrap of %v", err, tt.wantIs)
			}
			if res.EventID != eventID {
				t.Fatalf("result event id = %q, want %q", res.EventID, eventID)
			}
			if store.inboxCount() != 0 || store.caseCount() != 0 || store.outboxCount() != 0 {
				t.Fatalf("inbox=%d cases=%d outbox=%d, want nothing committed",
					store.inboxCount(), store.caseCount(), store.outboxCount())
			}
		})
	}
}

func TestIntake_BadBodyIsPermanent(t *testing.T) {
	t.Parallel()

	msg := cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "reclamacao"}
	valid := func(eventID, customerID string) []byte {
		return triagedBody(t, eventID, customerID, msg)
	}
	tests := []struct {
		name string
		body []byte
	}{
		{"invalid json", []byte(`{not json`)},
		{"missing event id", []byte(`{"occurred_at":"2026-09-29T14:00:00Z","customer_id":"` + identity.MustNewV7() + `","schema_version":1,"payload":{"intent":"reclamacao"}}`)},
		{"missing occurred at", []byte(`{"event_id":"` + identity.MustNewV7() + `","customer_id":"` + identity.MustNewV7() + `","schema_version":1,"payload":{"intent":"reclamacao"}}`)},
		{"bad schema version", []byte(`{"event_id":"` + identity.MustNewV7() + `","occurred_at":"2026-09-29T14:00:00Z","customer_id":"` + identity.MustNewV7() + `","schema_version":9,"payload":{"intent":"reclamacao"}}`)},
		{"event id not a uuid", valid("tr-1", identity.MustNewV7())},
		{"customer id not a uuidv7", valid(identity.MustNewV7(), "c01")},
		{"missing payload", []byte(`{"event_id":"` + identity.MustNewV7() + `","occurred_at":"2026-09-29T14:00:00Z","customer_id":"` + identity.MustNewV7() + `","schema_version":1}`)},
		{"payload of the wrong shape", []byte(`{"event_id":"` + identity.MustNewV7() + `","occurred_at":"2026-09-29T14:00:00Z","customer_id":"` + identity.MustNewV7() + `","schema_version":1,"payload":{"intent":7}}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			lookup := singular(identity.MustNewV7())
			_, err := cases.Intake(t.Context(), store, lookup, tt.body)
			wantPermanent(t, err)
			if store.inboxCount() != 0 || lookup.calls.Load() != 0 {
				t.Fatalf("inbox=%d lookups=%d, want 0/0", store.inboxCount(), lookup.calls.Load())
			}
		})
	}
}

func TestIntake_StoreFailureIsTransientAndRollsBack(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	store.failIns = "history"
	msg := cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "reclamacao"}
	_, err := cases.Intake(t.Context(), store, singular(identity.MustNewV7()),
		triagedBody(t, identity.MustNewV7(), identity.MustNewV7(), msg))
	if err == nil {
		t.Fatal("Intake error = nil, want error")
	}
	var perm cases.PermanentDeliveryError
	if errors.As(err, &perm) {
		t.Fatalf("error = %v, want transient", err)
	}
	if store.inboxCount() != 0 || store.caseCount() != 0 || store.outboxCount() != 0 || store.delayCount() != 0 {
		t.Fatal("a failed intake committed rows")
	}
}

func TestIntake_ConcurrentOpensOneCase(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	customerID := identity.MustNewV7()
	lookup := singular(identity.MustNewV7())
	const n = 8
	results := make([]cases.IntakeResult, n)
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		body := triagedBody(t, identity.MustNewV7(), customerID,
			cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "reclamacao"})
		wg.Go(func() {
			<-start
			results[i], errs[i] = cases.Intake(t.Context(), store, lookup, body)
		})
	}
	close(start)
	wg.Wait()

	opened, joined := 0, 0
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("Intake %d: %v", i, errs[i])
		}
		switch results[i].Decision {
		case cases.DecisionOpened:
			opened++
		case cases.DecisionJoined:
			joined++
		}
	}
	if opened != 1 || joined != n-1 {
		t.Fatalf("opened=%d joined=%d, want 1/%d", opened, joined, n-1)
	}
	if got := len(store.casesOf(customerID)); got != 1 {
		t.Fatalf("customer cases = %d, want 1", got)
	}
}

func TestIntake_RequiresDependencies(t *testing.T) {
	t.Parallel()

	if _, err := cases.Intake(t.Context(), nil, singular(identity.MustNewV7()), nil); err == nil {
		t.Fatal("nil store: error = nil")
	}
	if _, err := cases.Intake(t.Context(), newMemStore(), nil, nil); err == nil {
		t.Fatal("nil lookup: error = nil")
	}
}

func TestRetryTransient(t *testing.T) {
	t.Parallel()

	errTransient := errors.New("transient")
	tests := []struct {
		name      string
		failFirst int
		permanent bool
		wantCalls int
		wantErr   bool
	}{
		{name: "succeeds first", failFirst: 0, wantCalls: 1},
		{name: "succeeds on third", failFirst: 2, wantCalls: 3},
		{name: "gives up after attempts", failFirst: 100, wantCalls: cases.IntakeAttempts, wantErr: true},
		{name: "permanent stops at once", failFirst: 100, permanent: true, wantCalls: 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				start := time.Now()
				err := cases.RetryTransient(t.Context(), cases.IntakeAttempts, cases.IntakeBaseBackoff,
					func(context.Context) error {
						calls++
						if calls <= tt.failFirst {
							if tt.permanent {
								return cases.PermanentDeliveryError{Err: errTransient}
							}
							return errTransient
						}
						return nil
					})
				if (err != nil) != tt.wantErr {
					t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
				}
				if tt.wantErr && !errors.Is(err, errTransient) {
					t.Fatalf("err = %v, want wrap of the last error", err)
				}
				if calls != tt.wantCalls {
					t.Fatalf("calls = %d, want %d", calls, tt.wantCalls)
				}
				// Waits are 200·2^k ms with equal jitter: at least half of each.
				var minWait time.Duration
				for k := range calls - 1 {
					minWait += (cases.IntakeBaseBackoff << k) / 2
				}
				if waited := time.Since(start); waited < minWait {
					t.Fatalf("waited %v, want at least %v", waited, minWait)
				}
			})
		})
	}
}

func TestRetryTransient_StopsOnContextDone(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		errTransient := errors.New("transient")
		calls := 0
		err := cases.RetryTransient(ctx, cases.IntakeAttempts, cases.IntakeBaseBackoff,
			func(context.Context) error {
				calls++
				cancel()
				return errTransient
			})
		if !errors.Is(err, context.Canceled) || !errors.Is(err, errTransient) {
			t.Fatalf("err = %v, want canceled and the last error", err)
		}
		if calls != 1 {
			t.Fatalf("calls = %d, want 1", calls)
		}
	})
}
