package outbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/outbox"
)

type memStore struct {
	mu     sync.Mutex
	rows   map[string]memRow
	failTx bool
	order  []string
}

type memRow struct {
	row         outbox.Row
	publishedAt *time.Time
}

type memTx struct {
	store  *memStore
	staged map[string]outbox.Row
	order  []string
}

func newMemStore() *memStore {
	return &memStore{rows: make(map[string]memRow)}
}

func (s *memStore) WithTx(ctx context.Context, fn func(outbox.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx := &memTx{
		store:  s,
		staged: make(map[string]outbox.Row),
	}
	if err := fn(tx); err != nil {
		return err
	}
	if s.failTx {
		return errors.New("forced rollback")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range tx.order {
		row := tx.staged[id]
		if _, exists := s.rows[id]; exists {
			continue
		}
		s.rows[id] = memRow{row: row}
		s.order = append(s.order, id)
	}
	return nil
}

func (t *memTx) Insert(ctx context.Context, row outbox.Row) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := t.staged[row.EventID]; ok {
		return nil
	}
	t.staged[row.EventID] = row
	t.order = append(t.order, row.EventID)
	return nil
}

func (s *memStore) ListUnpublished(ctx context.Context) ([]outbox.Row, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]outbox.Row, 0)
	for _, id := range s.order {
		r := s.rows[id]
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
	r, ok := s.rows[eventID]
	if !ok {
		return fmt.Errorf("unknown event_id %s", eventID)
	}
	ts := at
	r.publishedAt = &ts
	s.rows[eventID] = r
	return nil
}

func (s *memStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}

func (s *memStore) publishedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.rows {
		if r.publishedAt != nil {
			n++
		}
	}
	return n
}

type fakeBroker struct {
	mu        sync.Mutex
	sent      []outbox.Row
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
	b.sent = append(b.sent, outbox.Row{
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

func TestFire_FirstAndSecond(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()

	if err := outbox.Fire(ctx, store); err != nil {
		t.Fatalf("first Fire: %v", err)
	}
	if store.count() != 6 {
		t.Fatalf("after first Fire count = %d, want 6", store.count())
	}

	ids := map[string]struct{}{}
	unpublished, err := store.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}
	var messages, accounts int
	for _, row := range unpublished {
		ids[row.EventID] = struct{}{}
		switch row.RoutingKey {
		case event.NameMessageReceived:
			messages++
		case event.NameAccountEventRecorded:
			accounts++
		default:
			t.Fatalf("unexpected routing key %q", row.RoutingKey)
		}
		var body map[string]any
		if err := json.Unmarshal(row.Payload, &body); err != nil {
			t.Fatalf("unmarshal %s: %v", row.EventID, err)
		}
		if _, ok := body["payload"]; !ok {
			t.Fatalf("%s body missing payload", row.EventID)
		}
	}
	for _, id := range []string{"md-n01", "md-n02", "md-n03", "md-n04", "md-n05", "md-n06"} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("missing event_id %s", id)
		}
	}
	if messages != 4 || accounts != 2 {
		t.Fatalf("messages=%d accounts=%d, want 4 and 2", messages, accounts)
	}

	if err := outbox.Fire(ctx, store); err != nil {
		t.Fatalf("second Fire: %v", err)
	}
	if store.count() != 6 {
		t.Fatalf("after second Fire count = %d, want 6", store.count())
	}
}

func TestPublish_BrokerAccepts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	broker := &fakeBroker{}

	if err := outbox.Fire(ctx, store); err != nil {
		t.Fatalf("Fire: %v", err)
	}
	if err := outbox.Publish(ctx, store, broker); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if broker.sentCount() != 6 {
		t.Fatalf("sent = %d, want 6", broker.sentCount())
	}
	broker.mu.Lock()
	sent := append([]outbox.Row(nil), broker.sent...)
	broker.mu.Unlock()
	for _, row := range sent {
		if row.RoutingKey != event.NameMessageReceived && row.RoutingKey != event.NameAccountEventRecorded {
			t.Fatalf("sent routing key %q", row.RoutingKey)
		}
		if row.EventID == "" || len(row.Payload) == 0 {
			t.Fatalf("sent row missing id or body: %+v", row)
		}
	}
	if store.publishedCount() != 6 {
		t.Fatalf("published = %d, want 6", store.publishedCount())
	}

	if err := outbox.Publish(ctx, store, broker); err != nil {
		t.Fatalf("second Publish: %v", err)
	}
	if broker.sentCount() != 6 {
		t.Fatalf("after second Publish sent = %d, want 6 (no resend)", broker.sentCount())
	}
}

func TestPublish_BrokerRefuses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	refuse := errors.New("broker refused")
	broker := &fakeBroker{refuseID: "md-n01", refuseErr: refuse}

	if err := outbox.Fire(ctx, store); err != nil {
		t.Fatalf("Fire: %v", err)
	}
	err := outbox.Publish(ctx, store, broker)
	if err == nil {
		t.Fatal("Publish error = nil, want wrapped publish error")
	}
	if !errors.Is(err, refuse) {
		t.Fatalf("Publish error = %v, want wrap of %v", err, refuse)
	}
	if store.publishedCount() != 0 {
		t.Fatalf("published = %d, want 0 after refuse", store.publishedCount())
	}
	unpublished, err := store.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}
	if len(unpublished) != 6 {
		t.Fatalf("unpublished = %d, want 6", len(unpublished))
	}
}

func TestRunPublisher_RetriesThenStops(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := newMemStore()
	refuse := errors.New("broker refused")
	broker := &fakeBroker{refuseID: "md-n01", refuseErr: refuse}
	if err := outbox.Fire(ctx, store); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- outbox.RunPublisher(ctx, store, broker, 15*time.Millisecond)
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
	for store.publishedCount() != 6 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if store.publishedCount() != 6 {
		t.Fatalf("published = %d, want 6 after broker recovers", store.publishedCount())
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

func TestFire_Rollback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	store.failTx = true

	err := outbox.Fire(ctx, store)
	if err == nil {
		t.Fatal("Fire error = nil, want wrapped transaction error")
	}
	if store.count() != 0 {
		t.Fatalf("after rollback count = %d, want 0", store.count())
	}
}
