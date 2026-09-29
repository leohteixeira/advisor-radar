package bff_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	timelinev1 "github.com/leohteixeira/advisor-radar/gen/timeline/v1"
	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

func TestHTTP_QueueJSON(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/queue", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Items []bff.Signal `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Items == nil {
		t.Fatal("items nil")
	}
}

func TestHTTP_CustomerInvalidID(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/customers/c01", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHTTP_CustomerNotFound(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil, nil, nil, nil, nil)
	id := identity.MustNewV7()
	req := httptest.NewRequest(http.MethodGet, "/v1/customers/"+id, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestHTTP_ReviewInvalidID(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPut, "/v1/review/r01", strings.NewReader(`"Operacional"`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHTTP_ManagerEmpty(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/manager", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var snap bff.ManagerSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Intents == nil {
		t.Fatal("intents nil")
	}
}

func TestHTTP_CasesEmpty(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/cases", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestHTTP_TimelineViaGRPCBufconn(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	eid := identity.MustNewV7()
	occurred := time.Now().UTC()
	body, _ := json.Marshal(map[string]any{
		"event_id": eid, "occurred_at": occurred,
		"customer_id": cust, "schema_version": 1,
		"payload": map[string]any{"kind": "nota", "title": "Nota", "text": "Orlando", "meta": "x"},
	})
	if _, _, err := idx.ApplyDelivery(context.Background(), "advisory.note.recorded", body); err != nil {
		t.Fatal(err)
	}
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	timelinev1.RegisterTimelineServiceServer(srv, timeline.NewGRPCServer(idx))
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	tl := bff.NewGRPCTimeline(timelinev1.NewTimelineServiceClient(conn))
	h := bff.NewHandler(bff.NewBoard(), nil, tl, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/customers/"+cust+"/timeline?q=Orlando", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var out struct {
		Items []bff.TimelineEntry `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("items = %+v", out.Items)
	}
	if out.Items[0].Source != "advisory.note.recorded" || !out.Items[0].OccurredAt.Equal(occurred) {
		t.Errorf("source, occurred_at = %q, %v; want advisory.note.recorded, %v", out.Items[0].Source, out.Items[0].OccurredAt, occurred)
	}
}

func TestHTTP_TimelineOrderDescReverses(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	now := time.Now().UTC()
	rows := []struct {
		ago     time.Duration
		payload map[string]any
	}{
		{12 * time.Minute, map[string]any{"text": "msg", "channel": "chat", "meta": "x"}},
		{2900 * time.Minute, map[string]any{"kind": "nota", "title": "Nota", "text": "n", "meta": "a"}},
		{21000 * time.Minute, map[string]any{"kind": "aporte", "title": "Aporte", "text": "US$", "meta": "c"}},
	}
	for _, r := range rows {
		eid := identity.MustNewV7()
		key := "advisory.note.recorded"
		if r.payload["kind"] == "aporte" {
			key = "account.event.recorded"
		}
		if _, ok := r.payload["channel"]; ok {
			key = "message.received"
		}
		body, _ := json.Marshal(map[string]any{
			"event_id": eid, "occurred_at": now.Add(-r.ago),
			"customer_id": cust, "schema_version": 1, "payload": r.payload,
		})
		if _, _, err := idx.ApplyDelivery(context.Background(), key, body); err != nil {
			t.Fatal(err)
		}
	}
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	timelinev1.RegisterTimelineServiceServer(srv, timeline.NewGRPCServer(idx))
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	tl := bff.NewGRPCTimeline(timelinev1.NewTimelineServiceClient(conn))
	h := bff.NewHandler(bff.NewBoard(), nil, tl, nil, nil, nil)

	reqAsc := httptest.NewRequest(http.MethodGet, "/v1/customers/"+cust+"/timeline", nil)
	rrAsc := httptest.NewRecorder()
	h.ServeHTTP(rrAsc, reqAsc)
	var asc struct {
		Items []bff.TimelineEntry `json:"items"`
	}
	if err := json.Unmarshal(rrAsc.Body.Bytes(), &asc); err != nil {
		t.Fatal(err)
	}
	if len(asc.Items) != 3 || asc.Items[0].Ago > asc.Items[1].Ago {
		t.Fatalf("asc items = %+v", asc.Items)
	}

	reqDesc := httptest.NewRequest(http.MethodGet, "/v1/customers/"+cust+"/timeline?order=desc", nil)
	rrDesc := httptest.NewRecorder()
	h.ServeHTTP(rrDesc, reqDesc)
	var desc struct {
		Items []bff.TimelineEntry `json:"items"`
	}
	if err := json.Unmarshal(rrDesc.Body.Bytes(), &desc); err != nil {
		t.Fatal(err)
	}
	if len(desc.Items) != 3 {
		t.Fatalf("desc items = %+v", desc.Items)
	}
	if desc.Items[0].Ago < desc.Items[1].Ago || desc.Items[0].EventID != asc.Items[2].EventID {
		t.Fatalf("desc did not reverse asc: asc=%+v desc=%+v", asc.Items, desc.Items)
	}
}

func TestHTTP_ActionsUnavailable503(t *testing.T) {
	t.Parallel()
	h := bff.NewHandler(bff.NewBoard(), nil, nil, nil, nil, nil)
	id := identity.MustNewV7()
	req := httptest.NewRequest(http.MethodPut, "/v1/actions/"+id, strings.NewReader(`"contact"`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}
