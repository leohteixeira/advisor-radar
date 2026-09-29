package screen

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// productIDs lists the product ids of a rail or list in order.
func productIDs(items []ProductItem) []string {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ProductID)
	}
	return ids
}

// badged lists the ids of the items marked above profile.
func badged(items []ProductItem) []string {
	ids := []string{}
	for _, it := range items {
		if it.AboveProfile {
			ids = append(ids, it.ProductID)
		}
	}
	return ids
}

func TestInvestorProfile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		profile Fetched[InvestorProfile]
		want    string
		limit   int
		ok      bool
	}{
		{name: "known profile is lowercased", profile: Fetched[InvestorProfile]{Value: InvestorProfile{Profile: "Moderado", MaxRisk: 3}}, want: "moderado", limit: 3, ok: true},
		{name: "failed source", profile: Fetched[InvestorProfile]{Err: errDown}},
		{name: "empty profile", profile: Fetched[InvestorProfile]{Value: InvestorProfile{MaxRisk: 3}}},
		{name: "no max risk", profile: Fetched[InvestorProfile]{Value: InvestorProfile{Profile: "moderado"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, limit, ok := investorProfile(Snapshot{Profile: tt.profile})
			if got != tt.want || limit != tt.limit || ok != tt.ok {
				t.Errorf("investorProfile = %q %d %t, want %q %d %t", got, limit, ok, tt.want, tt.limit, tt.ok)
			}
		})
	}
}

func TestInvestSummary(t *testing.T) {
	t.Parallel()
	v := investSummary{}
	if !v.Matches(Snapshot{}) {
		t.Error("invest summary does not always match")
	}

	p := props[InvestSummary](t, build(t, v, thiagoFixture().snapshot()), "invest_summary", "default")
	expected := InvestSummary{CashLabel: "Disponível para investir", Cash: "US$ 60.520,00", CashCents: 6_052_000, ProfileChip: "Perfil arrojado"}
	if p != expected {
		t.Errorf("props = %+v, want %+v", p, expected)
	}

	noProfile := thiagoFixture().snapshot()
	noProfile.Profile = Fetched[InvestorProfile]{Err: errDown}
	p = props[InvestSummary](t, build(t, v, noProfile), "invest_summary", "default")
	if p.ProfileChip != "" || p.Cash != "US$ 60.520,00" {
		t.Errorf("props without profile = %+v, want cash and no chip", p)
	}

	noAccount := thiagoFixture().snapshot()
	noAccount.Account = Fetched[Account]{Err: errDown}
	if _, err := v.Build(noAccount, embedded(t)); err == nil {
		t.Error("Build without the account returned no error")
	}
}

// TestProductRail_OneVariantPerProfile checks that each known profile
// matches exactly its own rail variant, and that no variant matches without
// a known profile.
func TestProductRail_OneVariantPerProfile(t *testing.T) {
	t.Parallel()
	variants := investirVariants()
	rails := map[string]Variant{}
	for key, reg := range variants {
		if key.typ == typeProductRail {
			rails[key.name] = reg.variant
			if !slices.Equal(reg.needs, []Source{SourceCatalog, SourceProfile}) {
				t.Errorf("%s needs %v", key.name, reg.needs)
			}
		}
	}
	if len(rails) != 3 {
		t.Fatalf("rail variants = %v, want three", rails)
	}
	for _, profile := range []string{"conservador", "moderado", "arrojado"} {
		snap := Snapshot{Profile: Fetched[InvestorProfile]{Value: InvestorProfile{Profile: profile, MaxRisk: 3}}}
		for name, v := range rails {
			if got, want := v.Matches(snap), name == "profile_"+profile; got != want {
				t.Errorf("%s matches %s = %t, want %t", name, profile, got, want)
			}
		}
	}
	for _, snap := range []Snapshot{
		{},
		{Profile: Fetched[InvestorProfile]{Err: errDown}},
		{Profile: Fetched[InvestorProfile]{Value: InvestorProfile{Profile: "agressivo", MaxRisk: 5}}},
	} {
		for name, v := range rails {
			if v.Matches(snap) {
				t.Errorf("%s matches %+v", name, snap.Profile)
			}
		}
	}
}

