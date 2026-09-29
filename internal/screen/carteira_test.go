package screen

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// purchaseRow is the timeline row of Thiago's "Tudo" purchase of acoesg.
func purchaseRow() Activity {
	return Activity{
		Kind: "aplicacao", Title: "Aplicação", Source: "account.event.recorded",
		OccurredAt: testNow.Add(-2 * time.Minute), ProductID: "acoesg", AmountCents: 6_052_000,
	}
}

// thiagoAfterTudo is Thiago after buying all his cash in acoesg: the cash is
// now in the ETF position and the timeline holds the purchase.
func thiagoAfterTudo() fixture {
	f := thiagoFixture()
	f.account = Account{
		Acoes: 204_000, ETFs: 6_596_000, Cash: 0, Patrimony: 6_800_000,
		Positions: []Position{
			{ProductID: "cobalto", AssetClass: "acoes", AppliedCents: 190_000, ValueCents: 204_000},
			{ProductID: "acoesg", AssetClass: "etfs", AppliedCents: 6_572_000, ValueCents: 6_596_000},
		},
	}
	f.activity = append([]Activity{purchaseRow()}, f.activity...)
	return f
}

func positionNames(items []PositionItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ProductID)
	}
	return out
}

func TestPortfolioSummary(t *testing.T) {
	t.Parallel()
	v := portfolioSummary{}
	if !v.Matches(Snapshot{}) {
		t.Error("portfolio summary does not always match")
	}
	tests := []struct {
		name     string
		account  Account
		expected PortfolioSummary
	}{
		{
			name:    "mariana gains",
			account: marianaFixture().account,
			expected: PortfolioSummary{
				TotalLabel: "Patrimônio total",
				Total:      "US$ 248.300,00",
				Stats: []Stat{
					{Label: "Valor aplicado", Value: "US$ 168.900,00", Money: true},
					{Label: "Rentabilidade", Value: "+US$ 19.400,00 (+11,5%)", Tone: "pos", Money: true},
					{Label: "Caixa", Value: "US$ 60.000,00", Money: true},
					{Label: "Dia simulado", Value: "0"},
				},
			},
		},
		{
			name: "a loss on a later day",
			account: Account{
				Cash: 10_000, Patrimony: 60_000, SimDay: 12,
				Positions: []Position{{ProductID: "cobalto", AssetClass: "acoes", AppliedCents: 100_000, ValueCents: 50_000}},
			},
			expected: PortfolioSummary{
				TotalLabel: "Patrimônio total",
				Total:      "US$ 600,00",
				Stats: []Stat{
					{Label: "Valor aplicado", Value: "US$ 1.000,00", Money: true},
					{Label: "Rentabilidade", Value: "−US$ 500,00 (−50,0%)", Tone: "neg", Money: true},
					{Label: "Caixa", Value: "US$ 100,00", Money: true},
					{Label: "Dia simulado", Value: "12"},
				},
			},
		},
		{
			name:    "no positions",
			account: Account{Cash: 50_000, Patrimony: 50_000},
			expected: PortfolioSummary{
				TotalLabel: "Patrimônio total",
				Total:      "US$ 500,00",
				Stats: []Stat{
					{Label: "Valor aplicado", Value: "US$ 0,00", Money: true},
					{Label: "Rentabilidade", Value: "US$ 0,00 (0,0%)", Tone: "neutral", Money: true},
					{Label: "Caixa", Value: "US$ 500,00", Money: true},
					{Label: "Dia simulado", Value: "0"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snap := Snapshot{Account: Fetched[Account]{Value: tt.account}}
			p := props[PortfolioSummary](t, build(t, v, snap), "portfolio_summary", "default")
			if p.TotalLabel != tt.expected.TotalLabel || p.Total != tt.expected.Total || !slices.Equal(p.Stats, tt.expected.Stats) {
				t.Errorf("props = %+v, want %+v", p, tt.expected)
			}
		})
	}
	if _, err := v.Build(Snapshot{Account: Fetched[Account]{Err: errDown}}, embedded(t)); !errors.Is(err, errDown) {
		t.Errorf("Build without the account error = %v, want %v", err, errDown)
	}
}

func TestAllocationBreakdown(t *testing.T) {
	t.Parallel()
	v := allocationBreakdown{}
	if !v.Matches(Snapshot{}) {
		t.Error("allocation does not always match")
	}
	tests := []struct {
		name     string
		account  Account
		expected []BreakdownRow
	}{
		{
			name:    "mariana",
			account: marianaFixture().account,
			expected: []BreakdownRow{
				{Class: "stocks", Label: "Ações", Value: "US$ 90.900,00", Share: "37%", BarWidth: 37},
				{Class: "etfs", Label: "ETFs", Value: "US$ 60.600,00", Share: "24%", BarWidth: 24},
				{Class: "fixed_income", Label: "Renda fixa", Value: "US$ 36.800,00", Share: "15%", BarWidth: 15},
				{Class: "cash", Label: "Caixa", Value: "US$ 60.000,00", Share: "24%", BarWidth: 24},
			},
		},
		{
			name:    "thiago keeps the empty fixed income row",
			account: thiagoFixture().account,
			expected: []BreakdownRow{
				{Class: "stocks", Label: "Ações", Value: "US$ 2.040,00", Share: "3%", BarWidth: 3},
				{Class: "etfs", Label: "ETFs", Value: "US$ 5.440,00", Share: "8%", BarWidth: 8},
				{Class: "fixed_income", Label: "Renda fixa", Value: "US$ 0,00", Share: "0%", BarWidth: 0},
				{Class: "cash", Label: "Caixa", Value: "US$ 60.520,00", Share: "89%", BarWidth: 89},
			},
		},
		{
			name:    "empty account",
			account: Account{},
			expected: []BreakdownRow{
				{Class: "stocks", Label: "Ações", Value: "US$ 0,00", Share: "0%", BarWidth: 0},
				{Class: "etfs", Label: "ETFs", Value: "US$ 0,00", Share: "0%", BarWidth: 0},
				{Class: "fixed_income", Label: "Renda fixa", Value: "US$ 0,00", Share: "0%", BarWidth: 0},
				{Class: "cash", Label: "Caixa", Value: "US$ 0,00", Share: "0%", BarWidth: 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snap := Snapshot{Account: Fetched[Account]{Value: tt.account}}
			p := props[AllocationBreakdown](t, build(t, v, snap), "allocation_breakdown", "default")
			if p.Title != "Alocação" || !slices.Equal(p.Rows, tt.expected) {
				t.Errorf("props = %+v, want rows %+v", p, tt.expected)
			}
		})
	}

	t.Run("errors", func(t *testing.T) {
		t.Parallel()
		if _, err := v.Build(Snapshot{Account: Fetched[Account]{Err: errDown}}, embedded(t)); !errors.Is(err, errDown) {
			t.Errorf("Build without the account error = %v, want %v", err, errDown)
		}
		huge := Account{Acoes: 1 << 62, ETFs: 1 << 62, RendaFixa: 1 << 62, Cash: 1 << 62}
		if _, err := v.Build(Snapshot{Account: Fetched[Account]{Value: huge}}, embedded(t)); !errors.Is(err, errShareOverflow) {
			t.Errorf("Build past uint64 error = %v, want %v", err, errShareOverflow)
		}
	})
}

// TestPositionList covers each position_list variant for Mariana, who holds
// all three classes.
func TestPositionList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		variant  string
		class    string
		expected PositionList
	}{
		{
			variant: "stocks", class: classAcoes,
			expected: PositionList{
				Title: "Ações", Subtotal: "US$ 90.900,00", AppliedLabel: "Aplicado",
				Items: []PositionItem{
					{ProductID: "cobalto", Name: "Cobalto Semicondutores", Applied: "US$ 60.000,00", Value: "US$ 72.000,00", Return: "+20,0%", ReturnTone: "pos"},
					{ProductID: "farol", Name: "Farol Saúde", Applied: "US$ 17.500,00", Value: "US$ 18.900,00", Return: "+8,0%", ReturnTone: "pos"},
				},
			},
		},
		{
			variant: "etf", class: classETFs,
			expected: PositionList{
				Title: "ETFs", Subtotal: "US$ 60.600,00", AppliedLabel: "Aplicado",
				Items: []PositionItem{
					{ProductID: "acoesg", Name: "Maré Ações Globais ETF", Applied: "US$ 36.000,00", Value: "US$ 40.600,00", Return: "+12,8%", ReturnTone: "pos"},
					{ProductID: "renda", Name: "Maré Renda Global ETF", Applied: "US$ 19.400,00", Value: "US$ 20.000,00", Return: "+3,1%", ReturnTone: "pos"},
				},
			},
		},
		{
			variant: "fixed_income", class: classRendaFixa,
			expected: PositionList{
				Title: "Renda fixa", Subtotal: "US$ 36.800,00", AppliedLabel: "Aplicado",
				Items: []PositionItem{
					{ProductID: "corp", Name: "Orla Corporate IG 2029", Applied: "US$ 36.000,00", Value: "US$ 36.800,00", Return: "+2,2%", ReturnTone: "pos"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.variant, func(t *testing.T) {
			t.Parallel()
			v := positionList{name: tt.variant, class: tt.class}
			if !v.Matches(Snapshot{}) {
				t.Error("position list does not always match")
			}
			p := props[PositionList](t, build(t, v, marianaFixture().snapshot()), "position_list", tt.variant)
			if p.Title != tt.expected.Title || p.Subtotal != tt.expected.Subtotal || p.AppliedLabel != tt.expected.AppliedLabel ||
				!slices.Equal(p.Items, tt.expected.Items) {
				t.Errorf("props = %+v, want %+v", p, tt.expected)
			}
		})
	}
}

func TestPositionList_OrderAndReturns(t *testing.T) {
	t.Parallel()
	snap := marianaFixture().snapshot()
	snap.Account.Value.Positions = []Position{
		{ProductID: "renda", AssetClass: "etfs", AppliedCents: 100_000, ValueCents: 100_000},
		{ProductID: "acoesg", AssetClass: "etfs", AppliedCents: 200_000, ValueCents: 100_000},
		{ProductID: "tbill", AssetClass: "etfs", AppliedCents: 0, ValueCents: 300_000},
	}
	p := props[PositionList](t, build(t, positionList{name: "etf", class: classETFs}, snap), "position_list", "etf")
	if got, want := positionNames(p.Items), []string{"tbill", "acoesg", "renda"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v (value descending, then id)", got, want)
	}
	returns := make([]string, 0, len(p.Items))
	for _, it := range p.Items {
		returns = append(returns, it.Return+" "+it.ReturnTone)
	}
	if want := []string{"0,0% neutral", "−50,0% neg", "0,0% neutral"}; !slices.Equal(returns, want) {
		t.Errorf("returns = %v, want %v", returns, want)
	}
	if p.Subtotal != "US$ 5.000,00" {
		t.Errorf("subtotal = %q", p.Subtotal)
	}
}

func TestPositionList_BuildErrors(t *testing.T) {
	t.Parallel()
	v := positionList{name: "fixed_income", class: classRendaFixa}
	unknown := marianaFixture().snapshot()
	unknown.Products.Value = slices.DeleteFunc(testCatalog(), func(p Product) bool { return p.ID == "corp" })
	tests := []struct {
		name     string
		snap     Snapshot
		expected error
	}{
		{name: "account failed", snap: Snapshot{Account: Fetched[Account]{Err: errDown}}, expected: errDown},
		{name: "catalog failed", snap: Snapshot{Products: Fetched[[]Product]{Err: errDown}}, expected: errDown},
		{name: "class without a position is absent", snap: thiagoFixture().snapshot(), expected: errAbsent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := v.Build(tt.snap, embedded(t)); !errors.Is(err, tt.expected) {
				t.Errorf("Build error = %v, want %v", err, tt.expected)
			}
		})
	}
	t.Run("product missing from the catalog", func(t *testing.T) {
		t.Parallel()
		_, err := v.Build(unknown, embedded(t))
		if err == nil || errors.Is(err, errAbsent) || !strings.Contains(err.Error(), `"corp"`) {
			t.Errorf("Build error = %v, want a build error naming corp", err)
		}
	})
}

func TestHistoryActivity(t *testing.T) {
	t.Parallel()
	v := historyActivity{}
	if !v.Matches(Snapshot{}) {
		t.Error("history does not always match")
	}

	t.Run("mariana rows", func(t *testing.T) {
		t.Parallel()
		p := props[ActivityList](t, build(t, v, marianaFixture().snapshot()), "activity_list", "history")
		expected := []ActivityItem{
			{Icon: "msg", Title: "Mensagem · e-mail", Meta: "ontem"},
			{Icon: "out", Title: "Saque", Meta: "há 3 dias"},
		}
		if p.Title != "Movimentações" || p.EmptyText != "" || !slices.Equal(p.Items, expected) {
			t.Errorf("props = %+v, want items %+v", p, expected)
		}
	})
	t.Run("no rows", func(t *testing.T) {
		t.Parallel()
		p := props[ActivityList](t, build(t, v, fernandaFixture().snapshot()), "activity_list", "history")
		if p.Title != "Movimentações" || p.Items == nil || len(p.Items) != 0 || p.EmptyText != "Nenhuma movimentação ainda." {
			t.Errorf("props = %+v", p)
		}
	})
	t.Run("at most twenty, most recent first", func(t *testing.T) {
		t.Parallel()
		rows := make([]Activity, 0, 25)
		for i := range 25 {
			rows = append(rows, Activity{
				Kind: "aporte", Title: "Aporte", Source: "account.event.recorded",
				OccurredAt: testNow.Add(-time.Duration(i+1) * time.Minute),
			})
		}
		snap := Snapshot{Now: testNow, Activity: Fetched[[]Activity]{Value: rows}}
		p := props[ActivityList](t, build(t, v, snap), "activity_list", "history")
		if len(p.Items) != historyLimit || p.Items[0].Meta != "há 1 min" || p.Items[19].Meta != "há 20 min" {
			t.Errorf("items = %d, first %q, last %q", len(p.Items), p.Items[0].Meta, p.Items[len(p.Items)-1].Meta)
		}
	})
	t.Run("purchase row", func(t *testing.T) {
		t.Parallel()
		p := props[ActivityList](t, build(t, v, thiagoAfterTudo().snapshot()), "activity_list", "history")
		expected := []ActivityItem{
			{Icon: "out", Title: "Compra · Maré Ações Globais ETF", Meta: "há 2 min", Value: "−US$ 60.520,00"},
			{Icon: "in", Title: "Aporte", Meta: "há 4 dias"},
		}
		if !slices.Equal(p.Items, expected) {
			t.Errorf("items = %+v, want %+v", p.Items, expected)
		}
	})
	if _, err := v.Build(Snapshot{Activity: Fetched[[]Activity]{Err: errDown}}, embedded(t)); !errors.Is(err, errDown) {
		t.Errorf("Build without the timeline error = %v, want %v", err, errDown)
	}
}

func TestEngine_Build_Carteira(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		fixture   fixture
		sections  []string
		total     string
		ret       string
		etf       string
		history   []string
		emptyText string
	}{
		{
			name: "mariana holds three classes", fixture: marianaFixture(),
			sections: []string{"summary", "allocation", "positions_stocks", "positions_etf", "positions_fixed_income", "history"},
			total:    "US$ 248.300,00", ret: "+US$ 19.400,00 (+11,5%)", etf: "US$ 60.600,00",
			history: []string{"Mensagem · e-mail", "Saque"},
		},
		{
			name: "thiago has no fixed income", fixture: thiagoFixture(),
			sections: []string{"summary", "allocation", "positions_stocks", "positions_etf", "history"},
			total:    "US$ 68.000,00", ret: "+US$ 380,00 (+5,4%)", etf: "US$ 5.440,00",
			history: []string{"Aporte"},
		},
		{
			name: "fernanda holds three classes", fixture: fernandaFixture(),
			sections: []string{"summary", "allocation", "positions_stocks", "positions_etf", "positions_fixed_income", "history"},
			total:    "US$ 8.200,00", ret: "+US$ 212,00 (+3,1%)", etf: "US$ 3.690,00",
			history: []string{}, emptyText: "Nenhuma movimentação ainda.",
		},
		{
			name: "thiago after buying everything in acoesg", fixture: thiagoAfterTudo(),
			sections: []string{"summary", "allocation", "positions_stocks", "positions_etf", "history"},
			total:    "US$ 68.000,00", ret: "+US$ 380,00 (+0,6%)", etf: "US$ 65.960,00",
			history: []string{"Compra · Maré Ações Globais ETF", "Aporte"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reporter := &recordingReporter{}
			res, err := newTestEngine(t, tt.fixture.sources(), WithDropReporter(reporter)).Build(t.Context(), "carteira", tt.fixture.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			p := res.Page
			if p.Slug != "carteira" || p.Revision != "v1" || p.Title != "Carteira" || p.Subtitle != "Valores de mercado no dia simulado 0" {
				t.Errorf("envelope = %q %q %q / %q", p.Slug, p.Revision, p.Title, p.Subtitle)
			}
			if !slices.Equal(sectionIDs(p), tt.sections) {
				t.Errorf("sections = %v, want %v", sectionIDs(p), tt.sections)
			}
			if len(p.Omitted) != 0 || len(res.Failures) != 0 || len(reporter.all()) != 0 {
				t.Errorf("omitted %v, failures %v, drops %v", p.Omitted, res.Failures, reporter.all())
			}
			summary := props[PortfolioSummary](t, componentOf(t, p, "summary"), "portfolio_summary", "default")
			if summary.Total != tt.total || summary.Stats[1].Value != tt.ret {
				t.Errorf("summary = %+v, want total %q return %q", summary, tt.total, tt.ret)
			}
			alloc := props[AllocationBreakdown](t, componentOf(t, p, "allocation"), "allocation_breakdown", "default")
			sum := 0
			for _, row := range alloc.Rows {
				n, err := strconv.Atoi(strings.TrimSuffix(row.Share, "%"))
				if err != nil || n != row.BarWidth {
					t.Fatalf("row %+v: share and bar width disagree", row)
				}
				sum += n
			}
			if len(alloc.Rows) != 4 || sum != 100 {
				t.Errorf("allocation rows %d sum to %d, want 4 rows summing to 100", len(alloc.Rows), sum)
			}
			etf := props[PositionList](t, componentOf(t, p, "positions_etf"), "position_list", "etf")
			if etf.Subtotal != tt.etf {
				t.Errorf("etf subtotal = %q, want %q", etf.Subtotal, tt.etf)
			}
			history := props[ActivityList](t, componentOf(t, p, "history"), "activity_list", "history")
			titles := make([]string, 0, len(history.Items))
			for _, it := range history.Items {
				titles = append(titles, it.Title)
			}
			if !slices.Equal(titles, tt.history) || history.EmptyText != tt.emptyText {
				t.Errorf("history = %v %q, want %v %q", titles, history.EmptyText, tt.history, tt.emptyText)
			}
		})
	}
}

func TestEngine_Build_CarteiraFailurePolicy(t *testing.T) {
	t.Parallel()
	all := []string{"summary", "allocation", "positions_stocks", "positions_etf", "positions_fixed_income", "history"}
	tests := []struct {
		name     string
		change   func(*Sources)
		sections []string
		omitted  []Omitted
		subtitle string
	}{
		{
			name:     "timeline down",
			change:   func(s *Sources) { s.Activity = fakeActivity{err: errDown} },
			sections: all[:5],
			omitted:  []Omitted{{ID: "history", Type: "activity_list", Reason: "timeline"}},
			subtitle: "Valores de mercado no dia simulado 0",
		},
		{
			name:     "catalog down",
			change:   func(s *Sources) { s.Products = fakeProducts{err: errDown} },
			sections: []string{"summary", "allocation", "history"},
			omitted: []Omitted{
				{ID: "positions_stocks", Type: "position_list", Reason: "catalog"},
				{ID: "positions_etf", Type: "position_list", Reason: "catalog"},
				{ID: "positions_fixed_income", Type: "position_list", Reason: "catalog"},
			},
			subtitle: "Valores de mercado no dia simulado 0",
		},
		{
			name:     "account down",
			change:   func(s *Sources) { s.Accounts = fakeAccounts{err: errDown} },
			sections: []string{"history"},
			omitted: []Omitted{
				{ID: "summary", Type: "portfolio_summary", Reason: "account-sim"},
				{ID: "allocation", Type: "allocation_breakdown", Reason: "account-sim"},
				{ID: "positions_stocks", Type: "position_list", Reason: "account-sim"},
				{ID: "positions_etf", Type: "position_list", Reason: "account-sim"},
				{ID: "positions_fixed_income", Type: "position_list", Reason: "account-sim"},
			},
		},
		{
			name:     "advisory down keeps the day subtitle",
			change:   func(s *Sources) { s.Customers = fakeCustomers{err: errDown} },
			sections: all,
			subtitle: "Valores de mercado no dia simulado 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := marianaFixture()
			src := f.sources()
			tt.change(&src)
			reporter := &recordingReporter{}
			res, err := newTestEngine(t, src, WithDropReporter(reporter)).Build(t.Context(), "carteira", f.id)
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
			if p.Title != "Carteira" || p.Subtitle != tt.subtitle {
				t.Errorf("heading = %q / %q, want Carteira / %q", p.Title, p.Subtitle, tt.subtitle)
			}
		})
	}
}

// A position whose product the catalog lacks drops only its list, as a
// build error, and never shows the bare id.
func TestEngine_Build_CarteiraUnknownProduct(t *testing.T) {
	t.Parallel()
	f := marianaFixture()
	src := f.sources()
	src.Products = fakeProducts{products: slices.DeleteFunc(testCatalog(), func(p Product) bool { return p.ID == "corp" })}
	res, err := newTestEngine(t, src).Build(t.Context(), "carteira", f.id)
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	want := []Omitted{{ID: "positions_fixed_income", Type: "position_list", Reason: ReasonBuildError}}
	if !slices.Equal(res.Page.Omitted, want) || len(res.Failures) != 1 || res.Failures[0].Section != "positions_fixed_income" {
		t.Errorf("omitted %+v, failures %+v", res.Page.Omitted, res.Failures)
	}
}

// Every Carteira variant reports a catalog without its copy instead of
// serving blank labels.
func TestCarteiraVariants_MissingCopy(t *testing.T) {
	t.Parallel()
	snap := marianaFixture().snapshot()
	for key, reg := range carteiraVariants() {
		t.Run(key.typ+"/"+key.name, func(t *testing.T) {
			t.Parallel()
			if _, err := reg.variant.Build(snap, Catalog{}); err == nil || errors.Is(err, errAbsent) {
				t.Errorf("Build without copy error = %v, want a copy error", err)
			}
		})
	}
}
