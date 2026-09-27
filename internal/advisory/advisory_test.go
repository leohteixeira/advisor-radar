package advisory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/advisory"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

type memStore struct {
	mu      sync.Mutex
	inbox   map[string]struct{}
	alerts  map[string]advisory.AlertRow
	outbox  map[string]memOutbox
	order   []string
	failIns string
}

type memOutbox struct {
	row         advisory.OutboxRow
	publishedAt *time.Time
}

type memTx struct {
	store  *memStore
	inbox  map[string]struct{}
	alerts map[string]advisory.AlertRow
	outbox map[string]advisory.OutboxRow
	order  []string
}

func newMemStore() *memStore {
	return &memStore{
		inbox:  make(map[string]struct{}),
		alerts: make(map[string]advisory.AlertRow),
		outbox: make(map[string]memOutbox),
	}
}

func (s *memStore) WithTx(ctx context.Context, fn func(advisory.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx := &memTx{
		store:  s,
		inbox:  make(map[string]struct{}),
		alerts: make(map[string]advisory.AlertRow),
		outbox: make(map[string]advisory.OutboxRow),
	}
	if err := fn(tx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range tx.inbox {
		s.inbox[id] = struct{}{}
	}
	for id, row := range tx.alerts {
		s.alerts[id] = row
	}
	for _, id := range tx.order {
		row := tx.outbox[id]
		if _, exists := s.outbox[id]; exists {
			continue
		}
		s.outbox[id] = memOutbox{row: row}
		s.order = append(s.order, id)
	}
	return nil
}

func (t *memTx) ClaimInbox(ctx context.Context, eventID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, ok := t.store.inbox[eventID]; ok {
		return false, nil
	}
	if _, ok := t.inbox[eventID]; ok {
		return false, nil
	}
	t.inbox[eventID] = struct{}{}
	return true, nil
}

func (t *memTx) InsertAlert(ctx context.Context, row advisory.AlertRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.store.failIns == "alert" {
		return errForcedAlertInsert
	}
	if _, ok := t.alerts[row.ID]; ok {
		return nil
	}
	t.alerts[row.ID] = row
	return nil
}

func (t *memTx) InsertOutbox(ctx context.Context, row advisory.OutboxRow) error {
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

func (s *memStore) ListUnpublished(ctx context.Context) ([]advisory.OutboxRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]advisory.OutboxRow, 0)
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

func (s *memStore) alertCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.alerts)
}

func (s *memStore) outboxCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.outbox)
}

func (s *memStore) inboxCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.inbox)
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

func (s *memStore) alertKinds() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.alerts))
	for id, row := range s.alerts {
		out[id] = row.Kind
	}
	return out
}

var errForcedAlertInsert = errors.New("forced alert insert failure")