func TestProductRail_Build(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fixture  fixture
		variant  string
		products []string
		title    string
	}{
		{name: "conservador", fixture: fernandaFixture(), variant: "profile_conservador", products: []string{"tbill", "corp"}, title: "Para o seu perfil conservador"},
		{name: "moderado", fixture: marianaFixture(), variant: "profile_moderado", products: []string{"acoesg", "corp"}, title: "Para o seu perfil moderado"},
		{name: "arrojado", fixture: thiagoFixture(), variant: "profile_arrojado", products: []string{"cobalto", "acoesg"}, title: "Para o seu perfil arrojado"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := investirVariants()[variantKey{typ: typeProductRail, name: tt.variant}]
			snap := tt.fixture.snapshot()
			if !reg.variant.Matches(snap) {
				t.Fatalf("%s does not match its profile", tt.variant)
			}
			p := props[ProductRail](t, build(t, reg.variant, snap), "product_rail", tt.variant)
			if p.Title != tt.title || p.Subtitle != "Escolhidos pelo backend a partir do seu perfil de investidor." {
				t.Errorf("heading = %q / %q", p.Title, p.Subtitle)
			}
			if got := productIDs(p.Products); !slices.Equal(got, tt.products) {
				t.Errorf("products = %v, want %v", got, tt.products)
			}
			if got := badged(p.Products); len(got) != 0 {
				t.Errorf("above-profile picks = %v, want none", got)
			}
		})
	}
}

func TestProductRail_BuildErrors(t *testing.T) {
	t.Parallel()
	v := productRail{name: "profile_arrojado", profile: "arrojado", picks: []string{"cobalto", "acoesg"}}
	tests := []struct {
		name   string
		change func(*Snapshot)
	}{
		{name: "catalog down", change: func(s *Snapshot) { s.Products = Fetched[[]Product]{Err: errDown} }},
		{name: "profile down", change: func(s *Snapshot) { s.Profile = Fetched[InvestorProfile]{Err: errDown} }},
		{name: "pick missing from the catalog", change: func(s *Snapshot) {
			s.Products = Fetched[[]Product]{Value: slices.DeleteFunc(testCatalog(), func(p Product) bool { return p.ID == "cobalto" })}
		}},
		{name: "pick with a bad risk", change: func(s *Snapshot) {
			products := testCatalog()
			products[0].Risk = 9 // cobalto
			s.Products = Fetched[[]Product]{Value: products}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snap := thiagoFixture().snapshot()
			tt.change(&snap)
			if _, err := v.Build(snap, embedded(t)); err == nil {
				t.Error("Build returned no error")
			}
		})
	}
}

func TestProductList_Build(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fixture  fixture
		variant  string
		title    string
		products []string
		above    []string
	}{
		{name: "fixed income for fernanda", fixture: fernandaFixture(), variant: "fixed_income", title: "Renda fixa", products: []string{"tbill", "corp"}, above: []string{}},
		{name: "etfs for fernanda", fixture: fernandaFixture(), variant: "etf", title: "ETFs", products: []string{"renda", "acoesg"}, above: []string{"acoesg"}},
		{name: "stocks for fernanda", fixture: fernandaFixture(), variant: "stocks", title: "Ações", products: []string{"farol", "cobalto"}, above: []string{"farol", "cobalto"}},
		{name: "stocks for mariana", fixture: marianaFixture(), variant: "stocks", title: "Ações", products: []string{"farol", "cobalto"}, above: []string{"farol", "cobalto"}},
		{name: "stocks for thiago", fixture: thiagoFixture(), variant: "stocks", title: "Ações", products: []string{"farol", "cobalto"}, above: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := investirVariants()[variantKey{typ: typeProductList, name: tt.variant}]
			if !slices.Equal(reg.needs, []Source{SourceCatalog}) || !reg.variant.Matches(Snapshot{}) {
				t.Fatalf("%s needs %v or does not always match", tt.variant, reg.needs)
			}
			p := props[ProductList](t, build(t, reg.variant, tt.fixture.snapshot()), "product_list", tt.variant)
			if p.Title != tt.title {
				t.Errorf("title = %q, want %q", p.Title, tt.title)
			}
			if got := productIDs(p.Products); !slices.Equal(got, tt.products) {
				t.Errorf("products = %v, want %v", got, tt.products)
			}
			if got := badged(p.Products); !slices.Equal(got, tt.above) {
				t.Errorf("above profile = %v, want %v", got, tt.above)
			}
		})
	}
}

