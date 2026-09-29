package telemetry

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

// TestMain replaces the OpenTelemetry default globals, which delegate to the
// first provider ever set and cannot be set back, with concrete no-ops. Each
// test that changes a global then restores it with keepGlobals.
func TestMain(m *testing.M) {
	otel.SetTracerProvider(tracenoop.NewTracerProvider())
	otel.SetMeterProvider(metricnoop.NewMeterProvider())
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {}))
	os.Exit(m.Run())
}

// keepGlobals restores, when t ends, the global providers, propagator, and
// error handler that Setup or the test replaces.
func keepGlobals(t *testing.T) {
	t.Helper()
	tp, mp := otel.GetTracerProvider(), otel.GetMeterProvider()
	prop, eh := otel.GetTextMapPropagator(), otel.GetErrorHandler()
	t.Cleanup(func() {
		otel.SetTracerProvider(tp)
		otel.SetMeterProvider(mp)
		otel.SetTextMapPropagator(prop)
		otel.SetErrorHandler(eh)
	})
}

// useEndpoint points the OTLP exporters at endpoint through the shared
// variable only; the per-signal ones would override it.
func useEndpoint(t *testing.T, endpoint string) {
	t.Helper()
	t.Setenv(EndpointEnv, endpoint)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
}

// otlpSink is a local OTLP/HTTP receiver: it records the request paths and
// the service.name of every trace export.
type otlpSink struct {
	mu       sync.Mutex
	paths    []string
	services []string
}

