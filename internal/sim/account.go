package sim

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
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
	CmdPurchase   = "purchase"
)

// KindAplicacao is the account.event.recorded kind of a purchase: cash moved
// into a product position (schema version 3).
const KindAplicacao = "aplicacao"

// MaxAmountCents caps one deposit, withdrawal, or purchase at USD 1 billion,
// which keeps balances in int64 cents far from overflow. A larger amount is
// ErrAmount.
const MaxAmountCents int64 = 100_000_000_000

var (
	ErrUnknownCustomer = errors.New("sim: unknown customer")
	ErrInsufficient    = errors.New("sim: amount exceeds caixa")
	ErrAmount          = errors.New("sim: amount must be between 1 and 100000000000 cents")
	ErrKey             = errors.New("sim: idempotency key is required")
	ErrCommand         = errors.New("sim: command is not accepted")
	ErrProduct         = errors.New("sim: unknown product")
)

// Asset classes of the fictional catalog. Cash is not a product; it is the
// account's own balance.
const (
	ClassAcoes     = "acoes"
	ClassETFs      = "etfs"
	ClassRendaFixa = "renda_fixa"
)

// Account is one POV account in integer USD cents (ADR 0010). Acoes, ETFs,
// and RendaFixa are aggregates: the sum of the market values of the positions
// of that class at the current simulated day. Caixa is the cash balance, the
// only stored amount on the account itself. Positions are the positions the
// aggregates were summed from, valued at the same day.
type Account struct {
	CustomerID string
	Acoes      int64
	ETFs       int64
	RendaFixa  int64
	Caixa      int64
	Positions  []Position
}

// Assets is the patrimony: positions at market value plus cash.
func (a Account) Assets() int64 {
	return a.Acoes + a.ETFs + a.RendaFixa + a.Caixa
}

// Product is one entry of the fictional catalog. ReturnLabel is Portuguese
// display text; MinimumCents is the smallest purchase in USD cents.
type Product struct {
	ID           string
	Name         string
	AssetClass   string
	Risk         int
	ReturnLabel  string
	MinimumCents int64
}

// Position is a customer's holding of one product. UnitsCents is the holding
// expressed in day-0 cents; AppliedCents is the cash spent on it. ValueCents
// is Value(ProductID, UnitsCents, day) for the day it was read at.
type Position struct {
	ProductID    string
	AssetClass   string
	UnitsCents   int64
	AppliedCents int64
	ValueCents   int64
}

// Registration is the fictional registration data of one POV customer.
// Phone is already masked display text. Client-since lives in the advisory
// book, not here.
type Registration struct {
	CustomerID    string
	Email         string
	Phone         string
	City          string
	AccountNumber string
}

// SeedAccount is one POV account as seeded on day 0.
type SeedAccount struct {
	CustomerID   string
	CashCents    int64
	Positions    []Position
	Registration Registration
}

// Seed is everything Reseed restores: the catalog and the three POV accounts.
type Seed struct {
	Products []Product
	Accounts []SeedAccount
}

// Catalog is the fictional product catalog (architecture.md "Products"),
// ordered by risk and then id. seeds/account_sim holds the same rows.
func Catalog() []Product {
	return []Product{
		{ID: "tbill", Name: "Orla T-Bill 6 meses", AssetClass: ClassRendaFixa, Risk: 1, ReturnLabel: "4,9% a.a.", MinimumCents: 10_000},
		{ID: "corp", Name: "Orla Corporate IG 2029", AssetClass: ClassRendaFixa, Risk: 2, ReturnLabel: "5,6% a.a.", MinimumCents: 100_000},
		{ID: "renda", Name: "Maré Renda Global ETF", AssetClass: ClassETFs, Risk: 2, ReturnLabel: "+3,8% em 12 meses", MinimumCents: 5_000},
		{ID: "acoesg", Name: "Maré Ações Globais ETF", AssetClass: ClassETFs, Risk: 3, ReturnLabel: "+11,2% em 12 meses", MinimumCents: 5_000},
		{ID: "farol", Name: "Farol Saúde", AssetClass: ClassAcoes, Risk: 4, ReturnLabel: "+9,4% em 12 meses", MinimumCents: 1_000},
		{ID: "cobalto", Name: "Cobalto Semicondutores", AssetClass: ClassAcoes, Risk: 5, ReturnLabel: "+27,1% em 12 meses", MinimumCents: 1_000},
	}
}

