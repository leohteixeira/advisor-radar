package screen

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeRegistrations struct {
	registration Registration
	err          error
	delay        time.Duration
}

func (f fakeRegistrations) Registration(ctx context.Context, _ string) (Registration, error) {
	if err := wait(ctx, f.delay); err != nil {
		return Registration{}, err
	}
	if f.err != nil {
		return Registration{}, f.err
	}
	return f.registration, nil
}

type fakePreferences struct {
	preferences Preferences
	err         error
	delay       time.Duration
}

func (f fakePreferences) Preferences(ctx context.Context, _ string) (Preferences, error) {
	if err := wait(ctx, f.delay); err != nil {
		return Preferences{}, err
	}
	if f.err != nil {
		return Preferences{}, f.err
	}
	return f.preferences, nil
}

// testMaxRiskTable is the advisory max-risk table as GetInvestorProfile
// sends it.
func testMaxRiskTable() []ProfileMaxRisk {
	return []ProfileMaxRisk{
		{Profile: "conservador", MaxRisk: 2},
		{Profile: "moderado", MaxRisk: 3},
		{Profile: "arrojado", MaxRisk: 5},
	}
}

// seedRegistration is the account-sim registration seed of one fixture
// client (OrlaApp.dc.html clients()).
func seedRegistration(id string) Registration {
	switch id {
	case fernandaFixture().id:
		return Registration{Email: "fernanda.lima@example.com", Phone: "+55 (19) •••••-4471", City: "Campinas, SP · Brasil", AccountNumber: "Conta 3301-7 · Orla Invest"}
	case thiagoFixture().id:
		return Registration{Email: "thiago.azevedo@example.com", Phone: "+55 (48) •••••-2093", City: "Florianópolis, SC · Brasil", AccountNumber: "Conta 2847-1 · Orla Invest"}
	case marianaFixture().id:
		return Registration{Email: "mariana.costa@example.com", Phone: "+55 (11) •••••-7810", City: "São Paulo, SP · Brasil", AccountNumber: "Conta 1190-4 · Orla Invest"}
	}
	return Registration{}
}

// perfilFooter is the suitability footer for an assessment date.
func perfilFooter(date string) string {
	return "Última avaliação em " + date + ". Para refazer o questionário, fale com a sua assessora."
}

func TestParseCatalog_Perfil(t *testing.T) {
	t.Parallel()
	perfil, ok := embedded(t).screens["perfil"]
	if !ok {
		t.Fatal("catalog has no perfil screen")
	}
	var got []string
	for _, s := range perfil.sections {
		got = append(got, s.id+":"+s.typ+"/"+strings.Join(s.variants, ","))
	}
	want := []string{
		"header:profile_header/default",
		"suitability:profile_scale/conservador,moderado,arrojado",
		"registration:profile_field_list/default",
		"preferences:preference_list/default",
		"advisor:advisor_card/dedicated,default",
	}
	if perfil.revision != "v1" || !slices.Equal(got, want) {
		t.Errorf("perfil %s sections = %v, want v1 %v", perfil.revision, got, want)
	}
	title, err := execute(perfil.title, Fields{})
	if err != nil || title != "Perfil" {
		t.Errorf("title = %q, %v", title, err)
	}
	subtitle, err := execute(perfil.subtitle, Fields{})
	if err != nil || subtitle != "Seus dados, seu perfil de investidor e suas preferências" {
		t.Errorf("subtitle = %q, %v", subtitle, err)
	}
	// The Perfil heading is plain text: it reads no source.
	if len(perfil.titleNeeds) != 0 || len(perfil.subtitleNeeds) != 0 {
		t.Errorf("perfil heading needs %v and %v, want none", perfil.titleNeeds, perfil.subtitleNeeds)
	}
}

func TestProfileHeader(t *testing.T) {
	t.Parallel()
	snap := fernandaFixture().snapshot()
	v := profileHeader{}
	if !v.Matches(snap) || !v.Matches(Snapshot{}) {
		t.Fatal("the header default does not always match")
	}
	got := props[ProfileHeader](t, build(t, v, snap), "profile_header", "default")
	want := ProfileHeader{Initials: "FL", Name: "Fernanda Lima", Subtitle: "Cliente Essencial desde 2024", Account: "Conta 3301-7 · Orla Invest"}
	if got != want {
		t.Errorf("header = %+v, want %+v", got, want)
	}

	// Without the registration the header renders without the account.
	snap.Registration = Fetched[Registration]{Err: errDown}
	want.Account = ""
	if got := props[ProfileHeader](t, build(t, v, snap), "profile_header", "default"); got != want {
		t.Errorf("header without registration = %+v, want %+v", got, want)
	}
	raw, err := json.Marshal(want)
	if err != nil || strings.Contains(string(raw), "account") {
		t.Errorf("header JSON = %s, %v; want no account key", raw, err)
	}
}

