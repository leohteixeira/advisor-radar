// Package cases owns case state, SLA clocks, and TTL-based escalation.
package cases

import "time"

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

// halvedTotals are the totals TotalMinutes yields when the clock is halved.
// No halved total equals a base total, so a stored total says on its own
// whether the case's clock was halved (ADR 0012).
var halvedTotals = map[int]struct{}{
	BaseEssencial / 2: {},
	BaseAdvance / 2:   {},
	BaseSingular / 2:  {},
}

// RetotalMinutes is the SLA total of a case whose total was current after its
// client moved to segment: the segment's base, halved when current is a
// halved total, because the halving factors belong to what opened the case.
// An unknown segment yields 0.
func RetotalMinutes(current int, segment string) int {
	base := BaseMinutes(segment)
	if IsHalved(current) {
		return base / 2
	}
	return base
}

// IsHalved reports whether total is a halved SLA total.
func IsHalved(total int) bool {
	_, halved := halvedTotals[total]
	return halved
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

// SLA states of a case, as every screen names them. They are decided here
// only: the queue card, the queue SLA filter and the manager panel all read
// the state cases reports.
const (
	SLAStateOnTime  = "No prazo"
	SLAStateDueSoon = "Vencendo"
	SLAStateOverdue = "Vencido"
	SLAStateDone    = "Concluído"
)

// SLAState is the SLA state of a case at now. A resolved case is done. An
// open one is overdue once no time is left, due soon while less than a third
// of the SLA (and never less than 20 minutes) is left, and on time otherwise.
func SLAState(row CaseRow, now time.Time) string {
	if row.State == StateResolvido {
		return SLAStateDone
	}
	remaining := RemainingMinutes(row, now)
	switch {
	case remaining < 0:
		return SLAStateOverdue
	case remaining < max(row.SLATotalMinutes*33/100, 20):
		return SLAStateDueSoon
	default:
		return SLAStateOnTime
	}
}
