// Package telemetry bootstraps OpenTelemetry for one service process: the
// tracer and meter providers with OTLP/HTTP exporters, the W3C trace-context
// and baggage propagators, and the transport instrumentation every service
// shares (otelhttp for HTTP servers, otelgrpc stats handlers for gRPC).
//
// Export is enabled only when OTEL_EXPORTER_OTLP_ENDPOINT is set. Without it
// the global providers stay the OpenTelemetry no-ops, so tests and local runs
// without a collector record nothing, dial nothing, and log no export errors.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
)

// EndpointEnv is the variable that turns export on. The OTLP exporters read
// it themselves (and the per-signal OTEL_EXPORTER_OTLP_*_ENDPOINT overrides);
// Setup only checks that it is set.
const EndpointEnv = "OTEL_EXPORTER_OTLP_ENDPOINT"

// ShutdownTimeout bounds how long Stop waits for the last export.
const ShutdownTimeout = 5 * time.Second

// serviceNameKey is the resource attribute that names the service.
const serviceNameKey = attribute.Key("service.name")

// Setup installs the global propagators and, when OTEL_EXPORTER_OTLP_ENDPOINT
// is set, global tracer and meter providers that export over OTLP/HTTP.
// serviceName is set in code per command; OTEL_SERVICE_NAME in the process
// environment overrides it.
//
// The returned shutdown flushes and stops both providers; call it once, on
// exit, with a bounded context (see Stop). With export disabled it is a
// no-op that returns nil. Setup never dials: the exporters connect on their
// first export.
func Setup(ctx context.Context, serviceName string) (shutdown func(context.Context) error, err error) {
	if serviceName == "" {
		return nil, errors.New("telemetry: service name is required")
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	if os.Getenv(EndpointEnv) == "" {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(serviceNameKey.String(serviceName)),
		resource.WithTelemetrySDK(),
		// Last, so OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES win.
		resource.WithFromEnv(),
	)
	// A malformed OTEL_RESOURCE_ATTRIBUTES yields a partial resource that
	// still names the service: keep it and report the error once the error
	// handler is installed, rather than stop the service.
	var partial error
	if errors.Is(err, resource.ErrPartialResource) {
		partial, err = err, nil
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry: resource: %w", err)
	}

	traceExp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("telemetry: trace exporter: %w", err)
	}
	metricExp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("telemetry: metric exporter: %w", err),
			traceExp.Shutdown(ctx),
		)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetErrorHandler(exportErrorLogger(serviceName))
	if partial != nil {
		otel.Handle(fmt.Errorf("telemetry: resource: %w", partial))
	}

	return func(ctx context.Context) error {
		// Both run even when the first fails; each flushes before stopping.
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}, nil
}

// Stop runs shutdown bounded by ShutdownTimeout. It starts from a fresh
// context because the process context is already canceled on a signal.
func Stop(shutdown func(context.Context) error) error {
	if shutdown == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		return fmt.Errorf("telemetry: shutdown: %w", err)
	}
	return nil
}

// exportErrorLogger reports SDK errors (a failed export, a dropped batch) as
// slog JSON instead of the SDK's default stderr log line.
func exportErrorLogger(serviceName string) otel.ErrorHandler {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return otel.ErrorHandlerFunc(func(err error) {
		logger.Warn("telemetry error",
			slog.String("service", serviceName),
			slog.String("error", err.Error()),
		)
	})
}

// streamSuffix ends the path of every SSE route.
const streamSuffix = "/stream"

// HTTPHandler wraps a public, browser-facing h with otelhttp. The server span
// is named after the matched net/http route pattern ("GET /v1/queue"), so
// path values such as customer ids never become span names; an unmatched
// request is named after its method. operation names the handler in the
// otelhttp metrics.
//
// The endpoint is public: a traceparent the browser sends becomes a link,
// never the parent. SSE requests (a path ending in "/stream") are not
// instrumented, since a connection-long span would skew
// http.server.request.duration.
func HTTPHandler(h http.Handler, operation string, opts ...otelhttp.Option) http.Handler {
	defaults := []otelhttp.Option{
		// otelhttp v0.71 has only the Fn form of WithPublicEndpoint.
		otelhttp.WithPublicEndpointFn(func(*http.Request) bool { return true }),
		otelhttp.WithFilter(func(r *http.Request) bool {
			return !strings.HasSuffix(r.URL.Path, streamSuffix)
		}),
	}
	return otelhttp.NewHandler(h, operation, append(defaults, opts...)...)
}

// GRPCServerOption instruments a gRPC server with the otelgrpc stats handler:
// one server span per RPC, continuing the caller's propagated trace.
func GRPCServerOption() grpc.ServerOption {
	return grpc.StatsHandler(otelgrpc.NewServerHandler())
}

// GRPCClientOption instruments a gRPC client connection with the otelgrpc
// stats handler: one client span per RPC, with trace context sent in the
// request metadata.
func GRPCClientOption() grpc.DialOption {
	return grpc.WithStatsHandler(otelgrpc.NewClientHandler())
}
