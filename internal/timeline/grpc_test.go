package timeline_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	timelinev1 "github.com/leohteixeira/advisor-radar/gen/timeline/v1"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

func TestGRPC_SearchViaBufconn(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	eid := identity.MustNewV7()
	occurred := time.Now().UTC().Add(-12 * time.Minute)
	body, _ := json.Marshal(map[string]any{
		"event_id": eid, "occurred_at": occurred,
		"customer_id": cust, "schema_version": 1,
		"payload": map[string]any{"kind": "nota", "title": "Nota", "text": "Orlando", "meta": "x"},
	})
	if _, _, err := idx.ApplyDelivery(context.Background(), "advisory.note.recorded", body); err != nil {
		t.Fatal(err)
	}
	purchase, err := json.Marshal(map[string]any{
		"event_id": identity.MustNewV7(), "occurred_at": occurred,
		"customer_id": cust, "schema_version": 3,
		"payload": map[string]any{"kind": "aplicacao", "amount": 25000, "product_id": "cobalto"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := idx.ApplyDelivery(context.Background(), event.NameAccountEventRecorded, purchase); err != nil {
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

	client := timelinev1.NewTimelineServiceClient(conn)
	res, err := client.Search(context.Background(), &timelinev1.SearchRequest{CustomerId: cust, Query: "Orlando"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetItems()) != 1 {
		t.Fatalf("items = %+v", res.GetItems())
	}
	item := res.GetItems()[0]
	if item.GetSource() != "advisory.note.recorded" {
		t.Errorf("source = %q", item.GetSource())
	}
	if got, err := time.Parse(time.RFC3339Nano, item.GetOccurredAt()); err != nil || !got.Equal(occurred) {
		t.Errorf("occurred_at = %q (%v), want %v", item.GetOccurredAt(), err, occurred)
	}

	res, err = client.Search(context.Background(), &timelinev1.SearchRequest{CustomerId: cust, Kind: timeline.KindConta})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetItems()) != 1 {
		t.Fatalf("conta items = %+v", res.GetItems())
	}
	if got := res.GetItems()[0]; got.GetProductId() != "cobalto" || got.GetAmountCents() != 25000 {
		t.Errorf("purchase product, cents = %q, %d, want cobalto, 25000", got.GetProductId(), got.GetAmountCents())
	}
}