func TestProductList_OrdersByRiskThenID(t *testing.T) {
	t.Parallel()
	snap := thiagoFixture().snapshot()
	snap.Products = Fetched[[]Product]{Value: []Product{
		{ID: "zeta", Name: "Zeta", AssetClass: classAcoes, Risk: 4},
		{ID: "cobalto", Name: "Cobalto", AssetClass: classAcoes, Risk: 5},
		{ID: "alfa", Name: "Alfa", AssetClass: classAcoes, Risk: 4},
		{ID: "tbill", Name: "T-Bill", AssetClass: classRendaFixa, Risk: 1},
	}}
	input := slices.Clone(snap.Products.Value)
	p := props[ProductList](t, build(t, productList{name: "stocks", class: classAcoes}, snap), "product_list", "stocks")
	if got, want := productIDs(p.Products), []string{"alfa", "zeta", "cobalto"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if !slices.Equal(snap.Products.Value, input) {
		t.Error("Build reordered the snapshot catalog")
	}
}

func TestProductList_BuildErrors(t *testing.T) {
	t.Parallel()
	v := productList{name: "stocks", class: classAcoes}
	tests := []struct {
		name     string
		products Fetched[[]Product]
	}{
		{name: "catalog down", products: Fetched[[]Product]{Err: errDown}},
		{name: "no product of the class", products: Fetched[[]Product]{Value: []Product{{ID: "tbill", Name: "T-Bill", AssetClass: classRendaFixa, Risk: 1}}}},
		{name: "risk below one", products: Fetched[[]Product]{Value: []Product{{ID: "x", Name: "X", AssetClass: classAcoes, Risk: 0}}}},
		{name: "product without a name", products: Fetched[[]Product]{Value: []Product{{ID: "x", AssetClass: classAcoes, Risk: 3}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snap := thiagoFixture().snapshot()
			snap.Products = tt.products
			if _, err := v.Build(snap, embedded(t)); err == nil {
				t.Error("Build returned no error")
			}
		})
	}
}

func TestProductItem(t *testing.T) {
	t.Parallel()
	cobalto := Product{ID: "cobalto", Name: "Cobalto Semicondutores", AssetClass: classAcoes, Risk: 5, ReturnLabel: "+27,1% em 12 meses", MinimumCents: 1_000}
	action := Action{Type: ActionPanel, Label: "Investir", Target: PanelPurchase, ProductID: "cobalto"}
	within := ProductItem{
		ProductID: "cobalto", Name: "Cobalto Semicondutores", ClassLabel: "Ação",
		Risk: 5, RiskLabel: "Risco 5 de 5", ReturnLabel: "+27,1% em 12 meses",
		Minimum: "Mínimo US$ 10", MinimumCents: 1_000, Action: action,
	}
	above := within
	above.AboveProfile = true
	above.Badge = "Acima do seu perfil"
	above.Warning = "Este produto tem risco 5. Seu perfil é conservador, que vai até risco 2. " +
		"Você pode investir mesmo assim, e a sua assessora será avisada."

	noProfile := fernandaFixture().snapshot()
	noProfile.Profile = Fetched[InvestorProfile]{Err: errDown}
	tests := []struct {
		name     string
		snap     Snapshot
		expected ProductItem
	}{
		{name: "within an arrojado profile", snap: thiagoFixture().snapshot(), expected: within},
		{name: "above a conservador profile", snap: fernandaFixture().snapshot(), expected: above},
		{name: "profile unavailable", snap: noProfile, expected: within},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := productItem(tt.snap, embedded(t), cobalto)
			if err != nil {
				t.Fatalf("productItem error = %v", err)
			}
			if got != tt.expected {
				t.Errorf("item = %+v, want %+v", got, tt.expected)
			}
		})
	}

	for _, class := range []string{classRendaFixa, classETFs} {
		p := cobalto
		p.AssetClass = class
		got, err := productItem(thiagoFixture().snapshot(), embedded(t), p)
		if err != nil || got.ClassLabel == "" || strings.Contains(got.ClassLabel, "{{") {
			t.Errorf("class %s label = %q, %v", class, got.ClassLabel, err)
		}
	}
	unknown := cobalto
	unknown.AssetClass = "cripto"
	if _, err := productItem(thiagoFixture().snapshot(), embedded(t), unknown); err == nil {
		t.Error("a product of an unknown class built without error")
	}
}

