package screen

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// noopTracer is the tracer of Snapshot tests that do not look at spans.
var noopTracer = tracenoop.NewTracerProvider().Tracer("test")

// observed is an engine wired to an in-memory span recorder and a manual
// metric reader.
type observed struct {
	spans  *tracetest.SpanRecorder
	reader *sdkmetric.ManualReader
}

func newObserved() observed {
	return observed{spans: tracetest.NewSpanRecorder(), reader: sdkmetric.NewManualReader()}
}

func (o observed) options() []Option {
	return []Option{
		WithTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(o.spans))),
		WithMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(o.reader))),
	}
}

func (o observed) named(name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range o.spans.Ended() {
		if s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}

func (o observed) collect(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := o.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("Collect error = %v", err)
	}
	return rm
}

// counter returns the named Int64 sum as "k=v,k=v" label sets to values.
func counter(t *testing.T, rm metricdata.ResourceMetrics, name string) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s is %T, want an int64 sum", name, m.Data)
			}
			if !sum.IsMonotonic {
				t.Errorf("%s is not monotonic", name)
			}
			for _, dp := range sum.DataPoints {
				out[dp.Attributes.Encoded(attribute.DefaultEncoder())] = dp.Value
			}
		}
	}
	return out
}

// histogramCounts returns the named float64 histogram's observation count
// per label set.
func histogramCounts(t *testing.T, rm metricdata.ResourceMetrics, name string) map[string]uint64 {
	t.Helper()
	out := map[string]uint64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			if m.Unit != "s" {
				t.Errorf("%s unit = %q, want s", name, m.Unit)
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("%s is %T, want a float64 histogram", name, m.Data)
			}
			for _, dp := range h.DataPoints {
				out[dp.Attributes.Encoded(attribute.DefaultEncoder())] = dp.Count
			}
		}
	}
	return out
}

func stringAttr(s sdktrace.ReadOnlySpan, key attribute.Key) (string, bool) {
	for _, kv := range s.Attributes() {
		if kv.Key == key {
			return kv.Value.AsString(), true
		}
	}
	return "", false
}

func TestEngine_Build_TracesTheHome(t *testing.T) {
	t.Parallel()
	obs := newObserved()
	f := thiagoFixture()
	if _, err := newTestEngine(t, f.sources(), obs.options()...).Build(t.Context(), "home", f.id); err != nil {
		t.Fatalf("Build error = %v", err)
	}

	screens := obs.named(spanScreen)
	if len(screens) != 1 {
		t.Fatalf("%s spans = %d, want 1", spanScreen, len(screens))
	}
	root := screens[0]
	if slug, _ := stringAttr(root, attrSlug); slug != "home" {
		t.Errorf("sdui.slug = %q, want home", slug)
	}
	if rev, _ := stringAttr(root, attrRevision); rev != "v1" {
		t.Errorf("sdui.revision = %q, want v1", rev)
	}
	if root.Status().Code == codes.Error {
		t.Errorf("screen span status = %v", root.Status())
	}

	childOfRoot := func(s sdktrace.ReadOnlySpan) bool {
		return s.Parent().SpanID() == root.SpanContext().SpanID() &&
			s.SpanContext().TraceID() == root.SpanContext().TraceID()
	}

	var ids, variants []string
	for _, s := range obs.named(spanSection) {
		if !childOfRoot(s) {
			t.Errorf("section span %v is not a child of the screen span", s.Attributes())
		}
		id, _ := stringAttr(s, attrSection)
		variant, _ := stringAttr(s, attrVariant)
		ids, variants = append(ids, id), append(variants, variant)
		if reason, ok := stringAttr(s, attrOmittedReason); ok {
			t.Errorf("section %q has sdui.omitted_reason %q", id, reason)
		}
	}
	if want := []string{"moment", "wealth", "actions", "advisor", "activity"}; !slices.Equal(ids, want) {
		t.Errorf("section spans = %v, want %v", ids, want)
	}
	if want := []string{"idle_cash", "default", "default", "default", "recent"}; !slices.Equal(variants, want) {
		t.Errorf("section variants = %v, want %v", variants, want)
	}

	var sources []string
	for _, s := range obs.spans.Ended() {
		src, ok := stringAttr(s, attrSource)
		if !ok {
			continue
		}
		if s.Name() != snapshotSpanPrefix+src {
			t.Errorf("source %q span is named %q", src, s.Name())
		}
		if !childOfRoot(s) {
			t.Errorf("snapshot span %q is not a child of the screen span", s.Name())
		}
		sources = append(sources, src)
	}
	slices.Sort(sources)
	// home reads every source; activity uses the catalog for product names.
	want := make([]string, 0, len(allSources))
	for _, src := range allSources {
		want = append(want, string(src))
	}
	slices.Sort(want)
	if !slices.Equal(sources, want) {
		t.Errorf("snapshot spans = %v, want %v", sources, want)
	}
}

