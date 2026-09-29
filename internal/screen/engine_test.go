package screen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type drop struct {
	slug, typ, reason string
}

// recordingReporter collects ComponentDropped calls.
type recordingReporter struct {
	mu    sync.Mutex
	drops []drop
}

func (r *recordingReporter) ComponentDropped(_ context.Context, slug, typ, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drops = append(r.drops, drop{slug: slug, typ: typ, reason: reason})
}

func (r *recordingReporter) all() []drop {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.drops)
}

func sectionIDs(p Page) []string {
	ids := make([]string, 0, len(p.Sections))
	for _, s := range p.Sections {
		ids = append(ids, s.ID)
	}
	return ids
}

func variantOf(t *testing.T, p Page, id string) string {
	t.Helper()
	for _, s := range p.Sections {
		if s.ID == id {
			if len(s.Components) != 1 {
				t.Fatalf("section %q has %d components", id, len(s.Components))
			}
			return s.Components[0].Variant
		}
	}
	t.Fatalf("page has no section %q", id)
	return ""
}

func componentOf(t *testing.T, p Page, id string) Component {
	t.Helper()
	for _, s := range p.Sections {
		if s.ID == id {
			return s.Components[0]
		}
	}
	t.Fatalf("page has no section %q", id)
	return Component{}
}

func newTestEngine(t *testing.T, src Sources, opts ...Option) *Engine {
	t.Helper()
	clock := WithClock(func() time.Time { return testNow })
	e, err := New(src, append([]Option{clock}, opts...)...)
	if err != nil {
		t.Fatalf("New error = %v", err)
	}
	return e
}

func TestNew(t *testing.T) {
	t.Parallel()
	if _, err := New(thiagoFixture().sources()); err != nil {
		t.Fatalf("New error = %v", err)
	}
	missing := thiagoFixture().sources()
	missing.Activity = nil
	if _, err := New(missing); err == nil {
		t.Error("New without a source returned no error")
	}

	variants := builtinVariants(embedded(t))
	delete(variants, variantKey{typ: typeAdvisorCard, name: "dedicated"})
	if _, err := newEngine(thiagoFixture().sources(), embedded(t), variants); !errors.Is(err, errCatalog) {
		t.Errorf("newEngine with an unimplemented variant error = %v, want %v", err, errCatalog)
	}
}

func TestMustNew_Panics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("MustNew without sources did not panic")
		}
	}()
	MustNew(Sources{})
}

// TestEngine_EverySectionEndsInDefault checks that the last variant of every
// section matches whatever the snapshot holds. The highlights rail is the
// exception specs/http/bff.md names: its default is the variant of the
// customer's profile, which TestProductRail_OneVariantPerProfile covers.
func TestEngine_EverySectionEndsInDefault(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t, thiagoFixture().sources())
	allDown := Snapshot{
		Account:  Fetched[Account]{Err: errDown},
		Customer: Fetched[Customer]{Err: errDown},
		Activity: Fetched[[]Activity]{Err: errDown},
	}
	snaps := []Snapshot{
		{},
		allDown,
		fernandaFixture().snapshot(),
		thiagoFixture().snapshot(),
		marianaFixture().snapshot(),
	}
	for slug, p := range e.plans {
		for _, sec := range p.sections {
			if slug == "investir" && sec.id == "highlights" {
				continue
			}
			last := sec.variants[len(sec.variants)-1]
			for i, snap := range snaps {
				if !last.variant.Matches(snap) {
					t.Errorf("%s/%s default %q does not match snapshot %d", slug, sec.id, last.name, i)
				}
			}
		}
	}
}

