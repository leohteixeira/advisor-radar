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
			if got := Units(tt.productID, tt.amount, 0); got != tt.want {
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

func TestPriceFactor_ScriptedShock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		productID string
		day       int
		want      factor
	}{
		{name: "cobalto before the shock", productID: "cobalto", day: 2, want: factor{num: 1, den: 1}},
		{name: "cobalto on the shock day", productID: "cobalto", day: 3, want: factor{num: 465, den: 1000}},
		{name: "cobalto stays down", productID: "cobalto", day: 40, want: factor{num: 465, den: 1000}},
		{name: "another stock is flat", productID: "farol", day: 3, want: factor{num: 1, den: 1}},
		{name: "unknown product is flat", productID: "nope", day: 3, want: factor{num: 1, den: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := priceFactor(tt.productID, tt.day); got != tt.want {
				t.Fatalf("priceFactor(%s, %d) = %+v, want %+v", tt.productID, tt.day, got, tt.want)
			}
		})
	}
}

// The story 4 deferral: rounding is exercised with the real Cobalto factor.
func TestValue_RoundsHalfAwayFromZeroWithTheShockFactor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		units int64
		day   int
		want  int64
	}{
		{name: "mariana cobalto on day 3", units: 7_200_000, day: 3, want: 3_348_000},
		{name: "thiago cobalto on day 3", units: 204_000, day: 3, want: 94_860},
		{name: "mariana cobalto on day 2", units: 7_200_000, day: 2, want: 7_200_000},
		{name: "exact half rounds up", units: 100, day: 3, want: 47},
		{name: "exact negative half rounds down", units: -100, day: 3, want: -47},
		{name: "below half rounds down", units: 3, day: 3, want: 1},
		{name: "above half rounds up", units: 5, day: 3, want: 2},
		{name: "one unit rounds to zero", units: 1, day: 3, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Value("cobalto", tt.units, tt.day); got != tt.want {
				t.Fatalf("Value(cobalto, %d, %d) = %d, want %d", tt.units, tt.day, got, tt.want)
			}
		})
	}
}

func TestUnits_AtTheShockPrice(t *testing.T) {
	t.Parallel()
	// US$ 10,00 buys 1000/0.465 = 2150.54 day-0 cents, rounded to 2151.
	if got := Units("cobalto", 1_000, 3); got != 2_151 {
		t.Fatalf("Units(cobalto, 1000, 3) = %d, want 2151", got)
	}
	if got := Units("cobalto", 1_000, 2); got != 1_000 {
		t.Fatalf("Units(cobalto, 1000, 2) = %d, want 1000", got)
	}
}

func TestDayChangeBP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		productID string
		day       int
		want      int
	}{
		{name: "day 0 has no day before it", productID: "cobalto", day: 0, want: 0},
		{name: "negative day", productID: "cobalto", day: -1, want: 0},
		{name: "flat day", productID: "cobalto", day: 2, want: 0},
		{name: "shock day", productID: "cobalto", day: 3, want: -5350},
		{name: "day after the shock is flat", productID: "cobalto", day: 4, want: 0},
		{name: "flat product", productID: "acoesg", day: 3, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := DayChangeBP(tt.productID, tt.day); got != tt.want {
				t.Fatalf("DayChangeBP(%s, %d) = %d, want %d", tt.productID, tt.day, got, tt.want)
			}
		})
	}
}