func TestEngine_Build_TracesAnOmittedSection(t *testing.T) {
	t.Parallel()
	obs := newObserved()
	f := thiagoFixture()
	src := f.sources()
	src.Activity = fakeActivity{err: errDown}
	if _, err := newTestEngine(t, src, obs.options()...).Build(t.Context(), "home", f.id); err != nil {
		t.Fatalf("Build error = %v", err)
	}

	var activity sdktrace.ReadOnlySpan
	for _, s := range obs.named(spanSection) {
		if id, _ := stringAttr(s, attrSection); id == "activity" {
			activity = s
		}
	}
	if activity == nil {
		t.Fatal("no sdui.section span for activity")
	}
	if reason, _ := stringAttr(activity, attrOmittedReason); reason != string(SourceTimeline) {
		t.Errorf("sdui.omitted_reason = %q, want timeline", reason)
	}
	if variant, ok := stringAttr(activity, attrVariant); ok {
		t.Errorf("omitted section has sdui.variant %q", variant)
	}
	timeline := obs.named(snapshotSpanPrefix + string(SourceTimeline))
	if len(timeline) != 1 || timeline[0].Status().Code != codes.Error || timeline[0].Status().Description != "error" {
		t.Errorf("timeline snapshot span = %+v, want one span with status error", timeline)
	}

	dropped := counter(t, obs.collect(t), metricComponentDropped)
	if want := map[string]int64{"reason=timeline,slug=home,type=activity_list": 1}; !maps.Equal(dropped, want) {
		t.Errorf("%s = %v, want %v", metricComponentDropped, dropped, want)
	}
}

// A position class without a position is absent, not omitted: its section
// span carries the variant but no omitted reason and no error, and nothing is
// counted as dropped or served.
func TestEngine_Build_TracesAnAbsentSection(t *testing.T) {
	t.Parallel()
	obs := newObserved()
	f := thiagoFixture()
	if _, err := newTestEngine(t, f.sources(), obs.options()...).Build(t.Context(), "carteira", f.id); err != nil {
		t.Fatalf("Build error = %v", err)
	}
	var absent sdktrace.ReadOnlySpan
	for _, s := range obs.named(spanSection) {
		if id, _ := stringAttr(s, attrSection); id == "positions_fixed_income" {
			absent = s
		}
	}
	if absent == nil {
		t.Fatal("no sdui.section span for positions_fixed_income")
	}
	if reason, ok := stringAttr(absent, attrOmittedReason); ok {
		t.Errorf("absent section has sdui.omitted_reason %q", reason)
	}
	if absent.Status().Code == codes.Error {
		t.Errorf("absent section span status = %+v, want no error", absent.Status())
	}
	rm := obs.collect(t)
	if dropped := counter(t, rm, metricComponentDropped); len(dropped) != 0 {
		t.Errorf("%s = %v, want none", metricComponentDropped, dropped)
	}
	if served := counter(t, rm, metricVariantServed); served["section=positions_fixed_income,slug=carteira,variant=fixed_income"] != 0 {
		t.Errorf("%s counted the absent section: %v", metricVariantServed, served)
	}
}

func TestEngine_Build_Metrics(t *testing.T) {
	t.Parallel()
	obs := newObserved()
	f := thiagoFixture()
	e := newTestEngine(t, f.sources(), obs.options()...)
	for range 2 {
		if _, err := e.Build(t.Context(), "home", f.id); err != nil {
			t.Fatalf("Build error = %v", err)
		}
	}
	if _, err := e.Build(t.Context(), "nope", f.id); !errors.Is(err, ErrUnknownScreen) {
		t.Fatalf("Build(nope) error = %v, want ErrUnknownScreen", err)
	}

	rm := obs.collect(t)
	served := counter(t, rm, metricVariantServed)
	want := map[string]int64{
		"section=moment,slug=home,variant=idle_cash": 2,
		"section=wealth,slug=home,variant=default":   2,
		"section=actions,slug=home,variant=default":  2,
		"section=advisor,slug=home,variant=default":  2,
		"section=activity,slug=home,variant=recent":  2,
	}
	if !maps.Equal(served, want) {
		t.Errorf("%s = %v, want %v", metricVariantServed, served, want)
	}
	if dropped := counter(t, rm, metricComponentDropped); len(dropped) != 0 {
		t.Errorf("%s = %v, want none", metricComponentDropped, dropped)
	}
	builds := histogramCounts(t, rm, metricScreenBuild)
	if want := map[string]uint64{"slug=home": 2}; !maps.Equal(builds, want) {
		t.Errorf("%s counts = %v, want %v (an unknown slug is not measured)", metricScreenBuild, builds, want)
	}
	if n := len(obs.named(spanScreen)); n != 2 {
		t.Errorf("%s spans = %d, want 2 (an unknown slug is not traced)", spanScreen, n)
	}
}