func TestEngine_Build_SeedClients(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fixture  fixture
		title    string
		subtitle string
		advisor  string
		activity string
		total    string
		moment   string
	}{
		{
			name: "fernanda", fixture: fernandaFixture(),
			title: "Olá, Fernanda", subtitle: "Cliente Essencial desde 2024",
			advisor: "default", activity: "empty", total: "US$ 8.200,00", moment: "segment_upgrade_near",
		},
		{
			name: "thiago", fixture: thiagoFixture(),
			title: "Olá, Thiago", subtitle: "Cliente Advance desde 2024",
			advisor: "default", activity: "recent", total: "US$ 68.000,00", moment: "idle_cash",
		},
		{
			name: "mariana", fixture: marianaFixture(),
			title: "Olá, Mariana", subtitle: "Cliente Singular desde 2021",
			advisor: "dedicated", activity: "recent", total: "US$ 248.300,00", moment: "portfolio_review",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reporter := &recordingReporter{}
			e := newTestEngine(t, tt.fixture.sources(), WithDropReporter(reporter))
			res, err := e.Build(t.Context(), "home", tt.fixture.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			p := res.Page
			if p.SchemaVersion != 1 || p.Slug != "home" || p.Revision != "v1" {
				t.Errorf("envelope = %d %q %q", p.SchemaVersion, p.Slug, p.Revision)
			}
			if p.Title != tt.title || p.Subtitle != tt.subtitle {
				t.Errorf("heading = %q / %q, want %q / %q", p.Title, p.Subtitle, tt.title, tt.subtitle)
			}
			if want := []string{"moment", "wealth", "actions", "advisor", "activity"}; !slices.Equal(sectionIDs(p), want) {
				t.Errorf("sections = %v, want %v", sectionIDs(p), want)
			}
			if len(p.Omitted) != 0 || len(res.Failures) != 0 || len(reporter.all()) != 0 {
				t.Errorf("omitted %v, failures %v, drops %v", p.Omitted, res.Failures, reporter.all())
			}
			if got := variantOf(t, p, "moment"); got != tt.moment {
				t.Errorf("moment = %q, want %q", got, tt.moment)
			}
			if got := variantOf(t, p, "advisor"); got != tt.advisor {
				t.Errorf("advisor = %q, want %q", got, tt.advisor)
			}
			if got := variantOf(t, p, "activity"); got != tt.activity {
				t.Errorf("activity = %q, want %q", got, tt.activity)
			}
			wealth := props[WealthSummary](t, componentOf(t, p, "wealth"), "wealth_summary", "default")
			if wealth.Total != tt.total {
				t.Errorf("total = %q, want %q", wealth.Total, tt.total)
			}
			sum := 0
			for _, row := range wealth.Allocation {
				n, err := strconv.Atoi(strings.TrimSuffix(row.Share, "%"))
				if err != nil {
					t.Fatalf("share %q: %v", row.Share, err)
				}
				sum += n
			}
			if sum != 100 {
				t.Errorf("shares sum to %d, want 100", sum)
			}
		})
	}
}

