package sim_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/outbox"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// povIDs are the seeded POV customers in byte-wise id order, the order an
// advance writes its events in.
var povIDs = []string{sim.CustomerMariana, sim.CustomerFernanda, sim.CustomerThiago}

// revaluation is one decoded reavaliacao outbox row.
type revaluation struct {
	eventID string
	env     event.Envelope
	body    sim.AccountPayload
}

// revaluations decodes the reavaliacao rows of rows, in order.
func revaluations(t *testing.T, rows []outbox.Row) []revaluation {
	t.Helper()
	var out []revaluation
	for _, row := range rows {
		env, body, err := sim.DecodeOutboxAccount(row.Payload)
		if err != nil {
			t.Fatalf("decode %s: %v", row.EventID, err)
		}
		if body.Kind != sim.KindReavaliacao {
			continue
		}
		if row.RoutingKey != event.NameAccountEventRecorded || env.EventID != row.EventID {
			t.Fatalf("row %s: routing %q, envelope id %q", row.EventID, row.RoutingKey, env.EventID)
		}
		out = append(out, revaluation{eventID: row.EventID, env: env, body: body})
	}
	return out
}

// byCustomer indexes revaluations by customer id.
func byCustomer(revs []revaluation) map[string]revaluation {
	out := make(map[string]revaluation, len(revs))
	for _, r := range revs {
		out[r.env.CustomerID] = r
	}
	return out
}

func advance(t *testing.T, client accountv1.AccountServiceClient, key string) *accountv1.AdvanceDayReply {
	t.Helper()
	reply, err := client.AdvanceDay(t.Context(), &accountv1.AdvanceDayRequest{IdempotencyKey: key})
	if err != nil {
		t.Fatalf("AdvanceDay(%s): %v", key, err)
	}
	return reply
}

// advanceTo advances a fresh store from day 0 to day, one key per day.
func advanceTo(t *testing.T, client accountv1.AccountServiceClient, day int) {
	t.Helper()
	for d := 1; d <= day; d++ {
		if got := advance(t, client, "to-day-"+strconv.Itoa(d)).GetSimDay(); got != int32(d) {
			t.Fatalf("advance to %d answered day %d", d, got)
		}
	}
}

func account(t *testing.T, client accountv1.AccountServiceClient, customerID string) *accountv1.Account {
	t.Helper()
	got, err := client.GetAccount(t.Context(), &accountv1.GetAccountRequest{CustomerId: customerID})
	if err != nil {
		t.Fatalf("GetAccount(%s): %v", customerID, err)
	}
	return got
}

// outboxRows reads every outbox row of a store, in insert order.
type outboxRows func() []outbox.Row

// TestAdvanceDay_Memory runs the advance-day matrix over sim.Memory; the
// gated TestPGXStore_AdvanceDay runs the same rows over PostgreSQL.
func TestAdvanceDay_Memory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(*testing.T, accountv1.AccountServiceClient, outboxRows, func())
	}{
		{name: "day 0 to 1 is flat", run: assertDay0To1},
		{name: "day 3 shocks cobalto holders", run: assertShock},
		{name: "replay advances nothing", run: assertAdvanceReplay},
		{name: "commands use the stored day", run: assertCommandsUseStoredDay},
		{name: "reseed returns to day 0 in a new epoch", run: assertReseedDay},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := sim.NewMemory()
			reseed := func() {
				if err := sim.Reseed(t.Context(), store); err != nil {
					t.Fatalf("Reseed: %v", err)
				}
			}
			tt.run(t, startAccountServer(t, store), store.PendingOutbox, reseed)
		})
	}
}

