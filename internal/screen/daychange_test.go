package screen

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// marianaShocked is Mariana on day 3, after Cobalto fell 53.5%: her Cobalto
// is worth US$ 33.480,00 and her patrimony lost US$ 38.520,00 (−15,5%) on the
// day. Advisory reports the drop, and the timeline holds the revaluation.
func marianaShocked() fixture {
	f := marianaFixture()
	f.account.Positions[0].ValueCents = 3_348_000
	f.account.Acoes = 5_238_000
	f.account.Patrimony = 20_978_000
	f.account.SimDay = 3
	f.account.DayChange = -3_852_000
	f.moments = MomentFacts{
		PortfolioReview: true, CashCents: 6_000_000, PatrimonyCents: 20_978_000,
		PortfolioDrop: true, DropBP: 1551, DropProductID: "cobalto", DropProductBP: -5350, DropDay: 3,
	}
	f.activity = append([]Activity{revaluationRow()}, f.activity...)
	return f
}

// revaluationRow is Mariana's day-3 revaluation on the timeline, a minute
// old.
func revaluationRow() Activity {
	return Activity{
		Kind: "reavaliacao", Title: "Reavaliação", Source: "account.event.recorded",
		OccurredAt: testNow.Add(-time.Minute), ProductID: "cobalto",
		AmountCents: -3_852_000, SimDay: 3, ProductChangeBP: -5350,
	}
}

// failingVariant always fails to build.
type failingVariant struct{}

func (failingVariant) Matches(Snapshot) bool { return true }

func (failingVariant) Build(Snapshot, Catalog) (Component, error) {
	return Component{}, errDown
}

// otherVariant builds a component that has no day-change pill.
type otherVariant struct{}

func (otherVariant) Matches(Snapshot) bool { return true }

func (otherVariant) Build(Snapshot, Catalog) (Component, error) {
	return Component{Type: typeActionGrid, Variant: "default", Props: ActionGrid{}}, nil
}

func TestDayChangeVariant_Matches(t *testing.T) {
	t.Parallel()
	v := dayChangeVariant{base: defaultWealth{}, typ: typeWealthSummary}
	down := marianaShocked().snapshot()
	down.Account = Fetched[Account]{Err: errDown}
	flat := marianaShocked().snapshot()
	flat.Account.Value.DayChange = 0
	tests := []struct {
		name     string
		snap     Snapshot
		expected bool
	}{
		{name: "mariana on the shock day", snap: marianaShocked().snapshot(), expected: true},
		{name: "a flat day", snap: flat},
		{name: "day 0", snap: marianaFixture().snapshot()},
		{name: "account down", snap: down},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := v.Matches(tt.snap); got != tt.expected {
				t.Errorf("Matches = %t, want %t", got, tt.expected)
			}
		})
	}
}

func TestDayChangeVariant_Build(t *testing.T) {
	t.Parallel()
	gain := marianaShocked().snapshot()
	gain.Account.Value.Patrimony = 25_330_000
	gain.Account.Value.DayChange = 500_000
	gain.Account.Value.SimDay = 5
	tests := []struct {
		name     string
		base     Variant
		typ      string
		snap     Snapshot
		text     string
		tone     string
		pillFrom func(any) (string, string)
	}{
		{
			name: "home wealth on the shock day", base: defaultWealth{}, typ: typeWealthSummary,
			snap: marianaShocked().snapshot(), text: "−US$ 38.520,00 (−15,5%) no dia 3", tone: ToneNeg,
			pillFrom: func(p any) (string, string) { w := p.(WealthSummary); return w.DayChange, w.DayChangeTone },
		},
		{
			name: "carteira summary on the shock day", base: portfolioSummary{}, typ: typePortfolioSummary,
			snap: marianaShocked().snapshot(), text: "−US$ 38.520,00 (−15,5%) no dia 3", tone: ToneNeg,
			pillFrom: func(p any) (string, string) { s := p.(PortfolioSummary); return s.DayChange, s.DayChangeTone },
		},
		{
			name: "a gain", base: defaultWealth{}, typ: typeWealthSummary,
			snap: gain, text: "+US$ 5.000,00 (+2,0%) no dia 5", tone: TonePos,
			pillFrom: func(p any) (string, string) { w := p.(WealthSummary); return w.DayChange, w.DayChangeTone },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cat := embedded(t)
			got, err := dayChangeVariant{base: tt.base, typ: tt.typ}.Build(tt.snap, cat)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			if got.Type != tt.typ || got.Variant != "with_day_change" {
				t.Fatalf("component = %s/%s, want %s/with_day_change", got.Type, got.Variant, tt.typ)
			}
			if text, tone := tt.pillFrom(got.Props); text != tt.text || tone != tt.tone {
				t.Errorf("pill = %q %q, want %q %q", text, tone, tt.text, tt.tone)
			}
			// The rest of the props are the base variant's.
			base, err := tt.base.Build(tt.snap, cat)
			if err != nil {
				t.Fatalf("base Build error = %v", err)
			}
			switch p := got.Props.(type) {
			case WealthSummary:
				p.DayChange, p.DayChangeTone = "", ""
				if !reflect.DeepEqual(p, base.Props) {
					t.Errorf("props = %+v, want the default's %+v", p, base.Props)
				}
			case PortfolioSummary:
				p.DayChange, p.DayChangeTone = "", ""
				if !reflect.DeepEqual(p, base.Props) {
					t.Errorf("props = %+v, want the default's %+v", p, base.Props)
				}
			}
		})
	}
}