// DemoSeed is the day-0 state of the three POV accounts: cash, per-product
// positions, and registration. Each class aggregate equals the phase-2 seed.
// seeds/account_sim holds the same rows.
func DemoSeed() Seed {
	products := Catalog()
	classOf := make(map[string]string, len(products))
	for _, product := range products {
		classOf[product.ID] = product.AssetClass
	}
	pos := func(productID string, appliedCents, unitsCents int64) Position {
		class, ok := classOf[productID]
		if !ok {
			panic(fmt.Sprintf("sim: seed position names unknown product %q", productID))
		}
		return Position{
			ProductID:    productID,
			AssetClass:   class,
			UnitsCents:   unitsCents,
			AppliedCents: appliedCents,
			ValueCents:   Value(productID, unitsCents, SeedDay),
		}
	}
	return Seed{
		Products: products,
		Accounts: []SeedAccount{
			{
				CustomerID: CustomerMariana,
				CashCents:  6_000_000,
				Positions: []Position{
					pos("cobalto", 6_000_000, 7_200_000),
					pos("farol", 1_750_000, 1_890_000),
					pos("acoesg", 3_600_000, 4_060_000),
					pos("renda", 1_940_000, 2_000_000),
					pos("corp", 3_600_000, 3_680_000),
				},
				Registration: Registration{
					CustomerID:    CustomerMariana,
					Email:         "mariana.costa@example.com",
					Phone:         "+55 (11) •••••-7810",
					City:          "São Paulo, SP · Brasil",
					AccountNumber: "Conta 1190-4 · Orla Invest",
				},
			},
			{
				CustomerID: CustomerFernanda,
				CashCents:  114_800,
				Positions: []Position{
					pos("farol", 159_000, 164_000),
					pos("renda", 166_000, 169_000),
					pos("acoesg", 190_000, 200_000),
					pos("tbill", 169_000, 172_200),
				},
				Registration: Registration{
					CustomerID:    CustomerFernanda,
					Email:         "fernanda.lima@example.com",
					Phone:         "+55 (19) •••••-4471",
					City:          "Campinas, SP · Brasil",
					AccountNumber: "Conta 3301-7 · Orla Invest",
				},
			},
			{
				CustomerID: CustomerThiago,
				CashCents:  6_052_000,
				Positions: []Position{
					pos("cobalto", 190_000, 204_000),
					pos("acoesg", 520_000, 544_000),
				},
				Registration: Registration{
					CustomerID:    CustomerThiago,
					Email:         "thiago.azevedo@example.com",
					Phone:         "+55 (48) •••••-2093",
					City:          "Florianópolis, SC · Brasil",
					AccountNumber: "Conta 2847-1 · Orla Invest",
				},
			},
		},
	}
}

// POVSeed is the three demo accounts on day 0, as class aggregates of
// DemoSeed with their positions valued at SeedDay.
func POVSeed() []Account {
	seed := DemoSeed()
	accounts := make([]Account, 0, len(seed.Accounts))
	for _, account := range seed.Accounts {
		valued, err := aggregate(account.CustomerID, account.CashCents, valuePositions(account.Positions, SeedDay))
		if err != nil {
			// DemoSeed takes every class from Catalog, so this is a programming error.
			panic(err)
		}
		accounts = append(accounts, valued)
	}
	return accounts
}

// aggregate sums position values per class, adds cash as Caixa, and keeps
// positions on the account. A position of an unknown class is an error, so it
// can never drop out of the totals and the patrimony silently.
func aggregate(customerID string, cashCents int64, positions []Position) (Account, error) {
	account := Account{CustomerID: customerID, Caixa: cashCents, Positions: positions}
	for _, position := range positions {
		switch position.AssetClass {
		case ClassAcoes:
			account.Acoes += position.ValueCents
		case ClassETFs:
			account.ETFs += position.ValueCents
		case ClassRendaFixa:
			account.RendaFixa += position.ValueCents
		default:
			return Account{}, fmt.Errorf("sim: position %s of %s has unknown asset class %q",
				position.ProductID, customerID, position.AssetClass)
		}
	}
	return account, nil
}

