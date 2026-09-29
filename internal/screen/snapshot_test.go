package screen

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// errDown is a source failure that is not a deadline.
var errDown = errors.New("source down")

// wait blocks for d or until ctx is done, as a well-behaved source does.
func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type fakeAccounts struct {
	account Account
	err     error
	delay   time.Duration
}

func (f fakeAccounts) Account(ctx context.Context, _ string) (Account, error) {
	if err := wait(ctx, f.delay); err != nil {
		return Account{}, err
	}
	if f.err != nil {
		return Account{}, f.err
	}
	return f.account, nil
}

type fakeCustomers struct {
	customer Customer
	err      error
	delay    time.Duration
}

func (f fakeCustomers) Customer(ctx context.Context, _ string) (Customer, error) {
	if err := wait(ctx, f.delay); err != nil {
		return Customer{}, err
	}
	if f.err != nil {
		return Customer{}, f.err
	}
	return f.customer, nil
}

type fakeActivity struct {
	rows  []Activity
	err   error
	delay time.Duration
}

func (f fakeActivity) Activity(ctx context.Context, _ string) ([]Activity, error) {
	if err := wait(ctx, f.delay); err != nil {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

// testNow is the request clock of the engine tests.
var testNow = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// fixture is one seed client as the three sources return it.
type fixture struct {
	id       string
	account  Account
	customer Customer
	activity []Activity
}

// The seed clients, from architecture.md "Seed", the advisory book seed, and
// OrlaApp.dc.html clients(). Amounts are day-0 cents.
func fernandaFixture() fixture {
	return fixture{
		id:       "01a0e3a4-9a44-757a-ac8f-dab7db5eb068",
		account:  Account{Acoes: 164_000, ETFs: 369_000, RendaFixa: 172_200, Cash: 114_800, Patrimony: 820_000},
		customer: Customer{Name: "Fernanda Lima", Segment: "Essencial", Advisor: "Ana Paula Ribeiro", Since: "2024"},
		activity: []Activity{},
	}
}

func thiagoFixture() fixture {
	return fixture{
		id:       "01a0e3a4-9a44-75dd-b3a0-403a7a87836e",
		account:  Account{Acoes: 204_000, ETFs: 544_000, RendaFixa: 0, Cash: 6_052_000, Patrimony: 6_800_000},
		customer: Customer{Name: "Thiago Azevedo", Segment: "Advance", Advisor: "Ana Paula Ribeiro", Since: "2024"},
		activity: []Activity{
			{Kind: "segmento", Title: "Segmento", Source: "alert.raised", Age: 4 * 24 * time.Hour},
			{Kind: "aporte", Title: "Aporte", Source: "account.event.recorded", OccurredAt: testNow.Add(-4*24*time.Hour - time.Minute), Age: time.Minute},
		},
	}
}

func marianaFixture() fixture {
	return fixture{
		id:       "01a0e3a4-9a44-7566-b5de-eb2e365799f8",
		account:  Account{Acoes: 9_090_000, ETFs: 6_060_000, RendaFixa: 3_680_000, Cash: 6_000_000, Patrimony: 24_830_000},
		customer: Customer{Name: "Mariana Costa", Segment: "Singular", Advisor: "Ana Paula Ribeiro", Since: "2021"},
		activity: []Activity{
			{Kind: "mensagem", Title: "Mensagem · chat", Source: "message.triaged", Age: 12 * time.Minute},
			{Kind: "saque", Title: "Saque", Source: "alert.raised", Age: 30 * time.Minute},
			{Kind: "mensagem", Title: "Mensagem · e-mail", Source: "message.received", Age: 30 * time.Hour},
			{Kind: "nota", Title: "Nota do assessor", Source: "advisory.note.recorded", Age: 2900 * time.Minute},
			{Kind: "saque", Title: "Saque", Source: "account.event.recorded", Age: 3 * 24 * time.Hour},
			{Kind: "caso", Title: "Caso k1 Resolvido", Source: "case.status.changed", Age: 10100 * time.Minute},
			{Kind: "telefone", Title: "Ligação", Source: "advisory.note.recorded", Age: 43000 * time.Minute},
		},
	}
}

func (f fixture) sources() Sources {
	return Sources{
		Accounts:  fakeAccounts{account: f.account},
		Customers: fakeCustomers{customer: f.customer},
		Activity:  fakeActivity{rows: f.activity},
	}
}

func (f fixture) snapshot() Snapshot {
	return Snapshot{
		CustomerID: f.id,
		Now:        testNow,
		Account:    Fetched[Account]{Value: f.account},
		Customer:   Fetched[Customer]{Value: f.customer},
		Activity:   Fetched[[]Activity]{Value: f.activity},
	}
}

func TestFetched_OK(t *testing.T) {
	t.Parallel()
	if !(Fetched[int]{Value: 1}).OK() {
		t.Error("answered source is not OK")
	}
	if (Fetched[int]{Err: errDown}).OK() {
		t.Error("failed source is OK")
	}
}

func TestSnapshot_failed(t *testing.T) {
	t.Parallel()
	snap := Snapshot{Customer: Fetched[Customer]{Err: errDown}}
	if snap.failed(SourceAccount) != nil || snap.failed(SourceTimeline) != nil {
		t.Error("answered sources report a failure")
	}
	if !errors.Is(snap.failed(SourceAdvisory), errDown) {
		t.Error("failed advisory source reports no failure")
	}
	if snap.failed(Source("cases")) == nil {
		t.Error("unknown source reports no failure")
	}
}

func TestFetchSnapshot_AllSourcesAnswer(t *testing.T) {
	t.Parallel()
	f := thiagoFixture()
	snap := fetchSnapshot(t.Context(), f.sources(), f.id, testNow, DefaultDeadline)
	if !snap.Account.OK() || !snap.Customer.OK() || !snap.Activity.OK() {
		t.Fatalf("snapshot errors: account %v, customer %v, activity %v", snap.Account.Err, snap.Customer.Err, snap.Activity.Err)
	}
	if snap.CustomerID != f.id || !snap.Now.Equal(testNow) || snap.Account.Value.Patrimony != 6_800_000 || snap.Customer.Value.Name != "Thiago Azevedo" || len(snap.Activity.Value) != 2 {
		t.Errorf("snapshot = %+v", snap)
	}
}

func TestFetchSnapshot_FailureDoesNotCancelOthers(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := thiagoFixture()
		src := Sources{
			Accounts:  fakeAccounts{err: errDown},
			Customers: fakeCustomers{customer: f.customer, delay: 100 * time.Millisecond},
			Activity:  fakeActivity{rows: f.activity, delay: 200 * time.Millisecond},
		}
		snap := fetchSnapshot(t.Context(), src, f.id, testNow, DefaultDeadline)
		if !errors.Is(snap.Account.Err, errDown) {
			t.Errorf("account error = %v, want %v", snap.Account.Err, errDown)
		}
		if !snap.Customer.OK() || !snap.Activity.OK() {
			t.Errorf("slower sources were cancelled: customer %v, activity %v", snap.Customer.Err, snap.Activity.Err)
		}
	})
}