func TestEngine_Build_Investir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		fixture    fixture
		chip       string
		cash       string
		highlights string
		picks      []string
		above      []string
	}{
		{
			name: "thiago", fixture: thiagoFixture(), chip: "Perfil arrojado", cash: "US$ 60.520,00",
			highlights: "profile_arrojado", picks: []string{"cobalto", "acoesg"}, above: []string{},
		},
		{
			name: "fernanda", fixture: fernandaFixture(), chip: "Perfil conservador", cash: "US$ 1.148,00",
			highlights: "profile_conservador", picks: []string{"tbill", "corp"}, above: []string{"acoesg", "farol", "cobalto"},
		},
		{
			name: "mariana", fixture: marianaFixture(), chip: "Perfil moderado", cash: "US$ 60.000,00",
			highlights: "profile_moderado", picks: []string{"acoesg", "corp"}, above: []string{"farol", "cobalto"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reporter := &recordingReporter{}
			res, err := newTestEngine(t, tt.fixture.sources(), WithDropReporter(reporter)).Build(t.Context(), "investir", tt.fixture.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			p := res.Page
			if p.Slug != "investir" || p.Revision != "v1" || p.Title != "Investir" || p.Subtitle != "Produtos fictícios · preço fixo da simulação" {
				t.Errorf("envelope = %q %q %q / %q", p.Slug, p.Revision, p.Title, p.Subtitle)
			}
			if want := []string{"cash", "highlights", "fixed_income", "etfs", "stocks"}; !slices.Equal(sectionIDs(p), want) {
				t.Errorf("sections = %v, want %v", sectionIDs(p), want)
			}
			if len(p.Omitted) != 0 || len(res.Failures) != 0 || len(reporter.all()) != 0 {
				t.Errorf("omitted %v, failures %v, drops %v", p.Omitted, res.Failures, reporter.all())
			}
			cash := props[InvestSummary](t, componentOf(t, p, "cash"), "invest_summary", "default")
			if cash.ProfileChip != tt.chip || cash.Cash != tt.cash {
				t.Errorf("cash = %+v", cash)
			}
			rail := props[ProductRail](t, componentOf(t, p, "highlights"), "product_rail", tt.highlights)
			if got := productIDs(rail.Products); !slices.Equal(got, tt.picks) {
				t.Errorf("highlights = %v, want %v", got, tt.picks)
			}
			var above []string
			for _, sec := range []struct{ id, variant string }{{"fixed_income", "fixed_income"}, {"etfs", "etf"}, {"stocks", "stocks"}} {
				list := props[ProductList](t, componentOf(t, p, sec.id), "product_list", sec.variant)
				above = append(above, badged(list.Products)...)
			}
			if !slices.Equal(above, tt.above) {
				t.Errorf("above profile = %v, want %v", above, tt.above)
			}
		})
	}
}

