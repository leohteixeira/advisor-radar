package sim

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/leohteixeira/advisor-radar/internal/outbox"
)

// Memory is an in-process POV store seeded with DemoSeed: the product
// catalog, the cash, positions, registration, and preferences of the three
// accounts, and the simulated day. One mutex serializes every transaction, so
// an advance and a command never interleave.
type Memory struct {
	mu            sync.Mutex
	products      []Product
	cash          map[string]int64
	positions     map[string][]Position
	registrations map[string]Registration
	preferences   map[string]Preferences
	keys          map[string]string
	outbox        []outbox.Row
	published     map[string]struct{}
	sim           SimState
	advances      map[string]AdvanceResult
}

// NewMemory returns a store holding DemoSeed.
func NewMemory() *Memory {
	m := &Memory{
		cash:          map[string]int64{},
		positions:     map[string][]Position{},
		registrations: map[string]Registration{},
		preferences:   map[string]Preferences{},
		keys:          map[string]string{},
		published:     map[string]struct{}{},
		advances:      map[string]AdvanceResult{},
	}
	m.reset(DemoSeed())
	return m
}

// reset replaces the seeded rows, returns the simulated day to 0 in a new
// epoch, and forgets every advance key, so a key from before the reseed
// advances again instead of replaying the old epoch. Stored slices are never mutated in place, so a shallow snapshot in
// WithTx is enough to roll back.
func (m *Memory) reset(seed Seed) {
	m.products = slices.Clone(seed.Products)
	for _, account := range seed.Accounts {
		m.cash[account.CustomerID] = account.CashCents
		m.positions[account.CustomerID] = slices.Clone(account.Positions)
		m.registrations[account.CustomerID] = account.Registration
		m.preferences[account.CustomerID] = seedPreferences(account.Preferences)
	}
	m.sim = SimState{Day: 0, Epoch: uuid.NewString()}
	m.advances = map[string]AdvanceResult{}
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
	snapPreferences := maps.Clone(m.preferences)
	snapKeys := maps.Clone(m.keys)
	snapOut := slices.Clone(m.outbox)
	snapSim := m.sim
	snapAdvances := maps.Clone(m.advances)

	tx := &memoryTx{store: m}
	if err := fn(tx); err != nil {
		m.products = snapProducts
		m.cash = snapCash
		m.positions = snapPositions
		m.registrations = snapRegistrations
		m.preferences = snapPreferences
		m.keys = snapKeys
		m.outbox = snapOut
		m.sim = snapSim
		m.advances = snapAdvances
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
	account, err := valueAccount(customerID, cash, t.store.positions[customerID], t.store.sim.Day)
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

// AddPosition adds delta to the customer's position in delta.ProductID, or
// appends it. It writes a new slice, so the WithTx snapshot stays intact.
func (t *memoryTx) AddPosition(ctx context.Context, customerID string, delta Position) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := t.store.cash[customerID]; !ok {
		return fmt.Errorf("sim memory: add position: %w", ErrUnknownCustomer)
	}
	if !slices.ContainsFunc(t.store.products, func(p Product) bool { return p.ID == delta.ProductID }) {
		return fmt.Errorf("sim memory: add position: %w", ErrProduct)
	}
	positions := slices.Clone(t.store.positions[customerID])
	i := slices.IndexFunc(positions, func(p Position) bool { return p.ProductID == delta.ProductID })
	if i < 0 {
		positions = append(positions, Position{
			ProductID:    delta.ProductID,
			AssetClass:   delta.AssetClass,
			UnitsCents:   delta.UnitsCents,
			AppliedCents: delta.AppliedCents,
		})
	} else {
		positions[i].UnitsCents += delta.UnitsCents
		positions[i].AppliedCents += delta.AppliedCents
	}
	t.store.positions[customerID] = positions
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

// GetPreferences reads the preferences reset stored with the account; a
// known customer without an entry answers DefaultPreferences, as pgx does
// without a row. The store lock already serializes it with every writer.
func (t *memoryTx) GetPreferences(ctx context.Context, customerID string) (Preferences, bool, error) {
	if err := ctx.Err(); err != nil {
		return Preferences{}, false, err
	}
	if _, ok := t.store.cash[customerID]; !ok {
		return Preferences{}, false, nil
	}
	prefs, ok := t.store.preferences[customerID]
	if !ok {
		return DefaultPreferences(), true, nil
	}
	return prefs, true, nil
}

func (t *memoryTx) PutPreferences(ctx context.Context, customerID string, prefs Preferences) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := t.store.cash[customerID]; !ok {
		return fmt.Errorf("sim memory: put preferences: %w", ErrUnknownCustomer)
	}
	t.store.preferences[customerID] = prefs
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

func (t *memoryTx) ResetPOV(ctx context.Context, seed Seed) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.store.reset(seed)
	return nil
}

// LockSimulation returns the stored day; WithTx already holds the store lock.
func (t *memoryTx) LockSimulation(ctx context.Context) (SimState, error) {
	return t.Simulation(ctx)
}

func (t *memoryTx) Simulation(ctx context.Context) (SimState, error) {
	if err := ctx.Err(); err != nil {
		return SimState{}, err
	}
	return t.store.sim, nil
}

func (t *memoryTx) SetSimDay(ctx context.Context, day int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.store.sim.Day = day
	return nil
}

// CustomerIDs lists the POV accounts in byte-wise id order.
func (t *memoryTx) CustomerIDs(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return slices.Sorted(maps.Keys(t.store.cash)), nil
}

func (t *memoryTx) LookupAdvance(ctx context.Context, key string) (AdvanceResult, bool, error) {
	if err := ctx.Err(); err != nil {
		return AdvanceResult{}, false, err
	}
	result, ok := t.store.advances[key]
	result.EventIDs = slices.Clone(result.EventIDs)
	return result, ok, nil
}

func (t *memoryTx) SaveAdvance(ctx context.Context, key string, result AdvanceResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result.EventIDs = slices.Clone(result.EventIDs)
	result.Replay = false
	t.store.advances[key] = result
	return nil
}
