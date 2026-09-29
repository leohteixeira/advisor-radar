package sim_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/outbox"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

type memStore struct {
	accounts map[string]sim.Account
	keys     map[string]string
	outbox   []outbox.Row
	fail     string
}

func newStore() *memStore {
	s := &memStore{
		accounts: map[string]sim.Account{},
		keys:     map[string]string{},
	}
	for _, account := range sim.POVSeed() {
		s.accounts[account.CustomerID] = account
	}
	return s
}

func (s *memStore) WithTx(ctx context.Context, fn func(sim.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	snapAccounts := map[string]sim.Account{}
	for id, account := range s.accounts {
		snapAccounts[id] = account
	}
	snapKeys := map[string]string{}
	for key, id := range s.keys {
		snapKeys[key] = id
	}
	snapOut := append([]outbox.Row(nil), s.outbox...)
	tx := &memTx{store: s}
	if err := fn(tx); err != nil {
		s.accounts = snapAccounts
		s.keys = snapKeys
		s.outbox = snapOut
		return err
	}
	return nil
}

type memTx struct {
	store *memStore
}

func (t *memTx) GetAccount(ctx context.Context, customerID string) (sim.Account, bool, error) {
	if err := ctx.Err(); err != nil {
		return sim.Account{}, false, err
	}
	account, ok := t.store.accounts[customerID]
	return account, ok, nil
}

func (t *memTx) PutAccount(ctx context.Context, account sim.Account) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.store.fail == "put" {
		return errors.New("forced put failure")
	}
	t.store.accounts[account.CustomerID] = account
	return nil
}

func (t *memTx) LookupKey(ctx context.Context, customerID, key string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	id, ok := t.store.keys[customerID+"\x00"+key]
	return id, ok, nil
}

func (t *memTx) SaveKey(ctx context.Context, customerID, key, eventID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.store.keys[customerID+"\x00"+key] = eventID
	return nil
}

func (t *memTx) InsertOutbox(ctx context.Context, row outbox.Row) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.store.fail == "outbox" {
		return errors.New("forced outbox failure")
	}
	t.store.outbox = append(t.store.outbox, row)
	return nil
}

// AddPosition adds delta to the class aggregate and the position list; this
// fake keeps the account as stored aggregates.
func (t *memTx) AddPosition(ctx context.Context, customerID string, delta sim.Position) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.store.fail == "position" {
		return errors.New("forced position failure")
	}
	account, ok := t.store.accounts[customerID]
	if !ok {
		return sim.ErrUnknownCustomer
	}
	switch delta.AssetClass {
	case sim.ClassAcoes:
		account.Acoes += delta.UnitsCents
	case sim.ClassETFs:
		account.ETFs += delta.UnitsCents
	case sim.ClassRendaFixa:
		account.RendaFixa += delta.UnitsCents
	}
	delta.ValueCents = delta.UnitsCents
	account.Positions = append(slices.Clone(account.Positions), delta)
	t.store.accounts[customerID] = account
	return nil
}

func (t *memTx) ListProducts(ctx context.Context) ([]sim.Product, error) {
	return sim.Catalog(), ctx.Err()
}

func (t *memTx) GetRegistration(ctx context.Context, _ string) (sim.Registration, bool, error) {
	return sim.Registration{}, false, ctx.Err()
}

// GetPreferences and PutPreferences are unused by Apply; this fake stores
// nothing.
func (t *memTx) GetPreferences(ctx context.Context, _ string) (sim.Preferences, bool, error) {
	return sim.Preferences{}, false, ctx.Err()
}

func (t *memTx) PutPreferences(ctx context.Context, _ string, _ sim.Preferences) error {
	return ctx.Err()
}

// ResetPOV keeps only the class aggregates of the seed: this fake models the
// account as four balances.
func (t *memTx) ResetPOV(ctx context.Context, seed sim.Seed) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, account := range seed.Accounts {
		t.store.accounts[account.CustomerID] = aggregateOf(account)
	}
	return nil
}

// errNoSimulation is what the simulation methods of this command fake answer:
// the advance-day tests run on sim.Memory and PGXStore instead.
var errNoSimulation = errors.New("fake store has no simulation")

