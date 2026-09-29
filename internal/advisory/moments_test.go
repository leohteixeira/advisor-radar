package advisory

import (
	"cmp"
	"testing"
	"time"
)

var momentNow = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

func TestSegmentUpgraded(t *testing.T) {
	t.Parallel()
	live := func(from, to string, ago time.Duration) SegmentAlert {
		return SegmentAlert{From: from, To: to, SchemaVersion: 2, RaisedAt: momentNow.Add(-ago)}
	}
	tests := []struct {
		name     string
		book     string
		alerts   []SegmentAlert
		expected bool
		segment  string
	}{
		{name: "no alerts"},
		{
			name:   "reseed moved the book back",
			book:   "Essencial",
			alerts: []SegmentAlert{live("Essencial", "Advance", 2*time.Minute)},
		},
		{
			name:   "book moved on since the upgrade",
			book:   "Singular",
			alerts: []SegmentAlert{live("Essencial", "Advance", 2*time.Minute)},
		},
		{
			name:     "live upgrade minutes ago",
			alerts:   []SegmentAlert{live("Essencial", "Advance", 2*time.Minute)},
			expected: true, segment: "Advance",
		},
		{
			name:     "live upgrade to singular",
			alerts:   []SegmentAlert{live("Advance", "Singular", 23*time.Hour)},
			expected: true, segment: "Singular",
		},
		{
			name:   "seeded upgrade is schema version 1",
			alerts: []SegmentAlert{{From: "Essencial", To: "Advance", SchemaVersion: 1, RaisedAt: momentNow.Add(-4 * time.Hour)}},
		},
		{
			name:   "seeded upgrade has no schema version",
			alerts: []SegmentAlert{{From: "Essencial", To: "Advance", RaisedAt: momentNow.Add(-238 * time.Minute)}},
		},
		{
			name:   "upgrade 24 h ago is over",
			alerts: []SegmentAlert{live("Essencial", "Advance", 24*time.Hour)},
		},
		{
			name:   "upgrade in the future is ignored",
			alerts: []SegmentAlert{live("Essencial", "Advance", -time.Minute)},
		},
		{
			name:   "downgrade",
			alerts: []SegmentAlert{live("Advance", "Essencial", time.Hour)},
		},
		{
			name:   "same segment",
			alerts: []SegmentAlert{live("Advance", "Advance", time.Hour)},
		},
		{
			name:   "unknown from segment",
			alerts: []SegmentAlert{live("", "Advance", time.Hour)},
		},
		{
			name:   "later downgrade cancels the upgrade",
			alerts: []SegmentAlert{live("Advance", "Essencial", time.Hour), live("Essencial", "Advance", 2*time.Hour)},
		},
		{
			name:     "later upgrade after a downgrade",
			alerts:   []SegmentAlert{live("Advance", "Essencial", 2*time.Hour), live("Essencial", "Advance", time.Hour)},
			expected: true, segment: "Advance",
		},
		{
			name: "seeded alert after a live upgrade does not cancel it",
			alerts: []SegmentAlert{
				{From: "Advance", To: "Essencial", SchemaVersion: 1, RaisedAt: momentNow.Add(-time.Minute)},
				live("Essencial", "Advance", time.Hour),
			},
			expected: true, segment: "Advance",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Without a stated book, the book holds the expected segment,
			// or Advance when no upgrade is expected.
			book := cmp.Or(tt.book, tt.segment, "Advance")
			got, segment := segmentUpgraded(book, tt.alerts, momentNow)
			if got != tt.expected || segment != tt.segment {
				t.Errorf("segmentUpgraded = %t %q, want %t %q", got, segment, tt.expected, tt.segment)
			}
		})
	}
}

func TestSegmentUpgradeNear(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		segment   string
		patrimony int64
		expected  bool
		gap       int64
	}{
		{name: "fernanda day 0", patrimony: 820_000, expected: true, gap: 180_000},
		{name: "lower bound", patrimony: 750_000, expected: true, gap: 250_000},
		{name: "one cent below the lower bound", patrimony: 749_999},
		{name: "one cent below the essencial bound", patrimony: 999_999, expected: true, gap: 1},
		{name: "at the essencial bound is still essencial", patrimony: 1_000_000, expected: true, gap: 1},
		{name: "one cent above the bound is advance", patrimony: 1_000_001},
		{name: "fernanda after the deposit", patrimony: 1_820_000},
		{name: "empty", patrimony: 0},
		{name: "advance client in the band", segment: "Advance", patrimony: 820_000},
		{name: "singular client in the band", segment: "Singular", patrimony: 999_999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, gap := segmentUpgradeNear(cmp.Or(tt.segment, "Essencial"), Balance{PatrimonyCents: tt.patrimony})
			if got != tt.expected || gap != tt.gap {
				t.Errorf("segmentUpgradeNear(%d) = %t %d, want %t %d", tt.patrimony, got, gap, tt.expected, tt.gap)
			}
		})
	}
}

