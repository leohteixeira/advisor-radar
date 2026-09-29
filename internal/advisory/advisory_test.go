package advisory_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

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
	revals  map[string]advisory.Revaluation
	order   []string
	failIns string
	// profiles is the book investor profile per customer; a customer
	// without one is outside the book.
	profiles map[string]string
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
	revals map[string]advisory.Revaluation
	order  []string
}

func newMemStore() *memStore {
	return &memStore{
		inbox:  make(map[string]struct{}),
		alerts: make(map[string]advisory.AlertRow),
		outbox: make(map[string]memOutbox),
		book:   make(map[string]bookRow),
		revals: make(map[string]advisory.Revaluation),
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
		revals: make(map[string]advisory.Revaluation),
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
	for id, rev := range tx.revals {
		// As the pgx upsert: only another epoch or a later day replaces.
		if old, ok := s.revals[id]; ok && old.Epoch == rev.Epoch && rev.SimDay <= old.SimDay {
			continue
		}
		s.revals[id] = rev
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

func (t *memTx) InvestorProfile(ctx context.Context, customerID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	profile, ok := t.store.profiles[customerID]
	if !ok {
		return "", fmt.Errorf("mem: %w", advisory.ErrUnknownCustomer)
	}
	return profile, nil
}

func (t *memTx) UpdateBook(ctx context.Context, customerID string, aum float64, segment string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.book[customerID] = bookRow{aum: aum, segment: segment}
	return nil
}

func (t *memTx) SaveRevaluation(ctx context.Context, customerID string, rev advisory.Revaluation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.store.failIns == "revaluation" {
		return errors.New("forced revaluation save failure")
	}
	t.revals[customerID] = rev
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
	if seg.SourceSchemaVersion != event.SchemaVersionMVP || aporte.SourceSchemaVersion != event.SchemaVersionMVP {
		t.Fatalf("source schema versions = %d/%d, want 1", seg.SourceSchemaVersion, aporte.SourceSchemaVersion)
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
		if row.SourceSchemaVersion != event.SchemaVersionCents {
			t.Fatalf("%s source schema version = %d, want 2", row.Kind, row.SourceSchemaVersion)
		}
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

// purchaseEnv is a v3 aplicacao of productID at risk for customerID, in
// cents, leaving patrimony (before = after) unchanged.
func purchaseEnv(eventID, customerID, productID, class string, risk int, amount, patrimony float64) event.Envelope {
	return event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       eventID,
		OccurredAt:    time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
		CustomerID:    customerID,
		SchemaVersion: event.SchemaVersionPositions,
		Payload: sim.AccountPayload{
			Kind: sim.KindAplicacao, Amount: amount, Before: patrimony, After: patrimony,
			ProductID: productID, AssetClass: class, Risk: risk,
		},
	}
}

// Fernanda (conservador, max risk 2) buys US$ 1,000 of cobalto (risk 5): one
// perfil alert that names the product and amount, and the book follows the
// unchanged patrimony.
func TestApply_PurchaseAboveProfileRaisesPerfil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := newMemStore()
	store.profiles = map[string]string{"c-fernanda": advisory.ProfileConservador}

	env := purchaseEnv("ev-buy-cobalto", "c-fernanda", "cobalto", sim.ClassAcoes, 5, 100_000, 820_000)
	if err := advisory.Apply(ctx, store, env); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if store.alertCount() != 1 || store.outboxCount() != 1 {
		t.Fatalf("alerts %d outbox %d, want 1 and 1", store.alertCount(), store.outboxCount())
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, row := range store.alerts {
		if row.Kind != advisory.KindPerfil || row.Rule != "Compra acima do perfil de investidor" ||
			row.SourceEventID != env.EventID || row.SourceSchemaVersion != event.SchemaVersionPositions {
			t.Fatalf("alert row = %+v", row)
		}
		var payload advisory.AlertPayload
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		want := advisory.AlertPayload{
			Kind: advisory.KindPerfil, Rule: advisory.RuleSuitability, SourceEventID: env.EventID,
			Amount: 1_000, Before: 8_200, After: 8_200,
			ProductID: "cobalto", AssetClass: sim.ClassAcoes, Risk: 5, Profile: advisory.ProfileConservador, MaxRisk: 2,
		}
		if payload != want {
			t.Fatalf("payload = %+v, want %+v", payload, want)
		}
		// The BFF board decodes these literal keys; keep them in sync.
		var raw map[string]any
		if err := json.Unmarshal(row.Payload, &raw); err != nil {
			t.Fatalf("unmarshal raw: %v", err)
		}
		wantRaw := map[string]any{
			"product_id":  "cobalto",
			"asset_class": sim.ClassAcoes,
			"risk":        float64(5),
			"profile":     "conservador",
			"max_risk":    float64(2),
		}
		for key, want := range wantRaw {
			if got, ok := raw[key]; !ok || got != want {
				t.Fatalf("payload[%q] = %v (present %v), want %v", key, got, ok, want)
			}
		}
	}
	for _, out := range store.outbox {
		if out.row.RoutingKey != event.NameAlertRaised {
			t.Fatalf("outbox routing = %s", out.row.RoutingKey)
		}
	}
	if row := store.book["c-fernanda"]; row.aum != 8_200 || row.segment != "Essencial" {
		t.Fatalf("book = %+v, want aum 8200 Essencial", row)
	}
}

// Thiago (arrojado) buying cobalto, or anyone buying within the profile,
// raises nothing; the inbox is still claimed and the book still follows.
func TestApply_PurchaseWithinProfileRaisesNothing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		profile  string
		product  string
		class    string
		risk     int
		customer string
	}{
		{name: "arrojado buys cobalto", profile: advisory.ProfileArrojado, product: "cobalto", class: sim.ClassAcoes, risk: 5, customer: "c-thiago"},
		{name: "moderado buys acoesg at the limit", profile: advisory.ProfileModerado, product: "acoesg", class: sim.ClassETFs, risk: 3, customer: "c-mariana"},
		{name: "conservador buys corp at the limit", profile: advisory.ProfileConservador, product: "corp", class: sim.ClassRendaFixa, risk: 2, customer: "c-fernanda"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			store.profiles = map[string]string{tt.customer: tt.profile}
			env := purchaseEnv("ev-"+tt.customer, tt.customer, tt.product, tt.class, tt.risk, 3_000_000, 6_800_000)
			if err := advisory.Apply(context.Background(), store, env); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if store.alertCount() != 0 || store.outboxCount() != 0 {
				t.Fatalf("alerts %d outbox %d, want none", store.alertCount(), store.outboxCount())
			}
			if store.inboxCount() != 1 {
				t.Fatalf("inbox = %d, want 1", store.inboxCount())
			}
			if row := store.book[tt.customer]; row.aum != 68_000 || row.segment != "Advance" {
				t.Fatalf("book = %+v, want aum 68000 Advance", row)
			}
		})
	}
}

// A purchase for a customer outside the book fails the whole transaction.
func TestApply_PurchaseWithoutProfileRollsBack(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	env := purchaseEnv("ev-nobody", "c-nobody", "cobalto", sim.ClassAcoes, 5, 100_000, 820_000)
	err := advisory.Apply(context.Background(), store, env)
	if !errors.Is(err, advisory.ErrUnknownCustomer) {
		t.Fatalf("Apply error = %v, want ErrUnknownCustomer", err)
	}
	if store.inboxCount() != 0 || store.alertCount() != 0 || len(store.book) != 0 {
		t.Fatalf("partial write: inbox %d alerts %d book %d", store.inboxCount(), store.alertCount(), len(store.book))
	}
}

func TestApply_PurchaseRedeliveryRaisesOnce(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	store.profiles = map[string]string{"c-fernanda": advisory.ProfileConservador}
	env := purchaseEnv("ev-buy-twice", "c-fernanda", "farol", sim.ClassAcoes, 4, 100_000, 820_000)
	for range 2 {
		if err := advisory.Apply(context.Background(), store, env); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
	if store.alertCount() != 1 || store.outboxCount() != 1 {
		t.Fatalf("alerts %d outbox %d, want 1 and 1", store.alertCount(), store.outboxCount())
	}
}

// An aplicacao that cannot be evaluated is rejected before anything is written,
// with ErrInvalidPurchase, so the consumer dead-letters it instead of requeuing.
func TestApply_InvalidPurchaseIsRejected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		version int
		product string
		risk    int
	}{
		{name: "aplicacao at schema_version 2", version: event.SchemaVersionCents, product: "cobalto", risk: 5},
		{name: "aplicacao at schema_version 1", version: event.SchemaVersionMVP, product: "cobalto", risk: 5},
		{name: "risk 0", version: event.SchemaVersionPositions, product: "cobalto", risk: 0},
		{name: "risk 6", version: event.SchemaVersionPositions, product: "cobalto", risk: 6},
		{name: "empty product_id", version: event.SchemaVersionPositions, product: "", risk: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			store.profiles = map[string]string{"c-fernanda": advisory.ProfileConservador}
			env := purchaseEnv("ev-bad-buy", "c-fernanda", tt.product, sim.ClassAcoes, tt.risk, 100_000, 820_000)
			env.SchemaVersion = tt.version
			err := advisory.Apply(context.Background(), store, env)
			if !errors.Is(err, advisory.ErrInvalidPurchase) {
				t.Fatalf("Apply error = %v, want ErrInvalidPurchase", err)
			}
			if store.inboxCount() != 0 || store.alertCount() != 0 || store.outboxCount() != 0 || len(store.book) != 0 {
				t.Fatalf("partial write: inbox %d alerts %d outbox %d book %d",
					store.inboxCount(), store.alertCount(), store.outboxCount(), len(store.book))
			}
		})
	}
}

