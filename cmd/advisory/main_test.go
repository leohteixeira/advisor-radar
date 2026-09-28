package main

import (
	"errors"
	"fmt"
	"testing"

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
