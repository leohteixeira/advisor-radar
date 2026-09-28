package sim_test

import (
	"context"
	"errors"
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

func (t *memTx) ResetPOV(ctx context.Context, accounts []sim.Account) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, account := range accounts {
		t.store.accounts[account.CustomerID] = account
	}
	return nil
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
	if store.accounts[sim.CustomerMariana] != before {
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
	if store.accounts[sim.CustomerFernanda] != before {
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
	if store.accounts[sim.CustomerMariana] != before {
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