func TestProfileHeader_BuildErrors(t *testing.T) {
	t.Parallel()
	noName := fernandaFixture().snapshot()
	noName.Customer.Value.Name = " "
	for name, snap := range map[string]Snapshot{
		"advisory down": {Customer: Fetched[Customer]{Err: errDown}},
		"no name":       noName,
	} {
		if _, err := (profileHeader{}).Build(snap, embedded(t)); err == nil {
			t.Errorf("%s: Build succeeded", name)
		}
	}
}

func TestProfileScale_OneVariantPerProfile(t *testing.T) {
	t.Parallel()
	for _, profile := range profileLevels {
		snap := Snapshot{Profile: Fetched[InvestorProfile]{Value: InvestorProfile{Profile: profile, MaxRisk: 3}}}
		for _, other := range profileLevels {
			if got := (profileScale{profile: other}).Matches(snap); got != (other == profile) {
				t.Errorf("variant %s matches a %s profile = %t", other, profile, got)
			}
		}
	}
	for name, snap := range map[string]Snapshot{
		"profile down":    {Profile: Fetched[InvestorProfile]{Err: errDown}},
		"unknown profile": {Profile: Fetched[InvestorProfile]{Value: InvestorProfile{Profile: "agressivo", MaxRisk: 5}}},
	} {
		for _, level := range profileLevels {
			if (profileScale{profile: level}).Matches(snap) {
				t.Errorf("%s: variant %s matches", name, level)
			}
		}
	}
}

func TestProfileScale_Build(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fixture fixture
		current string
		footer  string
	}{
		{name: "conservador", fixture: fernandaFixture(), current: "conservador", footer: perfilFooter("12/03/2026")},
		{name: "moderado", fixture: marianaFixture(), current: "moderado", footer: perfilFooter("20/01/2026")},
		{name: "arrojado", fixture: thiagoFixture(), current: "arrojado", footer: perfilFooter("04/08/2026")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := props[ProfileScale](t, build(t, profileScale{profile: tt.current}, tt.fixture.snapshot()), "profile_scale", tt.current)
			want := ProfileScale{
				Title:        "Seu perfil de investidor",
				Subtitle:     "Define os destaques de Investir e quando uma compra recebe aviso.",
				CurrentLabel: "Seu perfil",
				Levels: []ProfileLevel{
					{Key: "conservador", Label: "Conservador", Description: "Prioriza preservar o patrimônio. Aceita pouca oscilação.", Limit: "Produtos até risco 2", MaxRisk: 2},
					{Key: "moderado", Label: "Moderado", Description: "Aceita oscilação moderada em busca de mais retorno.", Limit: "Produtos até risco 3", MaxRisk: 3},
					{Key: "arrojado", Label: "Arrojado", Description: "Aceita oscilação alta no curto prazo para buscar retorno maior.", Limit: "Produtos até risco 5", MaxRisk: 5},
				},
				Footer: tt.footer,
			}
			for i := range want.Levels {
				want.Levels[i].Current = want.Levels[i].Key == tt.current
			}
			if got.Title != want.Title || got.Subtitle != want.Subtitle || got.CurrentLabel != want.CurrentLabel || got.Footer != want.Footer {
				t.Errorf("scale = %+v, want %+v", got, want)
			}
			if !slices.Equal(got.Levels, want.Levels) {
				t.Errorf("levels = %+v, want %+v", got.Levels, want.Levels)
			}
		})
	}
}

// The client's own level takes its max risk from the profile answer; the
// others come from the advisory table, never from a BFF constant.
func TestProfileScale_MaxRiskComesFromAdvisory(t *testing.T) {
	t.Parallel()
	snap := fernandaFixture().snapshot()
	snap.Profile.Value.MaxRisk = 1
	snap.Profile.Value.MaxRiskTable = []ProfileMaxRisk{
		{Profile: "Conservador", MaxRisk: 2},
		{Profile: "moderado", MaxRisk: 4},
		{Profile: "arrojado", MaxRisk: 5},
	}
	got := props[ProfileScale](t, build(t, profileScale{profile: "conservador"}, snap), "profile_scale", "conservador")
	var limits []string
	var risks []int
	for _, level := range got.Levels {
		limits = append(limits, level.Limit)
		risks = append(risks, level.MaxRisk)
	}
	if !slices.Equal(risks, []int{1, 4, 5}) || !slices.Equal(limits, []string{"Produtos até risco 1", "Produtos até risco 4", "Produtos até risco 5"}) {
		t.Errorf("risks = %v, limits = %v", risks, limits)
	}
}

