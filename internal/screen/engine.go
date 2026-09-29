package screen

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// DefaultDeadline bounds the Snapshot of one screen. A source still running
// at the deadline counts as failed.
const DefaultDeadline = 800 * time.Millisecond

// ErrUnknownScreen is returned for a slug the catalog does not serve.
var ErrUnknownScreen = errors.New("screen: unknown screen")

// Variant is one way to render a section. The first variant of a section
// whose Matches is true is built; the last variant is the default and always
// matches. Implementations must be safe for concurrent use.
type Variant interface {
	Matches(Snapshot) bool
	Build(Snapshot, Catalog) (Component, error)
}

// DropReporter learns about every section a response omits, with the
// component type and the reason (a source name or ReasonBuildError). It must
// be safe for concurrent use. The engine always counts drops in
// sdui_component_dropped_total; a DropReporter set with WithDropReporter is
// told as well.
type DropReporter interface {
	ComponentDropped(ctx context.Context, slug, componentType, reason string)
}

type noopReporter struct{}

func (noopReporter) ComponentDropped(context.Context, string, string, string) {}

// Failure is one error behind a response that still answered 200: a failed
// Source, or the Section whose variant failed to build or whose heading
// failed to render.
type Failure struct {
	Source  Source
	Section string
	Err     error
}

// Result is a composed screen and the failures behind its omitted parts. The
// failures are for logs and never reach the client.
type Result struct {
	Page     Page
	Failures []Failure
}

// variantKey names one catalog variant of a component type.
type variantKey struct {
	typ  string
	name string
}

// registered is a variant implementation and the sources its Build reads.
type registered struct {
	variant Variant
	needs   []Source
}

// Engine composes screens from the embedded catalog. It is immutable after
// New and safe for concurrent use.
//
// Every Build is traced and measured: one sdui.screen span with a child span
// per Snapshot source and per section, and the sdui_* metrics of
// architecture.md. The providers default to the OpenTelemetry globals read at
// New.
type Engine struct {
	sources        Sources
	catalog        Catalog
	plans          map[string]plan
	deadline       time.Duration
	reporter       DropReporter
	now            func() time.Time
	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider
	tel            instruments
	drops          dropCounter
}

// plan is the served revision of one screen with its variants resolved.
// sources is what a Build of the screen fetches, in allSources order.
type plan struct {
	def      screenDef
	sections []plannedSection
	sources  []Source
}

type plannedSection struct {
	id       string
	typ      string
	variants []namedVariant
	// needs are the sources of the default variant: when one fails, the
	// section is omitted with that source as the reason.
	needs []Source
}

type namedVariant struct {
	name    string
	variant Variant
	// needs are the sources this variant reads; it is skipped when one failed.
	needs []Source
}

// Option configures an Engine.
type Option func(*Engine)

// WithDeadline replaces DefaultDeadline. A non-positive d is ignored.
func WithDeadline(d time.Duration) Option {
	return func(e *Engine) {
		if d > 0 {
			e.deadline = d
		}
	}
}

// WithClock replaces time.Now as the request clock that relative times are
// measured against. A nil clock is ignored.
func WithClock(now func() time.Time) Option {
	return func(e *Engine) {
		if now != nil {
			e.now = now
		}
	}
}

// WithDropReporter sets a reporter told about each omitted section, next to
// the sdui_component_dropped_total counter the engine always feeds. A nil
// reporter is ignored.
func WithDropReporter(r DropReporter) Option {
	return func(e *Engine) {
		if r != nil {
			e.reporter = r
		}
	}
}

// WithTracerProvider replaces the global tracer provider as the source of
// the engine's spans. A nil provider is ignored.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(e *Engine) {
		if tp != nil {
			e.tracerProvider = tp
		}
	}
}

// WithMeterProvider replaces the global meter provider as the source of the
// engine's sdui_* metrics. A nil provider is ignored.
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(e *Engine) {
		if mp != nil {
			e.meterProvider = mp
		}
	}
}

// New returns an engine over the embedded catalog. Every source is required.
func New(src Sources, opts ...Option) (*Engine, error) {
	cat, err := parseCatalog(catalogJSON)
	if err != nil {
		return nil, err
	}
	return newEngine(src, cat, builtinVariants(cat), opts...)
}

// MustNew is New for wiring code whose sources are never nil. It panics on
// an invalid embedded catalog, which the package tests rule out.
func MustNew(src Sources, opts ...Option) *Engine {
	e, err := New(src, opts...)
	if err != nil {
		panic(err)
	}
	return e
}

