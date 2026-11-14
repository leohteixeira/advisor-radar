package cases_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// alertBody is an alert.raised delivery of the given kind raised at at.
func alertBody(t *testing.T, eventID, customerID, kind string, at time.Time) []byte {
	t.Helper()
	body, err := event.Envelope{
		Name:          event.NameAlertRaised,
		EventID:       eventID,
		OccurredAt:    at,
		CustomerID:    customerID,
		SchemaVersion: event.SchemaVersionMVP,
		Payload:       map[string]any{"kind": kind, "from": "Essencial", "to": "Advance"},
	}.MarshalBody()
	if err != nil {
		t.Fatalf("marshal alert: %v", err)
	}
	return body
}

func inSegment(segment string) *fakeLookup {
	return &fakeLookup{customer: cases.Customer{Segment: segment, AdvisorID: identity.MustNewV7()}}
}

// openFor opens a case for a new customer in segment at openedAt and returns
// the customer and case ids.
func openFor(t *testing.T, store *memStore, segment string, factors cases.ClockFactors, openedAt time.Time) (customerID, caseID string) {
	t.Helper()
	customerID, caseID = identity.MustNewV7(), identity.MustNewV7()
	if err := cases.Open(t.Context(), store, cases.OpenInput{
		ID:         caseID,
		CustomerID: customerID,
		AdvisorID:  identity.MustNewV7(),
		Segment:    segment,
		Factors:    factors,
		OccurredAt: openedAt,
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	return customerID, caseID
}

func (s *memStore) delaysOf(caseID string) []cases.DelayArm {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]cases.DelayArm, 0)
	for _, arm := range s.delays {
		if arm.CaseID == caseID {
			out = append(out, arm)
		}
	}
	return out
}

func TestRetotalMinutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		current  int
		segment  string
		expected int
	}{
		{name: "essencial to advance", current: 1440, segment: book.SegmentAdvance, expected: 240},
		{name: "advance to singular", current: 240, segment: book.SegmentSingular, expected: 60},
		{name: "singular down to essencial", current: 60, segment: book.SegmentEssencial, expected: 1440},
		{name: "halved essencial to advance stays halved", current: 720, segment: book.SegmentAdvance, expected: 120},
		{name: "halved advance to singular stays halved", current: 120, segment: book.SegmentSingular, expected: 30},
		{name: "halved singular down to advance stays halved", current: 30, segment: book.SegmentAdvance, expected: 120},
		{name: "same segment keeps the total", current: 240, segment: book.SegmentAdvance, expected: 240},
		{name: "same segment keeps a halved total", current: 120, segment: book.SegmentAdvance, expected: 120},
		{name: "unknown segment", current: 240, segment: "Prime", expected: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := cases.RetotalMinutes(tt.current, tt.segment); got != tt.expected {
				t.Fatalf("RetotalMinutes(%d, %q) = %d, want %d", tt.current, tt.segment, got, tt.expected)
			}
		})
	}
}

// RetotalMinutes reads "halved" from the stored total, which only works while
// no halved total equals a base total (ADR 0012). A new segment base that
// breaks it must add a column instead.
func TestTotals_HalvedNeverEqualsBase(t *testing.T) {
	t.Parallel()

	segments := []string{book.SegmentEssencial, book.SegmentAdvance, book.SegmentSingular}
	seen := make(map[int]string)
	for _, segment := range segments {
		for _, factors := range []cases.ClockFactors{{}, {ChurnRisk: true}} {
			total := cases.TotalMinutes(segment, factors)
			name := segment
			if factors.ChurnRisk {
				name += " halved"
			}
			if other, dup := seen[total]; dup {
				t.Fatalf("%s and %s share the total %d", other, name, total)
			}
			seen[total] = name
		}
	}
}