// revaluationEnv is a v3 reavaliacao in cents, as account-sim writes it on
// advance-day.
func revaluationEnv(eventID, customerID string, day int, amount, before float64, productID string, productBP int) event.Envelope {
	return event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       testEventID(eventID),
		OccurredAt:    time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
		CustomerID:    customerID,
		SchemaVersion: event.SchemaVersionPositions,
		Payload: map[string]any{
			"kind": "reavaliacao", "amount": amount, "before": before, "after": before + amount,
			"sim_day": day, "product_id": productID, "product_change_bp": productBP,
			"epoch": testEpoch,
		},
	}
}

// testEpoch is the account-sim simulation epoch of revaluationEnv.
const testEpoch = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

// testEventID is a stable UUID for a readable test event name, as account-sim
// ids its reavaliacao events.
func testEventID(name string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

// withPayload returns env with key set in its reavaliacao payload.
func withPayload(env event.Envelope, key string, value any) event.Envelope {
	p := maps.Clone(env.Payload.(map[string]any))
	p[key] = value
	env.Payload = p
	return env
}

// A v3 reavaliacao follows the book (AUM from after, the segment rule), raises
// queda on a loss above 15% of before, and is kept in cents as the latest
// revaluation.
func TestApply_Reavaliacao(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		env       event.Envelope
		wantKinds []string
		wantAUM   float64
		wantSeg   string
	}{
		{
			name:      "mariana loses 15.5% on day 3",
			env:       revaluationEnv("ev-mariana-3", "c-mariana", 3, -3_852_000, 24_830_000, "cobalto", -5350),
			wantKinds: []string{advisory.KindQueda},
			wantAUM:   209_780, wantSeg: "Singular",
		},
		{
			name:    "thiago loses 1.6% on day 3",
			env:     revaluationEnv("ev-thiago-3", "c-thiago", 3, -109_140, 6_800_000, "cobalto", -5350),
			wantAUM: 66_908.60, wantSeg: "Advance",
		},
		{
			name:    "fernanda has a flat day",
			env:     revaluationEnv("ev-fernanda-1", "c-fernanda", 1, 0, 820_000, "", 0),
			wantAUM: 8_200, wantSeg: "Essencial",
		},
		{
			name:      "a drop across the band raises queda and segmento",
			env:       revaluationEnv("ev-cross", "c-thiago", 3, -200_000, 1_100_000, "farol", -1818),
			wantKinds: []string{advisory.KindQueda, advisory.KindSegmento},
			wantAUM:   9_000, wantSeg: "Essencial",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			if err := advisory.Apply(context.Background(), store, tt.env); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			kinds := slices.Sorted(maps.Values(store.alertKinds()))
			if !slices.Equal(kinds, tt.wantKinds) {
				t.Fatalf("alerts = %v, want %v", kinds, tt.wantKinds)
			}
			customer := tt.env.CustomerID
			if row := store.book[customer]; math.Abs(row.aum-tt.wantAUM) > 1e-9 || row.segment != tt.wantSeg {
				t.Fatalf("book = %+v, want aum %v %s", row, tt.wantAUM, tt.wantSeg)
			}
			p := tt.env.Payload.(map[string]any)
			want := advisory.Revaluation{
				Epoch:           testEpoch,
				SimDay:          p["sim_day"].(int),
				AmountCents:     int64(p["amount"].(float64)),
				BeforeCents:     int64(p["before"].(float64)),
				ProductID:       p["product_id"].(string),
				ProductChangeBP: p["product_change_bp"].(int),
				SourceEventID:   tt.env.EventID,
			}
			if got := store.revals[customer]; got != want {
				t.Fatalf("revaluation = %+v, want %+v", got, want)
			}
		})
	}
}

