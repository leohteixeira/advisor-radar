package advisory_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/advisory"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// A purchase leaves patrimony unchanged, so the account rules stay quiet.
func TestEvaluateAccount_AplicacaoIsQuiet(t *testing.T) {
	t.Parallel()
	got := advisory.EvaluateAccount(sim.AccountPayload{
		Kind: sim.KindAplicacao, Amount: 30_000, Before: 68_000, After: 68_000,
		ProductID: "acoesg", AssetClass: sim.ClassETFs, Risk: 3,
	})
	if len(got) != 0 {
		t.Fatalf("decisions = %+v, want none", got)
	}
}

// The drop rule runs on a negative reavaliacao above 15% of before; money is
// in dollars, as Apply scales it.
func TestEvaluateAccount_Reavaliacao(t *testing.T) {
	t.Parallel()
	drop := advisory.Decision{
		RuleKey: advisory.RuleKeyDrop, Kind: advisory.KindQueda, Rule: "Queda acima de 15% em 5 dias úteis",
		Amount: -38_520, Before: 248_300, After: 209_780,
	}
	tests := []struct {
		name     string
		payload  sim.AccountPayload
		expected []advisory.Decision
	}{
		{
			name:     "mariana loses 15.5% on day 3",
			payload:  sim.AccountPayload{Kind: sim.KindReavaliacao, Amount: -38_520, Before: 248_300, After: 209_780, SimDay: 3},
			expected: []advisory.Decision{drop},
		},
		{
			name:    "thiago loses 1.6%",
			payload: sim.AccountPayload{Kind: sim.KindReavaliacao, Amount: -1_091.40, Before: 68_000, After: 66_908.60, SimDay: 3},
		},
		{
			name:    "a flat day",
			payload: sim.AccountPayload{Kind: sim.KindReavaliacao, Before: 8_200, After: 8_200, SimDay: 1},
		},
		{
			name:    "a gain above 15% is not a drop",
			payload: sim.AccountPayload{Kind: sim.KindReavaliacao, Amount: 20_000, Before: 60_000, After: 80_000, SimDay: 2},
		},
		{
			name:    "exactly 15% is not above",
			payload: sim.AccountPayload{Kind: sim.KindReavaliacao, Amount: -15, Before: 100, After: 85, SimDay: 2},
		},
		{
			// 1,23/8,20 is 0.15000000000000002 in float64; in cents 123·20 = 820·3.
			name:    "exactly 15% in cents, above in float division, is not above",
			payload: sim.AccountPayload{Kind: sim.KindReavaliacao, Amount: -1.23, Before: 8.20, After: 6.97, SimDay: 2},
		},
		{
			name:    "one cent above 15%",
			payload: sim.AccountPayload{Kind: sim.KindReavaliacao, Amount: -1.24, Before: 8.20, After: 6.96, SimDay: 2},
			expected: []advisory.Decision{
				{RuleKey: advisory.RuleKeyDrop, Kind: advisory.KindQueda, Rule: advisory.RuleDrop, Amount: -1.24, Before: 8.20, After: 6.96},
			},
		},
		{
			name:    "money beyond exact cents raises nothing",
			payload: sim.AccountPayload{Kind: sim.KindReavaliacao, Amount: -1e17, Before: 2e17, After: 1e17, SimDay: 2},
		},
		{
			name:    "a drop that crosses a segment also moves it",
			payload: sim.AccountPayload{Kind: sim.KindReavaliacao, Amount: -4_000, Before: 12_000, After: 8_000, SimDay: 3},
			expected: []advisory.Decision{
				{RuleKey: advisory.RuleKeyDrop, Kind: advisory.KindQueda, Rule: advisory.RuleDrop, Amount: -4_000, Before: 12_000, After: 8_000},
				{
					RuleKey: advisory.RuleKeySegment, Kind: advisory.KindSegmento, Rule: advisory.RuleSegment,
					Before: 12_000, After: 8_000, From: "Advance", To: "Essencial",
				},
			},
		},
		{
			name:     "the phase-1 asset_drop path is unchanged",
			payload:  sim.AccountPayload{Kind: "asset_drop", Amount: 38_520, Before: 248_300, After: 209_780},
			expected: []advisory.Decision{{RuleKey: advisory.RuleKeyDrop, Kind: advisory.KindQueda, Rule: advisory.RuleDrop, Amount: 38_520, Before: 248_300, After: 209_780}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := advisory.EvaluateAccount(tt.payload)
			if !slices.Equal(got, tt.expected) {
				t.Fatalf("decisions = %+v, want %+v", got, tt.expected)
			}
		})
	}
}

