// Package book holds segment bounds and the customer book shape.
package book

// Segment bounds in USD. Essencial is inclusive at the upper bound;
// Advance is exclusive below and inclusive at the Singular bound.
const (
	BoundEssencialMax = 10_000
	BoundAdvanceMax   = 200_000
)

// Segment names match the product labels.
const (
	SegmentEssencial = "Essencial"
	SegmentAdvance   = "Advance"
	SegmentSingular  = "Singular"
)

// Client is one book row. Cast rows live in advisory SQL seeds, not here.
type Client struct {
	ID        string
	Name      string
	Segment   string
	AUM       float64
	AdvisorID string
	Advisor   string
	Since     string
}

// SegmentFromAssets maps assets in USD to Essencial, Advance, or Singular.
// Essencial is up to BoundEssencialMax inclusive; Advance is above that through
// BoundAdvanceMax inclusive; Singular is above BoundAdvanceMax.
func SegmentFromAssets(assets float64) string {
	switch {
	case assets <= BoundEssencialMax:
		return SegmentEssencial
	case assets <= BoundAdvanceMax:
		return SegmentAdvance
	default:
		return SegmentSingular
	}
}
