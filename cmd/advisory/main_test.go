package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/advisory"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

func TestClassifyApplyError_MoneyScaleIsPermanent(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("advisory: apply evt-1: %w", sim.ErrMoneyScale)
	got := classifyApplyError(wrapped)

	var perm permanentDeliveryError
	if !errors.As(got, &perm) {
		t.Fatalf("classifyApplyError() type = %T, want permanentDeliveryError", got)
	}
	if !errors.Is(got, sim.ErrMoneyScale) {
		t.Fatalf("classifyApplyError() = %v, want wrap of sim.ErrMoneyScale", got)
	}
}

func TestClassifyApplyError_OtherErrorsRequeue(t *testing.T) {
	t.Parallel()

	other := errors.New("advisory: store unavailable")
	got := classifyApplyError(other)

	var perm permanentDeliveryError
	if errors.As(got, &perm) {
		t.Fatalf("classifyApplyError() = %v, want plain error for requeue", got)
	}
	if !errors.Is(got, other) {
		t.Fatalf("classifyApplyError() = %v, want %v", got, other)
	}
}

// Failures redelivery cannot fix are permanent, so the consumer nacks them
// without requeue instead of looping on them forever.
func TestClassifyApplyError_UnfixableFailuresArePermanent(t *testing.T) {
	t.Parallel()

	_, profileErr := advisory.MaxRisk("agressivo")
	_, _, invalidErr := advisory.EvaluateSuitability(sim.AccountPayload{Kind: sim.KindAplicacao, ProductID: "cobalto", Risk: 9}, advisory.ProfileArrojado)
	tests := []struct {
		name     string
		err      error
		sentinel error
	}{
		{name: "unknown customer", err: fmt.Errorf("advisory: raise: advisory pgx: update book c-1: %w", advisory.ErrUnknownCustomer), sentinel: advisory.ErrUnknownCustomer},
		{name: "unknown investor profile", err: fmt.Errorf("advisory: raise: %w", profileErr), sentinel: advisory.ErrUnknownProfile},
		{name: "invalid purchase", err: fmt.Errorf("advisory: raise: %w", invalidErr), sentinel: advisory.ErrInvalidPurchase},
		{name: "money scale", err: fmt.Errorf("advisory: apply evt-1: %w", sim.ErrMoneyScale), sentinel: sim.ErrMoneyScale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classifyApplyError(tt.err)
			var perm permanentDeliveryError
			if !errors.As(got, &perm) {
				t.Fatalf("classifyApplyError(%v) type = %T, want permanentDeliveryError", tt.err, got)
			}
			if !errors.Is(got, tt.sentinel) {
				t.Fatalf("classifyApplyError() = %v, want wrap of %v", got, tt.sentinel)
			}
		})
	}
}