func TestEvaluateSuitability(t *testing.T) {
	t.Parallel()

	purchase := func(risk int) sim.AccountPayload {
		return sim.AccountPayload{
			Kind: sim.KindAplicacao, Amount: 1_000, Before: 8_200, After: 8_200,
			ProductID: "p", AssetClass: sim.ClassAcoes, Risk: risk,
		}
	}
	tests := []struct {
		name    string
		payload sim.AccountPayload
		profile string
		fires   bool
		max     int
		wantErr error
	}{
		{name: "conservador buys risk 5", payload: purchase(5), profile: advisory.ProfileConservador, fires: true, max: 2},
		{name: "conservador buys risk 3", payload: purchase(3), profile: advisory.ProfileConservador, fires: true, max: 2},
		{name: "conservador buys risk 2", payload: purchase(2), profile: advisory.ProfileConservador},
		{name: "moderado buys risk 4", payload: purchase(4), profile: advisory.ProfileModerado, fires: true, max: 3},
		{name: "moderado buys risk 3", payload: purchase(3), profile: advisory.ProfileModerado},
		{name: "arrojado buys risk 5", payload: purchase(5), profile: advisory.ProfileArrojado},
		{
			name:    "a deposit never fires",
			payload: sim.AccountPayload{Kind: "aporte", Amount: 1_000, Before: 8_200, After: 9_200, Risk: 5},
			profile: advisory.ProfileConservador,
		},
		{name: "unknown profile is an error", payload: purchase(5), profile: "agressivo", wantErr: advisory.ErrUnknownProfile},
		{name: "risk 0 is invalid", payload: purchase(0), profile: advisory.ProfileArrojado, wantErr: advisory.ErrInvalidPurchase},
		{name: "risk 6 is invalid", payload: purchase(6), profile: advisory.ProfileArrojado, wantErr: advisory.ErrInvalidPurchase},
		{name: "negative risk is invalid", payload: purchase(-1), profile: advisory.ProfileConservador, wantErr: advisory.ErrInvalidPurchase},
		{
			name: "empty product_id is invalid",
			payload: sim.AccountPayload{
				Kind: sim.KindAplicacao, Amount: 1_000, Before: 8_200, After: 8_200, AssetClass: sim.ClassAcoes, Risk: 1,
			},
			profile: advisory.ProfileConservador,
			wantErr: advisory.ErrInvalidPurchase,
		},
		{name: "risk 1 is valid", payload: purchase(1), profile: advisory.ProfileConservador},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, ok, err := advisory.EvaluateSuitability(tt.payload, tt.profile)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || ok {
					t.Fatalf("got (%+v, %v, %v), want %v", d, ok, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("EvaluateSuitability: %v", err)
			}
			if ok != tt.fires {
				t.Fatalf("fires = %v, want %v", ok, tt.fires)
			}
			if !ok {
				return
			}
			want := advisory.Decision{
				RuleKey: advisory.RuleKeySuitability, Kind: advisory.KindPerfil, Rule: "Compra acima do perfil de investidor",
				Amount: 1_000, Before: 8_200, After: 8_200,
				ProductID: "p", AssetClass: sim.ClassAcoes, Risk: tt.payload.Risk, Profile: tt.profile, MaxRisk: tt.max,
			}
			if d != want {
				t.Fatalf("decision = %+v, want %+v", d, want)
			}
		})
	}
}