func (s *otlpSink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paths = append(s.paths, r.URL.Path)
	if r.URL.Path == "/v1/traces" {
		var req coltracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err == nil {
			for _, rs := range req.GetResourceSpans() {
				for _, kv := range rs.GetResource().GetAttributes() {
					if kv.GetKey() == string(serviceNameKey) {
						s.services = append(s.services, kv.GetValue().GetStringValue())
					}
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

func (s *otlpSink) snapshot() (paths, services []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.paths), slices.Clone(s.services)
}

// exportProbe runs Setup against a fresh sink, records one span and one
// counter, stops, and returns what the sink received.
func exportProbe(t *testing.T) (paths, services []string) {
	t.Helper()
	sink := &otlpSink{}
	srv := httptest.NewServer(sink)
	t.Cleanup(srv.Close)
	useEndpoint(t, srv.URL)

	shutdown, err := Setup(t.Context(), "advisory")
	if err != nil {
		t.Fatalf("Setup error = %v", err)
	}
	_, span := otel.Tracer("test").Start(t.Context(), "probe")
	span.End()
	counter, err := otel.Meter("test").Int64Counter("probe_total")
	if err != nil {
		t.Fatalf("counter error = %v", err)
	}
	counter.Add(t.Context(), 1)
	if err := Stop(shutdown); err != nil {
		t.Fatalf("Stop error = %v", err)
	}
	return sink.snapshot()
}

func TestSetup_RequiresServiceName(t *testing.T) {
	if _, err := Setup(t.Context(), ""); err == nil {
		t.Fatal("Setup with no service name returned no error")
	}
}

func TestSetup_WithoutEndpoint(t *testing.T) {
	keepGlobals(t)
	useEndpoint(t, "")
	before := otel.GetTracerProvider()

	shutdown, err := Setup(t.Context(), "bff")
	if err != nil {
		t.Fatalf("Setup error = %v", err)
	}
	if otel.GetTracerProvider() != before {
		t.Error("Setup without an endpoint replaced the global tracer provider")
	}
	if err := shutdown(t.Context()); err != nil {
		t.Errorf("shutdown error = %v", err)
	}
	if err := Stop(shutdown); err != nil {
		t.Errorf("Stop error = %v", err)
	}
	fields := otel.GetTextMapPropagator().Fields()
	for _, want := range []string{"traceparent", "baggage"} {
		if !slices.Contains(fields, want) {
			t.Errorf("propagator fields = %v, missing %q", fields, want)
		}
	}
}

func TestSetup_ExportsOverOTLPHTTP(t *testing.T) {
	tests := []struct {
		name     string
		override string
		want     string
	}{
		{name: "service name set in code", want: "advisory"},
		{name: "process override", override: "advisory-canary", want: "advisory-canary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keepGlobals(t)
			t.Setenv("OTEL_SERVICE_NAME", tt.override)
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")

			paths, services := exportProbe(t)
			for _, want := range []string{"/v1/traces", "/v1/metrics"} {
				if !slices.Contains(paths, want) {
					t.Errorf("export paths = %v, missing %q", paths, want)
				}
			}
			if len(services) == 0 || services[0] != tt.want {
				t.Errorf("service.name = %v, want %q", services, tt.want)
			}
		})
	}
}

// TestSetup_KeepsAPartialResource checks that a malformed
// OTEL_RESOURCE_ATTRIBUTES does not stop the service: Setup keeps the part of
// the resource it could build, service.name included.
func TestSetup_KeepsAPartialResource(t *testing.T) {
	keepGlobals(t)
	t.Setenv("OTEL_SERVICE_NAME", "")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=local,no-equals-sign")

	paths, services := exportProbe(t)
	if !slices.Contains(paths, "/v1/traces") {
		t.Errorf("export paths = %v, missing /v1/traces", paths)
	}
	if len(services) == 0 || services[0] != "advisory" {
		t.Errorf("service.name = %v, want advisory", services)
	}
}

func TestHTTPHandler(t *testing.T) {
	t.Parallel()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("GET /v1/client-pov/customers/{id}/screens/{slug}", ok)
	mux.HandleFunc("GET /v1/queue/stream", ok)
	h := HTTPHandler(mux, "bff",
		otelhttp.WithTracerProvider(tp),
		otelhttp.WithPropagators(propagation.TraceContext{}),
	)

	const remote = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	for _, path := range []string{
		"/v1/client-pov/customers/0190c0de-0000-7000-8000-000000000001/screens/home",
		"/nope",
		"/v1/queue/stream",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("traceparent", remote)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	ended := rec.Ended()
	var names []string
	for _, s := range ended {
		names = append(names, s.Name())
	}
	// The stream request has no span: SSE is filtered out.
	want := []string{"GET /v1/client-pov/customers/{id}/screens/{slug}", "GET"}
	if !slices.Equal(names, want) {
		t.Fatalf("span names = %q, want %q", names, want)
	}
	// The endpoint is public: the browser's traceparent is a link, not the parent.
	for _, s := range ended {
		if s.Parent().IsValid() {
			t.Errorf("span %q has the remote parent %v", s.Name(), s.Parent().TraceID())
		}
		links := s.Links()
		if len(links) != 1 || links[0].SpanContext.TraceID().String() != "0af7651916cd43dd8448eb211c80319c" {
			t.Errorf("span %q links = %+v, want the remote context", s.Name(), links)
		}
	}
}

// TestGRPCOptions_ContinueTheTrace checks that a client span and the server
// span of one RPC share a trace, the server span being the client's child.
func TestGRPCOptions_ContinueTheTrace(t *testing.T) {
	keepGlobals(t)
	rec := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	otel.SetTextMapPropagator(propagation.TraceContext{})

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(GRPCServerOption())
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		GRPCClientOption(),
	)
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if _, err := healthpb.NewHealthClient(conn).Check(t.Context(), &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatalf("Check error = %v", err)
	}
	srv.GracefulStop() // the server span ends before its handler returns

	var client, server sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		switch s.SpanKind().String() {
		case "client":
			client = s
		case "server":
			server = s
		}
	}
	if client == nil || server == nil {
		t.Fatalf("spans = %d, want one client and one server span", len(rec.Ended()))
	}
	if server.SpanContext().TraceID() != client.SpanContext().TraceID() {
		t.Error("server span is in another trace than the client span")
	}
	if server.Parent().SpanID() != client.SpanContext().SpanID() {
		t.Error("server span is not the client span's child")
	}
}