func TestResegment_UpgradeRecalculatesFromTheChange(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newMemStore()
	openedAt := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	changedAt := openedAt.Add(3 * time.Hour)
	customerID, caseID := openFor(t, store, book.SegmentEssencial, cases.ClockFactors{}, openedAt)
	lookup := inSegment(book.SegmentAdvance)

	res, err := cases.ResegmentAt(ctx, store, lookup, alertBody(t, identity.MustNewV7(), customerID, "segmento", changedAt), changedAt)
	if err != nil {
		t.Fatalf("Resegment: %v", err)
	}
	if res.Decision != cases.DecisionRecalculated || res.CaseID != caseID {
		t.Fatalf("result = %+v, want recalculated %s", res, caseID)
	}
	row := store.mustCase(t, caseID)
	if row.SLATotalMinutes != 240 {
		t.Fatalf("total = %d, want 240 (Advance)", row.SLATotalMinutes)
	}
	if !row.SLAStart().Equal(changedAt) {
		t.Fatalf("SLA start = %s, want the segment change %s", row.SLAStart(), changedAt)
	}
	if !row.OpenedAt.Equal(openedAt) {
		t.Fatalf("opened_at moved to %s", row.OpenedAt)
	}
	// One hour after the change, three of the four hours are left; counted
	// from the opening it would already be overdue against 240.
	if got := cases.RemainingMinutes(row, changedAt.Add(time.Hour)); got != 180 {
		t.Fatalf("remaining = %d, want 180", got)
	}
	arms := store.delaysOf(caseID)
	if len(arms) != 1 || arms[0].TTLMs != 240*60*1000 || arms[0].Published {
		t.Fatalf("delay arms = %+v, want one unpublished arm of 240 minutes", arms)
	}
	hist := store.historyOf(caseID)
	if len(hist) != 1 || hist[0].Kind != cases.HistoryKindSLA ||
		hist[0].Text != "Cliente passou para o segmento Advance: SLA recalculado para 4 h" {
		t.Fatalf("history = %+v", hist)
	}
	if n := len(store.casesOf(customerID)); n != 1 {
		t.Fatalf("cases = %d, want 1: an alert never opens a case", n)
	}
}

func TestResegment_DowngradeAndHalvedCase(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newMemStore()
	openedAt := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	changedAt := openedAt.Add(20 * time.Minute)
	// A churn-risk complaint from a Singular client: 30 minutes.
	customerID, caseID := openFor(t, store, book.SegmentSingular, cases.ClockFactors{ChurnRisk: true}, openedAt)

	res, err := cases.ResegmentAt(ctx, store, inSegment(book.SegmentAdvance),
		alertBody(t, identity.MustNewV7(), customerID, "segmento", changedAt), changedAt)
	if err != nil {
		t.Fatalf("Resegment: %v", err)
	}
	if res.Decision != cases.DecisionRecalculated {
		t.Fatalf("decision = %q, want recalculated", res.Decision)
	}
	row := store.mustCase(t, caseID)
	if row.SLATotalMinutes != 120 {
		t.Fatalf("total = %d, want 120: Advance, still halved", row.SLATotalMinutes)
	}
	if !row.SLAStart().Equal(changedAt) {
		t.Fatalf("SLA start = %s, want %s", row.SLAStart(), changedAt)
	}
	if hist := store.historyOf(caseID); len(hist) != 1 ||
		hist[0].Text != "Cliente passou para o segmento Advance: SLA recalculado para 2 h" {
		t.Fatalf("history = %+v", hist)
	}
}

func TestResegment_RedeliveryIsNoop(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newMemStore()
	openedAt := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	changedAt := openedAt.Add(time.Hour)
	customerID, caseID := openFor(t, store, book.SegmentEssencial, cases.ClockFactors{}, openedAt)
	lookup := inSegment(book.SegmentAdvance)
	body := alertBody(t, identity.MustNewV7(), customerID, "segmento", changedAt)

	if _, err := cases.ResegmentAt(ctx, store, lookup, body, changedAt); err != nil {
		t.Fatalf("first Resegment: %v", err)
	}
	// The delay of the first application is already with the broker.
	for _, arm := range store.delaysOf(caseID) {
		if err := store.MarkDelayPublished(ctx, arm, changedAt); err != nil {
			t.Fatalf("MarkDelayPublished: %v", err)
		}
	}
	calls := lookup.calls.Load()

	res, err := cases.ResegmentAt(ctx, store, lookup, body, changedAt)
	if err != nil {
		t.Fatalf("second Resegment: %v", err)
	}
	if res.Decision != cases.DecisionDuplicate {
		t.Fatalf("decision = %q, want duplicate", res.Decision)
	}
	if lookup.calls.Load() != calls {
		t.Fatal("a duplicate called advisory")
	}
	if hist := store.historyOf(caseID); len(hist) != 1 {
		t.Fatalf("history rows = %d, want 1", len(hist))
	}
	arms := store.delaysOf(caseID)
	if len(arms) != 1 || !arms[0].Published {
		t.Fatalf("delay arms = %+v, want the one published arm untouched", arms)
	}

	// Another alert for a segment the case already follows changes nothing.
	res, err = cases.ResegmentAt(ctx, store, lookup,
		alertBody(t, identity.MustNewV7(), customerID, "segmento", changedAt.Add(time.Hour)), changedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("third Resegment: %v", err)
	}
	if res.Decision != cases.DecisionIgnored {
		t.Fatalf("decision = %q, want ignored", res.Decision)
	}
	row := store.mustCase(t, caseID)
	if !row.SLAStart().Equal(changedAt) || row.SLATotalMinutes != 240 {
		t.Fatalf("case = %+v, want the clock of the first alert", row)
	}
	if hist := store.historyOf(caseID); len(hist) != 1 {
		t.Fatalf("history rows = %d, want 1", len(hist))
	}
}