func TestEngine_Build_CountsABuildError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		broken brokenVariant
		class  string
	}{
		{name: "error", broken: brokenVariant{err: errors.New("no copy")}, class: "error"},
		{name: "panic", broken: brokenVariant{panics: true}, class: "panic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			obs := newObserved()
			variants := builtinVariants(embedded(t))
			variants[variantKey{typ: typeWealthSummary, name: "default"}] = registered{variant: tt.broken, needs: []Source{SourceAccount}}
			f := thiagoFixture()
			e, err := newEngine(f.sources(), embedded(t), variants, obs.options()...)
			if err != nil {
				t.Fatalf("newEngine error = %v", err)
			}
			if _, err := e.Build(t.Context(), "home", f.id); err != nil {
				t.Fatalf("Build error = %v", err)
			}

			rm := obs.collect(t)
			dropped := counter(t, rm, metricComponentDropped)
			if want := map[string]int64{"reason=build_error,slug=home,type=wealth_summary": 1}; !maps.Equal(dropped, want) {
				t.Errorf("%s = %v, want %v", metricComponentDropped, dropped, want)
			}
			served := counter(t, rm, metricVariantServed)
			if _, ok := served["section=wealth,slug=home,variant=default"]; ok || len(served) != 4 {
				t.Errorf("%s = %v, want the four other sections only", metricVariantServed, served)
			}

			for _, s := range obs.named(spanSection) {
				if id, _ := stringAttr(s, attrSection); id != "wealth" {
					continue
				}
				if variant, _ := stringAttr(s, attrVariant); variant != "default" {
					t.Errorf("wealth sdui.variant = %q, want the failed default", variant)
				}
				if reason, _ := stringAttr(s, attrOmittedReason); reason != ReasonBuildError {
					t.Errorf("wealth sdui.omitted_reason = %q, want build_error", reason)
				}
				if st := s.Status(); st.Code != codes.Error || st.Description != tt.class {
					t.Errorf("wealth span status = %+v, want error %q", st, tt.class)
				}
			}
		})
	}
}

func TestWithProviders_IgnoreNil(t *testing.T) {
	t.Parallel()
	e := newTestEngine(t, thiagoFixture().sources(), WithTracerProvider(nil), WithMeterProvider(nil))
	if e.tracerProvider == nil || e.meterProvider == nil {
		t.Error("a nil provider replaced the global one")
	}
}

func TestEngine_Build_UnknownCustomerIsNotASpanError(t *testing.T) {
	t.Parallel()
	obs := newObserved()
	f := thiagoFixture()
	src := f.sources()
	src.Accounts = fakeAccounts{err: ErrUnknownCustomer}
	if _, err := newTestEngine(t, src, obs.options()...).Build(t.Context(), "home", f.id); !errors.Is(err, ErrUnknownCustomer) {
		t.Fatalf("Build error = %v, want ErrUnknownCustomer", err)
	}
	for _, name := range []string{snapshotSpanPrefix + string(SourceAccount), spanScreen} {
		spans := obs.named(name)
		if len(spans) != 1 {
			t.Fatalf("%s spans = %d, want 1", name, len(spans))
		}
		if st := spans[0].Status(); st.Code != codes.Unset {
			t.Errorf("%s status = %+v, want unset for a 404", name, st)
		}
	}
}

func TestFailureClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "panic", err: fmt.Errorf("screen: account-sim: %w: boom", ErrPanic), want: "panic"},
		{name: "context deadline", err: fmt.Errorf("screen: timeline: %w", context.DeadlineExceeded), want: "deadline"},
		{name: "grpc deadline", err: fmt.Errorf("screen: advisory: %w", status.Error(grpccodes.DeadlineExceeded, "slow")), want: "deadline"},
		{name: "context canceled", err: fmt.Errorf("screen: cases: %w", context.Canceled), want: "canceled"},
		{name: "grpc canceled", err: fmt.Errorf("screen: cases: %w", status.Error(grpccodes.Canceled, "gone")), want: "canceled"},
		{name: "grpc unavailable", err: status.Error(grpccodes.Unavailable, "down"), want: "error"},
		{name: "plain error", err: errDown, want: "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := failureClass(tt.err); got != tt.want {
				t.Errorf("failureClass(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
