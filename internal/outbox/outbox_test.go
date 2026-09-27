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
	tx := &memTx{store: s, staged: make(map[string]outbox.Row)}
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
	if !ok || r.publishedAt != nil {
		return fmt.Errorf("not found or already published")
	}
	r.publishedAt = &at
	s.rows[eventID] = r
	return nil
}

type memBroker struct {
	mu   sync.Mutex
	sent []struct {
		key  string
		body []byte
	}
	fail bool
}

func (b *memBroker) Publish(ctx context.Context, routingKey string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.fail {
		return errors.New("broker down")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, struct {
		key  string
		body []byte
	}{key: routingKey, body: body})
	return nil
}

func insertOne(t *testing.T, store *memStore, id string) {
	t.Helper()
	ctx := context.Background()
	body, _ := json.Marshal(map[string]any{
		"event_id": id, "occurred_at": time.Now().UTC(),
		"customer_id":    "00000000-0000-7000-8000-000000000099",
		"schema_version": 1, "payload": map[string]any{"channel": "chat", "text": "x"},
	})
	if err := store.WithTx(ctx, func(tx outbox.Tx) error {
		return tx.Insert(ctx, outbox.Row{EventID: id, RoutingKey: event.NameMessageReceived, Payload: body})
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPublish_MarksPublished(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	insertOne(t, store, "00000000-0000-7000-8000-000000000001")
	broker := &memBroker{}
	if err := outbox.Publish(context.Background(), store, broker); err != nil {
		t.Fatal(err)
	}
	if len(broker.sent) != 1 {
		t.Fatalf("sent = %d", len(broker.sent))
	}
	left, err := store.ListUnpublished(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("unpublished = %d", len(left))
	}
}

func TestPublish_BrokerErrorLeavesUnpublished(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	insertOne(t, store, "00000000-0000-7000-8000-000000000002")
	broker := &memBroker{fail: true}
	if err := outbox.Publish(context.Background(), store, broker); err == nil {
		t.Fatal("expected error")
	}
	left, _ := store.ListUnpublished(context.Background())
	if len(left) != 1 {
		t.Fatalf("unpublished = %d", len(left))
	}
}

func TestInsert_Idempotent(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	insertOne(t, store, "00000000-0000-7000-8000-000000000003")
	insertOne(t, store, "00000000-0000-7000-8000-000000000003")
	left, _ := store.ListUnpublished(context.Background())
	if len(left) != 1 {
		t.Fatalf("rows = %d", len(left))
	}
}