// Mariana's queda carries the rule text and the dollars of the loss, and a
// redelivery raises nothing more.
func TestApply_ReavaliacaoQuedaPayloadAndRedelivery(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	env := revaluationEnv("ev-mariana-3", "c-mariana", 3, -3_852_000, 24_830_000, "cobalto", -5350)
	for range 2 {
		if err := advisory.Apply(context.Background(), store, env); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
	if store.alertCount() != 1 || store.outboxCount() != 1 {
		t.Fatalf("alerts %d outbox %d, want 1 and 1", store.alertCount(), store.outboxCount())
	}
	for _, row := range store.alerts {
		var payload advisory.AlertPayload
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		want := advisory.AlertPayload{
			Kind: advisory.KindQueda, Rule: "Queda acima de 15% em 5 dias úteis", SourceEventID: env.EventID,
			Amount: -38_520, Before: 248_300, After: 209_780,
		}
		if payload != want || row.SourceSchemaVersion != event.SchemaVersionPositions {
			t.Fatalf("queda = %+v (v%d), want %+v (v3)", payload, row.SourceSchemaVersion, want)
		}
	}
}

// A reavaliacao that cannot be kept is rejected before anything is written,
// with ErrInvalidRevaluation, so the consumer dead-letters it; a failed save
// rolls the whole fact back.
func TestApply_InvalidReavaliacaoIsRejected(t *testing.T) {
	t.Parallel()
	valid := func() event.Envelope {
		return revaluationEnv("ev-bad-reval", "c-mariana", 3, -3_852_000, 24_830_000, "cobalto", -5350)
	}
	tests := []struct {
		name    string
		env     func() event.Envelope
		failIns string
		wantErr error
	}{
		{
			name:    "schema_version 2",
			env:     func() event.Envelope { e := valid(); e.SchemaVersion = event.SchemaVersionCents; return e },
			wantErr: advisory.ErrInvalidRevaluation,
		},
		{
			name: "sim_day 0",
			env: func() event.Envelope {
				return revaluationEnv("ev-bad-reval", "c-mariana", 0, -3_852_000, 24_830_000, "cobalto", -5350)
			},
			wantErr: advisory.ErrInvalidRevaluation,
		},
		{
			name: "fractional amount",
			env: func() event.Envelope {
				return revaluationEnv("ev-bad-reval", "c-mariana", 3, -3_852_000.5, 24_830_000, "cobalto", -5350)
			},
			wantErr: sim.ErrMoneyScale,
		},
		{
			name: "fractional before",
			env: func() event.Envelope {
				return revaluationEnv("ev-bad-reval", "c-mariana", 3, -3_852_000, 24_830_000.25, "cobalto", -5350)
			},
			wantErr: sim.ErrMoneyScale,
		},
		{
			name: "amount too large to be exact",
			env: func() event.Envelope {
				return revaluationEnv("ev-bad-reval", "c-mariana", 3, -(1 << 54), 1<<52, "cobalto", -5350)
			},
			wantErr: advisory.ErrInvalidRevaluation,
		},
		{
			name: "before too large to be exact",
			env: func() event.Envelope {
				return revaluationEnv("ev-bad-reval", "c-mariana", 3, 0, 1<<54, "cobalto", -5350)
			},
			wantErr: advisory.ErrInvalidRevaluation,
		},
		{
			name:    "sim_day beyond int32",
			env:     func() event.Envelope { return withPayload(valid(), "sim_day", int64(math.MaxInt32)+1) },
			wantErr: advisory.ErrInvalidRevaluation,
		},
		{
			name:    "product_change_bp beyond int32",
			env:     func() event.Envelope { return withPayload(valid(), "product_change_bp", int64(math.MinInt32)-1) },
			wantErr: advisory.ErrInvalidRevaluation,
		},
		{
			name:    "event id not a uuid",
			env:     func() event.Envelope { e := valid(); e.EventID = "ev-bad-reval"; return e },
			wantErr: advisory.ErrInvalidRevaluation,
		},
		{
			name:    "epoch missing",
			env:     func() event.Envelope { return withPayload(valid(), "epoch", "") },
			wantErr: advisory.ErrInvalidRevaluation,
		},
		{
			name:    "epoch not a uuid",
			env:     func() event.Envelope { return withPayload(valid(), "epoch", "epoch-1") },
			wantErr: advisory.ErrInvalidRevaluation,
		},
		{name: "save fails", env: valid, failIns: "revaluation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			store.failIns = tt.failIns
			err := advisory.Apply(context.Background(), store, tt.env())
			if err == nil || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Apply error = %v, want %v", err, tt.wantErr)
			}
			if store.inboxCount() != 0 || store.alertCount() != 0 || store.outboxCount() != 0 || len(store.book) != 0 || len(store.revals) != 0 {
				t.Fatalf("partial write: inbox %d alerts %d outbox %d book %d revaluations %d",
					store.inboxCount(), store.alertCount(), store.outboxCount(), len(store.book), len(store.revals))
			}
		})
	}
}