// valuePositions returns a copy of positions valued at day, ordered by class
// (acoes, etfs, renda_fixa) and then product id, so every store answers in
// the same order.
func valuePositions(positions []Position, day int) []Position {
	out := make([]Position, len(positions))
	for i, position := range positions {
		position.ValueCents = Value(position.ProductID, position.UnitsCents, day)
		out[i] = position
	}
	slices.SortFunc(out, func(a, b Position) int {
		if c := cmp.Compare(classRank(a.AssetClass), classRank(b.AssetClass)); c != 0 {
			return c
		}
		return strings.Compare(a.ProductID, b.ProductID)
	})
	return out
}

func classRank(class string) int {
	switch class {
	case ClassAcoes:
		return 0
	case ClassETFs:
		return 1
	case ClassRendaFixa:
		return 2
	default:
		return 3
	}
}

// Command is one client action applied by account-sim. ProductID is set
// only for a purchase. CommandID is the caller's correlation id; it never
// takes part in idempotency, which the key alone decides.
type Command struct {
	CustomerID     string
	IdempotencyKey string
	CommandID      string
	Kind           string
	Amount         int64
	ProductID      string
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
//
// GetAccount returns cash, the positions valued at the current day, and their
// class aggregates. PutAccount writes only the cash (Caixa); positions are
// untouched. AddPosition adds delta's UnitsCents and AppliedCents to the
// customer's position in delta.ProductID, creating it when absent. ResetPOV
// restores the catalog, cash, positions, and registration of seed.
type Tx interface {
	GetAccount(ctx context.Context, customerID string) (Account, bool, error)
	PutAccount(ctx context.Context, account Account) error
	AddPosition(ctx context.Context, customerID string, delta Position) error
	ListProducts(ctx context.Context) ([]Product, error)
	GetRegistration(ctx context.Context, customerID string) (Registration, bool, error)
	LookupKey(ctx context.Context, customerID, key string) (eventID string, ok bool, err error)
	SaveKey(ctx context.Context, customerID, key, eventID string) error
	InsertOutbox(ctx context.Context, row outbox.Row) error
	ResetPOV(ctx context.Context, seed Seed) error
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
	var committed outbox.Row // the event row of a new command, for metrics
	err := store.WithTx(ctx, func(tx Tx) error {
		committed = outbox.Row{}
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

		var products []Product
		if cmd.Kind == CmdPurchase {
			if products, err = tx.ListProducts(ctx); err != nil {
				return err
			}
		}
		plan, err := build(account, products, cmd)
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
			Name:          plan.routing,
			EventID:       eventID,
			OccurredAt:    occurred.UTC(),
			CustomerID:    cmd.CustomerID,
			SchemaVersion: plan.schemaVersion,
			Payload:       plan.body,
		}
		raw, err := env.MarshalBody()
		if err != nil {
			return fmt.Errorf("sim: marshal event: %w", err)
		}
		if plan.routing == event.NameAccountEventRecorded {
			if err := tx.PutAccount(ctx, plan.next); err != nil {
				return err
			}
		}
		if plan.position != nil {
			if err := tx.AddPosition(ctx, cmd.CustomerID, *plan.position); err != nil {
				return err
			}
		}
		row := outbox.Row{
			EventID:    eventID,
			RoutingKey: plan.routing,
			Payload:    raw,
		}
		if err := tx.InsertOutbox(ctx, row); err != nil {
			return err
		}
		if err := tx.SaveKey(ctx, cmd.CustomerID, cmd.IdempotencyKey, eventID); err != nil {
			return err
		}
		result = Result{EventID: eventID}
		committed = row
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if !result.Replay {
		countPurchase(ctx, committed.RoutingKey, committed.Payload)
	}
	return result, nil
}

// Reseed restores the catalog and the three POV accounts (cash, positions,
// and registration) to DemoSeed.
func Reseed(ctx context.Context, store Store) error {
	if store == nil {
		return fmt.Errorf("sim: store is required")
	}
	return store.WithTx(ctx, func(tx Tx) error {
		return tx.ResetPOV(ctx, DemoSeed())
	})
}

// plan is what one accepted command writes: the event body and routing key
// with its schema version, the account whose cash PutAccount stores, and the
// position delta of a purchase.
type plan struct {
	body          any
	routing       string
	schemaVersion int
	next          Account
	position      *Position
}

func build(account Account, products []Product, cmd Command) (plan, error) {
	switch cmd.Kind {
	case CmdDeposit:
		if err := checkAmount(cmd.Amount); err != nil {
			return plan{}, err
		}
		if cmd.Origin == "" {
			return plan{}, fmt.Errorf("%w: origin is required", ErrCommand)
		}
		before := account.Assets()
		next := account
		next.Caixa += cmd.Amount
		return accountPlan(AccountPayload{
			Kind:   "aporte",
			Amount: float64(cmd.Amount),
			Before: float64(before),
			After:  float64(next.Assets()),
			Origin: cmd.Origin,
		}, event.SchemaVersionCents, next), nil
	case CmdWithdrawal:
		if err := checkAmount(cmd.Amount); err != nil {
			return plan{}, err
		}
		if cmd.Destination == "" {
			return plan{}, fmt.Errorf("%w: destination is required", ErrCommand)
		}
		if cmd.Amount > account.Caixa {
			return plan{}, ErrInsufficient
		}
		before := account.Assets()
		next := account
		next.Caixa -= cmd.Amount
		return accountPlan(AccountPayload{
			Kind:        "saque",
			Amount:      float64(cmd.Amount),
			Before:      float64(before),
			After:       float64(next.Assets()),
			Destination: cmd.Destination,
		}, event.SchemaVersionCents, next), nil
	case CmdPurchase:
		return buildPurchase(account, products, cmd)
	case CmdMessage:
		if cmd.Text == "" || (cmd.Channel != "chat" && cmd.Channel != "e-mail") {
			return plan{}, fmt.Errorf("%w: message needs chat or e-mail and text", ErrCommand)
		}
		return messagePlan(MessagePayload{Channel: cmd.Channel, Text: cmd.Text, Origin: OriginClientApp}, account), nil
	case CmdComplaint:
		if cmd.Text == "" {
			return plan{}, fmt.Errorf("%w: complaint needs text", ErrCommand)
		}
		return messagePlan(MessagePayload{Channel: "chat", Text: cmd.Text, Origin: OriginClientApp}, account), nil
	default:
		return plan{}, ErrCommand
	}
}

// buildPurchase validates a purchase in the contract order: an unknown
// product, then the amount (1 to MaxAmountCents and at least the product
// minimum), then the cash. It moves the amount from cash into units of the
// product at the current day's price, so patrimony is unchanged: before and
// after are both the patrimony before the purchase.
func buildPurchase(account Account, products []Product, cmd Command) (plan, error) {
	i := slices.IndexFunc(products, func(p Product) bool { return p.ID == cmd.ProductID })
	if cmd.ProductID == "" || i < 0 {
		return plan{}, ErrProduct
	}
	product := products[i]
	if err := checkAmount(cmd.Amount); err != nil {
		return plan{}, err
	}
	if cmd.Amount < product.MinimumCents {
		return plan{}, fmt.Errorf("%w: below the product minimum of %d cents", ErrAmount, product.MinimumCents)
	}
	if cmd.Amount > account.Caixa {
		return plan{}, ErrInsufficient
	}
	patrimony := account.Assets()
	next := account
	next.Caixa -= cmd.Amount
	p := accountPlan(AccountPayload{
		Kind:       KindAplicacao,
		Amount:     float64(cmd.Amount),
		Before:     float64(patrimony),
		After:      float64(patrimony),
		ProductID:  product.ID,
		AssetClass: product.AssetClass,
		Risk:       product.Risk,
	}, event.SchemaVersionPositions, next)
	p.position = &Position{
		ProductID:    product.ID,
		AssetClass:   product.AssetClass,
		UnitsCents:   Units(product.ID, cmd.Amount, SeedDay),
		AppliedCents: cmd.Amount,
	}
	return p, nil
}

func checkAmount(cents int64) error {
	if cents <= 0 || cents > MaxAmountCents {
		return ErrAmount
	}
	return nil
}

func accountPlan(body AccountPayload, schemaVersion int, next Account) plan {
	return plan{body: body, routing: event.NameAccountEventRecorded, schemaVersion: schemaVersion, next: next}
}

func messagePlan(body MessagePayload, account Account) plan {
	return plan{body: body, routing: event.NameMessageReceived, schemaVersion: event.SchemaVersionMVP, next: account}
}

// DecodeOutboxAccount reads a schema version 2 or 3 account event body.
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
