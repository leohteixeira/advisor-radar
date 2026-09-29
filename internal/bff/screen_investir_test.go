package bff_test

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

type investSummaryProps struct {
	CashLabel   string `json:"cash_label"`
	Cash        string `json:"cash"`
	CashCents   int64  `json:"cash_cents"`
	ProfileChip string `json:"profile_chip"`
}

type productProps struct {
	ProductID    string `json:"product_id"`
	Name         string `json:"name"`
	ClassLabel   string `json:"class_label"`
	Risk         int    `json:"risk"`
	RiskLabel    string `json:"risk_label"`
	ReturnLabel  string `json:"return_label"`
	Minimum      string `json:"minimum"`
	MinimumCents int64  `json:"minimum_cents"`
	AboveProfile bool   `json:"above_profile"`
	Badge        string `json:"badge"`
	Warning      string `json:"warning"`
	Action       struct {
		Type      string `json:"type"`
		Label     string `json:"label"`
		Target    string `json:"target"`
		ProductID string `json:"product_id"`
	} `json:"action"`
}

type productRailProps struct {
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle"`
	Products []productProps `json:"products"`
}

type productListProps struct {
	Title    string         `json:"title"`
	Products []productProps `json:"products"`
}

func ids(products []productProps) []string {
	out := make([]string, 0, len(products))
	for _, p := range products {
		out = append(out, p.ProductID)
	}
	return out
}

// TestGetScreen_Investir serves Investir from the real account-sim catalog
// over bufconn for the three seed profiles.
func TestGetScreen_Investir(t *testing.T) {
	t.Parallel()
	h := startScreens(t, screenBook(), bff.EmptyTimeline{})
	tests := []struct {
		name       string
		id         string
		cash       string
		cashCents  int64
		chip       string
		highlights string
		picks      []string
		lists      map[string][]string
		above      []string
	}{
		{
			name: "thiago", id: sim.CustomerThiago,
			cash: "US$ 60.520,00", cashCents: 6_052_000, chip: "Perfil arrojado",
			highlights: "product_rail/profile_arrojado", picks: []string{"cobalto", "acoesg"},
			above: []string{},
		},
		{
			name: "fernanda", id: sim.CustomerFernanda,
			cash: "US$ 1.148,00", cashCents: 114_800, chip: "Perfil conservador",
			highlights: "product_rail/profile_conservador", picks: []string{"tbill", "corp"},
			above: []string{"acoesg", "farol", "cobalto"},
		},
		{
			name: "mariana", id: sim.CustomerMariana,
			cash: "US$ 60.000,00", cashCents: 6_000_000, chip: "Perfil moderado",
			highlights: "product_rail/profile_moderado", picks: []string{"acoesg", "corp"},
			above: []string{"farol", "cobalto"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rr, body := getScreen(t, h, tt.id, "investir")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d %s", rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if body.Slug != "investir" || body.Revision != "v1" || body.Title != "Investir" || body.Subtitle != "Produtos fictícios · preço fixo da simulação" {
				t.Errorf("envelope = %q %q %q / %q", body.Slug, body.Revision, body.Title, body.Subtitle)
			}
			if want := []string{"cash", "highlights", "fixed_income", "etfs", "stocks"}; !slices.Equal(body.ids(), want) {
				t.Errorf("sections = %v, want %v", body.ids(), want)
			}
			if body.Omitted == nil || len(body.Omitted) != 0 {
				t.Errorf("omitted = %+v, want []", body.Omitted)
			}

			kind, raw := body.component(t, "cash")
			var cash investSummaryProps
			decodeProps(t, raw, &cash)
			if kind != "invest_summary/default" || cash.CashLabel != "Disponível para investir" || cash.Cash != tt.cash ||
				cash.CashCents != tt.cashCents || cash.ProfileChip != tt.chip {
				t.Errorf("cash = %s %+v", kind, cash)
			}

			kind, raw = body.component(t, "highlights")
			var rail productRailProps
			decodeProps(t, raw, &rail)
			if kind != tt.highlights || !slices.Equal(ids(rail.Products), tt.picks) || !strings.HasPrefix(rail.Title, "Para o seu perfil ") {
				t.Errorf("highlights = %s %q %v", kind, rail.Title, ids(rail.Products))
			}

			lists := []struct{ id, kind, title string }{
				{"fixed_income", "product_list/fixed_income", "Renda fixa"},
				{"etfs", "product_list/etf", "ETFs"},
				{"stocks", "product_list/stocks", "Ações"},
			}
			wantIDs := [][]string{{"tbill", "corp"}, {"renda", "acoesg"}, {"farol", "cobalto"}}
			above := []string{}
			for i, l := range lists {
				kind, raw := body.component(t, l.id)
				var list productListProps
				decodeProps(t, raw, &list)
				if kind != l.kind || list.Title != l.title || !slices.Equal(ids(list.Products), wantIDs[i]) {
					t.Errorf("%s = %s %q %v", l.id, kind, list.Title, ids(list.Products))
				}
				for _, p := range list.Products {
					if p.AboveProfile {
						above = append(above, p.ProductID)
					}
					if p.Action.Type != "panel" || p.Action.Target != "purchase" || p.Action.ProductID != p.ProductID || p.Action.Label != "Investir" {
						t.Errorf("%s action = %+v", p.ProductID, p.Action)
					}
					if (p.Badge != "") != p.AboveProfile || (p.Warning != "") != p.AboveProfile {
						t.Errorf("%s above %t badge %q warning %q", p.ProductID, p.AboveProfile, p.Badge, p.Warning)
					}
				}
			}
			if !slices.Equal(above, tt.above) {
				t.Errorf("above profile = %v, want %v", above, tt.above)
			}
		})
	}
}