func TestResegment_ResolvedCaseIsUntouched(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newMemStore()
	openedAt := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	customerID, caseID := openFor(t, store, book.SegmentEssencial, cases.ClockFactors{}, openedAt)
	for _, to := range []string{cases.StateEmAtendimento, cases.StateAguardandoCliente, cases.StateResolvido} {
		if err := cases.Advance(ctx, store, caseID, to); err != nil {
			t.Fatalf("Advance %s: %v", to, err)
		}
	}
	before := store.mustCase(t, caseID)
	lookup := inSegment(book.SegmentAdvance)

	res, err := cases.ResegmentAt(ctx, store, lookup,
		alertBody(t, identity.MustNewV7(), customerID, "segmento", openedAt.Add(time.Hour)), openedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("Resegment: %v", err)
	}
	if res.Decision != cases.DecisionIgnored || res.CaseID != "" {
		t.Fatalf("result = %+v, want ignored without a case", res)
	}
	if after := store.mustCase(t, caseID); after != before {
		t.Fatalf("resolved case changed: %+v -> %+v", before, after)
	}
	if lookup.calls.Load() != 0 {
		t.Fatal("a client without an open case called advisory")
	}
	// Three state changes are the whole history.
	if len(store.historyOf(caseID)) != 3 || len(store.delaysOf(caseID)) != 1 {
		t.Fatalf("history = %+v, delays = %+v", store.historyOf(caseID), store.delaysOf(caseID))
	}
	if n := len(store.casesOf(customerID)); n != 1 {
		t.Fatalf("cases = %d, want 1", n)
	}
}

// No alert kind opens a case, and only segmento reaches the store.
func TestResegment_OtherAlertKindsDoNothing(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"saque", "queda", "aporte", "contato", "perfil"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			openedAt := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
			customerID, caseID := openFor(t, store, book.SegmentEssencial, cases.ClockFactors{}, openedAt)
			before := store.mustCase(t, caseID)
			lookup := inSegment(book.SegmentAdvance)

			res, err := cases.ResegmentAt(t.Context(), store, lookup,
				alertBody(t, identity.MustNewV7(), customerID, kind, openedAt.Add(time.Hour)), openedAt.Add(time.Hour))
			if err != nil {
				t.Fatalf("Resegment: %v", err)
			}
			if res.Decision != cases.DecisionIgnored {
				t.Fatalf("decision = %q, want ignored", res.Decision)
			}
			if store.mustCase(t, caseID) != before || store.inboxCount() != 0 || lookup.calls.Load() != 0 {
				t.Fatalf("a %s alert touched the store or advisory", kind)
			}
			if n := len(store.casesOf(customerID)); n != 1 {
				t.Fatalf("cases = %d, want 1", n)
			}
		})
	}
}

func TestResegment_ClockNeverRestartsBeforeItsStart(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	openedAt := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	customerID, caseID := openFor(t, store, book.SegmentEssencial, cases.ClockFactors{}, openedAt)

	// The fact behind the alert is older than the case.
	if _, err := cases.ResegmentAt(t.Context(), store, inSegment(book.SegmentAdvance),
		alertBody(t, identity.MustNewV7(), customerID, "segmento", openedAt.Add(-time.Minute)), openedAt.Add(-time.Minute)); err != nil {
		t.Fatalf("Resegment: %v", err)
	}
	row := store.mustCase(t, caseID)
	if !row.SLAStart().Equal(openedAt) || row.SLATotalMinutes != 240 {
		t.Fatalf("case = %+v, want 240 minutes from the opening", row)
	}
}

