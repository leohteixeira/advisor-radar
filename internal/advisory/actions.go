package advisory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const snoozeDuration = time.Hour

// SignalAction is one persisted Contatado / Adiar row keyed by signal id.
type SignalAction struct {
	SignalID     string     `json:"signal_id"`
	ContactedAt  *time.Time `json:"contacted_at,omitempty"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
}

// ActionStore persists signal actions. Declared by this package for Actions.
type ActionStore interface {
	Upsert(ctx context.Context, action SignalAction) error
	Delete(ctx context.Context, signalID string) error
	List(ctx context.Context) ([]SignalAction, error)
	Get(ctx context.Context, signalID string) (SignalAction, bool, error)
}

// Actions applies contact, snooze, and undo against an ActionStore.
type Actions struct {
	store ActionStore
	now   func() time.Time
}

// NewActions returns an Actions service. now defaults to time.Now when nil.
func NewActions(store ActionStore, now func() time.Time) *Actions {
	if now == nil {
		now = time.Now
	}
	return &Actions{store: store, now: now}
}

// Contact sets contacted_at to the injected clock.
func (a *Actions) Contact(ctx context.Context, signalID string) error {
	if signalID == "" {
		return fmt.Errorf("advisory: contact: signal id is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	row, _, err := a.store.Get(ctx, signalID)
	if err != nil {
		return fmt.Errorf("advisory: contact get: %w", err)
	}
	at := a.now().UTC()
	row.SignalID = signalID
	row.ContactedAt = &at
	row.SnoozedUntil = nil
	if err := a.store.Upsert(ctx, row); err != nil {
		return fmt.Errorf("advisory: contact: %w", err)
	}
	return nil
}

// Snooze sets snoozed_until to one hour after the injected clock.
func (a *Actions) Snooze(ctx context.Context, signalID string) error {
	if signalID == "" {
		return fmt.Errorf("advisory: snooze: signal id is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	row, _, err := a.store.Get(ctx, signalID)
	if err != nil {
		return fmt.Errorf("advisory: snooze get: %w", err)
	}
	until := a.now().UTC().Add(snoozeDuration)
	row.SignalID = signalID
	row.SnoozedUntil = &until
	if err := a.store.Upsert(ctx, row); err != nil {
		return fmt.Errorf("advisory: snooze: %w", err)
	}
	return nil
}

// Undo deletes the action row for signalID.
func (a *Actions) Undo(ctx context.Context, signalID string) error {
	if signalID == "" {
		return fmt.Errorf("advisory: undo: signal id is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := a.store.Delete(ctx, signalID); err != nil {
		return fmt.Errorf("advisory: undo: %w", err)
	}
	return nil
}

// List returns every persisted action row.
func (a *Actions) List(ctx context.Context) ([]SignalAction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := a.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("advisory: list actions: %w", err)
	}
	return rows, nil
}

// MemoryActionStore is an in-process ActionStore for tests and HTTP without a database.
type MemoryActionStore struct {
	mu   sync.Mutex
	rows map[string]SignalAction
}

// NewMemoryActionStore returns an empty memory store.
func NewMemoryActionStore() *MemoryActionStore {
	return &MemoryActionStore{rows: make(map[string]SignalAction)}
}

// Upsert replaces the row for action.SignalID.
func (s *MemoryActionStore) Upsert(ctx context.Context, action SignalAction) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[action.SignalID] = action
	return nil
}

// Delete removes the row for signalID. Missing ids are a no-op.
func (s *MemoryActionStore) Delete(ctx context.Context, signalID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, signalID)
	return nil
}

// List returns a copy of every row.
func (s *MemoryActionStore) List(ctx context.Context) ([]SignalAction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SignalAction, 0, len(s.rows))
	for _, row := range s.rows {
		out = append(out, row)
	}
	return out, nil
}

// Get returns the row for signalID when present.
func (s *MemoryActionStore) Get(ctx context.Context, signalID string) (SignalAction, bool, error) {
	if err := ctx.Err(); err != nil {
		return SignalAction{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.rows[signalID]
	return row, ok, nil
}
