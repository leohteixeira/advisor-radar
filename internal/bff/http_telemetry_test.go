package bff_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// TestHandler_TracesRequestsByRoutePattern is not parallel: it swaps the
// global tracer provider, which the handler reads when it is built.
func TestHandler_TracesRequestsByRoutePattern(t *testing.T) {
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	rec := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))

	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, nil, nil, nil, nil, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/client-pov/customers/"+sim.CustomerThiago+"/screens/home", nil))
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("status = %d, content type = %q", rr.Code, rr.Header().Get("Content-Type"))
	}

	const route = "GET /v1/client-pov/customers/{id}/screens/{slug}"
	var server, screen sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		switch s.Name() {
		case route:
			server = s
		case "sdui.screen":
			screen = s
		}
		if strings.Contains(s.Name(), sim.CustomerThiago) {
			t.Errorf("span name %q carries the customer id", s.Name())
		}
	}
	if server == nil {
		t.Fatalf("no server span named %q", route)
	}
	if screen == nil || screen.Parent().SpanID() != server.SpanContext().SpanID() {
		t.Error("the sdui.screen span is not a child of the server span")
	}
}

// TestHandler_QueueStreamThroughTheWrapper checks that SSE still streams
// through the instrumented handler: a 200 text/event-stream, not a 500 for a
// ResponseWriter without Flush.
func TestHandler_QueueStreamThroughTheWrapper(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/queue/stream", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("content type = %q, want text/event-stream", got)
	}
}
