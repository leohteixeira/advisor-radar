package sim_test

import (
	"context"
	"slices"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// classCents is one account's phase-2 class totals and patrimony in cents.
type classCents struct {
	acoes, etfs, rendaFixa, caixa, patrimony int64
}

// phase2Totals are the phase-2 seed totals the positions seed must keep.
var phase2Totals = map[string]classCents{
	sim.CustomerFernanda: {acoes: 164_000, etfs: 369_000, rendaFixa: 172_200, caixa: 114_800, patrimony: 820_000},
	sim.CustomerThiago:   {acoes: 204_000, etfs: 544_000, rendaFixa: 0, caixa: 6_052_000, patrimony: 6_800_000},
	sim.CustomerMariana:  {acoes: 9_090_000, etfs: 6_060_000, rendaFixa: 3_680_000, caixa: 6_000_000, patrimony: 24_830_000},
}

func totalsOf(a *accountv1.Account) classCents {
	return classCents{
		acoes:     a.GetAcoesCents(),
		etfs:      a.GetEtfsCents(),
		rendaFixa: a.GetRendaFixaCents(),
		caixa:     a.GetCaixaCents(),
		patrimony: a.GetPatrimonyCents(),
	}
}

type positionCents struct {
	productID, class string
	applied, value   int64
}

func positionsOf(a *accountv1.Account) []positionCents {
	out := make([]positionCents, 0, len(a.GetPositions()))
	for _, p := range a.GetPositions() {
		out = append(out, positionCents{p.GetProductId(), p.GetAssetClass(), p.GetAppliedCents(), p.GetValueCents()})
	}
	return out
}

var marianaPositions = []positionCents{
	{"cobalto", sim.ClassAcoes, 6_000_000, 7_200_000},
	{"farol", sim.ClassAcoes, 1_750_000, 1_890_000},
	{"acoesg", sim.ClassETFs, 3_600_000, 4_060_000},
	{"renda", sim.ClassETFs, 1_940_000, 2_000_000},
	{"corp", sim.ClassRendaFixa, 3_600_000, 3_680_000},
}

var wantCatalog = []sim.Product{
	{ID: "tbill", Name: "Orla T-Bill 6 meses", AssetClass: sim.ClassRendaFixa, Risk: 1, ReturnLabel: "4,9% a.a.", MinimumCents: 10_000},
	{ID: "corp", Name: "Orla Corporate IG 2029", AssetClass: sim.ClassRendaFixa, Risk: 2, ReturnLabel: "5,6% a.a.", MinimumCents: 100_000},
	{ID: "renda", Name: "Maré Renda Global ETF", AssetClass: sim.ClassETFs, Risk: 2, ReturnLabel: "+3,8% em 12 meses", MinimumCents: 5_000},
	{ID: "acoesg", Name: "Maré Ações Globais ETF", AssetClass: sim.ClassETFs, Risk: 3, ReturnLabel: "+11,2% em 12 meses", MinimumCents: 5_000},
	{ID: "farol", Name: "Farol Saúde", AssetClass: sim.ClassAcoes, Risk: 4, ReturnLabel: "+9,4% em 12 meses", MinimumCents: 1_000},
	{ID: "cobalto", Name: "Cobalto Semicondutores", AssetClass: sim.ClassAcoes, Risk: 5, ReturnLabel: "+27,1% em 12 meses", MinimumCents: 1_000},
}

// assertSeedState checks the matrix rows "Seed aggregates", "Positions",
// "Catalog", and "Registration" against any seeded store.
func assertSeedState(t *testing.T, client accountv1.AccountServiceClient) {
	t.Helper()
	ctx := t.Context()

	list, err := client.ListAccounts(ctx, &accountv1.ListAccountsRequest{})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(list.GetAccounts()) != len(phase2Totals) {
		t.Fatalf("accounts = %d, want %d", len(list.GetAccounts()), len(phase2Totals))
	}
	for _, account := range list.GetAccounts() {
		want, ok := phase2Totals[account.GetCustomerId()]
		if !ok {
			t.Fatalf("unexpected account %s", account.GetCustomerId())
		}
		if got := totalsOf(account); got != want {
			t.Fatalf("account %s totals = %+v, want %+v", account.GetCustomerId(), got, want)
		}
		var sum int64
		for _, p := range account.GetPositions() {
			sum += p.GetValueCents()
		}
		if sum+account.GetCaixaCents() != account.GetPatrimonyCents() {
			t.Fatalf("account %s positions %d + cash %d != patrimony %d",
				account.GetCustomerId(), sum, account.GetCaixaCents(), account.GetPatrimonyCents())
		}
	}

	mariana, err := client.GetAccount(ctx, &accountv1.GetAccountRequest{CustomerId: sim.CustomerMariana})
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got := positionsOf(mariana); !slices.Equal(got, marianaPositions) {
		t.Fatalf("mariana positions = %+v, want %+v", got, marianaPositions)
	}

	products, err := client.ListProducts(ctx, &accountv1.ListProductsRequest{})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	got := make([]sim.Product, 0, len(products.GetProducts()))
	for _, p := range products.GetProducts() {
		got = append(got, sim.Product{
			ID: p.GetId(), Name: p.GetName(), AssetClass: p.GetAssetClass(), Risk: int(p.GetRisk()),
			ReturnLabel: p.GetReturnLabel(), MinimumCents: p.GetMinimumCents(),
		})
	}
	if !slices.Equal(got, wantCatalog) {
		t.Fatalf("catalog = %+v, want %+v", got, wantCatalog)
	}

	reg, err := client.GetRegistration(ctx, &accountv1.GetRegistrationRequest{CustomerId: sim.CustomerThiago})
	if err != nil {
		t.Fatalf("GetRegistration: %v", err)
	}
	if reg.GetCustomerId() != sim.CustomerThiago ||
		reg.GetEmail() != "thiago.azevedo@example.com" ||
		reg.GetPhone() != "+55 (48) •••••-2093" ||
		reg.GetCity() != "Florianópolis, SC · Brasil" ||
		reg.GetAccountNumber() != "Conta 2847-1 · Orla Invest" {
		t.Fatalf("registration = %+v", reg)
	}
	unknown, err := identity.NewV7()
	if err != nil {
		t.Fatalf("NewV7: %v", err)
	}
	_, err = client.GetRegistration(ctx, &accountv1.GetRegistrationRequest{CustomerId: unknown})
	wantCode(t, err, codes.NotFound)
	_, err = client.GetRegistration(ctx, &accountv1.GetRegistrationRequest{CustomerId: "not-a-uuid"})
	wantCode(t, err, codes.InvalidArgument)
}

// assertCashOnlyCommands checks "Deposit after migration" and "Withdrawal":
// both move only cash, and the event before/after are patrimony. payloadOf
// returns the outbox body of one event.
func assertCashOnlyCommands(t *testing.T, client accountv1.AccountServiceClient, payloadOf func(eventID string) []byte) {
	t.Helper()
	ctx := t.Context()
	get := func() *accountv1.Account {
		t.Helper()
		account, err := client.GetAccount(ctx, &accountv1.GetAccountRequest{CustomerId: sim.CustomerFernanda})
		if err != nil {
			t.Fatalf("GetAccount: %v", err)
		}
		return account
	}
	before := get()

	reply, err := client.Deposit(ctx, &accountv1.DepositRequest{
		CustomerId: sim.CustomerFernanda, IdempotencyKey: "dep-positions", AmountCents: 1_000_000, Origin: "Conta corrente",
	})
	if err != nil {
		t.Fatalf("Deposit: %v", err)
	}
	after := get()
	if after.GetCaixaCents() != 1_114_800 || after.GetPatrimonyCents() != 1_820_000 {
		t.Fatalf("after deposit cash %d patrimony %d, want 1114800 and 1820000",
			after.GetCaixaCents(), after.GetPatrimonyCents())
	}
	if !slices.Equal(positionsOf(after), positionsOf(before)) {
		t.Fatalf("deposit moved positions: %+v -> %+v", positionsOf(before), positionsOf(after))
	}
	_, body, err := sim.DecodeOutboxAccount(payloadOf(reply.GetEventId()))
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if body.Before != 820_000 || body.After != 1_820_000 {
		t.Fatalf("event before %v after %v, want 820000 and 1820000", body.Before, body.After)
	}

	_, err = client.Withdraw(ctx, &accountv1.WithdrawRequest{
		CustomerId: sim.CustomerFernanda, IdempotencyKey: "wd-positions",
		AmountCents: after.GetCaixaCents() + 1, Destination: "Conta corrente",
	})
	wantCode(t, err, codes.FailedPrecondition)
	if got := get(); totalsOf(got) != totalsOf(after) || !slices.Equal(positionsOf(got), positionsOf(after)) {
		t.Fatalf("refused withdrawal changed the account: %+v", got)
	}
}

func TestValue_FlatAndDeterministic(t *testing.T) {
	t.Parallel()
	for _, product := range sim.Catalog() {
		for day := range 30 {
			for _, units := range []int64{0, 1, 164_000, 7_200_000} {
				got := sim.Value(product.ID, units, day)
				if got != units {
					t.Fatalf("Value(%s, %d, %d) = %d, want %d", product.ID, units, day, got, units)
				}
				if again := sim.Value(product.ID, units, day); again != got {
					t.Fatalf("Value(%s, %d, %d) is not deterministic: %d then %d", product.ID, units, day, got, again)
				}
			}
		}
	}
}

func TestDemoSeed_AggregatesKeepPhase2(t *testing.T) {
	t.Parallel()
	seed := sim.DemoSeed()
	if !slices.Equal(seed.Products, wantCatalog) || !slices.Equal(sim.Catalog(), wantCatalog) {
		t.Fatalf("catalog = %+v", seed.Products)
	}
	classOf := map[string]string{}
	for _, product := range seed.Products {
		classOf[product.ID] = product.AssetClass
	}
	for _, account := range seed.Accounts {
		if account.Registration.CustomerID != account.CustomerID {
			t.Fatalf("registration of %s names %s", account.CustomerID, account.Registration.CustomerID)
		}
		for _, p := range account.Positions {
			if class, ok := classOf[p.ProductID]; !ok || class != p.AssetClass {
				t.Fatalf("%s position %s class %q, catalog %q", account.CustomerID, p.ProductID, p.AssetClass, class)
			}
			if p.ValueCents != p.UnitsCents {
				t.Fatalf("%s position %s day-0 value %d, units %d", account.CustomerID, p.ProductID, p.ValueCents, p.UnitsCents)
			}
		}
	}
	for _, account := range sim.POVSeed() {
		want := phase2Totals[account.CustomerID]
		got := classCents{account.Acoes, account.ETFs, account.RendaFixa, account.Caixa, account.Assets()}
		if got != want {
			t.Fatalf("%s aggregate = %+v, want %+v", account.CustomerID, got, want)
		}
	}
}

func TestGRPCServer_SeedPositionsCatalogRegistration(t *testing.T) {
	t.Parallel()
	assertSeedState(t, startAccountServer(t, sim.NewMemory()))
}

func TestGRPCServer_CashOnlyCommandsKeepPositions(t *testing.T) {
	t.Parallel()
	memory := sim.NewMemory()
	client := startAccountServer(t, memory)
	assertCashOnlyCommands(t, client, func(eventID string) []byte {
		t.Helper()
		for _, row := range memory.PendingOutbox() {
			if row.EventID == eventID {
				return row.Payload
			}
		}
		t.Fatalf("event %s not in outbox", eventID)
		return nil
	})
}

func TestReseed_RestoresPositionsAndCash(t *testing.T) {
	t.Parallel()
	memory := sim.NewMemory()
	client := startAccountServer(t, memory)
	if _, err := client.Withdraw(t.Context(), &accountv1.WithdrawRequest{
		CustomerId: sim.CustomerMariana, IdempotencyKey: "wd-reseed", AmountCents: 1_000_000, Destination: "Conta corrente",
	}); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if err := sim.Reseed(context.Background(), memory); err != nil {
		t.Fatalf("Reseed: %v", err)
	}
	assertSeedState(t, client)
}

// assertPurchases checks the purchase matrix rows against any seeded store:
// Thiago buys acoesg, the replay, the refusals, and a first position.
// payloadOf returns the outbox body of one event; events counts outbox rows.
func assertPurchases(
	t *testing.T,
	client accountv1.AccountServiceClient,
	payloadOf func(eventID string) []byte,
	events func() int,
) {
	t.Helper()
	ctx := t.Context()
	get := func(customerID string) *accountv1.Account {
		t.Helper()
		account, err := client.GetAccount(ctx, &accountv1.GetAccountRequest{CustomerId: customerID})
		if err != nil {
			t.Fatalf("GetAccount: %v", err)
		}
		return account
	}
	position := func(a *accountv1.Account, productID string) (positionCents, bool) {
		for _, p := range positionsOf(a) {
			if p.productID == productID {
				return p, true
			}
		}
		return positionCents{}, false
	}
	startEvents := events()

	buy := &accountv1.PurchaseRequest{
		CustomerId: sim.CustomerThiago, IdempotencyKey: "buy-acoesg", ProductId: "acoesg",
		AmountCents: 3_000_000, CommandId: "cmd-buy-acoesg",
	}
	reply, err := client.Purchase(ctx, buy)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	if reply.GetEventId() == "" || reply.GetReplay() {
		t.Fatalf("reply = %+v", reply)
	}
	thiago := get(sim.CustomerThiago)
	if thiago.GetCaixaCents() != 3_052_000 || thiago.GetPatrimonyCents() != 6_800_000 || thiago.GetEtfsCents() != 3_544_000 {
		t.Fatalf("after purchase = %+v, want cash 3052000, etfs 3544000, patrimony 6800000", totalsOf(thiago))
	}
	if got, _ := position(thiago, "acoesg"); got != (positionCents{"acoesg", sim.ClassETFs, 3_520_000, 3_544_000}) {
		t.Fatalf("acoesg = %+v, want applied 3520000 value 3544000", got)
	}
	env, body, err := sim.DecodeOutboxAccount(payloadOf(reply.GetEventId()))
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	wantBody := sim.AccountPayload{
		Kind: sim.KindAplicacao, Amount: 3_000_000, Before: 6_800_000, After: 6_800_000,
		ProductID: "acoesg", AssetClass: sim.ClassETFs, Risk: 3,
	}
	if env.SchemaVersion != 3 || env.CustomerID != sim.CustomerThiago || body != wantBody {
		t.Fatalf("event v%d %+v, want v3 %+v", env.SchemaVersion, body, wantBody)
	}

	replay, err := client.Purchase(ctx, buy)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replay.GetReplay() || replay.GetEventId() != reply.GetEventId() {
		t.Fatalf("replay = %+v, want replay of %s", replay, reply.GetEventId())
	}
	if got := events(); got != startEvents+1 {
		t.Fatalf("events after replay = %d, want %d", got, startEvents+1)
	}
	if got := get(sim.CustomerThiago); totalsOf(got) != totalsOf(thiago) {
		t.Fatalf("replay moved the account: %+v", totalsOf(got))
	}

	refusals := []struct {
		name string
		req  *accountv1.PurchaseRequest
		want codes.Code
	}{
		{"over cash", &accountv1.PurchaseRequest{ProductId: "acoesg", AmountCents: 3_052_001}, codes.FailedPrecondition},
		{"unknown product", &accountv1.PurchaseRequest{ProductId: "ouro", AmountCents: 100_000}, codes.InvalidArgument},
		{"zero amount", &accountv1.PurchaseRequest{ProductId: "acoesg"}, codes.InvalidArgument},
		{"negative amount", &accountv1.PurchaseRequest{ProductId: "acoesg", AmountCents: -100}, codes.InvalidArgument},
		{"below minimum", &accountv1.PurchaseRequest{ProductId: "corp", AmountCents: 99_999}, codes.InvalidArgument},
	}
	for i, r := range refusals {
		r.req.CustomerId = sim.CustomerThiago
		r.req.IdempotencyKey = "buy-refused-" + string(rune('a'+i))
		_, err := client.Purchase(ctx, r.req)
		if status.Code(err) != r.want {
			t.Fatalf("%s: code = %s (%v), want %s", r.name, status.Code(err), err, r.want)
		}
	}
	if got := get(sim.CustomerThiago); totalsOf(got) != totalsOf(thiago) || !slices.Equal(positionsOf(got), positionsOf(thiago)) {
		t.Fatalf("a refusal moved the account: %+v", got)
	}
	if got := events(); got != startEvents+1 {
		t.Fatalf("events after refusals = %d, want %d", got, startEvents+1)
	}

	// Fernanda holds no cobalto: the purchase creates the position.
	fernanda := get(sim.CustomerFernanda)
	if _, ok := position(fernanda, "cobalto"); ok {
		t.Fatal("fernanda already holds cobalto")
	}
	if _, err := client.Purchase(ctx, &accountv1.PurchaseRequest{
		CustomerId: sim.CustomerFernanda, IdempotencyKey: "buy-cobalto", ProductId: "cobalto", AmountCents: 1_000,
	}); err != nil {
		t.Fatalf("Purchase cobalto: %v", err)
	}
	after := get(sim.CustomerFernanda)
	if got, ok := position(after, "cobalto"); !ok || got != (positionCents{"cobalto", sim.ClassAcoes, 1_000, 1_000}) {
		t.Fatalf("cobalto = %+v (%v), want applied and value 1000", got, ok)
	}
	if after.GetCaixaCents() != fernanda.GetCaixaCents()-1_000 || after.GetPatrimonyCents() != fernanda.GetPatrimonyCents() ||
		after.GetAcoesCents() != fernanda.GetAcoesCents()+1_000 {
		t.Fatalf("fernanda %+v -> %+v", totalsOf(fernanda), totalsOf(after))
	}

	unknown, err := identity.NewV7()
	if err != nil {
		t.Fatalf("NewV7: %v", err)
	}
	_, err = client.Purchase(ctx, &accountv1.PurchaseRequest{
		CustomerId: unknown, IdempotencyKey: "buy-unknown", ProductId: "acoesg", AmountCents: 100_000,
	})
	wantCode(t, err, codes.NotFound)
	_, err = client.Purchase(ctx, &accountv1.PurchaseRequest{
		CustomerId: "not-a-uuid", IdempotencyKey: "buy-bad", ProductId: "acoesg", AmountCents: 100_000,
	})
	wantCode(t, err, codes.InvalidArgument)
	_, err = client.Purchase(ctx, &accountv1.PurchaseRequest{
		CustomerId: sim.CustomerThiago, ProductId: "acoesg", AmountCents: 100_000,
	})
	wantCode(t, err, codes.InvalidArgument)
}

func TestGRPCServer_Purchase(t *testing.T) {
	t.Parallel()
	memory := sim.NewMemory()
	client := startAccountServer(t, memory)
	assertPurchases(t, client, func(eventID string) []byte {
		t.Helper()
		for _, row := range memory.PendingOutbox() {
			if row.EventID == eventID {
				return row.Payload
			}
		}
		t.Fatalf("event %s not in outbox", eventID)
		return nil
	}, func() int { return len(memory.PendingOutbox()) })

	if err := sim.Reseed(t.Context(), memory); err != nil {
		t.Fatalf("Reseed: %v", err)
	}
	assertSeedState(t, client)
}