// The kept revaluation moves only forward within an epoch: a redelivered older
// day leaves the newer one, and any day of a new epoch replaces it.
func TestApply_ReavaliacaoKeepsTheNewestDay(t *testing.T) {
	t.Parallel()
	const nextEpoch = "0b0a7f1c-3a3e-4c1d-9a55-2f1d6c0e8b11"
	day3 := revaluationEnv("ev-mariana-3", "c-mariana", 3, -3_852_000, 24_830_000, "cobalto", -5350)
	day2 := revaluationEnv("ev-mariana-2", "c-mariana", 2, 0, 24_830_000, "", 0)
	reseeded := withPayload(revaluationEnv("ev-mariana-new-1", "c-mariana", 1, 0, 24_830_000, "", 0), "epoch", nextEpoch)
	tests := []struct {
		name      string
		envs      []event.Envelope
		wantDay   int
		wantEpoch string
	}{
		{name: "a later day replaces", envs: []event.Envelope{day2, day3}, wantDay: 3, wantEpoch: testEpoch},
		{name: "an older day redelivered late is ignored", envs: []event.Envelope{day3, day2}, wantDay: 3, wantEpoch: testEpoch},
		{name: "a new epoch replaces a later day", envs: []event.Envelope{day3, reseeded}, wantDay: 1, wantEpoch: nextEpoch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			for _, env := range tt.envs {
				if err := advisory.Apply(context.Background(), store, env); err != nil {
					t.Fatalf("Apply: %v", err)
				}
			}
			if got := store.revals["c-mariana"]; got.SimDay != tt.wantDay || got.Epoch != tt.wantEpoch {
				t.Fatalf("kept revaluation = %+v, want day %d of epoch %s", got, tt.wantDay, tt.wantEpoch)
			}
		})
	}
}

