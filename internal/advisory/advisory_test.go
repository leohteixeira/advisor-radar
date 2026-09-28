package advisory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/advisory"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

type bookRow struct {
	aum     float64
	segment string
}

type memStore struct {
	mu      sync.Mutex
	inbox   map[string]struct{}
	alerts  map[string]advisory.AlertRow
	outbox  map[string]memOutbox
	book    map[string]bookRow
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
	book   map[string]bookRow
	order  []string
}

func newMemStore() *memStore {
	return &memStore{
		inbox:  make(map[string]struct{}),
		alerts: make(map[string]advisory.AlertRow),
		outbox: make(map[string]memOutbox),
		book:   make(map[string]bookRow),
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
		book:   make(map[string]bookRow),
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
	for id, row := range tx.book {
		s.book[id] = row
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

func (t *memTx) UpdateBook(ctx context.Context, customerID string, aum float64, segment string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.book[customerID] = bookRow{aum: aum, segment: segment}
	return nil
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

func TestApply_CrossingDeposit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-c13-deposit",
		OccurredAt:    time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
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
	found := map[string]bool{}
	for _, kind := range kinds {
		found[kind] = true
	}
	if !found[advisory.KindAporte] || !found[advisory.KindSegmento] {
		t.Fatalf("kinds = %v, want aporte and segmento", kinds)
	}
	store.mu.Lock()
	var seg advisory.AlertRow
	var aporte advisory.AlertRow
	for _, row := range store.alerts {
		var payload advisory.AlertPayload
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		switch payload.Kind {
		case advisory.KindSegmento:
			seg = row
		case advisory.KindAporte:
			aporte = row
		}
	}
	store.mu.Unlock()
	var payload advisory.AlertPayload
	if err := json.Unmarshal(seg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal segment payload: %v", err)
	}
	if payload.From != "Essencial" || payload.To != "Advance" {
		t.Fatalf("segment from/to = %s/%s, want Essencial/Advance", payload.From, payload.To)
	}
	var aportePayload advisory.AlertPayload
	if err := json.Unmarshal(aporte.Payload, &aportePayload); err != nil {
		t.Fatalf("unmarshal aporte payload: %v", err)
	}
	if aportePayload.Amount != 60000 || aportePayload.Before != 8000 || aportePayload.After != 68000 {
		t.Fatalf(
			"aporte amounts = %v/%v/%v, want whole dollars 60000/8000/68000",
			aportePayload.Amount,
			aportePayload.Before,
			aportePayload.After,
		)
	}
}

func TestApply_SchemaVersionCentsDeposit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-cents-aporte",
		OccurredAt:    time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		CustomerID:    "c-fernanda",
		SchemaVersion: event.SchemaVersionCents,
		Payload: sim.AccountPayload{
			Kind:   "aporte",
			Amount: 1_000_000,
			Before: 820_000,
			After:  1_820_000,
			Origin: "pix",
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	kinds := store.alertKinds()
	if len(kinds) != 2 {
		t.Fatalf("alerts = %d, want 2: %v", len(kinds), kinds)
	}
	found := map[string]bool{}
	for _, kind := range kinds {
		found[kind] = true
	}
	if !found[advisory.KindAporte] || !found[advisory.KindSegmento] {
		t.Fatalf("kinds = %v, want aporte and segmento", kinds)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	for _, row := range store.alerts {
		var payload advisory.AlertPayload
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		switch payload.Kind {
		case advisory.KindAporte:
			if payload.Amount != 10_000 || payload.Before != 8_200 || payload.After != 18_200 {
				t.Fatalf(
					"aporte amounts = %v/%v/%v, want dollars 10000/8200/18200",
					payload.Amount,
					payload.Before,
					payload.After,
				)
			}
		case advisory.KindSegmento:
			if payload.From != "Essencial" || payload.To != "Advance" {
				t.Fatalf("segment from/to = %s/%s, want Essencial/Advance", payload.From, payload.To)
			}
			if payload.Before != 8_200 || payload.After != 18_200 {
				t.Fatalf(
					"segment before/after = %v/%v, want dollars 8200/18200",
					payload.Before,
					payload.After,
				)
			}
		}
	}
	row := store.book["c-fernanda"]
	if row.aum != 18_200 || row.segment != "Advance" {
		t.Fatalf("book = %+v, want aum 18200 segment Advance", row)
	}
}

func TestApply_SchemaVersionCentsMarianaWithdrawal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-cents-saque-mariana",
		OccurredAt:    time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		CustomerID:    "c-mariana",
		SchemaVersion: event.SchemaVersionCents,
		Payload: sim.AccountPayload{
			Kind:        "saque",
			Amount:      6_000_000,
			Before:      24_830_000,
			After:       18_830_000,
			Destination: "conta-eua",
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	foundSaque := false
	for _, kind := range store.alertKinds() {
		if kind == advisory.KindSaque {
			foundSaque = true
		}
	}
	if !foundSaque {
		t.Fatalf("kinds = %v, want saque", store.alertKinds())
	}
	row := store.book["c-mariana"]
	if row.aum != 188_300 || row.segment != "Advance" {
		t.Fatalf("book = %+v, want aum 188300 segment Advance", row)
	}
}

func TestApply_SchemaVersionCentsReplayKeepsBook(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-cents-replay",
		OccurredAt:    time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		CustomerID:    "c-fernanda",
		SchemaVersion: event.SchemaVersionCents,
		Payload: sim.AccountPayload{
			Kind:   "aporte",
			Amount: 1_000_000,
			Before: 820_000,
			After:  1_820_000,
			Origin: "pix",
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if store.alertCount() != 2 {
		t.Fatalf("alerts = %d, want 2", store.alertCount())
	}
	row := store.book["c-fernanda"]
	if row.aum != 18_200 || row.segment != "Advance" {
		t.Fatalf("book = %+v", row)
	}
}

func TestApply_SchemaVersionOneDoesNotWriteBook(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-dollars-book",
		OccurredAt:    time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		CustomerID:    "c13",
		SchemaVersion: event.SchemaVersionMVP,
		Payload: sim.AccountPayload{
			Kind:   "deposit",
			Amount: 60000,
			Before: 8000,
			After:  68000,
		},
	}
	if err := advisory.Apply(context.Background(), store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(store.book) != 0 {
		t.Fatalf("book = %+v, want empty", store.book)
	}
}

func TestApply_SchemaVersionCentsFractional(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-cents-fractional",
		OccurredAt:    time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		CustomerID:    "c-fernanda",
		SchemaVersion: event.SchemaVersionCents,
		Payload: sim.AccountPayload{
			Kind:   "deposit",
			Amount: 10000.5,
			Before: 820_000,
			After:  830_000,
			Origin: "pix",
		},
	}
	err := advisory.Apply(ctx, store, env)
	if err == nil {
		t.Fatal("Apply error = nil, want fractional cents error")
	}
	if !strings.Contains(err.Error(), "cents") {
		t.Fatalf("Apply error = %q, want substring %q", err.Error(), "cents")
	}
	if store.alertCount() != 0 {
		t.Fatalf("alerts = %d, want 0", store.alertCount())
	}
	if store.inboxCount() != 0 {
		t.Fatalf("inbox = %d, want 0", store.inboxCount())
	}
	if store.outboxCount() != 0 {
		t.Fatalf("outbox = %d, want 0", store.outboxCount())
	}
}

func TestApply_SchemaVersionCentsSaque(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-cents-saque",
		OccurredAt:    time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		CustomerID:    "c-mariana",
		SchemaVersion: event.SchemaVersionCents,
		Payload: sim.AccountPayload{
			Kind:        "saque",
			Amount:      5_500_000,
			Before:      19_600_000,
			After:       14_100_000,
			Destination: "pix",
		},
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if store.alertCount() != 1 {
		t.Fatalf("alerts = %d, want 1", store.alertCount())
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	for _, row := range store.alerts {
		var payload advisory.AlertPayload
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload.Kind != advisory.KindSaque {
			t.Fatalf("kind = %q, want %q", payload.Kind, advisory.KindSaque)
		}
		if payload.Amount != 55_000 || payload.Before != 196_000 || payload.After != 141_000 {
			t.Fatalf(
				"saque amounts = %v/%v/%v, want dollars 55000/196000/141000",
				payload.Amount,
				payload.Before,
				payload.After,
			)
		}
	}
}

func TestApply_ExactWithdrawal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-exact-withdrawal",
		OccurredAt:    time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
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
		OccurredAt:    time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
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
		OccurredAt:    time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
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
		OccurredAt:    time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
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

	if err := advisory.ApplySilence(ctx, store, "c14", 90, 175000, 175000, "seed-s15", "al-s15", time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ApplySilence 90: %v", err)
	}
	if store.alertCount() != 0 {
		t.Fatalf("after 90 days alerts = %d, want 0", store.alertCount())
	}

	if err := advisory.ApplySilence(ctx, store, "c14", 94, 175000, 175000, "seed-s15", "al-s15", time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)); err != nil {
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
		OccurredAt:    time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
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

func TestApply_Rollback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemStore()
	store.failIns = "alert"
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "ev-rollback",
		OccurredAt:    time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
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