func TestDayChangeVariant_BuildErrors(t *testing.T) {
	t.Parallel()
	down := marianaShocked().snapshot()
	down.Account = Fetched[Account]{Err: errDown}
	tests := []struct {
		name string
		v    dayChangeVariant
		snap Snapshot
		cat  func(*testing.T) Catalog
	}{
		{name: "account down", v: dayChangeVariant{base: defaultWealth{}, typ: typeWealthSummary}, snap: down, cat: embedded},
		{name: "base fails", v: dayChangeVariant{base: failingVariant{}, typ: typeWealthSummary}, snap: marianaShocked().snapshot(), cat: embedded},
		{
			name: "no pill copy", v: dayChangeVariant{base: defaultWealth{}, typ: typeAdvisorCard},
			snap: marianaShocked().snapshot(), cat: embedded,
		},
		{
			name: "base without a pill", v: dayChangeVariant{base: otherVariant{}, typ: typeWealthSummary},
			snap: marianaShocked().snapshot(), cat: embedded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tt.v.Build(tt.snap, tt.cat(t)); err == nil {
				t.Error("Build returned no error")
			}
		})
	}
	t.Run("base error is kept", func(t *testing.T) {
		t.Parallel()
		v := dayChangeVariant{base: failingVariant{}, typ: typeWealthSummary}
		if _, err := v.Build(marianaShocked().snapshot(), embedded(t)); !errors.Is(err, errDown) {
			t.Errorf("Build error = %v, want the base's", err)
		}
	})
}

// TestEngine_Build_DayChange checks which summary variant the engine picks on
// the home and the Carteira, and the Carteira heading and day stat.
func TestEngine_Build_DayChange(t *testing.T) {
	t.Parallel()
	dayFour := marianaShocked()
	dayFour.account.SimDay = 4
	dayFour.account.DayChange = 0
	tests := []struct {
		name     string
		fixture  fixture
		expected string
		subtitle string
	}{
		{name: "shock day", fixture: marianaShocked(), expected: "with_day_change", subtitle: "Valores de mercado no dia simulado 3"},
		{name: "flat day after the shock", fixture: dayFour, expected: "default", subtitle: "Valores de mercado no dia simulado 4"},
		{name: "day 0", fixture: marianaFixture(), expected: "default", subtitle: "Valores de mercado no dia simulado 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engine := newTestEngine(t, tt.fixture.sources())
			home, err := engine.Build(t.Context(), "home", tt.fixture.id)
			if err != nil {
				t.Fatalf("Build home error = %v", err)
			}
			if got := variantOf(t, home.Page, "wealth"); got != tt.expected {
				t.Errorf("home wealth = %q, want %q", got, tt.expected)
			}
			carteira, err := engine.Build(t.Context(), "carteira", tt.fixture.id)
			if err != nil {
				t.Fatalf("Build carteira error = %v", err)
			}
			if got := variantOf(t, carteira.Page, "summary"); got != tt.expected {
				t.Errorf("carteira summary = %q, want %q", got, tt.expected)
			}
			if carteira.Page.Subtitle != tt.subtitle {
				t.Errorf("carteira subtitle = %q, want %q", carteira.Page.Subtitle, tt.subtitle)
			}
			stats := componentOf(t, carteira.Page, "summary").Props.(PortfolioSummary).Stats
			if day := stats[len(stats)-1]; day.Label != "Dia simulado" || day.Value != tt.subtitle[len(tt.subtitle)-1:] {
				t.Errorf("day stat = %+v, want the simulated day", day)
			}
		})
	}
}