func newEngine(src Sources, cat Catalog, variants map[variantKey]registered, opts ...Option) (*Engine, error) {
	if !src.complete() {
		return nil, errors.New("screen: every source is required")
	}
	e := &Engine{
		sources:        src,
		catalog:        cat,
		plans:          make(map[string]plan, len(cat.screens)),
		deadline:       DefaultDeadline,
		reporter:       noopReporter{},
		now:            time.Now,
		tracerProvider: otel.GetTracerProvider(),
		meterProvider:  otel.GetMeterProvider(),
	}
	for _, opt := range opts {
		opt(e)
	}
	tel, err := newInstruments(e.tracerProvider, e.meterProvider)
	if err != nil {
		return nil, err
	}
	e.tel = tel
	e.drops = dropCounter{counter: tel.dropped}
	for slug, def := range cat.screens {
		p, err := resolve(slug, def, variants)
		if err != nil {
			return nil, err
		}
		e.plans[slug] = p
	}
	return e, nil
}

// resolve binds every catalog variant name to its implementation.
func resolve(slug string, def screenDef, variants map[variantKey]registered) (plan, error) {
	p := plan{def: def, sections: make([]plannedSection, 0, len(def.sections))}
	for _, sec := range def.sections {
		ps := plannedSection{id: sec.id, typ: sec.typ, variants: make([]namedVariant, 0, len(sec.variants))}
		for _, name := range sec.variants {
			reg, ok := variants[variantKey{typ: sec.typ, name: name}]
			if !ok {
				return plan{}, fmt.Errorf("%w: %s/%s: %s variant %q is not implemented", errCatalog, slug, sec.id, sec.typ, name)
			}
			ps.variants = append(ps.variants, namedVariant{name: name, variant: reg.variant, needs: reg.needs})
			ps.needs = reg.needs // ends as the default's
		}
		p.sections = append(p.sections, ps)
	}
	p.sources = planSources(def, p.sections)
	return p, nil
}

// planSources is the union of the sources a screen reads: the account
// always, since it is what answers an unknown customer with 404; advisory
// when the heading has customer fields; and every source any variant needs.
func planSources(def screenDef, sections []plannedSection) []Source {
	used := map[Source]bool{SourceAccount: true}
	if !isPlainText(def.title) || !def.staticSubtitle {
		used[SourceAdvisory] = true
	}
	for _, sec := range sections {
		for _, v := range sec.variants {
			for _, src := range v.needs {
				used[src] = true
			}
		}
	}
	out := make([]Source, 0, len(used))
	for _, src := range allSources {
		if used[src] {
			out = append(out, src)
		}
	}
	return out
}

// Build composes one screen for a customer. It fails only for an unknown
// slug (ErrUnknownScreen), when account-sim does not know the customer
// (ErrUnknownCustomer), or with ctx.Err() when the request ended during the
// fetch, in which case nothing is reported; every other failure omits
// sections and is listed in Result.Failures.
//
// An unknown slug is neither traced nor measured, so a path value never
// becomes a span or a label.
func (e *Engine) Build(ctx context.Context, slug, customerID string) (Result, error) {
	p, ok := e.plans[slug]
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrUnknownScreen, slug)
	}
	start := time.Now()
	ctx, span := e.tel.tracer.Start(ctx, spanScreen, trace.WithAttributes(
		attrSlug.String(slug),
		attrRevision.String(p.def.revision),
	))
	defer func() {
		span.End()
		e.tel.buildSeconds.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(labelSlug.String(slug)))
	}()

	snap := fetchSnapshot(ctx, e.tel.tracer, e.sources, p.sources, customerID, e.now(), e.deadline)
	if err := ctx.Err(); err != nil {
		span.SetStatus(codes.Error, failureClass(err))
		return Result{}, err
	}
	if errors.Is(snap.Account.Err, ErrUnknownCustomer) {
		// A 404, not a server fault: the span stays unset.
		return Result{}, snap.Account.Err
	}

	var res Result
	for _, src := range p.sources {
		if err := snap.failed(src); err != nil {
			res.Failures = append(res.Failures, Failure{Source: src, Err: err})
		}
	}
	page := Page{
		SchemaVersion: SchemaVersion,
		Slug:          slug,
		Revision:      p.def.revision,
		Sections:      make([]Section, 0, len(p.sections)),
		Omitted:       []Omitted{},
	}
	title, subtitle, err := heading(p.def, snap)
	if err != nil {
		res.Failures = append(res.Failures, Failure{Section: "heading", Err: err})
		// No section span covers the heading, so the screen span carries it.
		span.SetStatus(codes.Error, failureClass(err))
	}
	page.Title, page.Subtitle = title, subtitle

	for _, sec := range p.sections {
		comp, reason, err := e.tracedSection(ctx, sec, snap)
		if err != nil {
			res.Failures = append(res.Failures, Failure{Section: sec.id, Err: err})
		}
		if reason != "" {
			page.Omitted = append(page.Omitted, Omitted{ID: sec.id, Type: sec.typ, Reason: reason})
			e.drops.ComponentDropped(ctx, slug, sec.typ, reason)
			e.reporter.ComponentDropped(ctx, slug, sec.typ, reason)
			continue
		}
		e.tel.variantServed(ctx, slug, sec.id, comp.Variant)
		page.Sections = append(page.Sections, Section{ID: sec.id, Components: []Component{comp}})
	}
	res.Page = page
	return res, nil
}

