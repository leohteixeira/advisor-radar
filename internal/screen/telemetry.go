package screen

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// instrumentationName scopes the engine's tracer and meter.
const instrumentationName = "github.com/leohteixeira/advisor-radar/internal/screen"

// Span names. A Snapshot source span is snapshotSpanPrefix plus the source
// name, for example "sdui.snapshot.account-sim".
const (
	spanScreen         = "sdui.screen"
	spanSection        = "sdui.section"
	snapshotSpanPrefix = "sdui.snapshot."
)

// Span attributes. Every value is a catalog key, a Source, or a fixed
// failure class; never a customer id, a value, or rendered copy.
const (
	attrSlug          = attribute.Key("sdui.slug")
	attrRevision      = attribute.Key("sdui.revision")
	attrSection       = attribute.Key("sdui.section")
	attrVariant       = attribute.Key("sdui.variant")
	attrOmittedReason = attribute.Key("sdui.omitted_reason")
	attrSource        = attribute.Key("sdui.source")
)

// Metric names, as in architecture.md "Observability", and their labels.
// The label values are bounded: slugs, section ids, component types, and
// variants come from the catalog, reasons are a Source or ReasonBuildError.
const (
	// metricVariantServed counts each section served, by the variant built.
	//
	//	sum by (section, variant) (rate(sdui_variant_served_total{slug="home"}[5m]))
	metricVariantServed = "sdui_variant_served_total"
	// metricComponentDropped counts each omitted section, by why.
	//
	//	sum by (type, reason) (rate(sdui_component_dropped_total[5m])) > 0
	metricComponentDropped = "sdui_component_dropped_total"
	// metricScreenBuild is the latency of one Build, Snapshot included.
	//
	//	histogram_quantile(0.95, sum by (le, slug) (rate(sdui_screen_build_seconds_bucket[5m])))
	metricScreenBuild = "sdui_screen_build_seconds"

	labelSlug    = attribute.Key("slug")
	labelSection = attribute.Key("section")
	labelVariant = attribute.Key("variant")
	labelType    = attribute.Key("type")
	labelReason  = attribute.Key("reason")
)

// buildBuckets are the sdui_screen_build_seconds boundaries, dense around
// the 800 ms screen deadline.
var buildBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.4, 0.6, 0.8, 1, 2.5}

// instruments are the engine's tracer and metric instruments.
type instruments struct {
	tracer       trace.Tracer
	served       metric.Int64Counter
	dropped      metric.Int64Counter
	buildSeconds metric.Float64Histogram
}

func newInstruments(tp trace.TracerProvider, mp metric.MeterProvider) (instruments, error) {
	meter := mp.Meter(instrumentationName)
	served, err := meter.Int64Counter(metricVariantServed,
		metric.WithDescription("Sections served, by screen, section, and variant."))
	if err != nil {
		return instruments{}, fmt.Errorf("screen: %s: %w", metricVariantServed, err)
	}
	dropped, err := meter.Int64Counter(metricComponentDropped,
		metric.WithDescription("Sections omitted from a served screen, by component type and reason."))
	if err != nil {
		return instruments{}, fmt.Errorf("screen: %s: %w", metricComponentDropped, err)
	}
	build, err := meter.Float64Histogram(metricScreenBuild,
		metric.WithDescription("Time to build one screen, Snapshot included."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(buildBuckets...))
	if err != nil {
		return instruments{}, fmt.Errorf("screen: %s: %w", metricScreenBuild, err)
	}
	return instruments{
		tracer:       tp.Tracer(instrumentationName),
		served:       served,
		dropped:      dropped,
		buildSeconds: build,
	}, nil
}

// variantServed counts one served section.
func (in instruments) variantServed(ctx context.Context, slug, section, variant string) {
	in.served.Add(ctx, 1, metric.WithAttributes(
		labelSlug.String(slug),
		labelSection.String(section),
		labelVariant.String(variant),
	))
}

// dropCounter is the DropReporter that feeds sdui_component_dropped_total.
// The engine always reports to it, before any reporter set with
// WithDropReporter.
type dropCounter struct {
	counter metric.Int64Counter
}

// ComponentDropped implements DropReporter.
func (d dropCounter) ComponentDropped(ctx context.Context, slug, componentType, reason string) {
	d.counter.Add(ctx, 1, metric.WithAttributes(
		labelSlug.String(slug),
		labelType.String(componentType),
		labelReason.String(reason),
	))
}

// failureClass is the fixed span status description of a failed source or
// build. Error text is never put on a span: it may carry copy or a panic
// value. A gRPC status from a source adapter counts like its context error.
func failureClass(err error) string {
	switch {
	case errors.Is(err, ErrPanic):
		return "panic"
	case errors.Is(err, context.DeadlineExceeded), status.Code(err) == grpccodes.DeadlineExceeded:
		return "deadline"
	case errors.Is(err, context.Canceled), status.Code(err) == grpccodes.Canceled:
		return "canceled"
	default:
		return "error"
	}
}
