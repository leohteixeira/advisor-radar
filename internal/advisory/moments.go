package advisory

import (
	"math"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/event"
)

// Moment thresholds in integer USD cents. essencialMaxCents is the bound
// book.SegmentFromAssets uses: at or below it a client is still Essencial.
const (
	upgradeNearMinCents = 750_000
	essencialMaxCents   = book.BoundEssencialMax * 100
)

// upgradeWindow is how long a live segment upgrade keeps the home on the
// segment_upgraded moment.
const upgradeWindow = 24 * time.Hour

// Balance is the account-sim balance of one client in integer USD cents.
// Patrimony is positions at market value plus cash. SimDay is the global
// simulated day account-sim values the positions at.
type Balance struct {
	PatrimonyCents int64
	CashCents      int64
	SimDay         int
}

// SegmentAlert is one segmento alert of a client: the segments it moved
// between, the schema version of the account event that raised it (0 when
// unknown, as for seeded alerts), and when it was raised.
type SegmentAlert struct {
	From          string
	To            string
	SchemaVersion int
	RaisedAt      time.Time
}

// MomentFacts are the home moment conditions of one client. Each is
// evaluated here so the BFF only orders them; money is integer USD cents.
type MomentFacts struct {
	SegmentUpgraded    bool
	UpgradedSegment    string
	SegmentUpgradeNear bool
	UpgradeGapCents    int64
	IdleCash           bool
	CashCents          int64
	PatrimonyCents     int64
	PortfolioReview    bool
	// PortfolioDrop holds when the latest revaluation is for the current
	// simulated day and lost more than 15% of the patrimony before it.
	// DropBP is that loss in positive basis points of before, DropProductID
	// and DropProductBP the product that moved the most and its signed day
	// change in basis points, and DropDay the simulated day. All four are
	// zero when PortfolioDrop is false.
	PortfolioDrop bool
	DropBP        int
	DropProductID string
	DropProductBP int
	DropDay       int
}

// MomentInput is everything the moment facts are evaluated from.
// Revaluation is the client's latest reavaliacao, nil when there is none.
type MomentInput struct {
	Segment     string
	Balance     Balance
	Alerts      []SegmentAlert
	Revaluation *Revaluation
	Now         time.Time
}

// EvaluateMoments applies one pure rule per moment fact.
func EvaluateMoments(in MomentInput) MomentFacts {
	upgraded, upgradedTo := segmentUpgraded(in.Segment, in.Alerts, in.Now)
	near, gap := segmentUpgradeNear(in.Segment, in.Balance)
	f := MomentFacts{
		SegmentUpgraded:    upgraded,
		UpgradedSegment:    upgradedTo,
		SegmentUpgradeNear: near,
		UpgradeGapCents:    gap,
		IdleCash:           idleCash(in.Balance),
		CashCents:          in.Balance.CashCents,
		PatrimonyCents:     in.Balance.PatrimonyCents,
		PortfolioReview:    portfolioReview(in.Segment),
	}
	if rev, ok := portfolioDrop(in.Revaluation, in.Balance.SimDay); ok {
		f.PortfolioDrop = true
		f.DropBP = lossBP(rev.AmountCents, rev.BeforeCents)
		f.DropProductID = rev.ProductID
		f.DropProductBP = rev.ProductChangeBP
		f.DropDay = rev.SimDay
	}
	return f
}

// portfolioDrop holds when rev is for simDay, the current simulated day
// account-sim reports, and its loss is above 15% of before
// (dropAboveThreshold, the predicate of the drop rule).
// A revaluation of an earlier day, a gain, or a flat day never holds, so the
// moment ends on the next advance.
func portfolioDrop(rev *Revaluation, simDay int) (Revaluation, bool) {
	if rev == nil || rev.SimDay != simDay || rev.AmountCents >= 0 {
		return Revaluation{}, false
	}
	if !dropAboveThreshold(-rev.AmountCents, rev.BeforeCents) {
		return Revaluation{}, false
	}
	return *rev, true
}

// lossBP is the loss −amount as positive basis points of before, rounded half
// away from zero. before is positive.
func lossBP(amount, before int64) int {
	return int(math.Round(float64(-amount) * 10_000 / float64(before)))
}

// segmentUpgraded looks at the client's most recent live segment change: an
// alert raised in the last 24 h from an account event of schema version 2 or
// later. It holds when that change moved the client up to the segment the
// book holds now, and returns that segment. Seeded alerts (version 1 or
// unknown) never count, a later live downgrade cancels an earlier upgrade,
// and a reseed that moved the book back cancels it too.
func segmentUpgraded(segment string, alerts []SegmentAlert, now time.Time) (bool, string) {
	var latest SegmentAlert
	found := false
	for _, a := range alerts {
		isLive := a.SchemaVersion >= event.SchemaVersionCents
		isRecent := !a.RaisedAt.After(now) && now.Sub(a.RaisedAt) < upgradeWindow
		if !isLive || !isRecent {
			continue
		}
		if !found || a.RaisedAt.After(latest.RaisedAt) {
			latest, found = a, true
		}
	}
	if !found {
		return false, ""
	}
	from, to := segmentRank(latest.From), segmentRank(latest.To)
	if from == 0 || to <= from || latest.To != segment {
		return false, ""
	}
	return true, latest.To
}

// segmentRank orders the segments; an unknown segment is 0.
func segmentRank(segment string) int {
	switch segment {
	case book.SegmentEssencial:
		return 1
	case book.SegmentAdvance:
		return 2
	case book.SegmentSingular:
		return 3
	default:
		return 0
	}
}

// segmentUpgradeNear holds for an Essencial client whose patrimony is at
// least 750000 cents and still inside Essencial by book.SegmentFromAssets,
// and returns what is missing to US$ 10.000,00. A client exactly at that
// bound is still Essencial and one cent away, so the gap is at least 1.
func segmentUpgradeNear(segment string, b Balance) (bool, int64) {
	if segment != book.SegmentEssencial || b.PatrimonyCents < upgradeNearMinCents {
		return false, 0
	}
	if book.SegmentFromAssets(float64(b.PatrimonyCents)/100) != book.SegmentEssencial {
		return false, 0
	}
	return true, max(essencialMaxCents-b.PatrimonyCents, 1)
}

// idleCash holds when patrimony is above 0 and cash is at least half of it.
// cash ≥ patrimony − cash is cash·2 ≥ patrimony without overflowing.
func idleCash(b Balance) bool {
	return b.PatrimonyCents > 0 && b.CashCents >= b.PatrimonyCents-b.CashCents
}

// portfolioReview holds for Singular clients.
func portfolioReview(segment string) bool {
	return segment == book.SegmentSingular
}
