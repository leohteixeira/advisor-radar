package screen

import (
	"slices"
	"testing"
	"time"
)

// marianaCaseID is the id of the complaint case Mariana files in the demo.
const marianaCaseID = "01a0e3a5-2f4c-7b1e-9d2a-5c6f7e8a9b0c"

// withCase is Mariana after she files a complaint: one open case, three
// minutes old.
func withCase(f fixture) fixture {
	f.cases = []OpenCase{{ID: marianaCaseID, State: "Aberto", Age: 3 * time.Minute}}
	return f
}

// fernandaUpgraded is Fernanda right after the US$ 10.000 deposit that moves
// her to Advance.
func fernandaUpgraded() fixture {
	f := fernandaFixture()
	f.customer.Segment = "Advance"
	f.moments = MomentFacts{
		SegmentUpgraded: true, UpgradedSegment: "Advance",
		IdleCash: true, CashCents: 1_114_800, PatrimonyCents: 1_820_000,
	}
	return f
}

func TestMomentVariants_Registry(t *testing.T) {
	t.Parallel()
	cat := embedded(t)
	var order []string
	for _, s := range cat.screens["home"].sections {
		if s.id == "moment" {
			order = s.variants
		}
	}
	expected := []string{
		momentCaseOpen, momentSegmentUpgraded, momentSegmentUpgradeNear,
		momentIdleCash, momentPortfolioReview, momentWelcome,
	}
	if !slices.Equal(order, expected) {
		t.Fatalf("moment priority = %v, want %v", order, expected)
	}
	needs := map[string][]Source{
		momentCaseOpen:           {SourceCases, SourceAdvisory},
		momentSegmentUpgraded:    {SourceMoments},
		momentSegmentUpgradeNear: {SourceMoments},
		momentIdleCash:           {SourceMoments, SourceProfile},
		momentPortfolioReview:    {SourceMoments, SourceAdvisory},
		momentWelcome:            nil,
	}
	reg := momentVariants(cat)
	if len(reg) != len(needs) {
		t.Errorf("registered %d moment variants, want %d", len(reg), len(needs))
	}
	for variant, want := range needs {
		r, ok := reg[variantKey{typeMomentCard, variant}]
		if !ok {
			t.Errorf("%s is not registered", variant)
			continue
		}
		if !slices.Equal(r.needs, want) {
			t.Errorf("%s needs %v, want %v", variant, r.needs, want)
		}
	}
}

