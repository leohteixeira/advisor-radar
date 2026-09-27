package cases_test

import (
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/cases"
)

func TestTotalMinutes_SegmentsAndHalve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		segment  string
		factors  cases.ClockFactors
		expected int
	}{
		{name: "Essencial", segment: book.SegmentEssencial, expected: 1440},
		{name: "Advance", segment: book.SegmentAdvance, expected: 240},
		{name: "Singular", segment: book.SegmentSingular, expected: 60},
		{
			name:     "Advance with frustration 2",
			segment:  book.SegmentAdvance,
			factors:  cases.ClockFactors{Frustration: 2},
			expected: 120,
		},
		{
			name:     "Advance with churn risk",
			segment:  book.SegmentAdvance,
			factors:  cases.ClockFactors{ChurnRisk: true},
			expected: 120,
		},
		{
			name:     "Advance with human requested",
			segment:  book.SegmentAdvance,
			factors:  cases.ClockFactors{HumanRequested: true},
			expected: 120,
		},
		{
			name:     "Singular with relevant withdrawal",
			segment:  book.SegmentSingular,
			factors:  cases.ClockFactors{RelevantWithdrawal: true},
			expected: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := cases.TotalMinutes(tt.segment, tt.factors)
			if got != tt.expected {
				t.Fatalf("TotalMinutes = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestDisplayBand(t *testing.T) {
	t.Parallel()

	const total = 120
	tests := []struct {
		name      string
		remaining int
		expected  string
	}{
		{name: "100 left", remaining: 100, expected: cases.DisplayNoPrazo},
		{name: "30 left", remaining: 30, expected: cases.DisplayVencendo},
		{name: "0 left", remaining: 0, expected: cases.DisplayVencido},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := cases.DisplayBand(total, tt.remaining)
			if got != tt.expected {
				t.Fatalf("DisplayBand = %q, want %q", got, tt.expected)
			}
		})
	}
}