func TestEngine_Build_FailurePolicy(t *testing.T) {
	t.Parallel()
	all := []string{"moment", "wealth", "actions", "advisor", "activity"}
	tests := []struct {
		name     string
		change   func(*Sources)
		sections []string
		omitted  []Omitted
		title    string
		subtitle string
		moment   string
		failures int
	}{
		{
			name:     "timeline down",
			change:   func(s *Sources) { s.Activity = fakeActivity{err: errDown} },
			sections: []string{"moment", "wealth", "actions", "advisor"},
			omitted:  []Omitted{{ID: "activity", Type: "activity_list", Reason: "timeline"}},
			title:    "Olá, Thiago", subtitle: "Cliente Advance desde 2024",
			moment: "idle_cash", failures: 1,
		},
		{
			name:     "advisory down",
			change:   func(s *Sources) { s.Customers = fakeCustomers{err: errDown} },
			sections: []string{"moment", "wealth", "actions", "activity"},
			omitted:  []Omitted{{ID: "advisor", Type: "advisor_card", Reason: "advisory"}},
			title:    "Olá", subtitle: "",
			moment: "idle_cash", failures: 1,
		},
		{
			name:     "account down",
			change:   func(s *Sources) { s.Accounts = fakeAccounts{err: errDown} },
			sections: []string{"moment", "actions", "advisor", "activity"},
			omitted:  []Omitted{{ID: "wealth", Type: "wealth_summary", Reason: "account-sim"}},
			title:    "Olá, Thiago", subtitle: "Cliente Advance desde 2024",
			moment: "idle_cash", failures: 1,
		},
		{
			name:     "moments down",
			change:   func(s *Sources) { s.Moments = fakeMoments{err: errDown} },
			sections: all,
			title:    "Olá, Thiago", subtitle: "Cliente Advance desde 2024",
			moment: "welcome", failures: 1,
		},
		{
			name:     "profile down",
			change:   func(s *Sources) { s.Profiles = fakeProfiles{err: errDown} },
			sections: all,
			title:    "Olá, Thiago", subtitle: "Cliente Advance desde 2024",
			moment: "welcome", failures: 1,
		},
		{
			name:     "cases down",
			change:   func(s *Sources) { s.Cases = fakeCases{err: errDown} },
			sections: all,
			title:    "Olá, Thiago", subtitle: "Cliente Advance desde 2024",
			moment: "idle_cash", failures: 1,
		},
		{
			name: "every source down",
			change: func(s *Sources) {
				s.Accounts = fakeAccounts{err: errDown}
				s.Customers = fakeCustomers{err: errDown}
				s.Activity = fakeActivity{err: errDown}
				s.Moments = fakeMoments{err: errDown}
				s.Profiles = fakeProfiles{err: errDown}
				s.Cases = fakeCases{err: errDown}
			},
			sections: []string{"moment", "actions"},
			omitted: []Omitted{
				{ID: "wealth", Type: "wealth_summary", Reason: "account-sim"},
				{ID: "advisor", Type: "advisor_card", Reason: "advisory"},
				{ID: "activity", Type: "activity_list", Reason: "timeline"},
			},
			title: "Olá", subtitle: "",
			moment: "welcome", failures: 6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := thiagoFixture()
			src := f.sources()
			tt.change(&src)
			reporter := &recordingReporter{}
			e := newTestEngine(t, src, WithDropReporter(reporter))
			res, err := e.Build(t.Context(), "home", f.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			p := res.Page
			if !slices.Equal(sectionIDs(p), tt.sections) {
				t.Errorf("sections = %v, want %v (of %v)", sectionIDs(p), tt.sections, all)
			}
			if !slices.Equal(p.Omitted, tt.omitted) {
				t.Errorf("omitted = %+v, want %+v", p.Omitted, tt.omitted)
			}
			if p.Title != tt.title || p.Subtitle != tt.subtitle {
				t.Errorf("heading = %q / %q, want %q / %q", p.Title, p.Subtitle, tt.title, tt.subtitle)
			}
			if got := variantOf(t, p, "moment"); got != tt.moment {
				t.Errorf("moment = %q, want %q", got, tt.moment)
			}
			drops := reporter.all()
			if len(drops) != len(tt.omitted) {
				t.Fatalf("drops = %+v, want one per omitted section", drops)
			}
			for i, o := range tt.omitted {
				if want := (drop{slug: "home", typ: o.Type, reason: o.Reason}); drops[i] != want {
					t.Errorf("drop %d = %+v, want %+v", i, drops[i], want)
				}
			}
			if len(res.Failures) != tt.failures {
				t.Errorf("failures = %+v, want %d, one per failed source", res.Failures, tt.failures)
			}
			for _, failure := range res.Failures {
				if !errors.Is(failure.Err, errDown) || failure.Source == "" {
					t.Errorf("failure = %+v", failure)
				}
			}
		})
	}
}

func TestEngine_Build_NoActivity(t *testing.T) {
	t.Parallel()
	f := fernandaFixture()
	res, err := newTestEngine(t, f.sources()).Build(t.Context(), "home", f.id)
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	p := props[ActivityList](t, componentOf(t, res.Page, "activity"), "activity_list", "empty")
	if p.EmptyText != "Suas movimentações aparecem aqui assim que acontecerem." {
		t.Errorf("empty text = %q", p.EmptyText)
	}
}

func TestEngine_Build_UnknownCustomer(t *testing.T) {
	t.Parallel()
	src := thiagoFixture().sources()
	src.Accounts = fakeAccounts{err: fmt.Errorf("adapter: %w", ErrUnknownCustomer)}
	reporter := &recordingReporter{}
	_, err := newTestEngine(t, src, WithDropReporter(reporter)).Build(t.Context(), "home", "id")
	if !errors.Is(err, ErrUnknownCustomer) {
		t.Errorf("Build error = %v, want %v", err, ErrUnknownCustomer)
	}
	if len(reporter.all()) != 0 {
		t.Errorf("drops for an unknown customer = %+v", reporter.all())
	}
}

