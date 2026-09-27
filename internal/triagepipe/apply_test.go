package triagepipe_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/sim"
	"github.com/leohteixeira/advisor-radar/internal/triage"
	"github.com/leohteixeira/advisor-radar/internal/triagepipe"
)

type memStore struct {
	mu      sync.Mutex
	inbox   map[string]struct{}
	results map[string]triagepipe.ResultRow
	outbox  map[string]memOutbox
	order   []string
	failIns string
}

type memOutbox struct {
	row         triagepipe.OutboxRow
	publishedAt *time.Time
}

type memTx struct {
	store   *memStore
	inbox   map[string]struct{}
	results map[string]triagepipe.ResultRow
	outbox  map[string]triagepipe.OutboxRow
	order   []string
}

func newMemStore() *memStore {
	return &memStore{
		inbox:   make(map[string]struct{}),
		results: make(map[string]triagepipe.ResultRow),
		outbox:  make(map[string]memOutbox),
	}
}

func (s *memStore) HasInbox(ctx context.Context, eventID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.inbox[eventID]
	return ok, nil
}

func (s *memStore) WithTx(ctx context.Context, fn func(triagepipe.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx := &memTx{
		store:   s,
		inbox:   make(map[string]struct{}),
		results: make(map[string]triagepipe.ResultRow),
		outbox:  make(map[string]triagepipe.OutboxRow),
	}
	if err := fn(tx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range tx.inbox {
		s.inbox[id] = struct{}{}
	}
	for id, row := range tx.results {
		s.results[id] = row
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

func (t *memTx) InsertResult(ctx context.Context, row triagepipe.ResultRow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.store.failIns == "result" {
		return errForcedResultInsert
	}
	if _, ok := t.results[row.ID]; ok {
		return nil
	}
	t.results[row.ID] = row
	return nil
}

func (t *memTx) InsertOutbox(ctx context.Context, row triagepipe.OutboxRow) error {
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

func (s *memStore) ListUnpublished(ctx context.Context) ([]triagepipe.OutboxRow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]triagepipe.OutboxRow, 0)
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

func (s *memStore) resultCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.results)
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

var errForcedResultInsert = errors.New("forced result insert failure")

type fakeBroker struct {
	mu        sync.Mutex
	sent      []triagepipe.OutboxRow
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
	b.sent = append(b.sent, triagepipe.OutboxRow{
		EventID:    eventID,
		RoutingKey: routingKey,
		Payload:    body,
	})
	return nil
}

type fixedClassifier struct {
	result triage.Result
	err    error
	calls  atomic.Int32
}

func (f *fixedClassifier) Classify(context.Context, triage.Message) (triage.Result, error) {
	f.calls.Add(1)
	return f.result, f.err
}

func messageEnv(id, customer string) event.Envelope {
	return event.Envelope{
		Name:          event.NameMessageReceived,
		EventID:       id,
		OccurredAt:    time.Date(2026, 9, 27, 15, 0, 0, 0, time.UTC),
		CustomerID:    customer,
		SchemaVersion: event.SchemaVersionMVP,
		Payload: sim.MessagePayload{
			Channel: "chat",
			Text:    "Preciso de uma remessa de cambio",
		},
	}
}

func TestApply_HappyModel(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	clf := &fixedClassifier{result: triage.Result{
		Intent: triage.IntentCambio, IntentProb: 0.9, Classifier: "jev", ModelVersion: "typesafe-ai/jev",
	}}

	env := messageEnv("md-n01", "c18")
	if err := triagepipe.Apply(context.Background(), store, clf, env); err != nil {
		t.Fatal(err)
	}
	if store.resultCount() != 1 || store.outboxCount() != 1 || store.inboxCount() != 1 {
		t.Fatalf("counts result=%d outbox=%d inbox=%d", store.resultCount(), store.outboxCount(), store.inboxCount())
	}
	store.mu.Lock()
	row := store.results["tr-md-n01"]
	ob := store.outbox["tr-md-n01"]
	store.mu.Unlock()
	if row.Classifier != "jev" || row.NeedsReview {
		t.Fatalf("row = %+v", row)
	}
	if ob.row.RoutingKey != event.NameMessageTriaged {
		t.Fatalf("routing key = %q, want %q", ob.row.RoutingKey, event.NameMessageTriaged)
	}
	var payload triagepipe.TriagedPayload
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.NeedsReview || payload.Classifier != "jev" || payload.Intent != "cambio" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestApply_DegradedHighProbNoReview(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	clf := &fixedClassifier{result: triage.Result{
		Intent: triage.IntentCambio, IntentProb: 0.9, Classifier: "heuristic", Degraded: true,
	}}
	if err := triagepipe.Apply(context.Background(), store, clf, messageEnv("md-n04", "c20")); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	row := store.results["tr-md-n04"]
	store.mu.Unlock()
	if !row.Degraded || row.NeedsReview {
		t.Fatalf("row = %+v", row)
	}
	var payload triagepipe.TriagedPayload
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Degraded || payload.NeedsReview {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestApply_LowProbabilityNeedsReview(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	clf := &fixedClassifier{result: triage.Result{
		Intent: triage.IntentCambio, IntentProb: 0.8, Classifier: "jev", ModelVersion: "typesafe-ai/jev",
	}}
	if err := triagepipe.Apply(context.Background(), store, clf, messageEnv("md-n03", "c17")); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	row := store.results["tr-md-n03"]
	store.mu.Unlock()
	if !row.NeedsReview || row.Degraded {
		t.Fatalf("row = %+v", row)
	}
}

func TestApply_Redelivery(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	clf := &fixedClassifier{result: triage.Result{
		Intent: triage.IntentCambio, IntentProb: 0.9, Classifier: "jev",
	}}
	env := messageEnv("md-n01", "c18")
	if err := triagepipe.Apply(context.Background(), store, clf, env); err != nil {
		t.Fatal(err)
	}
	if err := triagepipe.Apply(context.Background(), store, clf, env); err != nil {
		t.Fatal(err)
	}
	if store.resultCount() != 1 {
		t.Fatalf("results = %d, want 1", store.resultCount())
	}
	if clf.calls.Load() != 1 {
		t.Fatalf("classify calls = %d, want 1", clf.calls.Load())
	}
}

func TestApply_OtherEvent(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	clf := &fixedClassifier{result: triage.Result{Intent: triage.IntentOperacional, IntentProb: 0.9}}
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "md-n02",
		OccurredAt:    time.Now().UTC(),
		CustomerID:    "c19",
		SchemaVersion: event.SchemaVersionMVP,
		Payload:       sim.AccountPayload{Kind: "withdrawal", Amount: 1, Before: 2, After: 1},
	}
	if err := triagepipe.Apply(context.Background(), store, clf, env); err != nil {
		t.Fatal(err)
	}
	if store.resultCount() != 0 || store.inboxCount() != 0 || clf.calls.Load() != 0 {
		t.Fatalf("result=%d inbox=%d calls=%d", store.resultCount(), store.inboxCount(), clf.calls.Load())
	}
}

func TestApply_Rollback(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	store.failIns = "result"
	clf := &fixedClassifier{result: triage.Result{
		Intent: triage.IntentCambio, IntentProb: 0.9, Classifier: "jev",
	}}
	err := triagepipe.Apply(context.Background(), store, clf, messageEnv("md-n05", "c20"))
	if err == nil {
		t.Fatal("expected insert error")
	}
	if !errors.Is(err, errForcedResultInsert) {
		t.Fatalf("err = %v", err)
	}
	if store.inboxCount() != 0 || store.resultCount() != 0 || store.outboxCount() != 0 {
		t.Fatalf("inbox=%d result=%d outbox=%d", store.inboxCount(), store.resultCount(), store.outboxCount())
	}
}

func TestPublish_BrokerRefuseThenAccept(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	clf := &fixedClassifier{result: triage.Result{
		Intent: triage.IntentCambio, IntentProb: 0.9, Classifier: "jev",
	}}
	if err := triagepipe.Apply(context.Background(), store, clf, messageEnv("md-n06", "c21")); err != nil {
		t.Fatal(err)
	}

	refuse := errors.New("broker down")
	broker := &fakeBroker{refuseID: "tr-md-n06", refuseErr: refuse}
	err := triagepipe.Publish(context.Background(), store, broker)
	if err == nil || !errors.Is(err, refuse) {
		t.Fatalf("err = %v", err)
	}
	if store.publishedCount() != 0 {
		t.Fatalf("published = %d", store.publishedCount())
	}

	broker.refuseID = ""
	if err := triagepipe.Publish(context.Background(), store, broker); err != nil {
		t.Fatal(err)
	}
	if store.publishedCount() != 1 {
		t.Fatalf("published = %d, want 1", store.publishedCount())
	}
	broker.mu.Lock()
	n := len(broker.sent)
	key := ""
	if n > 0 {
		key = broker.sent[0].RoutingKey
	}
	broker.mu.Unlock()
	if n != 1 {
		t.Fatalf("sent = %d, want 1", n)
	}
	if key != event.NameMessageTriaged {
		t.Fatalf("routing key = %q, want %q", key, event.NameMessageTriaged)
	}
}

func TestRunPublisher_RetriesThenStops(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newMemStore()
	clf := &fixedClassifier{result: triage.Result{
		Intent: triage.IntentCambio, IntentProb: 0.9, Classifier: "jev",
	}}
	if err := triagepipe.Apply(ctx, store, clf, messageEnv("md-n01", "c18")); err != nil {
		t.Fatal(err)
	}

	refuse := errors.New("broker refused")
	broker := &fakeBroker{refuseID: "tr-md-n01", refuseErr: refuse}

	done := make(chan error, 1)
	go func() {
		done <- triagepipe.RunPublisher(ctx, store, broker, 15*time.Millisecond)
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
	for store.publishedCount() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if store.publishedCount() != 1 {
		t.Fatalf("published = %d, want 1 after broker recovers", store.publishedCount())
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
