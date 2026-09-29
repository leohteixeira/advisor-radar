package screen

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"go.opentelemetry.io/otel/codes"
)

// betaSources is a fixture's sources with its stored beta flag set.
func betaSources(f fixture, beta bool) Sources {
	src := f.sources()
	src.Preferences = fakePreferences{preferences: Preferences{Channel: "chat", Beta: beta}}
	return src
}

func TestInBeta(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		prefs    Fetched[Preferences]
		expected bool
	}{
		{name: "beta on", prefs: Fetched[Preferences]{Value: Preferences{Channel: "chat", Beta: true}}, expected: true},
		{name: "beta off", prefs: Fetched[Preferences]{Value: Preferences{Channel: "email"}}, expected: false},
		{name: "preferences failed", prefs: Fetched[Preferences]{Err: errDown}, expected: false},
		{name: "preferences failed with a stale value", prefs: Fetched[Preferences]{Value: Preferences{Beta: true}, Err: errDown}, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := inBeta(Snapshot{Preferences: tt.prefs}); got != tt.expected {
				t.Errorf("inBeta = %t, want %t", got, tt.expected)
			}
		})
	}
}

func TestServedPlan(t *testing.T) {
	t.Parallel()
	def := plan{def: screenDef{revision: "v1"}}
	beta := plan{def: screenDef{revision: "v2"}}
	on := Snapshot{Preferences: Fetched[Preferences]{Value: Preferences{Beta: true}}}
	off := Snapshot{Preferences: Fetched[Preferences]{Value: Preferences{}}}
	down := Snapshot{Preferences: Fetched[Preferences]{Err: errDown}}
	tests := []struct {
		name     string
		hasBeta  bool
		snap     Snapshot
		expected string
	}{
		{name: "beta client", hasBeta: true, snap: on, expected: "v2"},
		{name: "client outside the beta", hasBeta: true, snap: off, expected: "v1"},
		{name: "preferences down", hasBeta: true, snap: down, expected: "v1"},
		{name: "beta client on a screen without a beta revision", hasBeta: false, snap: on, expected: "v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := servedPlan(def, beta, tt.hasBeta, tt.snap).def.revision; got != tt.expected {
				t.Errorf("revision = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestWithBeta(t *testing.T) {
	t.Parallel()
	def := plan{
		sources:  []Source{SourceAccount, SourceTimeline, SourceCatalog},
		required: []Source{SourceAccount, SourceTimeline},
	}
	beta := plan{
		sources:  []Source{SourceAccount, SourceProfile, SourceCatalog},
		required: []Source{SourceAccount, SourceProfile, SourceCatalog},
	}
	gotDef, gotBeta := withBeta(def, beta)
	fetched := []Source{SourceAccount, SourceTimeline, SourceProfile, SourceCatalog, SourcePreferences}
	if !slices.Equal(gotDef.sources, fetched) || !slices.Equal(gotBeta.sources, fetched) {
		t.Errorf("sources = %v and %v, want both %v", gotDef.sources, gotBeta.sources, fetched)
	}
	// The preferences are fetched but never required: their failure is silent.
	if want := []Source{SourceAccount, SourceTimeline}; !slices.Equal(gotDef.required, want) {
		t.Errorf("default required = %v, want %v", gotDef.required, want)
	}
	if want := []Source{SourceAccount, SourceProfile, SourceCatalog}; !slices.Equal(gotBeta.required, want) {
		t.Errorf("beta required = %v, want %v", gotBeta.required, want)
	}
}

// TestEngine_Build_HomeRevisionPerSeedClient serves each seed client its home
// with the beta flag off and on: v1 without highlights, and v2 with the
// profile's rail right after the moment.
func TestEngine_Build_HomeRevisionPerSeedClient(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		fixture   fixture
		rail      string
		products  []string
		moment    string
		railTitle string
	}{
		{
			name: "fernanda", fixture: fernandaFixture(), rail: "profile_conservador",
			products: []string{"tbill", "corp"}, moment: "segment_upgrade_near", railTitle: "Para o seu perfil conservador",
		},
		{
			name: "thiago", fixture: thiagoFixture(), rail: "profile_arrojado",
			products: []string{"cobalto", "acoesg"}, moment: "idle_cash", railTitle: "Para o seu perfil arrojado",
		},
		{
			name: "mariana", fixture: marianaFixture(), rail: "profile_moderado",
			products: []string{"acoesg", "corp"}, moment: "portfolio_review", railTitle: "Para o seu perfil moderado",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+" outside the beta", func(t *testing.T) {
			t.Parallel()
			res, err := newTestEngine(t, betaSources(tt.fixture, false)).Build(t.Context(), "home", tt.fixture.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			p := res.Page
			if p.Revision != "v1" {
				t.Errorf("revision = %q, want v1", p.Revision)
			}
			if want := []string{"moment", "wealth", "actions", "advisor", "activity"}; !slices.Equal(sectionIDs(p), want) {
				t.Errorf("sections = %v, want %v", sectionIDs(p), want)
			}
			if len(p.Omitted) != 0 || len(res.Failures) != 0 {
				t.Errorf("omitted %+v, failures %+v", p.Omitted, res.Failures)
			}
		})
		t.Run(tt.name+" in the beta", func(t *testing.T) {
			t.Parallel()
			reporter := &recordingReporter{}
			res, err := newTestEngine(t, betaSources(tt.fixture, true), WithDropReporter(reporter)).Build(t.Context(), "home", tt.fixture.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			p := res.Page
			if p.SchemaVersion != SchemaVersion || p.Slug != "home" || p.Revision != "v2" {
				t.Errorf("envelope = %d %q %q, want %d home v2", p.SchemaVersion, p.Slug, p.Revision, SchemaVersion)
			}
			if want := []string{"moment", "highlights", "wealth", "actions", "advisor", "activity"}; !slices.Equal(sectionIDs(p), want) {
				t.Errorf("sections = %v, want %v", sectionIDs(p), want)
			}
			if len(p.Omitted) != 0 || len(res.Failures) != 0 || len(reporter.all()) != 0 {
				t.Errorf("omitted %+v, failures %+v, drops %+v", p.Omitted, res.Failures, reporter.all())
			}
			if got := variantOf(t, p, "moment"); got != tt.moment {
				t.Errorf("moment = %q, want %q", got, tt.moment)
			}
			rail := props[ProductRail](t, componentOf(t, p, "highlights"), typeProductRail, tt.rail)
			ids := make([]string, 0, len(rail.Products))
			for _, product := range rail.Products {
				ids = append(ids, product.ProductID)
			}
			if !slices.Equal(ids, tt.products) || rail.Title != tt.railTitle {
				t.Errorf("rail %q = %v, want %q %v", rail.Title, ids, tt.railTitle, tt.products)
			}
		})
	}
}

// TestEngine_Build_HomeRailMatchesInvestir pins that the v2 home rail is the
// Investir highlights component, built by the same variant.
func TestEngine_Build_HomeRailMatchesInvestir(t *testing.T) {
	t.Parallel()
	f := fernandaFixture()
	e := newTestEngine(t, betaSources(f, true))
	home, err := e.Build(t.Context(), "home", f.id)
	if err != nil {
		t.Fatalf("home Build error = %v", err)
	}
	investir, err := e.Build(t.Context(), "investir", f.id)
	if err != nil {
		t.Fatalf("investir Build error = %v", err)
	}
	homeRail := props[ProductRail](t, componentOf(t, home.Page, "highlights"), typeProductRail, "profile_conservador")
	investirRail := props[ProductRail](t, componentOf(t, investir.Page, "highlights"), typeProductRail, "profile_conservador")
	if homeRail.Title != investirRail.Title || homeRail.Subtitle != investirRail.Subtitle || !slices.Equal(homeRail.Products, investirRail.Products) {
		t.Errorf("home rail = %+v, want the investir rail %+v", homeRail, investirRail)
	}
}

func TestEngine_Build_HomeBetaFailurePolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		change   func(*Sources)
		revision string
		sections []string
		omitted  []Omitted
		failed   []Source
	}{
		{
			name:     "preferences down",
			change:   func(s *Sources) { s.Preferences = fakePreferences{err: errDown} },
			revision: "v1",
			sections: []string{"moment", "wealth", "actions", "advisor", "activity"},
		},
		{
			name:     "profile down",
			change:   func(s *Sources) { s.Profiles = fakeProfiles{err: errDown} },
			revision: "v2",
			sections: []string{"moment", "wealth", "actions", "advisor", "activity"},
			omitted:  []Omitted{{ID: "highlights", Type: typeProductRail, Reason: "profile"}},
			failed:   []Source{SourceProfile},
		},
		{
			name:     "catalog down",
			change:   func(s *Sources) { s.Products = fakeProducts{err: errDown} },
			revision: "v2",
			sections: []string{"moment", "wealth", "actions", "advisor", "activity"},
			omitted:  []Omitted{{ID: "highlights", Type: typeProductRail, Reason: "catalog"}},
			failed:   []Source{SourceCatalog},
		},
		{
			name: "unknown profile",
			change: func(s *Sources) {
				s.Profiles = fakeProfiles{profile: InvestorProfile{Profile: "agressivo", MaxRisk: 4}}
			},
			revision: "v2",
			sections: []string{"moment", "wealth", "actions", "advisor", "activity"},
			omitted:  []Omitted{{ID: "highlights", Type: typeProductRail, Reason: ReasonBuildError}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := fernandaFixture()
			src := betaSources(f, true)
			tt.change(&src)
			res, err := newTestEngine(t, src).Build(t.Context(), "home", f.id)
			if err != nil {
				t.Fatalf("Build error = %v, want a 200 page", err)
			}
			p := res.Page
			if p.Revision != tt.revision {
				t.Errorf("revision = %q, want %q", p.Revision, tt.revision)
			}
			if !slices.Equal(sectionIDs(p), tt.sections) {
				t.Errorf("sections = %v, want %v", sectionIDs(p), tt.sections)
			}
			if !slices.Equal(p.Omitted, tt.omitted) {
				t.Errorf("omitted = %+v, want %+v", p.Omitted, tt.omitted)
			}
			var failed []Source
			for _, f := range res.Failures {
				if f.Source != "" {
					if !errors.Is(f.Err, errDown) {
						t.Errorf("failure %+v does not wrap the source error", f)
					}
					failed = append(failed, f.Source)
				}
			}
			if !slices.Equal(failed, tt.failed) {
				t.Errorf("failed sources = %v, want %v", failed, tt.failed)
			}
		})
	}
}

// TestEngine_Build_OtherScreensStayV1 serves Investir, Carteira, and Perfil
// to a beta client: only the home has a beta revision.
func TestEngine_Build_OtherScreensStayV1(t *testing.T) {
	t.Parallel()
	f := marianaFixture()
	e := newTestEngine(t, betaSources(f, true))
	for _, slug := range []string{"investir", "carteira", "perfil"} {
		t.Run(slug, func(t *testing.T) {
			t.Parallel()
			res, err := e.Build(t.Context(), slug, f.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			if res.Page.Revision != "v1" {
				t.Errorf("%s revision = %q, want v1", slug, res.Page.Revision)
			}
		})
	}
}

// TestEngine_Build_EveryScreenCarriesSlugAndRevision builds every catalog
// screen for a client in and out of the beta and checks the envelope names
// the slug and a catalog revision of that screen.
func TestEngine_Build_EveryScreenCarriesSlugAndRevision(t *testing.T) {
	t.Parallel()
	expected := map[string][2]string{ // slug → revision outside, in the beta
		"home":     {"v1", "v2"},
		"investir": {"v1", "v1"},
		"carteira": {"v1", "v1"},
		"perfil":   {"v1", "v1"},
	}
	cat := embedded(t)
	if len(cat.screens) != len(expected) {
		t.Fatalf("catalog screens = %d, want %d", len(cat.screens), len(expected))
	}
	for slug, revisions := range expected {
		for i, beta := range []bool{false, true} {
			f := thiagoFixture()
			res, err := newTestEngine(t, betaSources(f, beta)).Build(t.Context(), slug, f.id)
			if err != nil {
				t.Fatalf("%s beta %t: Build error = %v", slug, beta, err)
			}
			if res.Page.Slug != slug || res.Page.Revision != revisions[i] || res.Page.SchemaVersion != SchemaVersion {
				t.Errorf("%s beta %t: envelope = %q %q %d, want %q %q %d",
					slug, beta, res.Page.Slug, res.Page.Revision, res.Page.SchemaVersion, slug, revisions[i], SchemaVersion)
			}
		}
	}
}

func TestEngine_Build_TracesTheServedRevision(t *testing.T) {
	t.Parallel()
	obs := newObserved()
	f := fernandaFixture()
	if _, err := newTestEngine(t, betaSources(f, true), obs.options()...).Build(t.Context(), "home", f.id); err != nil {
		t.Fatalf("Build error = %v", err)
	}
	screens := obs.named(spanScreen)
	if len(screens) != 1 {
		t.Fatalf("%s spans = %d, want 1", spanScreen, len(screens))
	}
	if rev, _ := stringAttr(screens[0], attrRevision); rev != "v2" {
		t.Errorf("sdui.revision = %q, want v2", rev)
	}
}

// TestEngine_Build_TracesTheDefaultRevisionOnEarlyEnd checks that a screen
// span ending before the revision is chosen still carries the default one.
func TestEngine_Build_TracesTheDefaultRevisionOnEarlyEnd(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cancel bool
		change func(*Sources)
	}{
		{name: "request ended", cancel: true, change: func(*Sources) {}},
		{
			name:   "unknown customer",
			change: func(s *Sources) { s.Accounts = fakeAccounts{err: fmt.Errorf("adapter: %w", ErrUnknownCustomer)} },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.cancel {
				cancel()
			}
			obs := newObserved()
			f := fernandaFixture()
			src := betaSources(f, true)
			tt.change(&src)
			if _, err := newTestEngine(t, src, obs.options()...).Build(ctx, "home", f.id); err == nil {
				t.Fatal("Build returned no error")
			}
			screens := obs.named(spanScreen)
			if len(screens) != 1 {
				t.Fatalf("%s spans = %d, want 1", spanScreen, len(screens))
			}
			if rev, _ := stringAttr(screens[0], attrRevision); rev != "v1" {
				t.Errorf("sdui.revision = %q, want the default v1", rev)
			}
		})
	}
}

// TestEngine_Build_PreferencesFailureOnlyOnItsSpan checks that a failed
// preferences read is silent in the response and in Result.Failures, and is
// recorded only on its Snapshot span.
func TestEngine_Build_PreferencesFailureOnlyOnItsSpan(t *testing.T) {
	t.Parallel()
	obs := newObserved()
	f := fernandaFixture()
	src := betaSources(f, true)
	src.Preferences = fakePreferences{err: errDown}
	res, err := newTestEngine(t, src, obs.options()...).Build(t.Context(), "home", f.id)
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	if res.Page.Revision != "v1" || len(res.Page.Omitted) != 0 || len(res.Failures) != 0 {
		t.Errorf("revision %q, omitted %+v, failures %+v; want v1 with nothing reported",
			res.Page.Revision, res.Page.Omitted, res.Failures)
	}
	spans := obs.named(snapshotSpanPrefix + string(SourcePreferences))
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Errorf("preferences spans = %d, want one with an error status", len(spans))
	}
	if root := obs.named(spanScreen); len(root) != 1 || root[0].Status().Code == codes.Error {
		t.Error("the screen span is marked as an error for a silent fallback")
	}
}