func TestFetchSnapshot_Deadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := marianaFixture()
		src := f.sources()
		src.Activity = fakeActivity{rows: f.activity, delay: time.Hour}
		start := time.Now()
		snap := fetchSnapshot(t.Context(), src, f.id, testNow, DefaultDeadline)
		if elapsed := time.Since(start); elapsed != DefaultDeadline {
			t.Errorf("snapshot took %v, want the %v deadline", elapsed, DefaultDeadline)
		}
		if !errors.Is(snap.Activity.Err, context.DeadlineExceeded) {
			t.Errorf("activity error = %v, want deadline exceeded", snap.Activity.Err)
		}
		if !snap.Account.OK() || !snap.Customer.OK() {
			t.Errorf("fast sources failed: account %v, customer %v", snap.Account.Err, snap.Customer.Err)
		}
	})
}

func TestFetchSnapshot_ErrorNamesSource(t *testing.T) {
	t.Parallel()
	src := Sources{
		Accounts:  fakeAccounts{err: errDown},
		Customers: fakeCustomers{err: errDown},
		Activity:  fakeActivity{err: errDown},
	}
	snap := fetchSnapshot(t.Context(), src, "id", testNow, DefaultDeadline)
	for name, err := range map[Source]error{
		SourceAccount:  snap.Account.Err,
		SourceAdvisory: snap.Customer.Err,
		SourceTimeline: snap.Activity.Err,
	} {
		if !errors.Is(err, errDown) {
			t.Errorf("%s error = %v, want it to wrap %v", name, err, errDown)
		}
		if want := "screen: " + string(name) + ": source down"; err.Error() != want {
			t.Errorf("%s error = %q, want %q", name, err, want)
		}
	}
}

// panicCustomers is an adapter with a bug.
type panicCustomers struct{}

func (panicCustomers) Customer(context.Context, string) (Customer, error) { panic("nil map") }

func TestFetchSnapshot_SourcePanicIsThatSourceError(t *testing.T) {
	t.Parallel()
	f := thiagoFixture()
	src := f.sources()
	src.Customers = panicCustomers{}
	snap := fetchSnapshot(t.Context(), src, f.id, testNow, DefaultDeadline)
	if !errors.Is(snap.Customer.Err, ErrPanic) || !strings.Contains(snap.Customer.Err.Error(), "screen: advisory:") {
		t.Errorf("customer error = %v, want an advisory panic", snap.Customer.Err)
	}
	if !snap.Account.OK() || !snap.Activity.OK() {
		t.Errorf("other sources failed: account %v, activity %v", snap.Account.Err, snap.Activity.Err)
	}
}