// TestGetScreen_InvestirCobaltoForFernanda pins every prop of one product
// above the profile, as the purchase form reads them.
func TestGetScreen_InvestirCobaltoForFernanda(t *testing.T) {
	t.Parallel()
	h := startScreens(t, screenBook(), bff.EmptyTimeline{})
	rr, body := getScreen(t, h, sim.CustomerFernanda, "investir")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	_, raw := body.component(t, "stocks")
	var list productListProps
	decodeProps(t, raw, &list)
	if len(list.Products) < 2 {
		t.Fatalf("stocks products = %d, want at least 2", len(list.Products))
	}
	cobalto := list.Products[1]
	expected := productProps{
		ProductID: "cobalto", Name: "Cobalto Semicondutores", ClassLabel: "Ação",
		Risk: 5, RiskLabel: "Risco 5 de 5", ReturnLabel: "+27,1% em 12 meses",
		Minimum: "Mínimo US$ 10", MinimumCents: 1_000, AboveProfile: true,
		Badge: "Acima do seu perfil",
		Warning: "Este produto tem risco 5. Seu perfil é conservador, que vai até risco 2. " +
			"Você pode investir mesmo assim, e a sua assessora será avisada.",
	}
	expected.Action.Type = "panel"
	expected.Action.Label = "Investir"
	expected.Action.Target = "purchase"
	expected.Action.ProductID = "cobalto"
	if cobalto != expected {
		t.Errorf("cobalto = %+v, want %+v", cobalto, expected)
	}
}

func TestGetScreen_InvestirSourceFailures(t *testing.T) {
	t.Parallel()
	thiago := bff.POVAccount{CustomerID: sim.CustomerThiago, Caixa: 6_052_000, Patrimony: 6_800_000}
	catalog := make([]bff.POVProduct, 0, len(sim.Catalog()))
	for _, p := range sim.Catalog() {
		catalog = append(catalog, bff.POVProduct{
			ID: p.ID, Name: p.Name, AssetClass: p.AssetClass, Risk: p.Risk, ReturnLabel: p.ReturnLabel, MinimumCents: p.MinimumCents,
		})
	}
	noProfile := screenBook()
	noProfile.profiles = nil
	tests := []struct {
		name     string
		pov      bff.POVSource
		queue    bff.QueueSource
		sections []string
		omitted  []string
		chip     string
	}{
		{
			name:     "catalog down",
			pov:      &fakePOV{accounts: []bff.POVAccount{thiago}, productsErr: errors.New("account-sim unavailable")},
			queue:    screenBook(),
			sections: []string{"cash"},
			omitted: []string{
				"highlights/product_rail/catalog", "fixed_income/product_list/catalog",
				"etfs/product_list/catalog", "stocks/product_list/catalog",
			},
			chip: "Perfil arrojado",
		},
		{
			name:     "profile down",
			pov:      &fakePOV{accounts: []bff.POVAccount{thiago}, products: catalog},
			queue:    noProfile,
			sections: []string{"cash", "fixed_income", "etfs", "stocks"},
			omitted:  []string{"highlights/product_rail/profile"},
		},
		{
			name:     "account-sim down",
			pov:      failingPOV{},
			queue:    screenBook(),
			sections: []string{},
			omitted: []string{
				"cash/invest_summary/account-sim", "highlights/product_rail/catalog", "fixed_income/product_list/catalog",
				"etfs/product_list/catalog", "stocks/product_list/catalog",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, bff.EmptyTimeline{}, tt.queue, nil, nil, tt.pov, nil)
			rr, body := getScreen(t, h, sim.CustomerThiago, "investir")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d %s", rr.Code, rr.Body.String())
			}
			if !slices.Equal(body.ids(), tt.sections) {
				t.Errorf("sections = %v, want %v", body.ids(), tt.sections)
			}
			omitted := make([]string, 0, len(body.Omitted))
			for _, o := range body.Omitted {
				omitted = append(omitted, o.ID+"/"+o.Type+"/"+o.Reason)
			}
			if !slices.Equal(omitted, tt.omitted) {
				t.Errorf("omitted = %v, want %v", omitted, tt.omitted)
			}
			if slices.Contains(body.ids(), "cash") {
				_, raw := body.component(t, "cash")
				var cash investSummaryProps
				decodeProps(t, raw, &cash)
				if cash.ProfileChip != tt.chip {
					t.Errorf("chip = %q, want %q", cash.ProfileChip, tt.chip)
				}
			}
			if slices.Contains(body.ids(), "stocks") {
				_, raw := body.component(t, "stocks")
				var list productListProps
				decodeProps(t, raw, &list)
				for _, p := range list.Products {
					if p.AboveProfile || p.Badge != "" || p.Warning != "" {
						t.Errorf("%s without a profile = above %t badge %q", p.ProductID, p.AboveProfile, p.Badge)
					}
				}
			}
		})
	}
}