func (t *memTx) LockSimulation(context.Context) (sim.SimState, error) {
	return sim.SimState{}, errNoSimulation
}

func (t *memTx) Simulation(context.Context) (sim.SimState, error) {
	return sim.SimState{}, errNoSimulation
}

func (t *memTx) SetSimDay(context.Context, int) error { return errNoSimulation }

func (t *memTx) CustomerIDs(context.Context) ([]string, error) { return nil, errNoSimulation }

func (t *memTx) LookupAdvance(context.Context, string) (sim.AdvanceResult, bool, error) {
	return sim.AdvanceResult{}, false, errNoSimulation
}

func (t *memTx) SaveAdvance(context.Context, string, sim.AdvanceResult) error { return errNoSimulation }

// sameAccount compares every field, including the positions.
func sameAccount(a, b sim.Account) bool {
	return a.CustomerID == b.CustomerID && a.Acoes == b.Acoes && a.ETFs == b.ETFs &&
		a.RendaFixa == b.RendaFixa && a.Caixa == b.Caixa && slices.Equal(a.Positions, b.Positions)
}

func aggregateOf(seed sim.SeedAccount) sim.Account {
	account := sim.Account{CustomerID: seed.CustomerID, Caixa: seed.CashCents, Positions: seed.Positions}
	for _, position := range seed.Positions {
		switch position.AssetClass {
		case sim.ClassAcoes:
			account.Acoes += position.ValueCents
		case sim.ClassETFs:
			account.ETFs += position.ValueCents
		case sim.ClassRendaFixa:
			account.RendaFixa += position.ValueCents
		}
	}
	return account
}

func TestPOVSeedSums(t *testing.T) {
	t.Parallel()
	want := map[string]int64{
		sim.CustomerMariana:  24_830_000,
		sim.CustomerFernanda: 820_000,
		sim.CustomerThiago:   6_800_000,
	}
	if len(sim.POVSeed()) != 3 {
		t.Fatalf("seed len = %d, want 3", len(sim.POVSeed()))
	}
	for _, account := range sim.POVSeed() {
		if account.Assets() != want[account.CustomerID] {
			t.Fatalf("%s assets = %d, want %d", account.CustomerID, account.Assets(), want[account.CustomerID])
		}
	}
	for _, account := range sim.POVSeed() {
		if account.CustomerID == sim.CustomerThiago && account.RendaFixa != 0 {
			t.Fatalf("thiago renda fixa = %d, want 0", account.RendaFixa)
		}
	}
}