type fakeBroker struct {
	mu        sync.Mutex
	sent      []advisory.OutboxRow
	refuseID  string
	refuseErr error
	attempts  int
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
	b.attempts++
	if b.refuseID != "" && eventID == b.refuseID {
		return b.refuseErr
	}
	b.sent = append(b.sent, advisory.OutboxRow{
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

func TestRaiseSeed_Twice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()

	if err := advisory.RaiseSeed(ctx, store); err != nil {
		t.Fatalf("first RaiseSeed: %v", err)
	}
	want := map[string]string{
		"al-s11": advisory.KindSaque,
		"al-s12": advisory.KindQueda,
		"al-s13": advisory.KindAporte,
		"al-s14": advisory.KindSegmento,
		"al-s15": advisory.KindContato,
		"al-s16": advisory.KindSaque,
		"al-s17": advisory.KindQueda,
	}
	if store.alertCount() != 7 {
		t.Fatalf("after first RaiseSeed alerts = %d, want 7", store.alertCount())
	}
	got := store.alertKinds()
	for id, kind := range want {
		if got[id] != kind {
			t.Fatalf("alert %s kind = %q, want %q", id, got[id], kind)
		}
	}
	if store.outboxCount() != 7 {
		t.Fatalf("outbox = %d, want 7", store.outboxCount())
	}

	if err := advisory.RaiseSeed(ctx, store); err != nil {
		t.Fatalf("second RaiseSeed: %v", err)
	}
	if store.alertCount() != 7 {
		t.Fatalf("after second RaiseSeed alerts = %d, want 7", store.alertCount())
	}
}

func TestApply_CrossingDeposit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-c13-deposit",
		OccurredAt:    advisory.SeedOccurredAt,
		CustomerID:    "c13",
		SchemaVersion: event.SchemaVersionMVP,
		Payload: sim.AccountPayload{
			Kind:   "deposit",
			Amount: 60000,
			Before: 8000,
			After:  68000,
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	kinds := store.alertKinds()
	if len(kinds) != 2 {
		t.Fatalf("alerts = %d, want 2: %v", len(kinds), kinds)
	}
	wantIDs := map[string]string{
		"al-ev-c13-deposit-deposit": advisory.KindAporte,
		"al-ev-c13-deposit-segment": advisory.KindSegmento,
	}
	for id, kind := range wantIDs {
		if kinds[id] != kind {
			t.Fatalf("alert %s kind = %q, want %q", id, kinds[id], kind)
		}
	}
	store.mu.Lock()
	seg := store.alerts["al-ev-c13-deposit-segment"]
	store.mu.Unlock()
	var payload advisory.AlertPayload
	if err := json.Unmarshal(seg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal segment payload: %v", err)
	}
	if payload.From != "Essencial" || payload.To != "Advance" {
		t.Fatalf("segment from/to = %s/%s, want Essencial/Advance", payload.From, payload.To)
	}
}

func TestApply_ExactWithdrawal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-exact-withdrawal",
		OccurredAt:    advisory.SeedOccurredAt,
		CustomerID:    "c01",
		SchemaVersion: event.SchemaVersionMVP,
		Payload: sim.AccountPayload{
			Kind:   "withdrawal",
			Amount: 20000,
			Before: 100000,
			After:  80000,
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if store.alertCount() != 0 {
		t.Fatalf("alerts = %d, want 0", store.alertCount())
	}
}

func TestApply_ExactDrop(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-exact-drop",
		OccurredAt:    advisory.SeedOccurredAt,
		CustomerID:    "c01",
		SchemaVersion: event.SchemaVersionMVP,
		Payload: sim.AccountPayload{
			Kind:   "asset_drop",
			Amount: -15000,
			Before: 100000,
			After:  85000,
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if store.alertCount() != 0 {
		t.Fatalf("alerts = %d, want 0", store.alertCount())
	}
}

func TestApply_EqualDeposit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-equal-deposit",
		OccurredAt:    advisory.SeedOccurredAt,
		CustomerID:    "c03",
		SchemaVersion: event.SchemaVersionMVP,
		Payload: sim.AccountPayload{
			Kind:   "deposit",
			Amount: 8000,
			Before: 8000,
			After:  10000,
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if store.alertCount() != 0 {
		t.Fatalf("alerts = %d, want 0", store.alertCount())
	}
}

func TestApply_SameBand(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-same-band",
		OccurredAt:    advisory.SeedOccurredAt,
		CustomerID:    "c02",
		SchemaVersion: event.SchemaVersionMVP,
		Payload: sim.AccountPayload{
			Kind:   "deposit",
			Amount: 10000,
			Before: 50000,
			After:  60000,
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	for id, kind := range store.alertKinds() {
		if kind == advisory.KindSegmento {
			t.Fatalf("unexpected segment alert %s", id)
		}
	}
}

func TestApplySilence_Boundary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()

	if err := advisory.ApplySilence(ctx, store, "c14", 90, 175000, 175000, "seed-s15", "al-s15", advisory.SeedOccurredAt); err != nil {
		t.Fatalf("ApplySilence 90: %v", err)
	}
	if store.alertCount() != 0 {
		t.Fatalf("after 90 days alerts = %d, want 0", store.alertCount())
	}

	if err := advisory.ApplySilence(ctx, store, "c14", 94, 175000, 175000, "seed-s15", "al-s15", advisory.SeedOccurredAt); err != nil {
		t.Fatalf("ApplySilence 94: %v", err)
	}
	if store.alertCount() != 1 {
		t.Fatalf("after 94 days alerts = %d, want 1", store.alertCount())
	}
	kinds := store.alertKinds()
	if kinds["al-s15"] != advisory.KindContato {
		t.Fatalf("al-s15 kind = %q, want %q", kinds["al-s15"], advisory.KindContato)
	}
}

func TestApply_Redelivery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-redeliver",
		OccurredAt:    advisory.SeedOccurredAt,
		CustomerID:    "c19",
		SchemaVersion: event.SchemaVersionMVP,
		Payload: sim.AccountPayload{
			Kind:   "withdrawal",
			Amount: 55000,
			Before: 196000,
			After:  141000,
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if store.alertCount() != 1 {
		t.Fatalf("alerts = %d, want 1 after redelivery", store.alertCount())
	}
}

func TestPublish_BrokerRefusesThenAccepts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	if err := advisory.RaiseSeed(ctx, store); err != nil {
		t.Fatalf("RaiseSeed: %v", err)
	}

	wantRules := map[string]string{
		"al-s11": advisory.RuleWithdrawal,
		"al-s12": advisory.RuleDrop,
		"al-s13": advisory.RuleDeposit,
		"al-s14": advisory.RuleSegment,
		"al-s15": advisory.RuleSilence,
		"al-s16": advisory.RuleWithdrawal,
		"al-s17": advisory.RuleDrop,
	}
	store.mu.Lock()
	for id, wantRule := range wantRules {
		row, ok := store.alerts[id]
		if !ok {
			store.mu.Unlock()
			t.Fatalf("missing seed alert %s", id)
		}
		if row.Rule != wantRule {
			store.mu.Unlock()
			t.Fatalf("alert %s rule = %q, want %q", id, row.Rule, wantRule)
		}
	}
	store.mu.Unlock()

	unpublished, err := store.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}
	if len(unpublished) != 7 {
		t.Fatalf("unpublished = %d, want 7", len(unpublished))
	}
	for _, row := range unpublished {
		if row.RoutingKey != event.NameAlertRaised {
			t.Fatalf("outbox %s routing key = %q, want %q", row.EventID, row.RoutingKey, event.NameAlertRaised)
		}
		if len(row.Payload) == 0 {
			t.Fatalf("outbox %s body is empty", row.EventID)
		}
	}

	refuse := errors.New("broker refused")
	broker := &fakeBroker{refuseID: "al-s11", refuseErr: refuse}
	err = advisory.Publish(ctx, store, broker)
	if err == nil {
		t.Fatal("Publish error = nil, want wrapped broker error")
	}
	if !errors.Is(err, refuse) {
		t.Fatalf("Publish error = %v, want wrap of %v", err, refuse)
	}
	if store.publishedCount() != 0 {
		t.Fatalf("published = %d, want 0 after refuse", store.publishedCount())
	}

	broker.refuseID = ""
	if err := advisory.Publish(ctx, store, broker); err != nil {
		t.Fatalf("Publish after accept: %v", err)
	}
	if store.publishedCount() != 7 {
		t.Fatalf("published = %d, want 7", store.publishedCount())
	}
	if broker.sentCount() != 7 {
		t.Fatalf("sent = %d, want 7", broker.sentCount())
	}
	broker.mu.Lock()
	sent := append([]advisory.OutboxRow(nil), broker.sent...)
	broker.mu.Unlock()
	for _, row := range sent {
		if row.RoutingKey != event.NameAlertRaised {
			t.Fatalf("sent %s routing key = %q, want %q", row.EventID, row.RoutingKey, event.NameAlertRaised)
		}
		if len(row.Payload) == 0 {
			t.Fatalf("sent %s body is empty", row.EventID)
		}
	}

	if err := advisory.Publish(ctx, store, broker); err != nil {
		t.Fatalf("second Publish: %v", err)
	}
	if broker.sentCount() != 7 {
		t.Fatalf("after second Publish sent = %d, want 7", broker.sentCount())
	}
}

func TestRunPublisher_RetriesThenStops(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newMemStore()
	refuse := errors.New("broker refused")
	broker := &fakeBroker{refuseID: "al-s11", refuseErr: refuse}
	if err := advisory.RaiseSeed(ctx, store); err != nil {
		t.Fatalf("RaiseSeed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- advisory.RunPublisher(ctx, store, broker, 15*time.Millisecond)
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		broker.mu.Lock()
		attempts := broker.attempts
		broker.mu.Unlock()
		if attempts > 0 && store.publishedCount() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	broker.mu.Lock()
	attempts := broker.attempts
	broker.mu.Unlock()
	if attempts == 0 || store.publishedCount() != 0 {
		t.Fatalf("attempts=%d published=%d, want a refused attempt and 0 published", attempts, store.publishedCount())
	}

	broker.mu.Lock()
	broker.refuseID = ""
	broker.mu.Unlock()

	deadline = time.Now().Add(time.Second)
	for store.publishedCount() != 7 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if store.publishedCount() != 7 {
		t.Fatalf("published = %d, want 7 after broker recovers", store.publishedCount())
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

func TestApply_Rollback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	store.failIns = "alert"
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-rollback",
		OccurredAt:    advisory.SeedOccurredAt,
		CustomerID:    "c19",
		SchemaVersion: event.SchemaVersionMVP,
		Payload: sim.AccountPayload{
			Kind:   "withdrawal",
			Amount: 55000,
			Before: 196000,
			After:  141000,
		},
	}
	err := advisory.Apply(ctx, store, env)
	if err == nil {
		t.Fatal("Apply error = nil, want wrapped insert error")
	}
	if !errors.Is(err, errForcedAlertInsert) {
		t.Fatalf("Apply error = %v, want wrap of %v", err, errForcedAlertInsert)
	}
	if store.inboxCount() != 0 {
		t.Fatalf("inbox = %d, want 0 after rollback", store.inboxCount())
	}
	if store.alertCount() != 0 {
		t.Fatalf("alerts = %d, want 0 after rollback", store.alertCount())
	}
	if store.outboxCount() != 0 {
		t.Fatalf("outbox = %d, want 0 after rollback", store.outboxCount())
	}
}

func TestApply_MarketDay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()

	for _, env := range sim.MarketDay() {
		if err := advisory.Apply(ctx, store, env); err != nil {
			t.Fatalf("Apply %s: %v", env.EventID, err)
		}
	}

	want := map[string]string{
		"al-md-n02-withdrawal": advisory.KindSaque,
		"al-md-n05-drop":       advisory.KindQueda,
	}
	if store.alertCount() != 2 {
		t.Fatalf("alerts = %d, want 2; got %v", store.alertCount(), store.alertKinds())
	}
	got := store.alertKinds()
	for id, kind := range want {
		if got[id] != kind {
			t.Fatalf("alert %s kind = %q, want %q", id, got[id], kind)
		}
	}
}

func TestApply_MarketDayJSONMap(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()

	for _, seed := range sim.MarketDay() {
		if seed.Name != event.NameAccountEventRecorded {
			continue
		}
		body, err := seed.MarshalBody()
		if err != nil {
			t.Fatalf("MarshalBody %s: %v", seed.EventID, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("unmarshal %s: %v", seed.EventID, err)
		}
		occurredAt, err := time.Parse(time.RFC3339Nano, raw["occurred_at"].(string))
		if err != nil {
			t.Fatalf("parse occurred_at %s: %v", seed.EventID, err)
		}
		env := event.Envelope{
			Name:          event.NameAccountEventRecorded,
			EventID:       raw["event_id"].(string),
			OccurredAt:    occurredAt,
			CustomerID:    raw["customer_id"].(string),
			SchemaVersion: int(raw["schema_version"].(float64)),
			Payload:       raw["payload"],
		}
		if err := advisory.Apply(ctx, store, env); err != nil {
			t.Fatalf("Apply %s: %v", env.EventID, err)
		}
	}

	want := map[string]string{
		"al-md-n02-withdrawal": advisory.KindSaque,
		"al-md-n05-drop":       advisory.KindQueda,
	}
	if store.alertCount() != 2 {
		t.Fatalf("alerts = %d, want 2; got %v", store.alertCount(), store.alertKinds())
	}
	got := store.alertKinds()
	for id, kind := range want {
		if got[id] != kind {
			t.Fatalf("alert %s kind = %q, want %q", id, got[id], kind)
		}
	}
}

func TestApply_Message(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameMessageReceived,
		EventID:       "md-n01",
		OccurredAt:    sim.MarketDayOccurredAt,
		CustomerID:    "c18",
		SchemaVersion: event.SchemaVersionMVP,
		Payload: sim.MessagePayload{
			Channel: "email",
			Text:    "ignored",
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if store.alertCount() != 0 {
		t.Fatalf("alerts = %d, want 0", store.alertCount())
	}
	if store.inboxCount() != 0 {
		t.Fatalf("inbox = %d, want 0", store.inboxCount())
	}
}
