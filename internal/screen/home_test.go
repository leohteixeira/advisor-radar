package screen

import (
	"slices"
	"testing"
	"time"
)

// props asserts the component's type and variant and returns its props.
func props[T any](t *testing.T, comp Component, typ, variant string) T {
	t.Helper()
	if comp.Type != typ || comp.Variant != variant {
		t.Fatalf("component = %s/%s, want %s/%s", comp.Type, comp.Variant, typ, variant)
	}
	p, ok := comp.Props.(T)
	if !ok {
		t.Fatalf("props are %T", comp.Props)
	}
	return p
}

func build(t *testing.T, v Variant, snap Snapshot) Component {
	t.Helper()
	comp, err := v.Build(snap, embedded(t))
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	return comp
}

func TestWelcomeMoment(t *testing.T) {
	t.Parallel()
	v := welcomeMoment{}
	if !v.Matches(thiagoFixture().snapshot()) || !v.Matches(Snapshot{}) {
		t.Error("welcome does not always match")
	}

	p := props[MomentCard](t, build(t, v, thiagoFixture().snapshot()), "moment_card", "welcome")
	expected := MomentCard{
		Kicker: "Tudo em dia",
		Title:  "Olá, Thiago. Sua conta está em dia.",
		Body:   "Quando algo mudar na sua carteira, você vê aqui primeiro.",
		Tone:   ToneNeutral,
		Icon:   "spark",
	}
	if p != expected {
		t.Errorf("props = %+v, want %+v", p, expected)
	}

	down := thiagoFixture().snapshot()
	down.Customer = Fetched[Customer]{Err: errDown}
	p = props[MomentCard](t, build(t, v, down), "moment_card", "welcome")
	if p.Title != "Olá. Sua conta está em dia." {
		t.Errorf("title without advisory = %q", p.Title)
	}
}

func TestDefaultWealth(t *testing.T) {
	t.Parallel()
	v := defaultWealth{}
	if !v.Matches(Snapshot{}) {
		t.Error("default wealth does not always match")
	}

	tests := []struct {
		name     string
		fixture  fixture
		total    string
		cash     string
		cents    int64
		expected []AllocationRow
	}{
		{
			name: "thiago skips empty fixed income", fixture: thiagoFixture(),
			total: "US$ 68.000,00", cash: "US$ 60.520,00", cents: 6_052_000,
			expected: []AllocationRow{
				{Class: "stocks", Label: "Ações", Share: "3%", BarWidth: 3},
				{Class: "etfs", Label: "ETFs", Share: "8%", BarWidth: 8},
				{Class: "cash", Label: "Caixa", Share: "89%", BarWidth: 89},
			},
		},
		{
			name: "fernanda", fixture: fernandaFixture(),
			total: "US$ 8.200,00", cash: "US$ 1.148,00", cents: 114_800,
			expected: []AllocationRow{
				{Class: "stocks", Label: "Ações", Share: "20%", BarWidth: 20},
				{Class: "etfs", Label: "ETFs", Share: "45%", BarWidth: 45},
				{Class: "fixed_income", Label: "Renda fixa", Share: "21%", BarWidth: 21},
				{Class: "cash", Label: "Caixa", Share: "14%", BarWidth: 14},
			},
		},
		{
			name: "mariana shares sum to 100", fixture: marianaFixture(),
			total: "US$ 248.300,00", cash: "US$ 60.000,00", cents: 6_000_000,
			expected: []AllocationRow{
				{Class: "stocks", Label: "Ações", Share: "37%", BarWidth: 37},
				{Class: "etfs", Label: "ETFs", Share: "24%", BarWidth: 24},
				{Class: "fixed_income", Label: "Renda fixa", Share: "15%", BarWidth: 15},
				{Class: "cash", Label: "Caixa", Share: "24%", BarWidth: 24},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := props[WealthSummary](t, build(t, v, tt.fixture.snapshot()), "wealth_summary", "default")
			if p.TotalLabel != "Patrimônio total" || p.CashLabel != "Disponível para saque" {
				t.Errorf("labels = %q, %q", p.TotalLabel, p.CashLabel)
			}
			if p.Total != tt.total || p.Cash != tt.cash || p.CashCents != tt.cents {
				t.Errorf("total, cash, cents = %q, %q, %d; want %q, %q, %d", p.Total, p.Cash, p.CashCents, tt.total, tt.cash, tt.cents)
			}
			if !slices.Equal(p.Allocation, tt.expected) {
				t.Errorf("allocation = %+v, want %+v", p.Allocation, tt.expected)
			}
		})
	}

	t.Run("empty account", func(t *testing.T) {
		t.Parallel()
		snap := Snapshot{Account: Fetched[Account]{Value: Account{}}}
		p := props[WealthSummary](t, build(t, v, snap), "wealth_summary", "default")
		if p.Total != "US$ 0,00" || p.Allocation == nil || len(p.Allocation) != 0 {
			t.Errorf("props = %+v", p)
		}
	})
	t.Run("account failed", func(t *testing.T) {
		t.Parallel()
		if _, err := v.Build(Snapshot{Account: Fetched[Account]{Err: errDown}}, embedded(t)); err == nil {
			t.Error("Build without the account returned no error")
		}
	})
}

