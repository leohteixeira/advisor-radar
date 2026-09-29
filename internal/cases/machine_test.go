package cases_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

type memStore struct {
	mu      sync.Mutex
	cases   map[string]cases.CaseRow
	inbox   map[string]struct{}
	outbox  map[string]memOutbox
	order   []string
	delays  []cases.DelayArm
	history []cases.HistoryRow
	locks   map[string]*sync.Mutex
	failIns string
}

type memOutbox struct {
	row         cases.OutboxRow
	publishedAt *time.Time
}

type memTx struct {
	store   *memStore
	cases   map[string]cases.CaseRow
	inbox   map[string]struct{}
	outbox  map[string]cases.OutboxRow
	order   []string
	delays  []cases.DelayArm
	history []cases.HistoryRow
	dirty   map[string]cases.CaseRow
	held    []*sync.Mutex
}

func newMemStore() *memStore {
	return &memStore{
		cases:  make(map[string]cases.CaseRow),
		inbox:  make(map[string]struct{}),
		outbox: make(map[string]memOutbox),
		locks:  make(map[string]*sync.Mutex),
	}
}

func (s *memStore) WithTx(ctx context.Context, fn func(cases.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx := &memTx{
		store:  s,
		cases:  make(map[string]cases.CaseRow),
		inbox:  make(map[string]struct{}),
		outbox: make(map[string]cases.OutboxRow),
		dirty:  make(map[string]cases.CaseRow),
	}
	// Customer locks are released after commit or rollback, like
	// pg_advisory_xact_lock.
	defer func() {
		for _, m := range tx.held {
			m.Unlock()
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, row := range tx.cases {
		if _, exists := s.cases[id]; !exists {
			s.cases[id] = row
		}
	}
	for id, row := range tx.dirty {
		s.cases[id] = row
	}
	for id := range tx.inbox {
		s.inbox[id] = struct{}{}
	}
	for _, id := range tx.order {
		row := tx.outbox[id]
		if _, exists := s.outbox[id]; exists {
			continue
		}
		s.outbox[id] = memOutbox{row: row}
		s.order = append(s.order, id)
	}
	s.delays = append(s.delays, tx.delays...)
	s.history = append(s.history, tx.history...)
	return nil
}

func (s *memStore) GetCase(ctx context.Context, id string) (cases.CaseRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return cases.CaseRow{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.cases[id]
	return row, ok, nil
}

func (s *memStore) InboxSeen(ctx context.Context, eventID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.inbox[eventID]
	return ok, nil
}

func (s *memStore) OpenCaseFor(ctx context.Context, customerID string) (cases.CaseRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return cases.CaseRow{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.cases {
		if row.CustomerID == customerID && row.State != cases.StateResolvido {
			return row, true, nil
		}
	}
	return cases.CaseRow{}, false, nil
}

func (t *memTx) GetCase(ctx context.Context, id string) (cases.CaseRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return cases.CaseRow{}, false, err
	}
	if row, ok := t.dirty[id]; ok {
		return row, true, nil
	}
	if row, ok := t.cases[id]; ok {
		return row, true, nil
	}
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	row, ok := t.store.cases[id]
	return row, ok, nil
}

func (t *memTx) ClaimInbox(ctx context.Context, eventID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	t.store.mu.Lock()
	_, seen := t.store.inbox[eventID]
	t.store.mu.Unlock()
	if seen {
		return false, nil
	}
	if _, ok := t.inbox[eventID]; ok {
		return false, nil
	}
	t.inbox[eventID] = struct{}{}
	return true, nil
}

func (t *memTx) InsertCase(ctx context.Context, row cases.CaseRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.store.failIns == "case" {
		return errForcedCaseInsert
	}
	t.store.mu.Lock()
	_, exists := t.store.cases[row.ID]
	t.store.mu.Unlock()
	if exists {
		return nil
	}
	if _, ok := t.cases[row.ID]; ok {
		return nil
	}
	t.cases[row.ID] = row
	return nil
}

func (t *memTx) UpdateCase(ctx context.Context, row cases.CaseRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.dirty[row.ID] = row
	return nil
}

func (t *memTx) InsertOutbox(ctx context.Context, row cases.OutboxRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.store.failIns == "outbox" {
		return errors.New("forced outbox insert failure")
	}
	if _, ok := t.outbox[row.EventID]; ok {
		return nil
	}
	t.outbox[row.EventID] = row
	t.order = append(t.order, row.EventID)
	return nil
}

func (t *memTx) ArmDelay(ctx context.Context, caseID string, ttlMs int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.store.failIns == "delay" {
		return errors.New("forced delay arm failure")
	}
	t.delays = append(t.delays, cases.DelayArm{CaseID: caseID, TTLMs: ttlMs})
	return nil
}

func (t *memTx) LockCustomer(ctx context.Context, customerID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.store.mu.Lock()
	m, ok := t.store.locks[customerID]
	if !ok {
		m = &sync.Mutex{}
		t.store.locks[customerID] = m
	}
	t.store.mu.Unlock()
	m.Lock()
	t.held = append(t.held, m)
	return nil
}

func (t *memTx) OpenCaseFor(ctx context.Context, customerID string) (cases.CaseRow, bool, error) {
	if err := ctx.Err(); err != nil {
		return cases.CaseRow{}, false, err
	}
	for _, row := range t.dirty {
		if row.CustomerID == customerID && row.State != cases.StateResolvido {
			return row, true, nil
		}
	}
	for _, row := range t.cases {
		if row.CustomerID == customerID && row.State != cases.StateResolvido {
			return row, true, nil
		}
	}
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	for id, row := range t.store.cases {
		if _, shadowed := t.dirty[id]; shadowed {
			continue
		}
		if row.CustomerID == customerID && row.State != cases.StateResolvido {
			return row, true, nil
		}
	}
	return cases.CaseRow{}, false, nil
}

func (t *memTx) InsertHistory(ctx context.Context, row cases.HistoryRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.store.failIns == "history" {
		return errors.New("forced history insert failure")
	}
	t.history = append(t.history, row)
	return nil
}

func (s *memStore) ListUnpublished(ctx context.Context) ([]cases.OutboxRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]cases.OutboxRow, 0)
	for _, id := range s.order {
		r := s.outbox[id]
		if r.publishedAt != nil {
			continue
		}
		out = append(out, r.row)
	}
	return out, nil
}

func (s *memStore) MarkPublished(ctx context.Context, eventID string, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.outbox[eventID]
	if !ok {
		return fmt.Errorf("unknown event_id %s", eventID)
	}
	ts := at
	r.publishedAt = &ts
	s.outbox[eventID] = r
	return nil
}

func (s *memStore) caseCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cases)
}