func assertDay0To1(t *testing.T, client accountv1.AccountServiceClient, rows outboxRows, _ func()) {
	t.Helper()
	start := len(rows())
	reply := advance(t, client, "day-1")
	if reply.GetSimDay() != 1 || reply.GetReplay() {
		t.Fatalf("reply = %+v, want day 1 and no replay", reply)
	}
	revs := revaluations(t, rows()[start:])
	if len(revs) != 3 {
		t.Fatalf("revaluations = %d, want one per POV account", len(revs))
	}
	ids := make([]string, 0, len(revs))
	for i, r := range revs {
		ids = append(ids, r.eventID)
		if r.env.CustomerID != povIDs[i] {
			t.Fatalf("revaluation %d is for %s, want %s (byte-wise id order)", i, r.env.CustomerID, povIDs[i])
		}
		want := sim.AccountPayload{
			Kind:   sim.KindReavaliacao,
			Amount: 0,
			Before: float64(phase2Totals[r.env.CustomerID].patrimony),
			After:  float64(phase2Totals[r.env.CustomerID].patrimony),
			SimDay: 1,
			Epoch:  revs[0].body.Epoch,
		}
		if r.body != want || r.env.SchemaVersion != event.SchemaVersionPositions {
			t.Fatalf("%s revaluation = %+v at v%d, want %+v at v3", r.env.CustomerID, r.body, r.env.SchemaVersion, want)
		}
		if parsed := uuid.MustParse(r.eventID); parsed.Version() != 5 {
			t.Fatalf("event id %s is version %d, want a name-based version 5 UUID", r.eventID, parsed.Version())
		}
		// The payload carries the epoch that namespaces the event id.
		if want, err := sim.RevaluationEventID(r.body.Epoch, r.env.CustomerID, 1); err != nil || want != r.eventID {
			t.Fatalf("event id %s is not the day-1 id of epoch %q (%s, %v)", r.eventID, r.body.Epoch, want, err)
		}
	}
	if !slices.Equal(reply.GetEventIds(), ids) {
		t.Fatalf("reply event ids = %v, outbox = %v", reply.GetEventIds(), ids)
	}
	for _, id := range povIDs {
		got := account(t, client, id)
		if got.GetSimDay() != 1 || got.GetDayChangeCents() != 0 || totalsOf(got) != phase2Totals[id] {
			t.Fatalf("%s on day 1 = %+v, want the seed totals and no day change", id, got)
		}
	}
}

func assertShock(t *testing.T, client accountv1.AccountServiceClient, rows outboxRows, _ func()) {
	t.Helper()
	advanceTo(t, client, 2)
	before := len(rows())

	reply := advance(t, client, "day-3")
	if reply.GetSimDay() != 3 {
		t.Fatalf("sim_day = %d, want 3", reply.GetSimDay())
	}
	revs := byCustomer(revaluations(t, rows()[before:]))
	tests := []struct {
		name       string
		customerID string
		want       sim.AccountPayload
	}{
		{
			name:       "mariana loses 15.5%",
			customerID: sim.CustomerMariana,
			want: sim.AccountPayload{
				Kind: sim.KindReavaliacao, Amount: -3_852_000, Before: 24_830_000, After: 20_978_000,
				SimDay: 3, ProductID: "cobalto", ProductChangeBP: -5350,
			},
		},
		{
			name:       "thiago loses 1.6%",
			customerID: sim.CustomerThiago,
			want: sim.AccountPayload{
				Kind: sim.KindReavaliacao, Amount: -109_140, Before: 6_800_000, After: 6_690_860,
				SimDay: 3, ProductID: "cobalto", ProductChangeBP: -5350,
			},
		},
		{
			name:       "fernanda holds no cobalto",
			customerID: sim.CustomerFernanda,
			want: sim.AccountPayload{
				Kind: sim.KindReavaliacao, Amount: 0, Before: 820_000, After: 820_000, SimDay: 3,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := revs[tt.customerID]
			if !ok {
				t.Fatalf("no revaluation for %s", tt.customerID)
			}
			want := tt.want
			want.Epoch = revs[sim.CustomerMariana].body.Epoch
			if got.body != want || want.Epoch == "" {
				t.Fatalf("revaluation = %+v, want %+v in one epoch", got.body, want)
			}
		})
	}

	mariana := account(t, client, sim.CustomerMariana)
	if mariana.GetSimDay() != 3 || mariana.GetDayChangeCents() != -3_852_000 || mariana.GetPatrimonyCents() != 20_978_000 {
		t.Fatalf("mariana on day 3 = %+v", mariana)
	}
	for _, p := range mariana.GetPositions() {
		if p.GetProductId() == "cobalto" && (p.GetValueCents() != 3_348_000 || p.GetAppliedCents() != 6_000_000) {
			t.Fatalf("mariana cobalto on day 3 = %+v, want value 3348000, applied unchanged", p)
		}
	}
	if thiago := account(t, client, sim.CustomerThiago); thiago.GetDayChangeCents() != -109_140 {
		t.Fatalf("thiago day change = %d, want -109140", thiago.GetDayChangeCents())
	}

	// Day 4 is flat: the pill disappears and the values stay shocked.
	advance(t, client, "day-4")
	mariana = account(t, client, sim.CustomerMariana)
	if mariana.GetSimDay() != 4 || mariana.GetDayChangeCents() != 0 || mariana.GetPatrimonyCents() != 20_978_000 {
		t.Fatalf("mariana on day 4 = %+v", mariana)
	}
}

