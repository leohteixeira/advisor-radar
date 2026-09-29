package sim

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/outbox"
)

// ErrEpoch marks a stored simulation epoch that is not a UUID, so no
// revaluation event id can be derived from it.
var ErrEpoch = errors.New("sim: simulation epoch is not a uuid")

// SimState is the global simulated market. Day starts at 0 and advances only
// by AdvanceDay. Epoch is a UUID that every reset replaces; it namespaces the
// revaluation event ids, so a day replayed after a reseed publishes new
// events instead of colliding with the ones consumers already claimed.
type SimState struct {
	Day   int
	Epoch string
}

// AdvanceCommand advances the simulated day by one. CommandID is the
// caller's correlation id; idempotency is decided by the key alone. Now is
// the event time, or the wall clock when zero; it never enters pricing.
type AdvanceCommand struct {
	IdempotencyKey string
	CommandID      string
	Now            time.Time
}

// AdvanceResult is the reply of an advance: the new day and the reavaliacao
// event ids, one per POV account in byte-wise customer id order. Replay is
// set when the key was already used; nothing advanced or was published then.
type AdvanceResult struct {
	SimDay   int
	EventIDs []string
	Replay   bool
}

// AdvanceDay moves the global simulated day forward by one and writes one
// account.event.recorded kind reavaliacao outbox row per POV account, all in
// one transaction. The simulation lock is taken before any customer lock, so
// an advance serializes with deposits, withdrawals, and purchases without
// deadlocking: a command that ran first is revalued with its cash, and one
// that runs after values its positions at the new day. Positions are never
// rewritten; values follow from the stored day at read time.
//
// A known key returns the original reply with Replay set.
func AdvanceDay(ctx context.Context, store Store, cmd AdvanceCommand) (AdvanceResult, error) {
	if store == nil {
		return AdvanceResult{}, errors.New("sim: store is required")
	}
	if cmd.IdempotencyKey == "" {
		return AdvanceResult{}, ErrKey
	}
	occurred := cmd.Now
	if occurred.IsZero() {
		occurred = time.Now()
	}
	var result AdvanceResult
	err := store.WithTx(ctx, func(tx Tx) error {
		state, err := tx.LockSimulation(ctx)
		if err != nil {
			return err
		}
		previous, ok, err := tx.LookupAdvance(ctx, cmd.IdempotencyKey)
		if err != nil {
			return err
		}
		if ok {
			previous.Replay = true
			result = previous
			return nil
		}
		next := state.Day + 1
		ids, err := tx.CustomerIDs(ctx)
		if err != nil {
			return err
		}
		eventIDs := make([]string, 0, len(ids))
		for _, customerID := range ids {
			account, found, err := tx.GetAccount(ctx, customerID)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("sim: advance day: %s: %w", customerID, ErrUnknownCustomer)
			}
			if account.SimDay != state.Day {
				return fmt.Errorf("sim: advance day: %s read at day %d, want %d", customerID, account.SimDay, state.Day)
			}
			row, err := revaluationRow(state.Epoch, account, occurred.UTC())
			if err != nil {
				return err
			}
			if err := tx.InsertOutbox(ctx, row); err != nil {
				return err
			}
			eventIDs = append(eventIDs, row.EventID)
		}
		if err := tx.SetSimDay(ctx, next); err != nil {
			return err
		}
		result = AdvanceResult{SimDay: next, EventIDs: eventIDs}
		return tx.SaveAdvance(ctx, cmd.IdempotencyKey, result)
	})
	if err != nil {
		return AdvanceResult{}, err
	}
	return result, nil
}

// revaluationRow is the outbox row of one account's revaluation from the
// day it was read at to the next one.
func revaluationRow(epoch string, account Account, occurred time.Time) (outbox.Row, error) {
	payload := Revaluation(account)
	payload.Epoch = epoch
	eventID, err := RevaluationEventID(epoch, account.CustomerID, payload.SimDay)
	if err != nil {
		return outbox.Row{}, err
	}
	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       eventID,
		OccurredAt:    occurred,
		CustomerID:    account.CustomerID,
		SchemaVersion: event.SchemaVersionPositions,
		Payload:       payload,
	}
	raw, err := env.MarshalBody()
	if err != nil {
		return outbox.Row{}, fmt.Errorf("sim: marshal revaluation: %w", err)
	}
	return outbox.Row{EventID: eventID, RoutingKey: event.NameAccountEventRecorded, Payload: raw}, nil
}

// Revaluation is the reavaliacao payload of account, read at account.SimDay,
// when the day advances to the next one. Before and After are the patrimony
// on both days and Amount their signed difference, in integer USD cents;
// cash does not move. ProductID is the position whose value changed most in
// absolute terms, the first in position order on a tie, with its price
// change in basis points; it is empty, with 0 basis points, when nothing
// changed.
func Revaluation(account Account) AccountPayload {
	day, next := account.SimDay, account.SimDay+1
	before, after := account.Caixa, account.Caixa
	var (
		moved   string
		largest int64
	)
	for _, position := range account.Positions {
		was := Value(position.ProductID, position.UnitsCents, day)
		now := Value(position.ProductID, position.UnitsCents, next)
		before += was
		after += now
		if change := absCents(now - was); change > largest {
			moved, largest = position.ProductID, change
		}
	}
	payload := AccountPayload{
		Kind:      KindReavaliacao,
		Amount:    float64(after - before),
		Before:    float64(before),
		After:     float64(after),
		SimDay:    next,
		ProductID: moved,
	}
	if moved != "" {
		payload.ProductChangeBP = DayChangeBP(moved, next)
	}
	return payload
}

func absCents(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// RevaluationEventID is the name-based (SHA-1, version 5) UUID of
// "reavaliacao:{customer_id}:{sim_day}" in the namespace of the simulation
// epoch. Within one epoch a customer's revaluation for a day has exactly one
// id; a reseed starts a new epoch, so its days get new ids.
func RevaluationEventID(epoch, customerID string, day int) (string, error) {
	namespace, err := uuid.Parse(epoch)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrEpoch, err)
	}
	name := KindReavaliacao + ":" + customerID + ":" + strconv.Itoa(day)
	return uuid.NewSHA1(namespace, []byte(name)).String(), nil
}
