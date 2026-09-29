package sim

import "testing"

func TestUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		productID string
		amount    int64
		want      int64
	}{
		{name: "flat factor buys one unit per cent", productID: "acoesg", amount: 3_000_000, want: 3_000_000},
		{name: "minimum purchase", productID: "cobalto", amount: 1_000, want: 1_000},
		{name: "zero amount buys nothing", productID: "tbill", amount: 0, want: 0},
		{name: "negative amount buys nothing", productID: "tbill", amount: -5, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Units(tt.productID, tt.amount, SeedDay); got != tt.want {
				t.Fatalf("Units(%s, %d) = %d, want %d", tt.productID, tt.amount, got, tt.want)
			}
		})
	}
}

// The division path is what a scripted shock (story 13) will exercise; it
// rounds the exact quotient amount·den/num half up.
func TestFactor_UnitsRoundsHalfUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		f      factor
		amount int64
		want   int64
	}{
		{name: "flat", f: factor{num: 1, den: 1}, amount: 777, want: 777},
		{name: "half price doubles units", f: factor{num: 1, den: 2}, amount: 500, want: 1_000},
		{name: "exact half rounds up", f: factor{num: 2, den: 1}, amount: 3, want: 2},
		{name: "below half rounds down", f: factor{num: 3, den: 1}, amount: 4, want: 1},
		{name: "above half rounds up", f: factor{num: 3, den: 1}, amount: 5, want: 2},
		{name: "shocked price", f: factor{num: 93, den: 200}, amount: 1_000, want: 2_151},
		{name: "zero factor buys nothing", f: factor{num: 0, den: 1}, amount: 1_000, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.f.units(tt.amount); got != tt.want {
				t.Fatalf("factor %d/%d units(%d) = %d, want %d", tt.f.num, tt.f.den, tt.amount, got, tt.want)
			}
		})
	}
}