func TestResegment_Errors(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	transient := errors.New("advisory unavailable")
	tests := []struct {
		name          string
		lookup        *fakeLookup
		wantPermanent bool
	}{
		{name: "advisory down is transient", lookup: &fakeLookup{err: transient}},
		{name: "unknown customer is permanent", lookup: &fakeLookup{err: cases.ErrUnknownCustomer}, wantPermanent: true},
		{name: "unknown segment is permanent", lookup: inSegment("Prime"), wantPermanent: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			customerID, caseID := openFor(t, store, book.SegmentEssencial, cases.ClockFactors{}, openedAt)
			before := store.mustCase(t, caseID)

			_, err := cases.ResegmentAt(t.Context(), store, tt.lookup,
				alertBody(t, identity.MustNewV7(), customerID, "segmento", openedAt.Add(time.Hour)), openedAt.Add(time.Hour))
			if err == nil {
				t.Fatal("Resegment error = nil")
			}
			var perm cases.PermanentDeliveryError
			if got := errors.As(err, &perm); got != tt.wantPermanent {
				t.Fatalf("permanent = %v, want %v (%v)", got, tt.wantPermanent, err)
			}
			if store.mustCase(t, caseID) != before || store.inboxCount() != 0 {
				t.Fatal("a failed recalculation changed the case or claimed the inbox")
			}
		})
	}

	t.Run("bad body is permanent", func(t *testing.T) {
		t.Parallel()
		_, err := cases.Resegment(t.Context(), newMemStore(), inSegment(book.SegmentAdvance), []byte(`{"event_id":`))
		wantPermanent(t, err)
	})
	t.Run("dependencies are required", func(t *testing.T) {
		t.Parallel()
		if _, err := cases.Resegment(t.Context(), nil, inSegment(book.SegmentAdvance), nil); err == nil {
			t.Fatal("nil store: error = nil")
		}
		if _, err := cases.Resegment(t.Context(), newMemStore(), nil, nil); err == nil {
			t.Fatal("nil lookup: error = nil")
		}
	})
}

// The escalation of a clock a segment change superseded must not fire; the
// re-armed one must, once.
func TestHandleBreach_SupersededTickIsIgnored(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newMemStore()
	openedAt := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	// Singular: 60 minutes, due 10:00.
	customerID, caseID := openFor(t, store, book.SegmentSingular, cases.ClockFactors{}, openedAt)
	// Downgrade to Advance at 09:30: 240 minutes, due 13:30.
	changedAt := openedAt.Add(30 * time.Minute)
	if _, err := cases.ResegmentAt(ctx, store, inSegment(book.SegmentAdvance),
		alertBody(t, identity.MustNewV7(), customerID, "segmento", changedAt), changedAt); err != nil {
		t.Fatalf("Resegment: %v", err)
	}
	breaches := func() int {
		n := 0
		for _, k := range store.routingKeys() {
			if k == event.NameCaseSLABreached {
				n++
			}
		}
		return n
	}

	// The original delay dead-letters at 10:00, before the current deadline.
	if err := cases.HandleBreach(ctx, store, caseID, openedAt.Add(time.Hour)); err != nil {
		t.Fatalf("superseded HandleBreach: %v", err)
	}
	if row := store.mustCase(t, caseID); row.Escalated || breaches() != 0 {
		t.Fatalf("superseded tick escalated: %+v, %d breach rows", row, breaches())
	}

	// The re-armed delay dead-letters at the current deadline and escalates.
	deadline := store.mustCase(t, caseID).SLADeadline()
	if !deadline.Equal(changedAt.Add(4 * time.Hour)) {
		t.Fatalf("deadline = %s, want %s", deadline, changedAt.Add(4*time.Hour))
	}
	for range 2 {
		if err := cases.HandleBreach(ctx, store, caseID, deadline); err != nil {
			t.Fatalf("HandleBreach at the deadline: %v", err)
		}
	}
	if row := store.mustCase(t, caseID); !row.Escalated || breaches() != 1 {
		t.Fatalf("case = %+v with %d breach rows, want escalated once", row, breaches())
	}
}

// A legacy delay (plain-text body, per-message TTL) of a case that was never
// recalculated dead-letters at or after its deadline and still escalates.
func TestApplyBreachDelivery_LegacyTickOfUntouchedCaseEscalates(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	_, caseID := openFor(t, store, book.SegmentSingular, cases.ClockFactors{}, time.Now().UTC().Add(-61*time.Minute))
	if err := cases.ApplyBreachDelivery(t.Context(), store, []byte(caseID)); err != nil {
		t.Fatalf("ApplyBreachDelivery: %v", err)
	}
	if !store.mustCase(t, caseID).Escalated {
		t.Fatal("escalated = false, want true")
	}
}