// heading renders the screen title and, when advisory answered or the
// subtitle is plain text, the subtitle. Without advisory the title template
// falls back to its plain greeting.
func heading(def screenDef, snap Snapshot) (title, subtitle string, err error) {
	f := customerFields(snap)
	title, err = execute(def.title, f)
	if err != nil {
		return "", "", err
	}
	if !snap.Customer.OK() && !def.staticSubtitle {
		return title, "", nil
	}
	subtitle, err = execute(def.subtitle, f)
	if err != nil {
		return title, "", err
	}
	return title, subtitle, nil
}

// tracedSection runs section inside its sdui.section span. The span carries
// the section id, the variant built or the one that failed to build, and the
// omitted reason. Only a build failure marks it as an error; a failed source
// already did so on its own Snapshot span.
func (e *Engine) tracedSection(ctx context.Context, sec plannedSection, snap Snapshot) (comp Component, reason string, err error) {
	_, span := e.tel.tracer.Start(ctx, spanSection, trace.WithAttributes(attrSection.String(sec.id)))
	defer span.End()
	comp, variant, reason, err := e.section(sec, snap)
	if variant != "" {
		span.SetAttributes(attrVariant.String(variant))
	}
	if reason != "" {
		span.SetAttributes(attrOmittedReason.String(reason))
	}
	if err != nil {
		span.SetStatus(codes.Error, failureClass(err))
	}
	return comp, reason, err
}

// section applies the failure policy to one section. variant names the
// variant built, or the one whose evaluation failed. A non-empty reason means
// the section is omitted; err is set when a variant failed.
func (e *Engine) section(sec plannedSection, snap Snapshot) (comp Component, variant, reason string, err error) {
	for _, src := range sec.needs {
		if snap.failed(src) != nil {
			return Component{}, "", string(src), nil
		}
	}
	// The default's needs answered, so the default is never skipped below.
	for _, v := range sec.variants {
		if anyFailed(snap, v.needs) {
			continue // fall back to a later variant, ending at the default
		}
		matched, comp, err := evaluate(v.variant, snap, e.catalog)
		if err != nil {
			return Component{}, v.name, ReasonBuildError, fmt.Errorf("screen: %s/%s: %w", sec.typ, v.name, err)
		}
		if !matched {
			continue
		}
		if comp.Type != sec.typ || comp.Variant != v.name {
			return Component{}, v.name, ReasonBuildError, fmt.Errorf("screen: %s/%s built %s/%s", sec.typ, v.name, comp.Type, comp.Variant)
		}
		return comp, v.name, "", nil
	}
	return Component{}, "", ReasonBuildError, fmt.Errorf("screen: section %q: no variant matched", sec.id)
}

// anyFailed reports whether one of sources failed in snap.
func anyFailed(snap Snapshot, sources []Source) bool {
	for _, src := range sources {
		if snap.failed(src) != nil {
			return true
		}
	}
	return false
}

// evaluate runs Matches and, when it matches, Build. A panic in either is
// turned into an error so one broken variant drops only its component.
func evaluate(v Variant, snap Snapshot, cat Catalog) (matched bool, comp Component, err error) {
	defer func() {
		if r := recover(); r != nil {
			matched, comp, err = false, Component{}, fmt.Errorf("screen: variant: %w: %v", ErrPanic, r)
		}
	}()
	if !v.Matches(snap) {
		return false, Component{}, nil
	}
	comp, err = v.Build(snap, cat)
	if err != nil {
		return false, Component{}, err
	}
	return true, comp, nil
}