func (s *memStore) outboxCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.outbox)
}

func (s *memStore) publishedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.outbox {
		if r.publishedAt != nil {
			n++
		}
	}
	return n
}

func (s *memStore) ListUnpublishedDelays(ctx context.Context) ([]cases.DelayArm, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]cases.DelayArm, 0)
	for _, row := range s.delays {
		if row.Published {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *memStore) MarkDelayPublished(ctx context.Context, caseID string, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.delays {
		if s.delays[i].CaseID == caseID && !s.delays[i].Published {
			s.delays[i].Published = true
			_ = at
			return nil
		}
	}
	return fmt.Errorf("unknown delay %s", caseID)
}

func (s *memStore) delayCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.delays)
}

func (s *memStore) mustCase(t *testing.T, id string) cases.CaseRow {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.cases[id]
	if !ok {
		t.Fatalf("missing case %s", id)
	}
	return row
}

func (s *memStore) routingKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.outbox[id].row.RoutingKey)
	}
	return out
}

var errForcedCaseInsert = errors.New("forced case insert failure")

type fakeBroker struct {
	mu        sync.Mutex
	sent      []cases.OutboxRow
	refuseID  string
	refuseErr error
}

func (b *fakeBroker) Publish(ctx context.Context, routingKey string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	eventID, _ := parsed["event_id"].(string)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.refuseID != "" && eventID == b.refuseID {
		return b.refuseErr
	}
	b.sent = append(b.sent, cases.OutboxRow{
		EventID:    eventID,
		RoutingKey: routingKey,
		Payload:    body,
	})
	return nil
}