// A tick for a case a reseed removed is dropped, not requeued forever.
func TestHandleBreach_MissingCaseIsPermanent(t *testing.T) {
	t.Parallel()

	err := cases.HandleBreach(context.Background(), newMemStore(), identity.MustNewV7(), time.Now())
	wantPermanent(t, err)
}

func TestPublishDelays_RearmedDelayIsNotMarkedByTheOldArm(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	store := newMemStore()
	openedAt := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	customerID, caseID := openFor(t, store, book.SegmentEssencial, cases.ClockFactors{}, openedAt)
	listed, err := store.ListUnpublishedDelays(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListUnpublishedDelays = %+v, %v", listed, err)
	}

	// The segment changes after the publisher listed the first arm and before
	// it marked it.
	if _, err := cases.ResegmentAt(ctx, store, inSegment(book.SegmentSingular),
		alertBody(t, identity.MustNewV7(), customerID, "segmento", openedAt.Add(time.Hour)), openedAt.Add(time.Hour)); err != nil {
		t.Fatalf("Resegment: %v", err)
	}
	if err := store.MarkDelayPublished(ctx, listed[0], openedAt); err != nil {
		t.Fatalf("MarkDelayPublished: %v", err)
	}

	broker := &delayBroker{}
	if err := cases.PublishDelays(ctx, store, broker); err != nil {
		t.Fatalf("PublishDelays: %v", err)
	}
	if len(broker.ttls) != 1 || broker.ttls[0] != 60*60*1000 || broker.bodies[0] != caseID {
		t.Fatalf("published = %v %v, want the 60-minute delay of %s", broker.ttls, broker.bodies, caseID)
	}
}

// The escalation delay is armed with the full SLA when the alert is
// processed. An alert processed late (a backlog, services stopped during a
// deploy or a reseed) therefore counts the SLA from the processing instant:
// counted from occurred_at the case would read overdue for the length of the
// lag while its tick is still that far away. An alert processed within the
// tolerance keeps occurred_at.
func TestResegment_LateAlertCountsFromProcessing(t *testing.T) {
	t.Parallel()

	openedAt := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	changedAt := openedAt.Add(time.Hour)
	tests := []struct {
		name      string
		lag       time.Duration
		wantStart time.Duration // after changedAt
	}{
		{name: "on time", lag: 2 * time.Second, wantStart: 0},
		{name: "at the tolerance", lag: cases.ResegmentLagTolerance, wantStart: 0},
		{name: "five hours late, longer than the new SLA", lag: 5 * time.Hour, wantStart: 5 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			store := newMemStore()
			customerID, caseID := openFor(t, store, book.SegmentEssencial, cases.ClockFactors{}, openedAt)
			processedAt := changedAt.Add(tt.lag)
			if _, err := cases.ResegmentAt(ctx, store, inSegment(book.SegmentAdvance),
				alertBody(t, identity.MustNewV7(), customerID, "segmento", changedAt), processedAt); err != nil {
				t.Fatalf("Resegment: %v", err)
			}
			row := store.mustCase(t, caseID)
			if want := changedAt.Add(tt.wantStart); !row.SLAStart().Equal(want) {
				t.Fatalf("SLA start = %s, want %s", row.SLAStart(), want)
			}
			// The delay is published now with the full SLA as TTL: the tick
			// lands at the deadline the screens show, give or take the lag
			// tolerance, and until then the case is not overdue.
			tick := processedAt.Add(240 * time.Minute)
			if early := tick.Sub(row.SLADeadline()); early < 0 || early > cases.ResegmentLagTolerance {
				t.Fatalf("tick at %s, deadline %s: they disagree by %s", tick, row.SLADeadline(), early)
			}
			if state := cases.SLAState(row, processedAt); state != cases.SLAStateOnTime {
				t.Fatalf("state at processing = %q, want %q", state, cases.SLAStateOnTime)
			}
			if err := cases.HandleBreach(ctx, store, caseID, tick); err != nil {
				t.Fatalf("HandleBreach: %v", err)
			}
			if !store.mustCase(t, caseID).Escalated {
				t.Fatal("the re-armed tick did not escalate the case")
			}
		})
	}
}