func assertAdvanceReplay(t *testing.T, client accountv1.AccountServiceClient, rows outboxRows, _ func()) {
	t.Helper()
	first := advance(t, client, "same-key")
	count := len(rows())
	again := advance(t, client, "same-key")
	if !again.GetReplay() || again.GetSimDay() != first.GetSimDay() || !slices.Equal(again.GetEventIds(), first.GetEventIds()) {
		t.Fatalf("replay = %+v, want the original %+v with replay", again, first)
	}
	if got := len(rows()); got != count {
		t.Fatalf("replay appended %d outbox rows", got-count)
	}
	state, err := client.GetSimulation(t.Context(), &accountv1.GetSimulationRequest{})
	if err != nil {
		t.Fatalf("GetSimulation: %v", err)
	}
	if state.GetSimDay() != 1 {
		t.Fatalf("sim_day after a replay = %d, want 1", state.GetSimDay())
	}
}

func TestAdvanceDay_RefusalsAndFailures(t *testing.T) {
	t.Parallel()

	t.Run("missing key is InvalidArgument", func(t *testing.T) {
		t.Parallel()
		client := startAccountServer(t, sim.NewMemory())
		_, err := client.AdvanceDay(t.Context(), &accountv1.AdvanceDayRequest{})
		wantCode(t, err, codes.InvalidArgument)
	})
	t.Run("outbox failure rolls the day back", func(t *testing.T) {
		t.Parallel()
		inner := sim.NewMemory()
		_, err := startAccountServer(t, failingStore{inner: inner}).AdvanceDay(t.Context(), &accountv1.AdvanceDayRequest{IdempotencyKey: "k"})
		wantCode(t, err, codes.Internal)
		client := startAccountServer(t, inner)
		if day := account(t, client, sim.CustomerMariana).GetSimDay(); day != 0 {
			t.Fatalf("sim_day after a failed advance = %d, want 0", day)
		}
		if again := advance(t, client, "k"); again.GetReplay() || again.GetSimDay() != 1 {
			t.Fatalf("the failed key was kept: %+v", again)
		}
	})
	t.Run("nil store", func(t *testing.T) {
		t.Parallel()
		if _, err := sim.AdvanceDay(t.Context(), nil, sim.AdvanceCommand{IdempotencyKey: "k"}); err == nil {
			t.Fatal("AdvanceDay with a nil store succeeded")
		}
	})
	t.Run("canceled context", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := sim.AdvanceDay(ctx, sim.NewMemory(), sim.AdvanceCommand{IdempotencyKey: "k"})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})
}

// TestAdvanceDay_SerializesWithADeposit runs an advance to the shock day and
// a deposit at once: whichever runs first, the other sees its result, and the
// deposit's before and after are the patrimony of the day it ran on.
func TestAdvanceDay_SerializesWithADeposit(t *testing.T) {
	t.Parallel()
	for i := range 20 {
		store := sim.NewMemory()
		assertAdvanceSerializes(t, i, startAccountServer(t, store), store.PendingOutbox)
	}
}

// assertAdvanceSerializes is one run of the advance and deposit race on a
// store at day 0.
func assertAdvanceSerializes(t *testing.T, i int, client accountv1.AccountServiceClient, rows outboxRows) {
	t.Helper()
	advanceTo(t, client, 2)
	start := len(rows())

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Go(func() {
		_, err := client.AdvanceDay(t.Context(), &accountv1.AdvanceDayRequest{IdempotencyKey: "day-3"})
		errs <- err
	})
	wg.Go(func() {
		_, err := client.Deposit(t.Context(), &accountv1.DepositRequest{
			CustomerId: sim.CustomerMariana, IdempotencyKey: "dep", AmountCents: 100_000, Origin: "Câmbio",
		})
		errs <- err
	})
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	var deposit, reval sim.AccountPayload
	for _, row := range rows()[start:] {
		env, body, err := sim.DecodeOutboxAccount(row.Payload)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if env.CustomerID != sim.CustomerMariana {
			continue
		}
		switch body.Kind {
		case "aporte":
			deposit = body
		case sim.KindReavaliacao:
			reval = body
		}
	}
	switch deposit.Before {
	case 24_830_000: // deposit on day 2, then the shock revalues the new cash too
		if deposit.After != 24_930_000 || reval.Before != 24_930_000 || reval.After != 21_078_000 {
			t.Fatalf("run %d deposit first: deposit %+v, revaluation %+v", i, deposit, reval)
		}
	case 20_978_000: // shock first, then the deposit on day 3
		if deposit.After != 21_078_000 || reval.Before != 24_830_000 || reval.After != 20_978_000 {
			t.Fatalf("run %d advance first: deposit %+v, revaluation %+v", i, deposit, reval)
		}
	default:
		t.Fatalf("run %d: deposit before %v is neither day's patrimony", i, deposit.Before)
	}
	if reval.Amount != -3_852_000 {
		t.Fatalf("run %d: revaluation amount %v, want -3852000", i, reval.Amount)
	}
}

