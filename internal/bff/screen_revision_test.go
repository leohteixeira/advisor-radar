package bff_test

import (
	"context"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// switchablePreferences fails the preferences read while down is set.
type switchablePreferences struct {
	bff.POVSource
	down *atomic.Bool
}

func (p switchablePreferences) Preferences(ctx context.Context, id string) (bff.POVPreferences, error) {
	if p.down.Load() {
		return bff.POVPreferences{}, errAccountSimDown
	}
	return p.POVSource.Preferences(ctx, id)
}

func homeOf(t *testing.T, h http.Handler, customerID string) screenBody {
	t.Helper()
	rr, body := getScreen(t, h, customerID, "home")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET home = %d %s", rr.Code, rr.Body.String())
	}
	return body
}

// TestGetScreen_HomeRevisionFollowsBeta stores the beta flag through the
// preferences route and checks each seed client's home: v2 with the profile
// rail right after moment in the beta, v1 outside it.
func TestGetScreen_HomeRevisionFollowsBeta(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		id       string
		rail     string
		title    string
		products []string
	}{
		{name: "fernanda", id: sim.CustomerFernanda, rail: "product_rail/profile_conservador", title: "Para o seu perfil conservador", products: []string{"tbill", "corp"}},
		{name: "thiago", id: sim.CustomerThiago, rail: "product_rail/profile_arrojado", title: "Para o seu perfil arrojado", products: []string{"cobalto", "acoesg"}},
		{name: "mariana", id: sim.CustomerMariana, rail: "product_rail/profile_moderado", title: "Para o seu perfil moderado", products: []string{"acoesg", "corp"}},
	}
	v1 := []string{"moment", "wealth", "actions", "advisor", "activity"}
	v2 := []string{"moment", "highlights", "wealth", "actions", "advisor", "activity"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, _ := startPerfil(t, screenBook(), nil)
			if body := homeOf(t, h, tt.id); body.Revision != "v1" || !slices.Equal(body.ids(), v1) {
				t.Errorf("before beta: %s %v, want v1 %v", body.Revision, body.ids(), v1)
			}

			if rr := putPreferences(t, h, tt.id, `{"channel":"chat","beta":true}`); rr.Code != http.StatusOK {
				t.Fatalf("PUT beta on = %d %s", rr.Code, rr.Body.String())
			}
			body := homeOf(t, h, tt.id)
			if body.SchemaVersion != 1 || body.Slug != "home" || body.Revision != "v2" || !slices.Equal(body.ids(), v2) {
				t.Errorf("in beta: schema %d %s %s %v, want 1 home v2 %v", body.SchemaVersion, body.Slug, body.Revision, body.ids(), v2)
			}
			kind, raw := body.component(t, "highlights")
			var rail productRailProps
			decodeProps(t, raw, &rail)
			if kind != tt.rail || !slices.Equal(ids(rail.Products), tt.products) || rail.Title != tt.title {
				t.Errorf("highlights = %s %q %v, want %s %q %v", kind, rail.Title, ids(rail.Products), tt.rail, tt.title, tt.products)
			}
			for _, slug := range []string{"investir", "carteira", "perfil"} {
				if rr, other := getScreen(t, h, tt.id, slug); rr.Code != http.StatusOK || other.Revision != "v1" || other.Slug != slug {
					t.Errorf("%s in beta = %d %q %q, want 200 v1", slug, rr.Code, other.Slug, other.Revision)
				}
			}

			if rr := putPreferences(t, h, tt.id, `{"channel":"chat","beta":false}`); rr.Code != http.StatusOK {
				t.Fatalf("PUT beta off = %d %s", rr.Code, rr.Body.String())
			}
			if body := homeOf(t, h, tt.id); body.Revision != "v1" || !slices.Equal(body.ids(), v1) {
				t.Errorf("after beta: %s %v, want v1 %v", body.Revision, body.ids(), v1)
			}
		})
	}
}

// TestGetScreen_HomeBetaPreferencesDown serves a beta client whose
// preferences read fails: home v1, 200, and nothing omitted.
func TestGetScreen_HomeBetaPreferencesDown(t *testing.T) {
	t.Parallel()
	down := &atomic.Bool{}
	h, _ := startPerfil(t, screenBook(), func(p bff.POVSource) bff.POVSource {
		return switchablePreferences{POVSource: p, down: down}
	})
	if rr := putPreferences(t, h, sim.CustomerFernanda, `{"channel":"chat","beta":true}`); rr.Code != http.StatusOK {
		t.Fatalf("PUT beta on = %d %s", rr.Code, rr.Body.String())
	}
	if body := homeOf(t, h, sim.CustomerFernanda); body.Revision != "v2" {
		t.Fatalf("beta home = %s, want v2", body.Revision)
	}
	down.Store(true)
	body := homeOf(t, h, sim.CustomerFernanda)
	if want := []string{"moment", "wealth", "actions", "advisor", "activity"}; body.Revision != "v1" || !slices.Equal(body.ids(), want) {
		t.Errorf("preferences down: %s %v, want v1 %v", body.Revision, body.ids(), want)
	}
	if len(body.Omitted) != 0 {
		t.Errorf("omitted = %+v, want none: the preferences failure is not propagated", body.Omitted)
	}
}

// TestGetScreen_HomeNonBetaPreferencesDown checks that a client outside the
// beta gets the same home whether or not the preferences read works.
func TestGetScreen_HomeNonBetaPreferencesDown(t *testing.T) {
	t.Parallel()
	down := &atomic.Bool{}
	down.Store(true)
	h, _ := startPerfil(t, screenBook(), func(p bff.POVSource) bff.POVSource {
		return switchablePreferences{POVSource: p, down: down}
	})
	body := homeOf(t, h, sim.CustomerThiago)
	if want := []string{"moment", "wealth", "actions", "advisor", "activity"}; body.Revision != "v1" || !slices.Equal(body.ids(), want) {
		t.Errorf("preferences down: %s %v, want v1 %v", body.Revision, body.ids(), want)
	}
	if len(body.Omitted) != 0 {
		t.Errorf("omitted = %+v, want none", body.Omitted)
	}
}
