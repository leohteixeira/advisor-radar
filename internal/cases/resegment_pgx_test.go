package cases_test

import (
	"testing"
	"time"

	casesv1 "github.com/leohteixeira/advisor-radar/gen/cases/v1"
	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// listedCase returns the case as the BFF reads it over gRPC.
func listedCase(t *testing.T, srv *cases.GRPCServer, customerID, caseID string) *casesv1.Case {
	t.Helper()
	res, err := srv.ListCases(t.Context(), &casesv1.ListCasesRequest{CustomerId: customerID})
	if err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	for _, c := range res.GetItems() {
		if c.GetId() == caseID {
			return c
		}
	}
	t.Fatalf("case %s is not listed for %s", caseID, customerID)
	return nil
}

// A segment change on PostgreSQL: the case's total and clock start move, the
// readers count from the new start, the delay is re-armed and published once,
// and only a tick of the current clock escalates.
func TestPGXStore_ResegmentRecalculatesRearmsAndEscalatesOnce(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)
	srv := cases.NewGRPCServer(cases.NewCaseReader(pool), store)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second)

	// Essencial, opened 20 h ago: 4 h of the 24 h are left.
	customerID, caseID := identity.MustNewV7(), identity.MustNewV7()
	if err := cases.Open(ctx, store, cases.OpenInput{
		ID: caseID, CustomerID: customerID, AdvisorID: identity.MustNewV7(),
		Segment: book.SegmentEssencial, OccurredAt: now.Add(-20 * time.Hour),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	first := &delayBroker{}
	if err := cases.PublishDelays(ctx, store, first); err != nil || len(first.ttls) != 1 || first.ttls[0] != 1440*60*1000 {
		t.Fatalf("first delay = %v, %v, want one of 24 h", first.ttls, err)
	}
	if got := listedCase(t, srv, customerID, caseID); got.GetSlaTotal() != 1440 || got.GetRemainingMinutes() < 239 || got.GetRemainingMinutes() > 240 {
		t.Fatalf("before the change: total %d, remaining %d, want 1440 and about 240", got.GetSlaTotal(), got.GetRemainingMinutes())
	}

	// The client became Advance one hour ago.
	changedAt := now.Add(-time.Hour)
	alert := alertBody(t, identity.MustNewV7(), customerID, "segmento", changedAt)
	res, err := cases.ResegmentAt(ctx, store, inSegment(book.SegmentAdvance), alert, changedAt)
	if err != nil || res.Decision != cases.DecisionRecalculated || res.CaseID != caseID {
		t.Fatalf("Resegment = %+v, %v, want recalculated", res, err)
	}
	row, ok, err := store.GetCase(ctx, caseID)
	if err != nil || !ok {
		t.Fatalf("GetCase = %v, %v", ok, err)
	}
	if row.SLATotalMinutes != 240 || !row.SLAStartedAt.Equal(changedAt) || !row.OpenedAt.Equal(now.Add(-20*time.Hour)) {
		t.Fatalf("case = %+v, want 240 minutes from %s and the opening untouched", row, changedAt)
	}
	got := listedCase(t, srv, customerID, caseID)
	if got.GetSlaTotal() != 240 || got.GetRemainingMinutes() < 179 || got.GetRemainingMinutes() > 180 {
		t.Fatalf("after the change: total %d, remaining %d, want 240 and about 180", got.GetSlaTotal(), got.GetRemainingMinutes())
	}
	if got.GetOpenedAgo() < 1200 || got.GetOpenedAgo() > 1201 {
		t.Fatalf("opened_ago = %d, want about 1200", got.GetOpenedAgo())
	}
	var slaLines []string
	for _, h := range got.GetHistory() {
		if h.GetKind() == cases.HistoryKindSLA {
			slaLines = append(slaLines, h.GetText())
		}
	}
	if len(slaLines) != 1 || slaLines[0] != "Cliente passou para o segmento Advance: SLA recalculado para 4 h" {
		t.Fatalf("sla history = %q", slaLines)
	}
	atRisk, err := srv.ListAtRisk(ctx, &casesv1.ListAtRiskRequest{})
	if err != nil || len(atRisk.GetItems()) != 0 {
		t.Fatalf("at risk = %v, %v, want none: three hours are left", atRisk.GetItems(), err)
	}

	// Redelivery changes nothing.
	if res, err := cases.ResegmentAt(ctx, store, inSegment(book.SegmentAdvance), alert, changedAt); err != nil || res.Decision != cases.DecisionDuplicate {
		t.Fatalf("redelivery = %+v, %v, want duplicate", res, err)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM case_history WHERE case_id = $1 AND kind = $2`, caseID, cases.HistoryKindSLA); n != 1 {
		t.Fatalf("sla history rows = %d, want 1", n)
	}

	// The publisher listed the re-armed delay, and the segment changed again
	// (Singular, 30 minutes ago) before it marked it: the mark of the older
	// arm must not hide the newer one.
	listed, err := store.ListUnpublishedDelays(ctx)
	if err != nil || len(listed) != 1 || listed[0].TTLMs != 240*60*1000 {
		t.Fatalf("unpublished delays = %+v, %v, want the 4 h arm", listed, err)
	}
	if res, err := cases.ResegmentAt(ctx, store, inSegment(book.SegmentSingular),
		alertBody(t, identity.MustNewV7(), customerID, "segmento", now.Add(-30*time.Minute)), now.Add(-30*time.Minute)); err != nil || res.Decision != cases.DecisionRecalculated {
		t.Fatalf("second Resegment = %+v, %v", res, err)
	}
	if err := store.MarkDelayPublished(ctx, listed[0], now); err != nil {
		t.Fatalf("MarkDelayPublished of the replaced arm: %v", err)
	}
	second := &delayBroker{}
	if err := cases.PublishDelays(ctx, store, second); err != nil || len(second.ttls) != 1 || second.ttls[0] != 60*60*1000 || second.bodies[0] != caseID {
		t.Fatalf("re-armed delay = %v %v, %v, want one of 1 h for the case", second.ttls, second.bodies, err)
	}
	if err := cases.PublishDelays(ctx, store, second); err != nil || len(second.ttls) != 1 {
		t.Fatalf("delay published again: %v, %v", second.ttls, err)
	}

	// Deadline now: 30 minutes from now. The 24 h and 4 h delays dead-letter
	// before it and are ignored without using up the one escalation.
	breached := func() int {
		return countWhere(t, pool, `SELECT count(*) FROM outbox WHERE routing_key = $1`, event.NameCaseSLABreached)
	}
	if err := cases.HandleBreach(ctx, store, caseID, now); err != nil {
		t.Fatalf("superseded HandleBreach: %v", err)
	}
	if row, _, _ := store.GetCase(ctx, caseID); row.Escalated || breached() != 0 {
		t.Fatalf("a superseded tick escalated: %+v, %d breach rows", row, breached())
	}
	for range 2 {
		if err := cases.HandleBreach(ctx, store, caseID, now.Add(30*time.Minute)); err != nil {
			t.Fatalf("HandleBreach at the deadline: %v", err)
		}
	}
	if row, _, _ := store.GetCase(ctx, caseID); !row.Escalated || breached() != 1 {
		t.Fatalf("case = %+v with %d breach rows, want escalated once", row, breached())
	}
}

func TestPGXStore_ResegmentLeavesResolvedCases(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)
	ctx := t.Context()
	openedAt := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

	customerID, caseID := identity.MustNewV7(), identity.MustNewV7()
	if err := cases.Open(ctx, store, cases.OpenInput{
		ID: caseID, CustomerID: customerID, AdvisorID: identity.MustNewV7(), Segment: book.SegmentEssencial, OccurredAt: openedAt,
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, to := range []string{cases.StateEmAtendimento, cases.StateAguardandoCliente, cases.StateResolvido} {
		if err := cases.Advance(ctx, store, caseID, to); err != nil {
			t.Fatalf("Advance to %s: %v", to, err)
		}
	}

	res, err := cases.ResegmentAt(ctx, store, inSegment(book.SegmentAdvance),
		alertBody(t, identity.MustNewV7(), customerID, "segmento", openedAt.Add(time.Hour)), openedAt.Add(time.Hour))
	if err != nil || res.Decision != cases.DecisionIgnored {
		t.Fatalf("Resegment = %+v, %v, want ignored", res, err)
	}
	row, ok, err := store.GetCase(ctx, caseID)
	if err != nil || !ok || row.SLATotalMinutes != 1440 || !row.SLAStartedAt.IsZero() || row.State != cases.StateResolvido {
		t.Fatalf("resolved case = %+v, %v, %v, want it untouched", row, ok, err)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM case_history WHERE case_id = $1 AND kind = $2`, caseID, cases.HistoryKindSLA); n != 0 {
		t.Fatalf("sla history rows = %d, want 0", n)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM cases`); n != 1 {
		t.Fatalf("cases = %d, want 1: an alert never opens a case", n)
	}
}

// The advisor's command on PostgreSQL: it opens a case for the signal, a
// second signal joins that case's history once, and the readers show which.
func TestPGXStore_OpenRequestedOpensThenJoinsOnce(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)
	srv := cases.NewGRPCServer(cases.NewCaseReader(pool), store)
	ctx := t.Context()
	lookup := inSegment(book.SegmentAdvance)
	customerID, first, second := identity.MustNewV7(), identity.MustNewV7(), identity.MustNewV7()

	opened, err := cases.OpenRequested(ctx, store, lookup,
		openCommandBody(t, identity.MustNewV7(), customerID, cases.OpenCommand{SignalID: first, Label: "Aporte grande"}))
	if err != nil || opened.Decision != cases.DecisionOpened {
		t.Fatalf("first command = %+v, %v, want opened", opened, err)
	}
	got := listedCase(t, srv, customerID, opened.CaseID)
	if got.GetSignal() != first || got.GetSlaTotal() != 240 || got.GetState() != 0 {
		t.Fatalf("opened case = %+v, want signal %s with the Advance SLA", got, first)
	}

	join := cases.OpenCommand{SignalID: second, Label: "Saque relevante", RelevantWithdrawal: true}
	for range 2 {
		// Two commands (two idempotency keys) for the same second signal.
		res, err := cases.OpenRequested(ctx, store, lookup, openCommandBody(t, identity.MustNewV7(), customerID, join))
		if err != nil || res.Decision != cases.DecisionJoined || res.CaseID != opened.CaseID {
			t.Fatalf("join = %+v, %v, want joined %s", res, err, opened.CaseID)
		}
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM cases WHERE customer_id = $1`, customerID); n != 1 {
		t.Fatalf("cases = %d, want 1 open case per customer", n)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM case_history WHERE case_id = $1 AND signal_id = $2`, opened.CaseID, second); n != 1 {
		t.Fatalf("history rows of the joined signal = %d, want 1", n)
	}
	got = listedCase(t, srv, customerID, opened.CaseID)
	if got.GetSignal() != first || got.GetSlaTotal() != 240 {
		t.Fatalf("joined case = %+v, want its own signal and SLA untouched", got)
	}
	var joined []*casesv1.CaseHistoryEntry
	for _, h := range got.GetHistory() {
		if h.GetSignal() != "" {
			joined = append(joined, h)
		}
	}
	if len(joined) != 1 || joined[0].GetSignal() != second || joined[0].GetKind() != cases.HistoryKindSignal ||
		joined[0].GetText() != "Sinal anexado ao caso pela assessoria: Saque relevante" {
		t.Fatalf("history with a signal = %+v", joined)
	}
}