func TestEngine_Build_UnknownScreen(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t, thiagoFixture().sources())
	for _, slug := range []string{"perfil", "x", ""} {
		t.Run("slug "+strconv.Quote(slug), func(t *testing.T) {
			t.Parallel()
			if _, err := e.Build(t.Context(), slug, "id"); !errors.Is(err, ErrUnknownScreen) {
				t.Errorf("Build(%q) error = %v, want %v", slug, err, ErrUnknownScreen)
			}
		})
	}
}

// brokenVariant always matches and fails to build in the chosen way.
type brokenVariant struct {
	err     error
	panics  bool
	wrongAs string
}

func (brokenVariant) Matches(Snapshot) bool { return true }

func (v brokenVariant) Build(Snapshot, Catalog) (Component, error) {
	if v.panics {
		panic("boom")
	}
	if v.wrongAs != "" {
		return Component{Type: v.wrongAs, Variant: "default"}, nil
	}
	return Component{}, v.err
}

func TestEngine_Build_BuildError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		broken brokenVariant
	}{
		{name: "error", broken: brokenVariant{err: errors.New("no copy")}},
		{name: "panic", broken: brokenVariant{panics: true}},
		{name: "wrong type", broken: brokenVariant{wrongAs: "moment_card"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			variants := builtinVariants(embedded(t))
			variants[variantKey{typ: typeWealthSummary, name: "default"}] = registered{variant: tt.broken, needs: []Source{SourceAccount}}
			reporter := &recordingReporter{}
			f := thiagoFixture()
			e, err := newEngine(f.sources(), embedded(t), variants, WithDropReporter(reporter))
			if err != nil {
				t.Fatalf("newEngine error = %v", err)
			}
			res, err := e.Build(t.Context(), "home", f.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			if want := []string{"moment", "actions", "advisor", "activity"}; !slices.Equal(sectionIDs(res.Page), want) {
				t.Errorf("sections = %v, want %v", sectionIDs(res.Page), want)
			}
			want := []Omitted{{ID: "wealth", Type: "wealth_summary", Reason: ReasonBuildError}}
			if !slices.Equal(res.Page.Omitted, want) {
				t.Errorf("omitted = %+v, want %+v", res.Page.Omitted, want)
			}
			if drops := reporter.all(); !slices.Equal(drops, []drop{{slug: "home", typ: "wealth_summary", reason: "build_error"}}) {
				t.Errorf("drops = %+v", drops)
			}
			if len(res.Failures) != 1 || res.Failures[0].Section != "wealth" || res.Failures[0].Err == nil {
				t.Fatalf("failures = %+v", res.Failures)
			}
			if isPanic := errors.Is(res.Failures[0].Err, ErrPanic); isPanic != tt.broken.panics {
				t.Errorf("failure %v wraps ErrPanic = %t, want %t", res.Failures[0].Err, isPanic, tt.broken.panics)
			}
		})
	}
}

// stubVariant always matches and builds type/name, reading nothing.
type stubVariant struct{ typ, name string }

func (stubVariant) Matches(Snapshot) bool { return true }

func (v stubVariant) Build(Snapshot, Catalog) (Component, error) {
	return Component{Type: v.typ, Variant: v.name, Props: struct{}{}}, nil
}