func assertCommandsUseStoredDay(t *testing.T, client accountv1.AccountServiceClient, rows outboxRows, _ func()) {
	t.Helper()
	advanceTo(t, client, 3)
	start := len(rows())

	if _, err := client.Deposit(t.Context(), &accountv1.DepositRequest{
		CustomerId: sim.CustomerThiago, IdempotencyKey: "dep", AmountCents: 1_000, Origin: "Câmbio",
	}); err != nil {
		t.Fatalf("Deposit: %v", err)
	}
	if _, err := client.Purchase(t.Context(), &accountv1.PurchaseRequest{
		CustomerId: sim.CustomerThiago, IdempotencyKey: "buy", ProductId: "cobalto", AmountCents: 1_000,
	}); err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	written := rows()[start:]
	if len(written) != 2 {
		t.Fatalf("outbox rows = %d, want the deposit and the purchase", len(written))
	}
	_, deposit, err := sim.DecodeOutboxAccount(written[0].Payload)
	if err != nil {
		t.Fatalf("decode deposit: %v", err)
	}
	if deposit.Before != 6_690_860 || deposit.After != 6_691_860 {
		t.Fatalf("deposit on day 3 = %+v, want before 6690860 and after 6691860", deposit)
	}
	_, purchase, err := sim.DecodeOutboxAccount(written[1].Payload)
	if err != nil {
		t.Fatalf("decode purchase: %v", err)
	}
	if purchase.Before != 6_691_860 || purchase.After != 6_691_860 {
		t.Fatalf("purchase on day 3 = %+v, want patrimony 6691860 on both sides", purchase)
	}
	// US$ 10,00 at the shocked price buys 2151 day-0 cents, worth 1000 today.
	for _, p := range account(t, client, sim.CustomerThiago).GetPositions() {
		if p.GetProductId() == "cobalto" && (p.GetAppliedCents() != 191_000 || p.GetValueCents() != 94_860+1_000) {
			t.Fatalf("thiago cobalto after the day-3 purchase = %+v", p)
		}
	}
}

func assertReseedDay(t *testing.T, client accountv1.AccountServiceClient, _ outboxRows, reseed func()) {
	t.Helper()
	first := advance(t, client, "first-day-1")
	advance(t, client, "day-2")
	advance(t, client, "day-3")

	reseed()
	for _, id := range povIDs {
		got := account(t, client, id)
		if got.GetSimDay() != 0 || got.GetDayChangeCents() != 0 || totalsOf(got) != phase2Totals[id] {
			t.Fatalf("%s after reseed = %+v, want day 0 at the seed values", id, got)
		}
	}
	// The reseed forgets the advance keys: the key of the old day 1 advances
	// the new epoch instead of replaying the old day and ids.
	again := advance(t, client, "first-day-1")
	if again.GetSimDay() != 1 || again.GetReplay() {
		t.Fatalf("first advance after reseed = day %d replay %v, want a fresh day 1", again.GetSimDay(), again.GetReplay())
	}
	for i, id := range again.GetEventIds() {
		if id == first.GetEventIds()[i] {
			t.Fatalf("day 1 after reseed reuses event id %s", id)
		}
	}
}

func TestRevaluationEventID(t *testing.T) {
	t.Parallel()
	const epoch = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	got, err := sim.RevaluationEventID(epoch, sim.CustomerMariana, 3)
	if err != nil {
		t.Fatalf("RevaluationEventID: %v", err)
	}
	want := uuid.NewSHA1(uuid.MustParse(epoch), []byte("reavaliacao:"+sim.CustomerMariana+":3")).String()
	if got != want {
		t.Fatalf("event id = %s, want %s", got, want)
	}
	if other, _ := sim.RevaluationEventID(epoch, sim.CustomerMariana, 4); other == got {
		t.Fatal("two days share an event id")
	}
	if _, err := sim.RevaluationEventID("not-a-uuid", sim.CustomerMariana, 3); !errors.Is(err, sim.ErrEpoch) {
		t.Fatalf("bad epoch err = %v, want ErrEpoch", err)
	}
}

func TestRevaluation_PicksTheLargestMove(t *testing.T) {
	t.Parallel()
	account := sim.Account{
		CustomerID: sim.CustomerMariana,
		Caixa:      500,
		SimDay:     2,
		Positions: []sim.Position{
			{ProductID: "farol", AssetClass: sim.ClassAcoes, UnitsCents: 1_000},
			{ProductID: "cobalto", AssetClass: sim.ClassAcoes, UnitsCents: 2_000},
		},
	}
	got := sim.Revaluation(account)
	want := sim.AccountPayload{
		Kind: sim.KindReavaliacao, Amount: -1_070, Before: 3_500, After: 2_430,
		SimDay: 3, ProductID: "cobalto", ProductChangeBP: -5350,
	}
	if got != want {
		t.Fatalf("Revaluation = %+v, want %+v", got, want)
	}
}