// TestHome_Moment builds the home and checks the moment the engine picks,
// over the seed clients, the two live transitions of the demo, and the
// failure of each source a moment reads.
func TestHome_Moment(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fixture  fixture
		change   func(*Sources)
		expected string
	}{
		{name: "fernanda near advance", fixture: fernandaFixture(), expected: momentSegmentUpgradeNear},
		{name: "thiago idle cash", fixture: thiagoFixture(), expected: momentIdleCash},
		{name: "mariana portfolio review", fixture: marianaFixture(), expected: momentPortfolioReview},
		{name: "fernanda after the deposit", fixture: fernandaUpgraded(), expected: momentSegmentUpgraded},
		{name: "mariana after a complaint", fixture: withCase(marianaFixture()), expected: momentCaseOpen},
		{
			name: "an open case outranks an upgrade", fixture: withCase(fernandaUpgraded()),
			expected: momentCaseOpen,
		},
		{
			name: "cases down skips case_open", fixture: withCase(marianaFixture()),
			change:   func(s *Sources) { s.Cases = fakeCases{err: errDown} },
			expected: momentPortfolioReview,
		},
		{
			name: "moments down keeps case_open", fixture: withCase(marianaFixture()),
			change:   func(s *Sources) { s.Moments = fakeMoments{err: errDown} },
			expected: momentCaseOpen,
		},
		{
			name: "moments down without a case", fixture: marianaFixture(),
			change:   func(s *Sources) { s.Moments = fakeMoments{err: errDown} },
			expected: momentWelcome,
		},
		{
			name: "profile down falls through idle_cash", fixture: thiagoFixture(),
			change:   func(s *Sources) { s.Profiles = fakeProfiles{err: errDown} },
			expected: momentWelcome,
		},
		{
			name: "profile down after the deposit keeps the upgrade", fixture: fernandaUpgraded(),
			change:   func(s *Sources) { s.Profiles = fakeProfiles{err: errDown} },
			expected: momentSegmentUpgraded,
		},
		{
			name: "advisory down falls through case_open and review", fixture: withCase(marianaFixture()),
			change:   func(s *Sources) { s.Customers = fakeCustomers{err: errDown} },
			expected: momentWelcome,
		},
		{
			name: "advisory down keeps the near upgrade", fixture: fernandaFixture(),
			change:   func(s *Sources) { s.Customers = fakeCustomers{err: errDown} },
			expected: momentSegmentUpgradeNear,
		},
		{
			name: "advisor without a name falls through case_open", fixture: withCase(marianaFixture()),
			change: func(s *Sources) {
				c := marianaFixture().customer
				c.Advisor = ""
				s.Customers = fakeCustomers{customer: c}
			},
			expected: momentWelcome,
		},
		{
			name: "case in a segment without an SLA falls through", fixture: withCase(marianaFixture()),
			change: func(s *Sources) {
				c := marianaFixture().customer
				c.Segment = "Platinum"
				s.Customers = fakeCustomers{customer: c}
			},
			expected: momentPortfolioReview,
		},
		{
			name: "upgrade to a segment without an SLA falls through", fixture: fernandaUpgraded(),
			change: func(s *Sources) {
				m := fernandaUpgraded().moments
				m.UpgradedSegment = "Platinum"
				s.Moments = fakeMoments{facts: m}
			},
			expected: momentIdleCash,
		},
		{
			name: "timeline down keeps idle_cash", fixture: thiagoFixture(),
			change:   func(s *Sources) { s.Activity = fakeActivity{err: errDown} },
			expected: momentIdleCash,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := tt.fixture.sources()
			if tt.change != nil {
				tt.change(&src)
			}
			res, err := newTestEngine(t, src).Build(t.Context(), "home", tt.fixture.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			if got := variantOf(t, res.Page, "moment"); got != tt.expected {
				t.Errorf("moment = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCaseOpenMoment(t *testing.T) {
	t.Parallel()
	v := caseOpenMoment{cat: embedded(t)}
	snap := withCase(marianaFixture()).snapshot()
	snap.Cases.Value = append(snap.Cases.Value, OpenCase{ID: "01a0e3a4-0000-7000-8000-000000000001", State: "Em análise", Age: 2 * 24 * time.Hour})
	if !v.Matches(snap) {
		t.Fatal("case_open does not match an open case")
	}
	p := props[MomentCard](t, build(t, v, snap), typeMomentCard, momentCaseOpen)
	expected := MomentCard{
		Kicker: "Atendimento em andamento",
		Title:  "Sua reclamação está com a Ana Paula Ribeiro",
		Body:   "Como cliente Singular, você recebe resposta em até 1 h.",
		Meta:   "Protocolo 01A0E3A5-2F4C · aberto há 3 min",
		Tone:   ToneInfo,
		Icon:   "inbox",
		Action: &Action{Type: ActionPanel, Label: "Ver conversa", Target: PanelMessage},
	}
	assertMoment(t, p, expected)

	for name, s := range map[string]Snapshot{
		"no case":       marianaFixture().snapshot(),
		"cases down":    func() Snapshot { s := snap; s.Cases = Fetched[[]OpenCase]{Err: errDown}; return s }(),
		"advisory down": func() Snapshot { s := snap; s.Customer = Fetched[Customer]{Err: errDown}; return s }(),
	} {
		if v.Matches(s) {
			t.Errorf("case_open matches with %s", name)
		}
		if _, err := v.Build(s, embedded(t)); err == nil {
			t.Errorf("case_open builds with %s", name)
		}
	}
	unknown := snap
	unknown.Customer.Value.Segment = "Platinum"
	if v.Matches(unknown) {
		t.Error("case_open matches a segment without an SLA")
	}
	if _, err := v.Build(unknown, embedded(t)); err == nil {
		t.Error("case_open builds for a segment without an SLA")
	}
}

func TestSegmentUpgradedMoment(t *testing.T) {
	t.Parallel()
	v := segmentUpgradedMoment{cat: embedded(t)}
	snap := fernandaUpgraded().snapshot()
	if !v.Matches(snap) {
		t.Fatal("segment_upgraded does not match a live upgrade")
	}
	p := props[MomentCard](t, build(t, v, snap), typeMomentCard, momentSegmentUpgraded)
	expected := MomentCard{
		Kicker: "Novo segmento",
		Title:  "Fernanda, você agora é cliente Advance",
		Body:   "Sua assessoria passa a responder em até 4 h.",
		Tone:   ToneGold,
		Icon:   "segment",
		Action: &Action{Type: ActionNavigate, Label: "Ver produtos", Target: ScreenInvestir},
	}
	assertMoment(t, p, expected)

	// The segment comes from the moment facts, not the customer read.
	anonymous := snap
	anonymous.Customer = Fetched[Customer]{Err: errDown}
	p = props[MomentCard](t, build(t, v, anonymous), typeMomentCard, momentSegmentUpgraded)
	if p.Title != "Você agora é cliente Advance" || p.Body != expected.Body {
		t.Errorf("without advisory = %q / %q", p.Title, p.Body)
	}

	noSegment := snap
	noSegment.Moments.Value.UpgradedSegment = ""
	if v.Matches(noSegment) || v.Matches(fernandaFixture().snapshot()) {
		t.Error("segment_upgraded matches without a live upgrade")
	}
	down := snap
	down.Moments = Fetched[MomentFacts]{Err: errDown}
	if v.Matches(down) {
		t.Error("segment_upgraded matches with the facts down")
	}
	if _, err := v.Build(down, embedded(t)); err == nil {
		t.Error("segment_upgraded builds with the facts down")
	}
	if _, err := v.Build(noSegment, embedded(t)); err == nil {
		t.Error("segment_upgraded builds without a segment SLA")
	}
	unknown := snap
	unknown.Moments.Value.UpgradedSegment = "Platinum"
	if v.Matches(unknown) {
		t.Error("segment_upgraded matches a segment without an SLA")
	}
}

func TestSegmentUpgradeNearMoment(t *testing.T) {
	t.Parallel()
	v := segmentUpgradeNearMoment{}
	snap := fernandaFixture().snapshot()
	if !v.Matches(snap) {
		t.Fatal("segment_upgrade_near does not match fernanda")
	}
	p := props[MomentCard](t, build(t, v, snap), typeMomentCard, momentSegmentUpgradeNear)
	expected := MomentCard{
		Kicker: "Perto do Advance",
		Title:  "Fernanda, faltam US$ 1.800,00 para o Advance",
		Body:   "A partir de US$ 10.000,00 você vira cliente Advance, com resposta da assessoria em até 4 h.",
		Tone:   ToneGold,
		Icon:   "segment",
		Action: &Action{Type: ActionPanel, Label: "Depositar", Target: PanelDeposit},
	}
	assertMoment(t, p, expected)

	anonymous := snap
	anonymous.Customer = Fetched[Customer]{Err: errDown}
	p = props[MomentCard](t, build(t, v, anonymous), typeMomentCard, momentSegmentUpgradeNear)
	if p.Title != "Faltam US$ 1.800,00 para o Advance" {
		t.Errorf("title without advisory = %q", p.Title)
	}
	if v.Matches(thiagoFixture().snapshot()) {
		t.Error("segment_upgrade_near matches thiago")
	}
	down := snap
	down.Moments = Fetched[MomentFacts]{Err: errDown}
	if v.Matches(down) {
		t.Error("segment_upgrade_near matches with the facts down")
	}
	if _, err := v.Build(down, embedded(t)); err == nil {
		t.Error("segment_upgrade_near builds with the facts down")
	}
}

func TestIdleCashMoment(t *testing.T) {
	t.Parallel()
	v := idleCashMoment{}
	snap := thiagoFixture().snapshot()
	if !v.Matches(snap) {
		t.Fatal("idle_cash does not match thiago")
	}
	p := props[MomentCard](t, build(t, v, snap), typeMomentCard, momentIdleCash)
	expected := MomentCard{
		Kicker: "Caixa parado",
		Title:  "Thiago, 89% do seu patrimônio está em caixa",
		Body:   "US$ 60.520,00 parados há 4 dias. Veja produtos para o seu perfil arrojado.",
		Tone:   ToneInfo,
		Icon:   "cash",
		Action: &Action{Type: ActionNavigate, Label: "Ver produtos", Target: ScreenInvestir},
	}
	assertMoment(t, p, expected)

	undated := snap
	undated.Activity = Fetched[[]Activity]{Err: errDown}
	undated.Customer = Fetched[Customer]{Err: errDown}
	p = props[MomentCard](t, build(t, v, undated), typeMomentCard, momentIdleCash)
	if p.Title != "89% do seu patrimônio está em caixa" ||
		p.Body != "US$ 60.520,00 parados em caixa. Veja produtos para o seu perfil arrojado." {
		t.Errorf("undated = %q / %q", p.Title, p.Body)
	}

	// Patrimony includes cash, so more cash than patrimony is a bad read;
	// the share caps at 100% instead of failing the section.
	over := snap
	over.Moments.Value.CashCents = 7_000_000
	p = props[MomentCard](t, build(t, v, over), typeMomentCard, momentIdleCash)
	if p.Title != "Thiago, 100% do seu patrimônio está em caixa" {
		t.Errorf("title with cash above patrimony = %q", p.Title)
	}

	upper := snap
	upper.Profile.Value.Profile = "Arrojado"
	if p := props[MomentCard](t, build(t, v, upper), typeMomentCard, momentIdleCash); p.Body != expected.Body {
		t.Errorf("body with a capitalized profile = %q", p.Body)
	}

	for name, s := range map[string]Snapshot{
		"fernanda":      fernandaFixture().snapshot(),
		"facts down":    func() Snapshot { s := snap; s.Moments = Fetched[MomentFacts]{Err: errDown}; return s }(),
		"profile down":  func() Snapshot { s := snap; s.Profile = Fetched[InvestorProfile]{Err: errDown}; return s }(),
		"empty profile": func() Snapshot { s := snap; s.Profile.Value = InvestorProfile{}; return s }(),
	} {
		if v.Matches(s) {
			t.Errorf("idle_cash matches with %s", name)
		}
	}
	for name, s := range map[string]Snapshot{
		"facts down":   func() Snapshot { s := snap; s.Moments = Fetched[MomentFacts]{Err: errDown}; return s }(),
		"profile down": func() Snapshot { s := snap; s.Profile = Fetched[InvestorProfile]{Err: errDown}; return s }(),
	} {
		if _, err := v.Build(s, embedded(t)); err == nil {
			t.Errorf("idle_cash builds with %s", name)
		}
	}
}

func TestIdleDays(t *testing.T) {
	t.Parallel()
	account := func(age time.Duration, dated bool) Activity {
		a := Activity{Kind: "aporte", Source: "account.event.recorded", Age: age}
		if dated {
			a.OccurredAt = testNow.Add(-age)
		}
		return a
	}
	tests := []struct {
		name     string
		rows     []Activity
		down     bool
		expected int
		ok       bool
	}{
		{name: "four days", rows: []Activity{account(4*24*time.Hour+time.Minute, true)}, expected: 4, ok: true},
		{name: "one day is singular input", rows: []Activity{account(30*time.Hour, true)}, expected: 1, ok: true},
		{name: "minutes ago is at least 1", rows: []Activity{account(5*time.Minute, true)}, expected: 1, ok: true},
		{
			name: "newest account row wins",
			rows: []Activity{
				account(9*24*time.Hour, true),
				{Kind: "telefone", Source: "advisory.note.recorded", Age: time.Minute},
				account(2*24*time.Hour, true),
			},
			expected: 2, ok: true,
		},
		{name: "undated row uses its age", rows: []Activity{account(3*24*time.Hour, false)}, expected: 3, ok: true},
		{name: "no account rows", rows: []Activity{{Kind: "telefone", Source: "advisory.note.recorded", Age: time.Hour}}},
		{name: "empty timeline", rows: []Activity{}},
		{name: "timeline down", down: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snap := Snapshot{Now: testNow, Activity: Fetched[[]Activity]{Value: tt.rows}}
			if tt.down {
				snap.Activity.Err = errDown
			}
			got, ok := idleDays(snap)
			if got != tt.expected || ok != tt.ok {
				t.Errorf("idleDays = %d %t, want %d %t", got, ok, tt.expected, tt.ok)
			}
		})
	}
}

func TestPortfolioReviewMoment(t *testing.T) {
	t.Parallel()
	v := portfolioReviewMoment{}
	snap := marianaFixture().snapshot()
	if !v.Matches(snap) {
		t.Fatal("portfolio_review does not match mariana")
	}
	p := props[MomentCard](t, build(t, v, snap), typeMomentCard, momentPortfolioReview)
	expected := MomentCard{
		Kicker: "Sua assessora",
		Title:  "Mariana, sua revisão de carteira está disponível",
		Body:   "A Ana Paula Ribeiro separou 30 minutos nesta semana para revisar a carteira com você.",
		Tone:   ToneNeutral,
		Icon:   "calendar",
		Action: &Action{Type: ActionPanel, Label: "Conversar", Target: PanelMessage},
	}
	assertMoment(t, p, expected)

	down := snap
	down.Customer = Fetched[Customer]{Err: errDown}
	if v.Matches(down) || v.Matches(thiagoFixture().snapshot()) {
		t.Error("portfolio_review matches without the advisor or the fact")
	}
	if _, err := v.Build(down, embedded(t)); err == nil {
		t.Error("portfolio_review builds without the advisor")
	}
	factsDown := snap
	factsDown.Moments = Fetched[MomentFacts]{Err: errDown}
	if v.Matches(factsDown) {
		t.Error("portfolio_review matches with the facts down")
	}
}

func TestMomentComponent_CopyError(t *testing.T) {
	t.Parallel()
	if _, err := momentComponent(momentWelcome, MomentCard{}, errDown); err == nil {
		t.Error("momentComponent hides the copy error")
	}
}

func assertMoment(t *testing.T, got, expected MomentCard) {
	t.Helper()
	if got.Action == nil || expected.Action == nil || *got.Action != *expected.Action {
		t.Errorf("action = %+v, want %+v", got.Action, expected.Action)
	}
	got.Action, expected.Action = nil, nil
	if got != expected {
		t.Errorf("props = %+v, want %+v", got, expected)
	}
}

func TestLatestCase(t *testing.T) {
	t.Parallel()
	newest := OpenCase{ID: "b", Age: time.Minute}
	older := OpenCase{ID: "a", Age: time.Hour}
	for _, cases := range [][]OpenCase{{newest, older}, {older, newest}, {newest}} {
		if got := latestCase(cases); got != newest {
			t.Errorf("latestCase(%v) = %v, want %v", cases, got, newest)
		}
	}
}
