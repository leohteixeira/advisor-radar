package timeline_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	timelinev1 "github.com/leohteixeira/advisor-radar/gen/timeline/v1"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

func TestGRPC_SearchViaBufconn(t *testing.T) {
	t.Parallel()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	timelinev1.RegisterTimelineServiceServer(srv, timeline.NewGRPCServer(timeline.NewIndex()))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := timelinev1.NewTimelineServiceClient(conn)
	res, err := client.Search(context.Background(), &timelinev1.SearchRequest{
		CustomerId: "c01",
		Query:      "Orlando",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.GetItems()) != 1 || res.GetItems()[0].GetKind() != "nota" {
		t.Fatalf("items = %+v", res.GetItems())
	}
}