func TestProfileScale_BuildErrors(t *testing.T) {
	t.Parallel()
	missing := fernandaFixture().snapshot()
	missing.Profile.Value.MaxRiskTable = []ProfileMaxRisk{{Profile: "conservador", MaxRisk: 2}, {Profile: "moderado", MaxRisk: 3}}
	outOfRange := fernandaFixture().snapshot()
	outOfRange.Profile.Value.MaxRiskTable = []ProfileMaxRisk{{Profile: "moderado", MaxRisk: 3}, {Profile: "arrojado", MaxRisk: 6}}
	undated := fernandaFixture().snapshot()
	undated.Profile.Value.AssessedOn = time.Time{}
	for name, snap := range map[string]Snapshot{
		"profile down":        {Profile: Fetched[InvestorProfile]{Err: errDown}},
		"no table row":        missing,
		"risk outside 1 to 5": outOfRange,
		"no assessment date":  undated,
	} {
		if _, err := (profileScale{profile: "conservador"}).Build(snap, embedded(t)); err == nil {
			t.Errorf("%s: Build succeeded", name)
		}
	}
}

func TestProfileFields(t *testing.T) {
	t.Parallel()
	v := profileFields{}
	if !v.Matches(Snapshot{}) {
		t.Fatal("the registration default does not always match")
	}
	got := props[ProfileFieldList](t, build(t, v, fernandaFixture().snapshot()), "profile_field_list", "default")
	want := []ProfileField{
		{Label: "Nome", Value: "Fernanda Lima"},
		{Label: "E-mail", Value: "fernanda.lima@example.com"},
		{Label: "Telefone", Value: "+55 (19) •••••-4471"},
		{Label: "Cidade", Value: "Campinas, SP · Brasil"},
		{Label: "Segmento", Value: "Essencial"},
		{Label: "Cliente desde", Value: "2024"},
	}
	if got.Title != "Dados cadastrais" || !slices.Equal(got.Fields, want) ||
		got.Footnote != "Dados fictícios. Alterar cadastro fica fora da simulação." {
		t.Errorf("registration = %+v", got)
	}
	noSince := fernandaFixture().snapshot()
	noSince.Customer.Value.Since = ""
	got = props[ProfileFieldList](t, build(t, v, noSince), "profile_field_list", "default")
	if !slices.Equal(got.Fields, want[:5]) {
		t.Errorf("registration without since = %+v, want no Cliente desde row", got.Fields)
	}
	for name, snap := range map[string]Snapshot{
		"registration down": {Registration: Fetched[Registration]{Err: errDown}, Customer: Fetched[Customer]{Value: Customer{Name: "x"}}},
		"advisory down":     {Customer: Fetched[Customer]{Err: errDown}},
	} {
		if _, err := v.Build(snap, embedded(t)); err == nil {
			t.Errorf("%s: Build succeeded", name)
		}
	}
}

func TestPreferenceList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		prefs Preferences
		hint  string
	}{
		{name: "chat, beta off", prefs: Preferences{Channel: "chat"}, hint: "Veja antes as novas versões das telas."},
		{name: "email, beta on", prefs: Preferences{Channel: "email", Beta: true}, hint: "Ligado. Você recebe a revision v2 do início antes dos outros clientes."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := preferenceList{}
			if !v.Matches(Snapshot{}) {
				t.Fatal("the preferences default does not always match")
			}
			got := props[PreferenceList](t, build(t, v, Snapshot{Preferences: Fetched[Preferences]{Value: tt.prefs}}), "preference_list", "default")
			if got.Title != "Preferências" ||
				got.Theme != (PreferenceTheme{Label: "Tema", Hint: "Fica salvo só neste navegador"}) {
				t.Errorf("preferences = %+v", got)
			}
			options := []PreferenceOption{{Value: "chat", Label: "Chat"}, {Value: "email", Label: "E-mail"}}
			if got.Channel.Label != "Canal preferido" || got.Channel.Hint != "Por onde a assessoria fala com você" ||
				got.Channel.Value != tt.prefs.Channel || !slices.Equal(got.Channel.Options, options) {
				t.Errorf("channel = %+v", got.Channel)
			}
			if got.Beta != (PreferenceBeta{Label: "Programa beta", Hint: tt.hint, Enabled: tt.prefs.Beta}) {
				t.Errorf("beta = %+v", got.Beta)
			}
		})
	}
	for name, snap := range map[string]Snapshot{
		"preferences down": {Preferences: Fetched[Preferences]{Err: errDown}},
		"unknown channel":  {Preferences: Fetched[Preferences]{Value: Preferences{Channel: "sms"}}},
	} {
		if _, err := (preferenceList{}).Build(snap, embedded(t)); err == nil {
			t.Errorf("%s: Build succeeded", name)
		}
	}
}

