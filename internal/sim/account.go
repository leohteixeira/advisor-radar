package sim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/outbox"
)

// POV customer ids match the advisory book (ADR 0007).
const (
	CustomerMariana  = "01a0e3a4-9a44-7566-b5de-eb2e365799f8"
	CustomerFernanda = "01a0e3a4-9a44-757a-ac8f-dab7db5eb068"
	CustomerThiago   = "01a0e3a4-9a44-75dd-b3a0-403a7a87836e"
)

const (
	CmdDeposit    = "deposit"
	CmdWithdrawal = "withdrawal"
	CmdMessage    = "message"
	CmdComplaint  = "complaint"
)

// MaxAmountCents caps one deposit or withdrawal at USD 1 billion, which keeps
// balances in int64 cents far from overflow. A larger amount is ErrAmount.
const MaxAmountCents int64 = 100_000_000_000

var (
	ErrUnknownCustomer = errors.New("sim: unknown customer")
	ErrInsufficient    = errors.New("sim: withdrawal exceeds caixa")
	ErrAmount          = errors.New("sim: amount must be between 1 and 100000000000 cents")
	ErrKey             = errors.New("sim: idempotency key is required")
	ErrCommand         = errors.New("sim: command is not accepted")
)

// Account is one POV balance. Amounts are integer USD cents.
type Account struct {
	CustomerID string
	Acoes      int64
	ETFs       int64
	RendaFixa  int64
	Caixa      int64
}

// Assets is the sum of the four classes.
func (a Account) Assets() int64 {
	return a.Acoes + a.ETFs + a.RendaFixa + a.Caixa
}

// POVSeed is the three demo accounts.
func POVSeed() []Account {
	return []Account{
		{CustomerID: CustomerMariana, Acoes: 9_090_000, ETFs: 6_060_000, RendaFixa: 3_680_000, Caixa: 6_000_000},
		{CustomerID: CustomerFernanda, Acoes: 164_000, ETFs: 369_000, RendaFixa: 172_200, Caixa: 114_800},
		{CustomerID: CustomerThiago, Acoes: 204_000, ETFs: 544_000, RendaFixa: 0, Caixa: 6_052_000},
	}
}

// Command is one client action applied by account-sim.
type Command struct {
	CustomerID     string
	IdempotencyKey string
	Kind           string
	Amount         int64
	Origin         string
	Destination    string
	Channel        string
	Text           string
	Now            time.Time
}

// Result is the outbox event written for an accepted command.
type Result struct {
	EventID string
	Replay  bool
}

// Tx is one account-sim transaction. A returned error rolls the whole tx back.
type Tx interface {
	GetAccount(ctx context.Context, customerID string) (Account, bool, error)
	PutAccount(ctx context.Context, account Account) error
	LookupKey(ctx context.Context, customerID, key string) (eventID string, ok bool, err error)
	SaveKey(ctx context.Context, customerID, key, eventID string) error
	InsertOutbox(ctx context.Context, row outbox.Row) error
	ResetPOV(ctx context.Context, accounts []Account) error
}

// Store runs one function inside a transaction.
type Store interface {
	WithTx(ctx context.Context, fn func(Tx) error) error
}