// TestEngine_Build_FallsBackWhenVariantSourceFails gives the advisor's
// non-default variant a source the default does not need: when that source
// fails, the section falls back to its default instead of being omitted.
func TestEngine_Build_FallsBackWhenVariantSourceFails(t *testing.T) {
	t.Parallel()
	variants := builtinVariants(embedded(t))
	variants[variantKey{typ: typeAdvisorCard, name: "dedicated"}] = registered{
		variant: stubVariant{typ: typeAdvisorCard, name: "dedicated"},
		needs:   []Source{SourceTimeline},
	}
	tests := []struct {
		name     string
		timeline ActivitySource
		expected string
	}{
		{name: "source answers", timeline: fakeActivity{}, expected: "dedicated"},
		{name: "source fails", timeline: fakeActivity{err: errDown}, expected: "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := thiagoFixture()
			src := f.sources()
			src.Activity = tt.timeline
			e, err := newEngine(src, embedded(t), variants)
			if err != nil {
				t.Fatalf("newEngine error = %v", err)
			}
			res, err := e.Build(t.Context(), "home", f.id)
			if err != nil {
				t.Fatalf("Build error = %v", err)
			}
			if got := variantOf(t, res.Page, "advisor"); got != tt.expected {
				t.Errorf("advisor = %q, want %q", got, tt.expected)
			}
			for _, o := range res.Page.Omitted {
				if o.ID == "advisor" {
					t.Errorf("advisor omitted: %+v", o)
				}
			}
		})
	}
}

func TestEngine_Build_RequestEnded(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	reporter := &recordingReporter{}
	f := thiagoFixture()
	src := f.sources()
	src.Activity = fakeActivity{err: errDown}
	res, err := newTestEngine(t, src, WithDropReporter(reporter)).Build(ctx, "home", f.id)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Build error = %v, want %v", err, context.Canceled)
	}
	if len(res.Failures) != 0 || len(reporter.all()) != 0 {
		t.Errorf("failures %+v, drops %+v for an ended request", res.Failures, reporter.all())
	}
}

func TestEngine_Build_Deadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := marianaFixture()
		src := f.sources()
		src.Activity = fakeActivity{rows: f.activity, delay: time.Hour}
		reporter := &recordingReporter{}
		e := newTestEngine(t, src, WithDeadline(50*time.Millisecond), WithDropReporter(reporter))
		start := time.Now()
		res, err := e.Build(t.Context(), "home", f.id)
		if err != nil {
			t.Fatalf("Build error = %v", err)
		}
		if elapsed := time.Since(start); elapsed != 50*time.Millisecond {
			t.Errorf("Build took %v, want the 50ms deadline", elapsed)
		}
		want := []Omitted{{ID: "activity", Type: "activity_list", Reason: "timeline"}}
		if !slices.Equal(res.Page.Omitted, want) {
			t.Errorf("omitted = %+v, want %+v", res.Page.Omitted, want)
		}
		if len(res.Failures) != 1 || !errors.Is(res.Failures[0].Err, context.DeadlineExceeded) {
			t.Errorf("failures = %+v", res.Failures)
		}
		if got := variantOf(t, res.Page, "advisor"); got != "dedicated" {
			t.Errorf("advisor = %q, want dedicated", got)
		}
	})
}

func TestWithOptions_IgnoreZeroValues(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t, thiagoFixture().sources(), WithDeadline(0), WithDropReporter(nil), WithClock(nil))
	if e.deadline != DefaultDeadline {
		t.Errorf("deadline = %v, want %v", e.deadline, DefaultDeadline)
	}
	if _, ok := e.reporter.(noopReporter); !ok {
		t.Errorf("reporter = %T, want the no-op reporter", e.reporter)
	}
	if !e.now().Equal(testNow) {
		t.Error("a nil clock replaced the configured one")
	}
}

// TestPage_JSON pins the envelope keys of specs/http/bff.md.
func TestPage_JSON(t *testing.T) {
	t.Parallel()
	f := thiagoFixture()
	src := f.sources()
	src.Customers = fakeCustomers{err: errDown}
	res, err := newTestEngine(t, src).Build(t.Context(), "home", f.id)
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	raw, err := json.Marshal(res.Page)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := slices.Sorted(maps.Keys(body))
	if want := []string{"omitted", "revision", "schema_version", "sections", "slug", "title"}; !slices.Equal(keys, want) {
		t.Errorf("keys = %v, want %v (no subtitle without advisory)", keys, want)
	}
	if got := string(body["omitted"]); got != `[{"id":"advisor","type":"advisor_card","reason":"advisory"}]` {
		t.Errorf("omitted = %s", got)
	}
	if strings.Contains(string(raw), `"action":null`) {
		t.Errorf("page encodes a null action: %s", raw)
	}
}
