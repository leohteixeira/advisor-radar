package sim

import (
	"context"
	"sync"

	"github.com/leohteixeira/advisor-radar/internal/outbox"
)

// Memory is an in-process POV store seeded with the three demo accounts.
type Memory struct {
	mu        sync.Mutex
	accounts  map[string]Account
	keys      map[string]string
	outbox    []outbox.Row
	published map[string]struct{}
}

// NewMemory returns the seeded POV balances.
func NewMemory() *Memory {
	m := &Memory{
		accounts:  map[string]Account{},
		keys:      map[string]string{},
		published: map[string]struct{}{},
	}
	for _, account := range POVSeed() {
		m.accounts[account.CustomerID] = account
	}
	return m
}

// WithTx runs fn under the store lock. An error leaves the previous snapshot.
func (m *Memory) WithTx(ctx context.Context, fn func(Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	snapAccounts := map[string]Account{}
	for id, account := range m.accounts {
		snapAccounts[id] = account
	}
	snapKeys := map[string]string{}
	for key, id := range m.keys {
		snapKeys[key] = id
	}
	snapOut := append([]outbox.Row(nil), m.outbox...)

	tx := &memoryTx{store: m}
	if err := fn(tx); err != nil {
		m.accounts = snapAccounts
		m.keys = snapKeys
		m.outbox = snapOut
		return err
	}
	return nil
}

// PendingOutbox returns outbox rows that have not been published yet.
func (m *Memory) PendingOutbox() []outbox.Row {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]outbox.Row, 0, len(m.outbox))
	for _, row := range m.outbox {
		if _, ok := m.published[row.EventID]; ok {
			continue
		}
		out = append(out, row)
	}
	return out
}

// MarkPublished records that the broker accepted this outbox row.
func (m *Memory) MarkPublished(eventID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.published == nil {
		m.published = map[string]struct{}{}
	}
	m.published[eventID] = struct{}{}
}

type memoryTx struct {
	store *Memory
}

func (t *memoryTx) GetAccount(ctx context.Context, customerID string) (Account, bool, error) {
	if err := ctx.Err(); err != nil {
		return Account{}, false, err
	}
	account, ok := t.store.accounts[customerID]
	return account, ok, nil
}

func (t *memoryTx) PutAccount(ctx context.Context, account Account) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.store.accounts[account.CustomerID] = account
	return nil
}

func (t *memoryTx) LookupKey(ctx context.Context, customerID, key string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	id, ok := t.store.keys[customerID+"\x00"+key]
	return id, ok, nil
}

func (t *memoryTx) SaveKey(ctx context.Context, customerID, key, eventID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.store.keys[customerID+"\x00"+key] = eventID
	return nil
}

func (t *memoryTx) InsertOutbox(ctx context.Context, row outbox.Row) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.store.outbox = append(t.store.outbox, row)
	return nil
}

func (t *memoryTx) ResetPOV(ctx context.Context, accounts []Account) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, account := range accounts {
		t.store.accounts[account.CustomerID] = account
	}
	return nil
}
