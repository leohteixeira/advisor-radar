package advisory_test

import (
	"errors"
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