// Apply writes the new account state and the outbox event in one transaction.
// A refusal returns an error and leaves no event. A repeated key returns the
// original event id and does not append a second row.
func Apply(ctx context.Context, store Store, cmd Command) (Result, error) {
	if store == nil {
		return Result{}, fmt.Errorf("sim: store is required")
	}
	if cmd.IdempotencyKey == "" {
		return Result{}, ErrKey
	}
	if _, err := identity.ParseV7(cmd.CustomerID); err != nil {
		return Result{}, fmt.Errorf("sim: customer: %w", err)
	}

	var result Result
	err := store.WithTx(ctx, func(tx Tx) error {
		existing, ok, err := tx.LookupKey(ctx, cmd.CustomerID, cmd.IdempotencyKey)
		if err != nil {
			return err
		}
		if ok {
			result = Result{EventID: existing, Replay: true}
			return nil
		}

		account, found, err := tx.GetAccount(ctx, cmd.CustomerID)
		if err != nil {
			return err
		}
		if !found {
			return ErrUnknownCustomer
		}

		body, routing, next, err := build(account, cmd)
		if err != nil {
			return err
		}
		eventID, err := identity.NewV7()
		if err != nil {
			return err
		}
		occurred := cmd.Now
		if occurred.IsZero() {
			occurred = time.Now().UTC()
		}
		env := event.Envelope{
			Name:          routing,
			EventID:       eventID,
			OccurredAt:    occurred.UTC(),
			CustomerID:    cmd.CustomerID,
			SchemaVersion: schemaVersion(routing),
			Payload:       body,
		}
		raw, err := env.MarshalBody()
		if err != nil {
			return fmt.Errorf("sim: marshal event: %w", err)
		}
		if routing == event.NameAccountEventRecorded {
			if err := tx.PutAccount(ctx, next); err != nil {
				return err
			}
		}
		if err := tx.InsertOutbox(ctx, outbox.Row{
			EventID:    eventID,
			RoutingKey: routing,
			Payload:    raw,
		}); err != nil {
			return err
		}
		if err := tx.SaveKey(ctx, cmd.CustomerID, cmd.IdempotencyKey, eventID); err != nil {
			return err
		}
		result = Result{EventID: eventID}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// Reseed restores the three POV accounts to the starting balances.
func Reseed(ctx context.Context, store Store) error {
	if store == nil {
		return fmt.Errorf("sim: store is required")
	}
	return store.WithTx(ctx, func(tx Tx) error {
		return tx.ResetPOV(ctx, POVSeed())
	})
}

func schemaVersion(routing string) int {
	if routing == event.NameAccountEventRecorded {
		return event.SchemaVersionCents
	}
	return event.SchemaVersionMVP
}

func build(account Account, cmd Command) (any, string, Account, error) {
	switch cmd.Kind {
	case CmdDeposit:
		if cmd.Amount <= 0 || cmd.Amount > MaxAmountCents {
			return nil, "", Account{}, ErrAmount
		}
		if cmd.Origin == "" {
			return nil, "", Account{}, fmt.Errorf("%w: origin is required", ErrCommand)
		}
		before := account.Assets()
		next := account
		next.Caixa += cmd.Amount
		return AccountPayload{
			Kind:   "aporte",
			Amount: float64(cmd.Amount),
			Before: float64(before),
			After:  float64(next.Assets()),
			Origin: cmd.Origin,
		}, event.NameAccountEventRecorded, next, nil
	case CmdWithdrawal:
		if cmd.Amount <= 0 || cmd.Amount > MaxAmountCents {
			return nil, "", Account{}, ErrAmount
		}
		if cmd.Destination == "" {
			return nil, "", Account{}, fmt.Errorf("%w: destination is required", ErrCommand)
		}
		if cmd.Amount > account.Caixa {
			return nil, "", Account{}, ErrInsufficient
		}
		before := account.Assets()
		next := account
		next.Caixa -= cmd.Amount
		return AccountPayload{
			Kind:        "saque",
			Amount:      float64(cmd.Amount),
			Before:      float64(before),
			After:       float64(next.Assets()),
			Destination: cmd.Destination,
		}, event.NameAccountEventRecorded, next, nil
	case CmdMessage:
		if cmd.Text == "" || (cmd.Channel != "chat" && cmd.Channel != "e-mail") {
			return nil, "", Account{}, fmt.Errorf("%w: message needs chat or e-mail and text", ErrCommand)
		}
		return MessagePayload{Channel: cmd.Channel, Text: cmd.Text}, event.NameMessageReceived, account, nil
	case CmdComplaint:
		if cmd.Text == "" {
			return nil, "", Account{}, fmt.Errorf("%w: complaint needs text", ErrCommand)
		}
		return MessagePayload{Channel: "chat", Text: cmd.Text}, event.NameMessageReceived, account, nil
	default:
		return nil, "", Account{}, ErrCommand
	}
}

// DecodeOutboxAccount reads a schema version 2 account event body.
func DecodeOutboxAccount(raw []byte) (event.Envelope, AccountPayload, error) {
	var env event.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return event.Envelope{}, AccountPayload{}, fmt.Errorf("sim: decode envelope: %w", err)
	}
	payload, err := json.Marshal(env.Payload)
	if err != nil {
		return event.Envelope{}, AccountPayload{}, fmt.Errorf("sim: encode payload: %w", err)
	}
	var body AccountPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return event.Envelope{}, AccountPayload{}, fmt.Errorf("sim: decode account payload: %w", err)
	}
	return env, body, nil
}