func TestIdleCash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		balance  Balance
		expected bool
	}{
		{name: "thiago day 0", balance: Balance{CashCents: 6_052_000, PatrimonyCents: 6_800_000}, expected: true},
		{name: "exactly half", balance: Balance{CashCents: 500, PatrimonyCents: 1_000}, expected: true},
		{name: "one cent below half", balance: Balance{CashCents: 499, PatrimonyCents: 999}},
		{name: "half of an odd patrimony rounds up", balance: Balance{CashCents: 500, PatrimonyCents: 999}, expected: true},
		{name: "fernanda day 0", balance: Balance{CashCents: 114_800, PatrimonyCents: 820_000}},
		{name: "mariana day 0", balance: Balance{CashCents: 6_000_000, PatrimonyCents: 24_830_000}},
		{name: "all cash", balance: Balance{CashCents: 100, PatrimonyCents: 100}, expected: true},
		{name: "empty account", balance: Balance{}},
		{name: "thiago after buying 30000", balance: Balance{CashCents: 3_052_000, PatrimonyCents: 6_800_000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := idleCash(tt.balance); got != tt.expected {
				t.Errorf("idleCash(%+v) = %t, want %t", tt.balance, got, tt.expected)
			}
		})
	}
}

func TestPortfolioReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		segment  string
		expected bool
	}{
		{segment: "Singular", expected: true},
		{segment: "Advance"},
		{segment: "Essencial"},
		{segment: ""},
	}
	for _, tt := range tests {
		t.Run("segment "+tt.segment, func(t *testing.T) {
			t.Parallel()
			if got := portfolioReview(tt.segment); got != tt.expected {
				t.Errorf("portfolioReview(%q) = %t, want %t", tt.segment, got, tt.expected)
			}
		})
	}
}

// TestEvaluateMoments covers the seed clients on day 0 and the two live
// changes of the demo.
func TestEvaluateMoments(t *testing.T) {
	t.Parallel()
	thiagoSeededUpgrade := SegmentAlert{From: "Essencial", To: "Advance", RaisedAt: momentNow.Add(-238 * time.Minute)}
	tests := []struct {
		name     string
		in       MomentInput
		expected MomentFacts
	}{
		{
			name: "fernanda day 0",
			in:   MomentInput{Segment: "Essencial", Balance: Balance{PatrimonyCents: 820_000, CashCents: 114_800}},
			expected: MomentFacts{
				SegmentUpgradeNear: true, UpgradeGapCents: 180_000,
				CashCents: 114_800, PatrimonyCents: 820_000,
			},
		},
		{
			name: "thiago day 0 with the seeded upgrade",
			in: MomentInput{
				Segment: "Advance", Balance: Balance{PatrimonyCents: 6_800_000, CashCents: 6_052_000},
				Alerts: []SegmentAlert{thiagoSeededUpgrade},
			},
			expected: MomentFacts{IdleCash: true, CashCents: 6_052_000, PatrimonyCents: 6_800_000},
		},
		{
			name: "mariana day 0",
			in:   MomentInput{Segment: "Singular", Balance: Balance{PatrimonyCents: 24_830_000, CashCents: 6_000_000}},
			expected: MomentFacts{
				PortfolioReview: true, CashCents: 6_000_000, PatrimonyCents: 24_830_000,
			},
		},
		{
			name: "fernanda reseeded within 24 h of a live upgrade",
			in: MomentInput{
				Segment: "Essencial", Balance: Balance{PatrimonyCents: 820_000, CashCents: 114_800},
				Alerts: []SegmentAlert{{From: "Essencial", To: "Advance", SchemaVersion: 2, RaisedAt: momentNow.Add(-time.Hour)}},
			},
			expected: MomentFacts{
				SegmentUpgradeNear: true, UpgradeGapCents: 180_000,
				CashCents: 114_800, PatrimonyCents: 820_000,
			},
		},
		{
			name: "fernanda after a 10000 deposit",
			in: MomentInput{
				Segment: "Advance", Balance: Balance{PatrimonyCents: 1_820_000, CashCents: 1_114_800},
				Alerts: []SegmentAlert{{From: "Essencial", To: "Advance", SchemaVersion: 2, RaisedAt: momentNow.Add(-time.Minute)}},
			},
			expected: MomentFacts{
				SegmentUpgraded: true, UpgradedSegment: "Advance",
				IdleCash: true, CashCents: 1_114_800, PatrimonyCents: 1_820_000,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := tt.in
			in.Now = momentNow
			if got := EvaluateMoments(in); got != tt.expected {
				t.Errorf("EvaluateMoments = %+v, want %+v", got, tt.expected)
			}
		})
	}
}