func (b *fakeBroker) sentCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.sent)
}

func TestOpen_AdvanceComplaint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	err := cases.Open(ctx, store, cases.OpenInput{
		ID:         "k-open-c02",
		CustomerID: "c02",
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		Factors:    cases.ClockFactors{Frustration: 3},
		OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	row := store.mustCase(t, "k-open-c02")
	if row.State != cases.StateAberto {
		t.Fatalf("state = %q, want %q", row.State, cases.StateAberto)
	}
	if row.SLATotalMinutes != 120 {
		t.Fatalf("sla = %d, want 120", row.SLATotalMinutes)
	}
	keys := store.routingKeys()
	if len(keys) != 1 || keys[0] != event.NameCaseOpened {
		t.Fatalf("outbox keys = %v, want [%s]", keys, event.NameCaseOpened)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.delays) != 1 {
		t.Fatalf("delays = %d, want 1", len(store.delays))
	}
	wantTTL := 120 * 60 * 1000
	if store.delays[0].TTLMs != wantTTL {
		t.Fatalf("delay TTL = %d, want %d", store.delays[0].TTLMs, wantTTL)
	}
}

func TestAdvance_ForwardPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	if err := cases.Open(ctx, store, cases.OpenInput{
		ID:         "k-adv",
		CustomerID: "c02",
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}

	path := []string{
		cases.StateEmAtendimento,
		cases.StateAguardandoCliente,
		cases.StateResolvido,
	}
	for _, to := range path {
		if err := cases.Advance(ctx, store, "k-adv", to); err != nil {
			t.Fatalf("Advance to %s: %v", to, err)
		}
	}

	row := store.mustCase(t, "k-adv")
	if row.State != cases.StateResolvido {
		t.Fatalf("state = %q, want %q", row.State, cases.StateResolvido)
	}
	keys := store.routingKeys()
	want := []string{
		event.NameCaseOpened,
		event.NameCaseStatusChanged,
		event.NameCaseStatusChanged,
		event.NameCaseStatusChanged,
	}
	if len(keys) != len(want) {
		t.Fatalf("outbox keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("outbox[%d] = %q, want %q", i, keys[i], want[i])
		}
	}
}

func TestAdvance_BackwardRejected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	if err := cases.Open(ctx, store, cases.OpenInput{
		ID:         "k-back",
		CustomerID: "c02",
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := cases.Advance(ctx, store, "k-back", cases.StateEmAtendimento); err != nil {
		t.Fatalf("Advance forward: %v", err)
	}

	err := cases.Advance(ctx, store, "k-back", cases.StateAberto)
	if err == nil {
		t.Fatal("Advance backward error = nil, want error")
	}
	row := store.mustCase(t, "k-back")
	if row.State != cases.StateEmAtendimento {
		t.Fatalf("state = %q, want %q", row.State, cases.StateEmAtendimento)
	}
}

func TestAdvance_PastEndRejected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	if err := cases.Open(ctx, store, cases.OpenInput{
		ID:         "k-end",
		CustomerID: "c02",
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, to := range []string{
		cases.StateEmAtendimento,
		cases.StateAguardandoCliente,
		cases.StateResolvido,
	} {
		if err := cases.Advance(ctx, store, "k-end", to); err != nil {
			t.Fatalf("Advance to %s: %v", to, err)
		}
	}

	err := cases.Advance(ctx, store, "k-end", cases.StateResolvido)
	if err == nil {
		t.Fatal("Advance past end error = nil, want error")
	}
	row := store.mustCase(t, "k-end")
	if row.State != cases.StateResolvido {
		t.Fatalf("state = %q, want %q", row.State, cases.StateResolvido)
	}
}

func TestHandleBreach_Idempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	if err := cases.Open(ctx, store, cases.OpenInput{
		ID:         "k-breach",
		CustomerID: "c02",
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		Factors:    cases.ClockFactors{Frustration: 2},
		OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	before := store.mustCase(t, "k-breach")

	if err := cases.HandleBreach(ctx, store, "k-breach", time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("first HandleBreach: %v", err)
	}
	after := store.mustCase(t, "k-breach")
	if !after.Escalated {
		t.Fatal("escalated = false, want true")
	}
	if after.State != before.State {
		t.Fatalf("state changed from %q to %q", before.State, after.State)
	}
	keys := store.routingKeys()
	breachCount := 0
	for _, k := range keys {
		if k == event.NameCaseSLABreached {
			breachCount++
		}
	}
	if breachCount != 1 {
		t.Fatalf("breach outbox rows = %d, want 1", breachCount)
	}

	if err := cases.HandleBreach(ctx, store, "k-breach", time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("second HandleBreach: %v", err)
	}
	if store.outboxCount() != 2 { // opened + one breach
		t.Fatalf("outbox = %d, want 2 after second breach", store.outboxCount())
	}
}

func TestSLADelayArgs_Queue(t *testing.T) {
	t.Parallel()

	args := cases.SLADelayArgs(60)
	if args.TTLMs != 3_600_000 {
		t.Fatalf("TTLMs = %d, want 3600000", args.TTLMs)
	}
	if args.DeadLetterRoutingKey != event.NameCaseSLABreached {
		t.Fatalf("dead-letter key = %q, want %q", args.DeadLetterRoutingKey, event.NameCaseSLABreached)
	}

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	for _, rel := range []string{
		"cmd/cases/main.go",
		"internal/cases/machine.go",
		"internal/cases/clock.go",
		"internal/cases/queue.go",
		"internal/cases/pgx.go",
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		lower := strings.ToLower(string(data))
		if strings.Contains(lower, "cron") {
			t.Fatalf("%s must not contain cron", rel)
		}
	}
}

func TestPublish_BrokerRefusesThenAccepts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	if err := cases.Open(ctx, store, cases.OpenInput{
		ID:         "k-pub",
		CustomerID: "c02",
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		Factors:    cases.ClockFactors{Frustration: 2},
		OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}

	okBroker := &fakeBroker{}
	if err := cases.Publish(ctx, store, okBroker); err != nil {
		t.Fatalf("Publish opened: %v", err)
	}
	if store.publishedCount() != 1 {
		t.Fatalf("published = %d, want 1 after opened", store.publishedCount())
	}

	if err := cases.HandleBreach(ctx, store, "k-pub", time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("HandleBreach: %v", err)
	}

	refuse := errors.New("broker refused")
	breachID := ""
	store.mu.Lock()
	for id, r := range store.outbox {
		if r.row.RoutingKey == event.NameCaseSLABreached {
			breachID = id
		}
	}
	store.mu.Unlock()
	if breachID == "" {
		t.Fatal("no breach outbox row")
	}
	broker := &fakeBroker{refuseID: breachID, refuseErr: refuse}
	err := cases.Publish(ctx, store, broker)
	if err == nil {
		t.Fatal("Publish error = nil, want wrapped broker error")
	}
	if !errors.Is(err, refuse) {
		t.Fatalf("Publish error = %v, want wrap of %v", err, refuse)
	}
	if store.publishedCount() != 1 {
		t.Fatalf("published = %d, want 1 after refuse", store.publishedCount())
	}

	broker.refuseID = ""
	if err := cases.Publish(ctx, store, broker); err != nil {
		t.Fatalf("Publish after accept: %v", err)
	}
	if store.publishedCount() != 2 {
		t.Fatalf("published = %d, want 2", store.publishedCount())
	}
	if broker.sentCount() != 1 {
		t.Fatalf("sent = %d, want 1 breach", broker.sentCount())
	}
	broker.mu.Lock()
	gotKey := broker.sent[0].RoutingKey
	broker.mu.Unlock()
	if gotKey != event.NameCaseSLABreached {
		t.Fatalf("sent routing key = %q, want %q", gotKey, event.NameCaseSLABreached)
	}
}

func TestOpen_Rollback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	store.failIns = "case"
	err := cases.Open(ctx, store, cases.OpenInput{
		ID:         "k-roll",
		CustomerID: "c02",
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		Factors:    cases.ClockFactors{Frustration: 2},
		OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("Open error = nil, want wrapped insert error")
	}
	if !errors.Is(err, errForcedCaseInsert) {
		t.Fatalf("Open error = %v, want wrap of %v", err, errForcedCaseInsert)
	}
	if store.caseCount() != 0 {
		t.Fatalf("cases = %d, want 0 after rollback", store.caseCount())
	}
	if store.outboxCount() != 0 {
		t.Fatalf("outbox = %d, want 0 after rollback", store.outboxCount())
	}
	if store.delayCount() != 0 {
		t.Fatalf("delays = %d, want 0 after rollback", store.delayCount())
	}
}

type delayBroker struct {
	mu     sync.Mutex
	ttls   []int
	bodies []string
	fail   bool
}

func (b *delayBroker) PublishDelay(_ context.Context, _ string, ttlMs int, body []byte) error {
	if b.fail {
		return errors.New("broker refused delay")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ttls = append(b.ttls, ttlMs)
	b.bodies = append(b.bodies, string(body))
	return nil
}

func TestPublishDelays_OnceAfterAck(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	if err := cases.Open(ctx, store, cases.OpenInput{
		ID:         "k-delay",
		CustomerID: "c02",
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		Factors:    cases.ClockFactors{Frustration: 3},
		OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}

	broker := &delayBroker{fail: true}
	if err := cases.PublishDelays(ctx, store, broker); err == nil {
		t.Fatal("PublishDelays error = nil, want broker refusal")
	}
	pending, err := store.ListUnpublishedDelays(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("unpublished delays = %d, want 1 after refusal", len(pending))
	}

	broker.fail = false
	if err := cases.PublishDelays(ctx, store, broker); err != nil {
		t.Fatalf("PublishDelays: %v", err)
	}
	wantTTL := 120 * 60 * 1000
	if len(broker.ttls) != 1 || broker.ttls[0] != wantTTL {
		t.Fatalf("published TTL = %v, want [%d]", broker.ttls, wantTTL)
	}
	if len(broker.bodies) != 1 || broker.bodies[0] != "k-delay" {
		t.Fatalf("delay body = %v, want [k-delay]", broker.bodies)
	}
	if err := cases.PublishDelays(ctx, store, broker); err != nil {
		t.Fatalf("second PublishDelays: %v", err)
	}
	if len(broker.ttls) != 1 {
		t.Fatalf("published delays = %d, want 1", len(broker.ttls))
	}
}

// dualBroker implements Broker and DelayBroker for RunPublisher coverage.
type dualBroker struct {
	mu         sync.Mutex
	refuse     bool
	refuseErr  error
	attempts   int
	sentOutbox int
	sentDelay  int
}

func (b *dualBroker) Publish(ctx context.Context, routingKey string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.attempts++
	if b.refuse {
		return b.refuseErr
	}
	b.sentOutbox++
	_ = routingKey
	_ = body
	return nil
}

func (b *dualBroker) PublishDelay(ctx context.Context, caseID string, ttlMs int, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.attempts++
	if b.refuse {
		return b.refuseErr
	}
	b.sentDelay++
	_ = caseID
	_ = ttlMs
	_ = body
	return nil
}

func (b *dualBroker) snapshot() (attempts, sentOutbox, sentDelay int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.attempts, b.sentOutbox, b.sentDelay
}

func (s *memStore) delayPublishedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, d := range s.delays {
		if d.Published {
			n++
		}
	}
	return n
}

func TestRunPublisher_RetriesThenStops(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newMemStore()
	refuse := errors.New("broker refused")
	broker := &dualBroker{refuse: true, refuseErr: refuse}
	if err := cases.Open(ctx, store, cases.OpenInput{
		ID:         "k-run",
		CustomerID: "c02",
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		Factors:    cases.ClockFactors{Frustration: 2},
		OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Open: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cases.RunPublisher(ctx, store, broker, 15*time.Millisecond)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		attempts, _, _ := broker.snapshot()
		if attempts > 0 && store.publishedCount() == 0 && store.delayPublishedCount() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	attempts, _, _ := broker.snapshot()
	if attempts == 0 || store.publishedCount() != 0 || store.delayPublishedCount() != 0 {
		t.Fatalf(
			"attempts=%d published=%d delays=%d, want refused attempt and nothing published",
			attempts,
			store.publishedCount(),
			store.delayPublishedCount(),
		)
	}

	broker.mu.Lock()
	broker.refuse = false
	broker.mu.Unlock()

	deadline = time.Now().Add(time.Second)
	for (store.publishedCount() != 1 || store.delayPublishedCount() != 1) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if store.publishedCount() != 1 {
		t.Fatalf("published outbox = %d, want 1 after accept", store.publishedCount())
	}
	if store.delayPublishedCount() != 1 {
		t.Fatalf("published delays = %d, want 1 after accept", store.delayPublishedCount())
	}
	_, sentOutbox, sentDelay := broker.snapshot()
	if sentOutbox != 1 || sentDelay != 1 {
		t.Fatalf("sent outbox=%d delay=%d, want 1 each", sentOutbox, sentDelay)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunPublisher error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunPublisher did not return after cancel")
	}
}

func TestApplyBreachDelivery_PlainAndJSON(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("plain case id", func(t *testing.T) {
		t.Parallel()
		store := newMemStore()
		if err := cases.Open(ctx, store, cases.OpenInput{
			ID:         "k-plain",
			CustomerID: "c02",
			AdvisorID:  identity.MustNewV7(),
			Segment:    book.SegmentAdvance,
			OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("Open: %v", err)
		}
		if err := cases.ApplyBreachDelivery(ctx, store, []byte("k-plain")); err != nil {
			t.Fatalf("ApplyBreachDelivery: %v", err)
		}
		row := store.mustCase(t, "k-plain")
		if !row.Escalated {
			t.Fatal("escalated = false, want true")
		}
		if err := cases.ApplyBreachDelivery(ctx, store, []byte("k-plain")); err != nil {
			t.Fatalf("second ApplyBreachDelivery: %v", err)
		}
		breachRows := 0
		for _, k := range store.routingKeys() {
			if k == event.NameCaseSLABreached {
				breachRows++
			}
		}
		if breachRows != 1 {
			t.Fatalf("breach outbox rows = %d, want 1", breachRows)
		}
	})

	t.Run("json payload case_id", func(t *testing.T) {
		t.Parallel()
		store := newMemStore()
		if err := cases.Open(ctx, store, cases.OpenInput{
			ID:         "k-json",
			CustomerID: "c02",
			AdvisorID:  identity.MustNewV7(),
			Segment:    book.SegmentAdvance,
			OccurredAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("Open: %v", err)
		}
		body := []byte(`{
			"event_id":"ignore-me",
			"occurred_at":"2026-09-27T12:00:00Z",
			"customer_id":"c02",
			"schema_version":1,
			"payload":{"case_id":"k-json","state":"Aberto","escalated":false,"sla_total_minutes":240}
		}`)
		if err := cases.ApplyBreachDelivery(ctx, store, body); err != nil {
			t.Fatalf("ApplyBreachDelivery: %v", err)
		}
		row := store.mustCase(t, "k-json")
		if !row.Escalated {
			t.Fatal("escalated = false, want true")
		}
		if err := cases.ApplyBreachDelivery(ctx, store, body); err != nil {
			t.Fatalf("second ApplyBreachDelivery: %v", err)
		}
		breachRows := 0
		for _, k := range store.routingKeys() {
			if k == event.NameCaseSLABreached {
				breachRows++
			}
		}
		if breachRows != 1 {
			t.Fatalf("breach outbox rows = %d, want 1", breachRows)
		}
	})
}