// TestEngine_Build_PerfilSeedClients builds Perfil for the three seed
// clients: the suitability variant is the client's profile and the advisor
// variant follows the segment.
func TestEngine_Build_PerfilSeedClients(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		fixture     fixture
		suitability string
		advisor     string
		email       string
		footer      string
	}{
		{name: "fernanda", fixture: fernandaFixture(), suitability: "conservador", advisor: "default", email: "fernanda.lima@example.com", footer: perfilFooter("12/03/2026")},
		{name: "thiago", fixture: thiagoFixture(), suitability: "arrojado", advisor: "default", email: "thiago.azevedo@example.com", footer: perfilFooter("04/08/2026")},
		{name: "mariana", fixture: marianaFixture(), suitability: "moderado", advisor: "dedicated", email: "mariana.costa@example.com", footer: perfilFooter("20/01/2026")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := newTestEngine(t, tt.fixture.sources()).Build(t.Context(), "perfil", tt.fixture.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			p := res.Page
			if len(res.Failures) != 0 || len(p.Omitted) != 0 {
				t.Fatalf("failures = %+v, omitted = %+v", res.Failures, p.Omitted)
			}
			if p.Slug != "perfil" || p.Revision != "v1" || p.Title != "Perfil" || p.Subtitle != "Seus dados, seu perfil de investidor e suas preferências" {
				t.Errorf("page = %s %s %q %q", p.Slug, p.Revision, p.Title, p.Subtitle)
			}
			if want := []string{"header", "suitability", "registration", "preferences", "advisor"}; !slices.Equal(sectionIDs(p), want) {
				t.Fatalf("sections = %v, want %v", sectionIDs(p), want)
			}
			if got := variantOf(t, p, "suitability"); got != tt.suitability {
				t.Errorf("suitability = %s, want %s", got, tt.suitability)
			}
			if got := variantOf(t, p, "advisor"); got != tt.advisor {
				t.Errorf("advisor = %s, want %s", got, tt.advisor)
			}
			scale := props[ProfileScale](t, componentOf(t, p, "suitability"), "profile_scale", tt.suitability)
			current := slices.IndexFunc(scale.Levels, func(l ProfileLevel) bool { return l.Current })
			if current < 0 || scale.Levels[current].Key != tt.suitability || scale.Footer != tt.footer {
				t.Errorf("scale current %d, footer %q", current, scale.Footer)
			}
			reg := props[ProfileFieldList](t, componentOf(t, p, "registration"), "profile_field_list", "default")
			if reg.Fields[1].Value != tt.email {
				t.Errorf("e-mail = %q, want %q", reg.Fields[1].Value, tt.email)
			}
			prefs := props[PreferenceList](t, componentOf(t, p, "preferences"), "preference_list", "default")
			if prefs.Channel.Value != "chat" || prefs.Beta.Enabled {
				t.Errorf("preferences = %+v, want chat and beta off", prefs)
			}
		})
	}
}

func TestEngine_Build_PerfilFetchesItsSources(t *testing.T) {
	t.Parallel()
	got := newTestEngine(t, thiagoFixture().sources()).plans["perfil"].sources
	want := []Source{SourceAccount, SourceAdvisory, SourceProfile, SourceRegistration, SourcePreferences}
	if !slices.Equal(got, want) {
		t.Errorf("perfil sources = %v, want %v", got, want)
	}
	// The home reads the preferences only to pick its revision.
	for _, slug := range []string{"home", "investir"} {
		sources := newTestEngine(t, thiagoFixture().sources()).plans[slug].sources
		readsPreferences := slices.Contains(sources, SourcePreferences)
		if slices.Contains(sources, SourceRegistration) || readsPreferences != (slug == "home") {
			t.Errorf("%s sources = %v, want no Perfil read but the home preferences", slug, sources)
		}
	}
}

