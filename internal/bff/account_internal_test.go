package bff

import (
	"context"
	"slices"
	"testing"
	"time"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
)

func TestBackoff_EqualJitterBounds(t *testing.T) {
	t.Parallel()
	const base = 50 * time.Millisecond
	tests := []struct {
		name     string
		n        int
		min, max time.Duration
	}{
		{name: "first retry", n: 1, min: 25 * time.Millisecond, max: 50 * time.Millisecond},
		{name: "second retry", n: 2, min: 50 * time.Millisecond, max: 100 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			seen := map[time.Duration]struct{}{}
			for range 1000 {
				d := backoff(base, tt.n)
				if d < tt.min || d > tt.max {
					t.Fatalf("backoff(%s, %d) = %s, want in [%s, %s]", base, tt.n, d, tt.min, tt.max)
				}
				seen[d] = struct{}{}
			}
			if len(seen) < 2 {
				t.Fatalf("backoff(%s, %d) returned one value in 1000 draws, want jitter", base, tt.n)
			}
		})
	}
}

func TestSleepCtx(t *testing.T) {
	t.Parallel()

	t.Run("returns false promptly on cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(10*time.Millisecond, cancel)
		start := time.Now()
		if sleepCtx(ctx, time.Hour) {
			t.Fatal("sleepCtx = true, want false after cancel")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("sleepCtx returned after %s, want soon after cancel", elapsed)
		}
	})

	t.Run("returns true after the wait", func(t *testing.T) {
		t.Parallel()
		if !sleepCtx(t.Context(), time.Millisecond) {
			t.Fatal("sleepCtx = false, want true with a live context")
		}
	})
}

func TestPovFromProto(t *testing.T) {
	t.Parallel()
	got := povFromProto(&accountv1.Account{
		CustomerId: "c", AcoesCents: 204_000, EtfsCents: 544_000, CaixaCents: 6_052_000, PatrimonyCents: 6_800_000,
		Positions: []*accountv1.Position{
			{ProductId: "cobalto", AssetClass: "acoes", AppliedCents: 190_000, ValueCents: 204_000},
			{ProductId: "acoesg", AssetClass: "etfs", AppliedCents: 520_000, ValueCents: 544_000},
		},
	})
	if got.CustomerID != "c" || got.Acoes != 204_000 || got.ETFs != 544_000 || got.RendaFixa != 0 ||
		got.Caixa != 6_052_000 || got.Patrimony != 6_800_000 {
		t.Errorf("account = %+v", got)
	}
	want := []POVPosition{
		{ProductID: "cobalto", AssetClass: "acoes", AppliedCents: 190_000, ValueCents: 204_000},
		{ProductID: "acoesg", AssetClass: "etfs", AppliedCents: 520_000, ValueCents: 544_000},
	}
	if !slices.Equal(got.Positions, want) {
		t.Errorf("positions = %+v, want %+v", got.Positions, want)
	}
}
