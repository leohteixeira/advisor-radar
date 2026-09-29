package sim

// Scripted market shock (architecture.md "Simulated market day"): Cobalto
// Semicondutores trades at 46.5% of its day-0 price from shockDay on.
const (
	shockProduct = "cobalto"
	shockDay     = 3
)

// shockFactor is Cobalto's price from shockDay on, relative to day 0: a
// 53.5% fall.
var shockFactor = factor{num: 465, den: 1000}

// flat is the factor of every product and day the script does not move.
var flat = factor{num: 1, den: 1}

// factor is a product's price on one simulated day relative to day 0, as an
// exact ratio so values stay integer cents with no float drift.
type factor struct {
	num, den int64
}

// priceFactor is the scripted price path of a product: flat, except Cobalto
// from shockDay on. It is keyed only by product and day, never by randomness
// or the wall clock.
func priceFactor(productID string, day int) factor {
	if productID == shockProduct && day >= shockDay {
		return shockFactor
	}
	return flat
}

// Value is the market value in USD cents of unitsCents of productID on the
// simulated day: unitsCents times the product's factor for that day, rounded
// half away from zero. It is a pure function of its arguments, so equal
// holdings move equally.
func Value(productID string, unitsCents int64, day int) int64 {
	f := priceFactor(productID, day)
	scaled := unitsCents * f.num
	half := f.den / 2
	if scaled < 0 {
		return (scaled - half) / f.den
	}
	return (scaled + half) / f.den
}

// Units is how many day-0 cents amountCents buys of productID on the
// simulated day: amountCents divided by the product's factor for that day,
// rounded half up. With a flat factor it equals amountCents. A non-positive
// amount buys nothing.
func Units(productID string, amountCents int64, day int) int64 {
	return priceFactor(productID, day).units(amountCents)
}

// units divides amountCents by the factor, num/den, rounding half up: the
// exact quotient is amountCents·den/num.
func (f factor) units(amountCents int64) int64 {
	if amountCents <= 0 || f.num <= 0 {
		return 0
	}
	scaled := amountCents * f.den
	return (scaled + f.num/2) / f.num
}

// DayChangeBP is productID's price change from the day before day to day, in
// basis points, rounded half away from zero: −5350 for Cobalto on day 3, 0 on
// a flat day. Day 0 has no day before it, so its change is 0.
func DayChangeBP(productID string, day int) int {
	if day <= 0 {
		return 0
	}
	prev, cur := priceFactor(productID, day-1), priceFactor(productID, day)
	// cur/prev − 1 = (cur.num·prev.den − prev.num·cur.den) / (cur.den·prev.num)
	num := (cur.num*prev.den - prev.num*cur.den) * 10_000
	den := cur.den * prev.num
	half := den / 2
	if num < 0 {
		return int((num - half) / den)
	}
	return int((num + half) / den)
}