// Consumers accept schema versions 1, 2, and 3 and reject any other before a
// rule runs.
func TestApply_SchemaVersions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		version int
		amount  float64
		before  float64
		wantErr bool
		wantAUM float64
	}{
		{name: "version 1 dollars", version: 1, amount: 5_000, before: 20_000},
		{name: "version 2 cents", version: 2, amount: 500_000, before: 2_000_000, wantAUM: 15_000},
		{name: "version 3 cents", version: 3, amount: 500_000, before: 2_000_000, wantAUM: 15_000},
		{name: "version 4 is rejected", version: 4, amount: 500_000, before: 2_000_000, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newMemStore()
			env := event.Envelope{
				Name:          event.NameAccountEventRecorded,
				EventID:       "ev-v",
				OccurredAt:    time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
				CustomerID:    "c-x",
				SchemaVersion: tt.version,
				Payload:       sim.AccountPayload{Kind: "saque", Amount: tt.amount, Before: tt.before, After: tt.before - tt.amount},
			}
			err := advisory.Apply(context.Background(), store, env)
			if tt.wantErr {
				if !errors.Is(err, sim.ErrMoneyScale) {
					t.Fatalf("Apply error = %v, want ErrMoneyScale", err)
				}
				if store.inboxCount() != 0 {
					t.Fatalf("inbox = %d, want 0", store.inboxCount())
				}
				return
			}
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			kinds := store.alertKinds()
			if len(kinds) != 1 {
				t.Fatalf("alerts = %v, want one saque", kinds)
			}
			for id, kind := range kinds {
				var payload advisory.AlertPayload
				store.mu.Lock()
				raw := store.alerts[id].Payload
				store.mu.Unlock()
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if kind != advisory.KindSaque || payload.Amount != 5_000 || payload.Before != 20_000 {
					t.Fatalf("%s payload = %+v, want dollars 5000 of 20000", kind, payload)
				}
			}
			if row, ok := store.book["c-x"]; tt.wantAUM == 0 && ok || tt.wantAUM != 0 && row.aum != tt.wantAUM {
				t.Fatalf("book = %+v (%v), want aum %v", row, ok, tt.wantAUM)
			}
		})
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
