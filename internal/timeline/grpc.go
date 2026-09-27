package timeline

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	timelinev1 "github.com/leohteixeira/advisor-radar/gen/timeline/v1"
)

// GRPCServer exposes Index.Search over gRPC.
type GRPCServer struct {
	timelinev1.UnimplementedTimelineServiceServer
	index *Index
}

// NewGRPCServer wraps idx for TimelineService.
func NewGRPCServer(idx *Index) *GRPCServer {
	return &GRPCServer{index: idx}
}

// Search implements timeline.v1.TimelineService.
func (s *GRPCServer) Search(ctx context.Context, req *timelinev1.SearchRequest) (*timelinev1.SearchResponse, error) {
	if req == nil || req.GetCustomerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "customer_id is required")
	}
	items, err := s.index.Search(ctx, req.GetCustomerId(), req.GetQuery(), req.GetKind())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search: %v", err)
	}
	out := make([]*timelinev1.TimelineItem, 0, len(items))
	for _, e := range items {
		out = append(out, &timelinev1.TimelineItem{
			EventId:    e.EventID,
			CustomerId: e.CustomerID,
			Kind:       e.Kind,
			Title:      e.Title,
			Text:       e.Text,
			Meta:       e.Meta,
			Ago:        int32(e.Ago),
		})
	}
	return &timelinev1.SearchResponse{Items: out}, nil
}

var _ timelinev1.TimelineServiceServer = (*GRPCServer)(nil)
