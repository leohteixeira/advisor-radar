// Package cases owns case state, SLA clocks, and TTL-based escalation.
package cases

import "github.com/leohteixeira/advisor-radar/internal/book"

// Base SLA minutes by segment.
const (
	BaseEssencial = 1440
	BaseAdvance   = 240
	BaseSingular  = 60
)

// Display band labels shown to the advisor.
const (
	DisplayNoPrazo  = "No prazo"
	DisplayVencendo = "Vencendo"
	DisplayVencido  = "Vencido"
)

// FrustrationFrustrado is the lowest score that halves the clock
// (Frustrado or Muito frustrado).
const FrustrationFrustrado = 2

// ClockFactors are the boolean and score inputs that may halve the SLA.
type ClockFactors struct {
	ChurnRisk          bool
	Frustration        int
	HumanRequested     bool
	RelevantWithdrawal bool
}

// BaseMinutes returns the segment base SLA in minutes.
func BaseMinutes(segment string) int {
	switch segment {
	case book.SegmentEssencial:
		return BaseEssencial
	case book.SegmentAdvance:
		return BaseAdvance
	case book.SegmentSingular:
		return BaseSingular
	default:
		return 0
	}
}

// TotalMinutes returns the SLA total after applying the halving rule.
func TotalMinutes(segment string, f ClockFactors) int {
	base := BaseMinutes(segment)
	if shouldHalve(f) {
		return base / 2
	}
	return base
}

func shouldHalve(f ClockFactors) bool {
	return f.ChurnRisk ||
		f.Frustration >= FrustrationFrustrado ||
		f.HumanRequested ||
		f.RelevantWithdrawal
}

// DueThreshold is max(33% of total, 20 minutes). Remaining at or above
// this band is "No prazo".
func DueThreshold(totalMinutes int) int {
	pct := (totalMinutes * 33) / 100
	if pct < 20 {
		return 20
	}
	return pct
}

// DisplayBand maps remaining minutes against the total clock.
func DisplayBand(totalMinutes, remainingMinutes int) string {
	if remainingMinutes <= 0 {
		return DisplayVencido
	}
	if remainingMinutes >= DueThreshold(totalMinutes) {
		return DisplayNoPrazo
	}
	return DisplayVencendo
}
