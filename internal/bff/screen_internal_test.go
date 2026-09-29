package bff

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/screen"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// onePOV answers Get with account or err.
type onePOV struct {
	emptyPOV
	account POVAccount
	err     error
}

func (p onePOV) Get(context.Context, string) (POVAccount, error) { return p.account, p.err }

func TestScreenAccounts_Account(t *testing.T) {
	t.Parallel()
	src := screenAccounts{pov: onePOV{account: POVAccount{
		Acoes: 1, ETFs: 2, RendaFixa: 3, Caixa: 4, Patrimony: 10,
		Positions: []POVPosition{{ProductID: "tbill", AssetClass: "renda_fixa", AppliedCents: 2, ValueCents: 3}},
	}}}
	got, err := src.Account(t.Context(), "c")
	if err != nil {
		t.Fatalf("Account error = %v", err)
	}
	if got.Acoes != 1 || got.ETFs != 2 || got.RendaFixa != 3 || got.Cash != 4 || got.Patrimony != 10 {
		t.Errorf("account = %+v", got)
	}
	want := []screen.Position{{ProductID: "tbill", AssetClass: "renda_fixa", AppliedCents: 2, ValueCents: 3}}
	if !slices.Equal(got.Positions, want) {
		t.Errorf("positions = %+v, want %+v", got.Positions, want)
	}

	_, err = screenAccounts{pov: onePOV{err: sim.ErrUnknownCustomer}}.Account(t.Context(), "c")
	if !errors.Is(err, screen.ErrUnknownCustomer) || !errors.Is(err, sim.ErrUnknownCustomer) {
		t.Errorf("unknown customer error = %v", err)
	}
	down := errors.New("down")
	_, err = screenAccounts{pov: onePOV{err: down}}.Account(t.Context(), "c")
	if !errors.Is(err, down) || errors.Is(err, screen.ErrUnknownCustomer) {
		t.Errorf("failure error = %v", err)
	}
}

type rowsTimeline struct{ rows []TimelineEntry }

func (r rowsTimeline) Search(context.Context, string, string, string) ([]TimelineEntry, error) {
	return r.rows, nil
}

func TestScreenActivity_Activity(t *testing.T) {
	t.Parallel()
	src := screenActivity{timeline: rowsTimeline{rows: []TimelineEntry{
		{Kind: "aporte", Title: "Aporte", Ago: 90, Source: "account.event.recorded", OccurredAt: time.Unix(1_700_000_000, 0)},
		{Kind: "saque", Title: "Saque", Ago: -3},
	}}}
	got, err := src.Activity(t.Context(), "c")
	if err != nil {
		t.Fatalf("Activity error = %v", err)
	}
	want := []screen.Activity{
		{Kind: "aporte", Title: "Aporte", Source: "account.event.recorded", OccurredAt: time.Unix(1_700_000_000, 0), Age: 90 * time.Minute},
		{Kind: "saque", Title: "Saque", Age: 0},
	}
	if !slices.Equal(got, want) {
		t.Errorf("activity = %+v, want %+v", got, want)
	}
}

func TestScreenAccounts_DisabledIsAFailedSource(t *testing.T) {
	t.Parallel()
	_, err := screenAccounts{pov: emptyPOV{}}.Account(t.Context(), "c")
	if !errors.Is(err, errPOVDisabled) || errors.Is(err, screen.ErrUnknownCustomer) {
		t.Errorf("disabled account error = %v", err)
	}
}

func TestFailureClass(t *testing.T) {
	t.Parallel()
	if got := failureClass(errors.New("screen: no sla for segment \"X\"")); got != "build_error" {
		t.Errorf("class = %q, want build_error", got)
	}
	if got := failureClass(fmt.Errorf("screen: variant: %w: boom", screen.ErrPanic)); got != "panic" {
		t.Errorf("class = %q, want panic", got)
	}
}