func TestEngine_Build_InvestirFailurePolicy(t *testing.T) {
	t.Parallel()
	lists := []Omitted{
		{ID: "fixed_income", Type: "product_list", Reason: "catalog"},
		{ID: "etfs", Type: "product_list", Reason: "catalog"},
		{ID: "stocks", Type: "product_list", Reason: "catalog"},
	}
	tests := []struct {
		name     string
		change   func(*Sources)
		sections []string
		omitted  []Omitted
		subtitle string
		chip     bool
		badges   bool
	}{
		{
			name:     "catalog down",
			change:   func(s *Sources) { s.Products = fakeProducts{err: errDown} },
			sections: []string{"cash"},
			omitted:  append([]Omitted{{ID: "highlights", Type: "product_rail", Reason: "catalog"}}, lists...),
			chip:     true,
		},
		{
			name:     "profile down",
			change:   func(s *Sources) { s.Profiles = fakeProfiles{err: errDown} },
			sections: []string{"cash", "fixed_income", "etfs", "stocks"},
			omitted:  []Omitted{{ID: "highlights", Type: "product_rail", Reason: "profile"}},
		},
		{
			name: "unknown profile",
			change: func(s *Sources) {
				s.Profiles = fakeProfiles{profile: InvestorProfile{Profile: "agressivo", MaxRisk: 3}}
			},
			sections: []string{"cash", "fixed_income", "etfs", "stocks"},
			omitted:  []Omitted{{ID: "highlights", Type: "product_rail", Reason: "build_error"}},
			chip:     true,
			badges:   true,
		},
		{
			name:     "account down",
			change:   func(s *Sources) { s.Accounts = fakeAccounts{err: errDown} },
			sections: []string{"highlights", "fixed_income", "etfs", "stocks"},
			omitted:  []Omitted{{ID: "cash", Type: "invest_summary", Reason: "account-sim"}},
			badges:   true,
		},
		{
			name:     "advisory down keeps the plain subtitle",
			change:   func(s *Sources) { s.Customers = fakeCustomers{err: errDown} },
			sections: []string{"cash", "highlights", "fixed_income", "etfs", "stocks"},
			chip:     true,
			badges:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := fernandaFixture()
			src := f.sources()
			tt.change(&src)
			reporter := &recordingReporter{}
			res, err := newTestEngine(t, src, WithDropReporter(reporter)).Build(t.Context(), "investir", f.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			p := res.Page
			if !slices.Equal(sectionIDs(p), tt.sections) {
				t.Errorf("sections = %v, want %v", sectionIDs(p), tt.sections)
			}
			omitted := tt.omitted
			if omitted == nil {
				omitted = []Omitted{}
			}
			if !slices.Equal(p.Omitted, omitted) {
				t.Errorf("omitted = %+v, want %+v", p.Omitted, omitted)
			}
			if len(reporter.all()) != len(omitted) {
				t.Errorf("drops = %+v, want one per omitted section", reporter.all())
			}
			if p.Title != "Investir" || p.Subtitle != "Produtos fictícios · preço fixo da simulação" {
				t.Errorf("heading = %q / %q", p.Title, p.Subtitle)
			}
			if slices.Contains(sectionIDs(p), "cash") {
				cash := props[InvestSummary](t, componentOf(t, p, "cash"), "invest_summary", "default")
				if (cash.ProfileChip != "") != tt.chip {
					t.Errorf("chip = %q, want present %t", cash.ProfileChip, tt.chip)
				}
			}
			if slices.Contains(sectionIDs(p), "stocks") {
				list := props[ProductList](t, componentOf(t, p, "stocks"), "product_list", "stocks")
				for _, it := range list.Products {
					if it.AboveProfile != tt.badges || (it.Badge != "") != tt.badges || (it.Warning != "") != tt.badges {
						t.Errorf("stocks item %s above %t badge %q warning %q, want badges %t", it.ProductID, it.AboveProfile, it.Badge, it.Warning, tt.badges)
					}
				}
			}
		})
	}
}

// countingActivity counts its calls; a screen that does not read the
// timeline must never call it.
type countingActivity struct{ calls *atomic.Int32 }

func (c countingActivity) Activity(context.Context, string) ([]Activity, error) {
	c.calls.Add(1)
	return nil, nil
}

func TestEngine_Build_FetchesOnlyThePlanSources(t *testing.T) {
	t.Parallel()
	if got, want := newTestEngine(t, thiagoFixture().sources()).plans["investir"].sources, []Source{SourceAccount, SourceProfile, SourceCatalog}; !slices.Equal(got, want) {
		t.Errorf("investir sources = %v, want %v", got, want)
	}
	home := newTestEngine(t, thiagoFixture().sources()).plans["home"]
	if !slices.Contains(home.sources, SourceCatalog) || slices.Contains(home.required, SourceCatalog) {
		t.Errorf("home sources = %v, required = %v, want the catalog fetched but not required", home.sources, home.required)
	}
}

func TestEngine_Build_HomeIgnoresTheCatalog(t *testing.T) {
	t.Parallel()
	f := thiagoFixture()
	src := f.sources()
	src.Products = fakeProducts{err: errDown}
	res, err := newTestEngine(t, src).Build(t.Context(), "home", f.id)
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	if len(res.Failures) != 0 {
		t.Errorf("failures = %+v, want none", res.Failures)
	}
	if len(res.Page.Omitted) != 0 || len(res.Page.Sections) != 5 {
		t.Errorf("sections = %d, omitted = %+v", len(res.Page.Sections), res.Page.Omitted)
	}
}

func TestEngine_Build_InvestirMakesNoTimelineCall(t *testing.T) {
	t.Parallel()
	f := thiagoFixture()
	src := f.sources()
	var calls atomic.Int32
	src.Activity = countingActivity{calls: &calls}
	res, err := newTestEngine(t, src).Build(t.Context(), "investir", f.id)
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("timeline calls = %d, want 0", n)
	}
	if len(res.Failures) != 0 || len(res.Page.Sections) != 5 {
		t.Errorf("failures = %+v, sections = %d", res.Failures, len(res.Page.Sections))
	}
}
