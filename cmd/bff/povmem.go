package main

import (
	"context"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// memoryPOV serves the three seeded POV accounts inside the BFF process.
// pub is the stand-in for the account-sim outbox publisher. The HTTP handler
// still does not publish; this store drains its own outbox after Apply.
type memoryPOV struct {
	store *sim.Memory
	pub   interface {
		Publish(ctx context.Context, routingKey string, body []byte) error
	}
}

func newMemoryPOV() memoryPOV {
	return memoryPOV{store: sim.NewMemory()}
}

func (m memoryPOV) List(ctx context.Context) ([]bff.POVAccount, error) {
	ids := []string{sim.CustomerMariana, sim.CustomerFernanda, sim.CustomerThiago}
	out := make([]bff.POVAccount, 0, len(ids))
	err := m.store.WithTx(ctx, func(tx sim.Tx) error {
		for _, id := range ids {
			account, ok, err := tx.GetAccount(ctx, id)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			out = append(out, toPOV(account))
		}
		return nil
	})
	return out, err
}

func (m memoryPOV) Get(ctx context.Context, customerID string) (bff.POVAccount, error) {
	var found bff.POVAccount
	err := m.store.WithTx(ctx, func(tx sim.Tx) error {
		account, ok, err := tx.GetAccount(ctx, customerID)
		if err != nil {
			return err
		}
		if !ok {
			return sim.ErrUnknownCustomer
		}
		found = toPOV(account)
		return nil
	})
	return found, err
}

func (m memoryPOV) Apply(ctx context.Context, cmd bff.POVCommand) (bff.POVResult, error) {
	result, err := sim.Apply(ctx, m.store, sim.Command{
		CustomerID:     cmd.CustomerID,
		IdempotencyKey: cmd.IdempotencyKey,
		Kind:           cmd.Kind,
		Amount:         cmd.Amount,
		Origin:         cmd.Origin,
		Destination:    cmd.Destination,
		Channel:        cmd.Channel,
		Text:           cmd.Text,
	})
	if err != nil {
		return bff.POVResult{}, err
	}
	if !result.Replay {
		_ = m.flush(ctx)
	}
	return bff.POVResult{EventID: result.EventID, Replay: result.Replay}, nil
}

func (m memoryPOV) flush(ctx context.Context) error {
	if m.pub == nil {
		return nil
	}
	for _, row := range m.store.PendingOutbox() {
		if err := m.pub.Publish(ctx, row.RoutingKey, row.Payload); err != nil {
			return err
		}
		m.store.MarkPublished(row.EventID)
	}
	return nil
}

func toPOV(account sim.Account) bff.POVAccount {
	return bff.POVAccount{
		CustomerID: account.CustomerID,
		Acoes:      account.Acoes,
		ETFs:       account.ETFs,
		RendaFixa:  account.RendaFixa,
		Caixa:      account.Caixa,
	}
}