func TestWithdrawalDebitsOnlyCaixa(t *testing.T) {
	t.Parallel()
	store := newStore()
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	result, err := sim.Apply(context.Background(), store, sim.Command{
		CustomerID:     sim.CustomerMariana,
		IdempotencyKey: "wd-1",
		Kind:           sim.CmdWithdrawal,
		Amount:         5_000_000,
		Destination:    "conta-eua",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.EventID == "" || result.Replay {
		t.Fatalf("result = %+v", result)
	}
	account := store.accounts[sim.CustomerMariana]
	if account.Caixa != 1_000_000 || account.Acoes != 9_090_000 || account.ETFs != 6_060_000 || account.RendaFixa != 3_680_000 {
		t.Fatalf("classes = %+v", account)
	}
	if account.Assets() != 19_830_000 {
		t.Fatalf("assets = %d", account.Assets())
	}
	if len(store.outbox) != 1 {
		t.Fatalf("outbox = %d, want 1", len(store.outbox))
	}
	env, payload, err := sim.DecodeOutboxAccount(store.outbox[0].Payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.SchemaVersion != 2 || payload.Kind != "saque" || payload.Amount != 5_000_000 || payload.Destination != "conta-eua" {
		t.Fatalf("event = %+v payload = %+v", env, payload)
	}
}

func TestWithdrawalAboveCaixaWritesNothing(t *testing.T) {
	t.Parallel()
	store := newStore()
	before := store.accounts[sim.CustomerMariana]
	_, err := sim.Apply(context.Background(), store, sim.Command{
		CustomerID:     sim.CustomerMariana,
		IdempotencyKey: "wd-over",
		Kind:           sim.CmdWithdrawal,
		Amount:         before.Caixa + 1,
		Destination:    "conta-eua",
	})
	if !errors.Is(err, sim.ErrInsufficient) {
		t.Fatalf("err = %v, want insufficient", err)
	}
	if !sameAccount(store.accounts[sim.CustomerMariana], before) {
		t.Fatalf("account changed: %+v", store.accounts[sim.CustomerMariana])
	}
	if len(store.outbox) != 0 || len(store.keys) != 0 {
		t.Fatalf("outbox=%d keys=%d, want 0", len(store.outbox), len(store.keys))
	}
}

func TestOutboxFailureRollsBackBalance(t *testing.T) {
	t.Parallel()
	store := newStore()
	store.fail = "outbox"
	before := store.accounts[sim.CustomerFernanda]
	_, err := sim.Apply(context.Background(), store, sim.Command{
		CustomerID:     sim.CustomerFernanda,
		IdempotencyKey: "dep-fail",
		Kind:           sim.CmdDeposit,
		Amount:         1_000_000,
		Origin:         "pix",
	})
	if err == nil {
		t.Fatal("Apply error = nil, want outbox failure")
	}
	if !sameAccount(store.accounts[sim.CustomerFernanda], before) {
		t.Fatalf("account changed: %+v", store.accounts[sim.CustomerFernanda])
	}
	if len(store.outbox) != 0 {
		t.Fatalf("outbox = %d, want 0", len(store.outbox))
	}
}

func TestIdempotencyReplayDoesNotAppend(t *testing.T) {
	t.Parallel()
	store := newStore()
	cmd := sim.Command{
		CustomerID:     sim.CustomerFernanda,
		IdempotencyKey: "dep-1",
		Kind:           sim.CmdDeposit,
		Amount:         1_000_000,
		Origin:         "pix",
	}
	first, err := sim.Apply(context.Background(), store, cmd)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := sim.Apply(context.Background(), store, cmd)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !second.Replay || second.EventID != first.EventID {
		t.Fatalf("second = %+v, first = %+v", second, first)
	}
	if len(store.outbox) != 1 {
		t.Fatalf("outbox = %d, want 1", len(store.outbox))
	}
	if store.accounts[sim.CustomerFernanda].Caixa != 114_800+1_000_000 {
		t.Fatalf("caixa = %d", store.accounts[sim.CustomerFernanda].Caixa)
	}
}

func TestPurchaseRefusalsFollowTheContractOrder(t *testing.T) {
	t.Parallel()

	thiago := sim.CustomerThiago // cash 6052000
	tests := []struct {
		name      string
		productID string
		amount    int64
		want      error
	}{
		{name: "unknown product", productID: "ouro", amount: 100_000, want: sim.ErrProduct},
		{name: "empty product", productID: "", amount: 100_000, want: sim.ErrProduct},
		{name: "unknown product before a bad amount", productID: "ouro", amount: 0, want: sim.ErrProduct},
		{name: "zero amount", productID: "acoesg", amount: 0, want: sim.ErrAmount},
		{name: "negative amount", productID: "acoesg", amount: -1, want: sim.ErrAmount},
		{name: "above the cap", productID: "acoesg", amount: sim.MaxAmountCents + 1, want: sim.ErrAmount},
		{name: "below the product minimum", productID: "corp", amount: 99_999, want: sim.ErrAmount},
		{name: "below the minimum before over cash", productID: "tbill", amount: 9_999, want: sim.ErrAmount},
		{name: "above cash", productID: "acoesg", amount: 6_052_001, want: sim.ErrInsufficient},
		{name: "above the cap before over cash", productID: "cobalto", amount: sim.MaxAmountCents + 1, want: sim.ErrAmount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newStore()
			before := store.accounts[thiago]
			_, err := sim.Apply(context.Background(), store, sim.Command{
				CustomerID:     thiago,
				IdempotencyKey: "buy-refused",
				Kind:           sim.CmdPurchase,
				ProductID:      tt.productID,
				Amount:         tt.amount,
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if !sameAccount(store.accounts[thiago], before) {
				t.Fatalf("account changed: %+v", store.accounts[thiago])
			}
			if len(store.outbox) != 0 || len(store.keys) != 0 {
				t.Fatalf("outbox=%d keys=%d, want 0", len(store.outbox), len(store.keys))
			}
		})
	}
}

func TestPurchaseMovesCashIntoAPositionAtV3(t *testing.T) {
	t.Parallel()
	store := newStore()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	result, err := sim.Apply(context.Background(), store, sim.Command{
		CustomerID:     sim.CustomerThiago,
		IdempotencyKey: "buy-acoesg",
		CommandID:      "cmd-1",
		Kind:           sim.CmdPurchase,
		ProductID:      "acoesg",
		Amount:         3_000_000,
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.EventID == "" || result.Replay {
		t.Fatalf("result = %+v", result)
	}
	account := store.accounts[sim.CustomerThiago]
	if account.Caixa != 3_052_000 || account.ETFs != 3_544_000 || account.Assets() != 6_800_000 {
		t.Fatalf("account = %+v assets %d, want cash 3052000, etfs 3544000, patrimony 6800000", account, account.Assets())
	}
	last := account.Positions[len(account.Positions)-1]
	if last.ProductID != "acoesg" || last.UnitsCents != 3_000_000 || last.AppliedCents != 3_000_000 {
		t.Fatalf("position delta = %+v", last)
	}
	if len(store.outbox) != 1 || store.outbox[0].RoutingKey != "account.event.recorded" {
		t.Fatalf("outbox = %+v", store.outbox)
	}
	env, payload, err := sim.DecodeOutboxAccount(store.outbox[0].Payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := sim.AccountPayload{
		Kind: sim.KindAplicacao, Amount: 3_000_000, Before: 6_800_000, After: 6_800_000,
		ProductID: "acoesg", AssetClass: sim.ClassETFs, Risk: 3,
	}
	if env.SchemaVersion != 3 || payload != want || !env.OccurredAt.Equal(now) {
		t.Fatalf("event = %+v payload = %+v, want v3 %+v", env, payload, want)
	}
}

func TestPurchasePositionFailureRollsBack(t *testing.T) {
	t.Parallel()
	store := newStore()
	store.fail = "position"
	before := store.accounts[sim.CustomerThiago]
	_, err := sim.Apply(context.Background(), store, sim.Command{
		CustomerID:     sim.CustomerThiago,
		IdempotencyKey: "buy-fail",
		Kind:           sim.CmdPurchase,
		ProductID:      "acoesg",
		Amount:         100_000,
	})
	if err == nil {
		t.Fatal("Apply error = nil, want position failure")
	}
	if !sameAccount(store.accounts[sim.CustomerThiago], before) || len(store.outbox) != 0 || len(store.keys) != 0 {
		t.Fatalf("partial write: account %+v outbox %d keys %d", store.accounts[sim.CustomerThiago], len(store.outbox), len(store.keys))
	}
}

func TestComplaintDoesNotMoveCash(t *testing.T) {
	t.Parallel()
	store := newStore()
	before := store.accounts[sim.CustomerMariana]
	_, err := sim.Apply(context.Background(), store, sim.Command{
		CustomerID:     sim.CustomerMariana,
		IdempotencyKey: "cmp-1",
		Kind:           sim.CmdComplaint,
		Text:           "Estou pensando em sair",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !sameAccount(store.accounts[sim.CustomerMariana], before) {
		t.Fatalf("account changed")
	}
	if store.outbox[0].RoutingKey != "message.received" {
		t.Fatalf("routing = %s", store.outbox[0].RoutingKey)
	}
}

func TestReseedRestoresCaixa(t *testing.T) {
	t.Parallel()
	store := newStore()
	_, err := sim.Apply(context.Background(), store, sim.Command{
		CustomerID:     sim.CustomerMariana,
		IdempotencyKey: "wd-reseed",
		Kind:           sim.CmdWithdrawal,
		Amount:         1_000_000,
		Destination:    "conta-eua",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := sim.Reseed(context.Background(), store); err != nil {
		t.Fatalf("Reseed: %v", err)
	}
	account := store.accounts[sim.CustomerMariana]
	if account.Caixa != 6_000_000 || account.Assets() != 24_830_000 {
		t.Fatalf("after reseed = %+v assets %d", account, account.Assets())
	}
}