func TestDefaultActions(t *testing.T) {
	t.Parallel()
	v := defaultActions{}
	if !v.Matches(Snapshot{}) {
		t.Error("default actions do not always match")
	}
	p := props[ActionGrid](t, build(t, v, Snapshot{}), "action_grid", "default")
	expected := []GridItem{
		{Label: "Depositar", Icon: "deposit", Action: Action{Type: "panel", Label: "Depositar", Target: "deposit"}},
		{Label: "Sacar", Icon: "withdraw", Action: Action{Type: "panel", Label: "Sacar", Target: "withdraw"}},
		{Label: "Mensagem", Icon: "msg", Action: Action{Type: "panel", Label: "Mensagem", Target: "message"}},
		{Label: "Reclamar", Icon: "alert", Action: Action{Type: "panel", Label: "Reclamar", Target: "complaint"}},
	}
	if !slices.Equal(p.Items, expected) {
		t.Errorf("items = %+v, want %+v", p.Items, expected)
	}
}

func TestAdvisorCard_Dedicated(t *testing.T) {
	t.Parallel()
	v := advisorCard{name: "dedicated", singularOnly: true}
	down := marianaFixture().snapshot()
	down.Customer = Fetched[Customer]{Err: errDown}
	tests := []struct {
		name     string
		snap     Snapshot
		expected bool
	}{
		{name: "singular", snap: marianaFixture().snapshot(), expected: true},
		{name: "advance", snap: thiagoFixture().snapshot(), expected: false},
		{name: "essencial", snap: fernandaFixture().snapshot(), expected: false},
		{name: "advisory failed", snap: down, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := v.Matches(tt.snap); got != tt.expected {
				t.Errorf("Matches = %t, want %t", got, tt.expected)
			}
		})
	}

	p := props[AdvisorCard](t, build(t, v, marianaFixture().snapshot()), "advisor_card", "dedicated")
	expected := AdvisorCard{
		Kicker:   "Sua assessora dedicada",
		Name:     "Ana Paula Ribeiro",
		Initials: "AP",
		Meta:     "Resposta em até 1 h · cliente Singular",
		Action:   &Action{Type: "panel", Label: "Conversar", Target: "message"},
	}
	if p.Kicker != expected.Kicker || p.Name != expected.Name || p.Initials != expected.Initials || p.Meta != expected.Meta {
		t.Errorf("props = %+v, want %+v", p, expected)
	}
	if p.Action == nil || *p.Action != *expected.Action {
		t.Errorf("action = %+v, want %+v", p.Action, expected.Action)
	}
}

func TestAdvisorCard_Default(t *testing.T) {
	t.Parallel()
	v := advisorCard{name: "default"}
	if !v.Matches(Snapshot{}) || !v.Matches(marianaFixture().snapshot()) {
		t.Error("default advisor does not always match")
	}
	tests := []struct {
		name    string
		fixture fixture
		meta    string
	}{
		{name: "essencial", fixture: fernandaFixture(), meta: "Resposta em até 24 h · cliente Essencial"},
		{name: "advance", fixture: thiagoFixture(), meta: "Resposta em até 4 h · cliente Advance"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := props[AdvisorCard](t, build(t, v, tt.fixture.snapshot()), "advisor_card", "default")
			if p.Kicker != "Sua assessora" || p.Name != "Ana Paula Ribeiro" || p.Initials != "AP" || p.Meta != tt.meta {
				t.Errorf("props = %+v", p)
			}
		})
	}

	failing := map[string]func(*Snapshot){
		"unknown segment": func(s *Snapshot) { s.Customer.Value.Segment = "Private" },
		"no advisor":      func(s *Snapshot) { s.Customer.Value.Advisor = "" },
		"advisory failed": func(s *Snapshot) { s.Customer = Fetched[Customer]{Err: errDown} },
	}
	for name, change := range failing {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snap := thiagoFixture().snapshot()
			change(&snap)
			if _, err := v.Build(snap, embedded(t)); err == nil {
				t.Error("Build returned no error")
			}
		})
	}
}

