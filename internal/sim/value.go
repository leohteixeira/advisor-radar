package sim

// SeedDay is the simulated day the seed describes. Until account-sim persists
// a global sim_day (advance-day), every read values positions at this day.
const SeedDay = 0

// factor is a product's price on one simulated day relative to day 0, as an
// exact ratio so values stay integer cents with no float drift.
type factor struct {
	num, den int64
}

// priceFactor is the scripted price path of a product. Every product is flat
// on every day for now; a scripted shock adds a case here keyed by product and
// day, never randomness or the wall clock.
func priceFactor(_ string, _ int) factor {
	return factor{num: 1, den: 1}
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
