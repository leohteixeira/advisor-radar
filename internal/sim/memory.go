package sim

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/leohteixeira/advisor-radar/internal/outbox"
)

// Memory is an in-process POV store seeded with DemoSeed: the product
// catalog, and the cash, positions, and registration of the three accounts.
type Memory struct {
	mu            sync.Mutex
	products      []Product
	cash          map[string]int64
	positions     map[string][]Position
	registrations map[string]Registration
	keys          map[string]string
	outbox        []outbox.Row
	published     map[string]struct{}
}

// NewMemory returns a store holding DemoSeed.
func NewMemory() *Memory {
	m := &Memory{
		cash:          map[string]int64{},
		positions:     map[string][]Position{},
		registrations: map[string]Registration{},
		keys:          map[string]string{},
		published:     map[string]struct{}{},
	}
	m.reset(DemoSeed())
	return m
}

// reset replaces the seeded rows. Stored slices are never mutated in place,
// so a shallow snapshot in WithTx is enough to roll back.
func (m *Memory) reset(seed Seed) {
	m.products = slices.Clone(seed.Products)
	for _, account := range seed.Accounts {
		m.cash[account.CustomerID] = account.CashCents
		m.positions[account.CustomerID] = slices.Clone(account.Positions)
		m.registrations[account.CustomerID] = account.Registration
	}
}

// WithTx runs fn under the store lock. An error leaves the previous snapshot.
func (m *Memory) WithTx(ctx context.Context, fn func(Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	snapProducts := m.products
	snapCash := maps.Clone(m.cash)
	snapPositions := maps.Clone(m.positions)
	snapRegistrations := maps.Clone(m.registrations)
	snapKeys := maps.Clone(m.keys)
	snapOut := slices.Clone(m.outbox)

	tx := &memoryTx{store: m}
	if err := fn(tx); err != nil {
		m.products = snapProducts
		m.cash = snapCash
		m.positions = snapPositions
		m.registrations = snapRegistrations
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
	cash, ok := t.store.cash[customerID]
	if !ok {
		return Account{}, false, nil
	}
	account, err := aggregate(customerID, cash, valuePositions(t.store.positions[customerID], SeedDay))
	if err != nil {
		return Account{}, false, err
	}
	return account, true, nil
}

// PutAccount writes only the cash; positions are never set from Account.
func (t *memoryTx) PutAccount(ctx context.Context, account Account) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.store.cash[account.CustomerID] = account.Caixa
	return nil
}

func (t *memoryTx) ListProducts(ctx context.Context) ([]Product, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	products := slices.Clone(t.store.products)
	slices.SortFunc(products, func(a, b Product) int {
		if c := cmp.Compare(a.Risk, b.Risk); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return products, nil
}

func (t *memoryTx) GetRegistration(ctx context.Context, customerID string) (Registration, bool, error) {
	if err := ctx.Err(); err != nil {
		return Registration{}, false, err
	}
	registration, ok := t.store.registrations[customerID]
	return registration, ok, nil
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

func (t *memoryTx) ResetPOV(ctx context.Context, seed Seed) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.store.reset(seed)
	return nil
}