func TestRecentActivity(t *testing.T) {
	t.Parallel()
	v := recentActivity{}
	teamOnly := Snapshot{Now: testNow, Activity: Fetched[[]Activity]{Value: []Activity{
		{Kind: "nota", Title: "Nota do assessor", Source: "advisory.note.recorded"},
		{Kind: "queda", Title: "Queda", Source: "alert.raised"},
		{Kind: "caso", Title: "Caso k1 aberto", Source: "case.opened"},
		{Kind: "mensagem", Title: "Mensagem · chat", Source: "message.triaged"},
	}}}
	tests := []struct {
		name     string
		snap     Snapshot
		expected bool
	}{
		{name: "client rows", snap: thiagoFixture().snapshot(), expected: true},
		{name: "no rows", snap: fernandaFixture().snapshot(), expected: false},
		{name: "only team rows", snap: teamOnly, expected: false},
		{name: "timeline failed", snap: Snapshot{Activity: Fetched[[]Activity]{Err: errDown}}, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := v.Matches(tt.snap); got != tt.expected {
				t.Errorf("Matches = %t, want %t", got, tt.expected)
			}
		})
	}

	t.Run("mixed rows keep only account events and received messages", func(t *testing.T) {
		t.Parallel()
		p := props[ActivityList](t, build(t, v, marianaFixture().snapshot()), "activity_list", "recent")
		expected := []ActivityItem{
			{Icon: "msg", Title: "Mensagem · e-mail", Meta: "ontem"},
			{Icon: "out", Title: "Saque", Meta: "há 3 dias"},
		}
		if p.Title != "Atividade recente" || p.EmptyText != "" || !slices.Equal(p.Items, expected) {
			t.Errorf("props = %+v, want items %+v", p, expected)
		}
	})
	t.Run("occurred_at wins over the indexed age", func(t *testing.T) {
		t.Parallel()
		p := props[ActivityList](t, build(t, v, thiagoFixture().snapshot()), "activity_list", "recent")
		expected := []ActivityItem{{Icon: "in", Title: "Aporte", Meta: "há 4 dias"}}
		if !slices.Equal(p.Items, expected) {
			t.Errorf("items = %+v, want %+v", p.Items, expected)
		}
	})
	t.Run("at most five, most recent first", func(t *testing.T) {
		t.Parallel()
		rows := make([]Activity, 0, 7)
		for i := range 7 {
			rows = append(rows, Activity{
				Kind: "aporte", Title: "Aporte", Source: "account.event.recorded",
				OccurredAt: testNow.Add(-time.Duration(7-i) * time.Hour),
			})
		}
		snap := Snapshot{Now: testNow, Activity: Fetched[[]Activity]{Value: rows}}
		p := props[ActivityList](t, build(t, v, snap), "activity_list", "recent")
		metas := make([]string, 0, len(p.Items))
		for _, it := range p.Items {
			metas = append(metas, it.Meta)
		}
		if want := []string{"há 1 h", "há 2 h", "há 3 h", "há 4 h", "há 5 h"}; !slices.Equal(metas, want) {
			t.Errorf("metas = %v, want %v", metas, want)
		}
		if rows[0].Age != 0 || !rows[0].OccurredAt.Equal(testNow.Add(-7*time.Hour)) {
			t.Error("Build changed the snapshot rows")
		}
	})
	t.Run("icons", func(t *testing.T) {
		t.Parallel()
		for kind, icon := range map[string]string{"aporte": "in", "saque": "out", "aplicacao": "out", "mensagem": "msg", "reavaliacao": ""} {
			if got := activityIcon(kind); got != icon {
				t.Errorf("activityIcon(%q) = %q, want %q", kind, got, icon)
			}
		}
	})
}

func TestEmptyActivity(t *testing.T) {
	t.Parallel()
	v := emptyActivity{}
	if !v.Matches(Snapshot{}) {
		t.Error("empty activity does not always match")
	}
	p := props[ActivityList](t, build(t, v, fernandaFixture().snapshot()), "activity_list", "empty")
	if p.Title != "Atividade recente" || p.Items == nil || len(p.Items) != 0 ||
		p.EmptyText != "Suas movimentações aparecem aqui assim que acontecerem." {
		t.Errorf("props = %+v", p)
	}
}