func TestEngine_Build_PerfilFailurePolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		change   func(*Sources)
		sections []string
		omitted  []Omitted
		account  bool
	}{
		{
			name:     "advisory down",
			change:   func(s *Sources) { s.Customers = fakeCustomers{err: errDown} },
			sections: []string{"preferences"},
			omitted: []Omitted{
				{ID: "header", Type: "profile_header", Reason: "advisory"},
				{ID: "suitability", Type: "profile_scale", Reason: "advisory"},
				{ID: "registration", Type: "profile_field_list", Reason: "advisory"},
				{ID: "advisor", Type: "advisor_card", Reason: "advisory"},
			},
		},
		{
			name: "whole advisory service down",
			change: func(s *Sources) {
				s.Customers = fakeCustomers{err: errDown}
				s.Profiles = fakeProfiles{err: errDown}
			},
			sections: []string{"preferences"},
			omitted: []Omitted{
				{ID: "header", Type: "profile_header", Reason: "advisory"},
				{ID: "suitability", Type: "profile_scale", Reason: "advisory"},
				{ID: "registration", Type: "profile_field_list", Reason: "advisory"},
				{ID: "advisor", Type: "advisor_card", Reason: "advisory"},
			},
		},
		{
			name:     "registration down",
			change:   func(s *Sources) { s.Registrations = fakeRegistrations{err: errDown} },
			sections: []string{"header", "suitability", "preferences", "advisor"},
			omitted:  []Omitted{{ID: "registration", Type: "profile_field_list", Reason: "registration"}},
		},
		{
			name:     "preferences down",
			change:   func(s *Sources) { s.Preferences = fakePreferences{err: errDown} },
			sections: []string{"header", "suitability", "registration", "advisor"},
			omitted:  []Omitted{{ID: "preferences", Type: "preference_list", Reason: "preferences"}},
			account:  true,
		},
		{
			name:     "profile down",
			change:   func(s *Sources) { s.Profiles = fakeProfiles{err: errDown} },
			sections: []string{"header", "registration", "preferences", "advisor"},
			omitted:  []Omitted{{ID: "suitability", Type: "profile_scale", Reason: "profile"}},
			account:  true,
		},
		{
			name: "unknown profile",
			change: func(s *Sources) {
				s.Profiles = fakeProfiles{profile: InvestorProfile{Profile: "agressivo", MaxRisk: 5, AssessedOn: testNow, MaxRiskTable: testMaxRiskTable()}}
			},
			sections: []string{"header", "registration", "preferences", "advisor"},
			omitted:  []Omitted{{ID: "suitability", Type: "profile_scale", Reason: "build_error"}},
			account:  true,
		},
		{
			name:     "account down changes nothing on Perfil",
			change:   func(s *Sources) { s.Accounts = fakeAccounts{err: errDown} },
			sections: []string{"header", "suitability", "registration", "preferences", "advisor"},
			account:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := fernandaFixture()
			src := f.sources()
			tt.change(&src)
			reporter := &recordingReporter{}
			res, err := newTestEngine(t, src, WithDropReporter(reporter)).Build(t.Context(), "perfil", f.id)
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
			// The heading has no customer field, so it never degrades.
			if p.Title != "Perfil" || p.Subtitle == "" {
				t.Errorf("heading = %q / %q", p.Title, p.Subtitle)
			}
			if slices.Contains(tt.sections, "header") {
				header := props[ProfileHeader](t, componentOf(t, p, "header"), "profile_header", "default")
				if (header.Account != "") != tt.account {
					t.Errorf("header account = %q, want present %t", header.Account, tt.account)
				}
			}
		})
	}
}

func TestEngine_Build_PerfilUnknownCustomer(t *testing.T) {
	t.Parallel()
	f := thiagoFixture()
	src := f.sources()
	src.Accounts = fakeAccounts{err: ErrUnknownCustomer}
	// A registration NotFound alone never makes the screen a 404.
	src.Registrations = fakeRegistrations{err: errors.New("not found")}
	if _, err := newTestEngine(t, src).Build(t.Context(), "perfil", f.id); !errors.Is(err, ErrUnknownCustomer) {
		t.Errorf("Build error = %v, want %v", err, ErrUnknownCustomer)
	}
}
