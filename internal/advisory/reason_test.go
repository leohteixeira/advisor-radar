package advisory_test

import (
	"math"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/advisory"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

func TestReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		decision advisory.Decision
		expected string
	}{
		{
			name:     "withdrawal with its share of the assets",
			decision: advisory.Decision{Kind: advisory.KindSaque, Amount: 190_000, Before: 640_000, After: 450_000},
			expected: "Saque de US$ 190.000,00, 30% do patrimônio",
		},
		{
			name:     "drop with the share and both ends",
			decision: advisory.Decision{Kind: advisory.KindQueda, Amount: -22_000, Before: 130_000, After: 108_000},
			expected: "Patrimônio caiu 16,9%, de US$ 130.000,00 para US$ 108.000,00",
		},
		{
			name:     "deposit with its multiple of the assets",
			decision: advisory.Decision{Kind: advisory.KindAporte, Amount: 60_000, Before: 8_000, After: 68_000},
			expected: "Aporte de US$ 60.000,00, 7,5× o patrimônio",
		},
		{
			name:     "deposit with a whole multiple",
			decision: advisory.Decision{Kind: advisory.KindAporte, Amount: 16_000, Before: 8_000, After: 24_000},
			expected: "Aporte de US$ 16.000,00, 2× o patrimônio",
		},
		{
			name:     "deposit into an empty account has no multiple",
			decision: advisory.Decision{Kind: advisory.KindAporte, Amount: 500.5, Before: 0, After: 500.5},
			expected: "Aporte de US$ 500,50",
		},
		{
			name:     "segment upgrade",
			decision: advisory.Decision{Kind: advisory.KindSegmento, Before: 8_000, After: 68_000, From: "Essencial", To: "Advance"},
			expected: "Subiu de Essencial para Advance",
		},
		{
			name:     "segment downgrade",
			decision: advisory.Decision{Kind: advisory.KindSegmento, Before: 68_000, After: 9_000, From: "Advance", To: "Essencial"},
			expected: "Desceu de Advance para Essencial",
		},
		{
			name:     "silence",
			decision: advisory.Decision{Kind: advisory.KindContato, Days: 94},
			expected: "Último contato há 94 dias",
		},
		{
			name:     "purchase above the profile",
			decision: advisory.Decision{Kind: advisory.KindPerfil, Amount: 250, Risk: 5, Profile: advisory.ProfileConservador, MaxRisk: 2},
			expected: "Compra de US$ 250,00 em produto de risco 5. Perfil conservador vai até risco 2.",
		},
		{
			name:     "unknown kind has no reason",
			decision: advisory.Decision{Kind: "outro", Amount: 10},
			expected: "",
		},
		{
			name:     "amount that is not finite has no reason",
			decision: advisory.Decision{Kind: advisory.KindSaque, Amount: math.Inf(1), Before: 10},
			expected: "",
		},
		{
			name:     "segment without its ends has no reason",
			decision: advisory.Decision{Kind: advisory.KindSegmento, Before: 1, After: 2},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := advisory.Reason(tt.decision); got != tt.expected {
				t.Fatalf("Reason = %q, want %q", got, tt.expected)
			}
		})
	}
}

// Every decision the account rules produce carries a description: a
// withdrawal that drops a client one segment yields both alerts, each with
// its own text.
func TestReason_EveryAccountDecisionHasOne(t *testing.T) {
	t.Parallel()

	facts := []sim.AccountPayload{
		{Kind: "withdrawal", Amount: 59_000, Before: 68_000, After: 9_000},
		{Kind: "deposit", Amount: 60_000, Before: 8_000, After: 68_000},
		{Kind: sim.KindReavaliacao, Amount: -30_000, Before: 130_000, After: 100_000},
	}
	seen := make(map[string]string)
	for _, fact := range facts {
		for _, d := range advisory.EvaluateAccount(fact) {
			reason := advisory.Reason(d)
			if reason == "" {
				t.Fatalf("%s decision of a %s has no reason", d.Kind, fact.Kind)
			}
			seen[fact.Kind+"/"+d.Kind] = reason
		}
	}
	want := map[string]string{
		"withdrawal/saque":    "Saque de US$ 59.000,00, 87% do patrimônio",
		"withdrawal/segmento": "Desceu de Advance para Essencial",
		"deposit/aporte":      "Aporte de US$ 60.000,00, 7,5× o patrimônio",
		"deposit/segmento":    "Subiu de Essencial para Advance",
		"reavaliacao/queda":   "Patrimônio caiu 23,1%, de US$ 130.000,00 para US$ 100.000,00",
	}
	for key, expected := range want {
		if seen[key] != expected {
			t.Errorf("%s reason = %q, want %q", key, seen[key], expected)
		}
	}
}
