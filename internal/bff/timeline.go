package bff

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	timelinev1 "github.com/leohteixeira/advisor-radar/gen/timeline/v1"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

// TimelineEntry is one customer 360 row as returned by the BFF.
type TimelineEntry struct {
	EventID    string `json:"event_id"`
	CustomerID string `json:"customer_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Text       string `json:"text"`
	Meta       string `json:"meta"`
	Ago        int    `json:"ago"`
}

// TimelineClient reads the customer 360. The BFF never stores the rows.
type TimelineClient interface {
	Search(ctx context.Context, customerID, query, kind string) ([]TimelineEntry, error)
}

// SeedTimeline serves the in-process c01 seed when TIMELINE_GRPC_TARGET is unset.
type SeedTimeline struct {
	index *timeline.Index
}

// NewSeedTimeline returns a client backed by the memory seed.
func NewSeedTimeline() *SeedTimeline {
	return &SeedTimeline{index: timeline.NewIndex()}
}

// Search implements TimelineClient.
func (s *SeedTimeline) Search(ctx context.Context, customerID, query, kind string) ([]TimelineEntry, error) {
	items, err := s.index.Search(ctx, customerID, query, kind)
	if err != nil {
		return nil, fmt.Errorf("bff: timeline seed: %w", err)
	}
	return toBFFEntries(items), nil
}

// GRPCTimeline dials the timeline-indexer. It does not keep a local copy of rows.
type GRPCTimeline struct {
	client timelinev1.TimelineServiceClient
}

// NewGRPCTimeline wraps an existing TimelineService client (tests use bufconn).
func NewGRPCTimeline(client timelinev1.TimelineServiceClient) *GRPCTimeline {
	return &GRPCTimeline{client: client}
}

// NewTimelineClient returns SeedTimeline when target is empty, otherwise a gRPC client.
func NewTimelineClient(target string) (TimelineClient, func(), error) {
	if target == "" {
		return NewSeedTimeline(), func() {}, nil
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("bff: dial timeline: %w", err)
	}
	return NewGRPCTimeline(timelinev1.NewTimelineServiceClient(conn)), func() { _ = conn.Close() }, nil
}

// Search implements TimelineClient.
func (g *GRPCTimeline) Search(ctx context.Context, customerID, query, kind string) ([]TimelineEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.Search(callCtx, &timelinev1.SearchRequest{
		CustomerId: customerID,
		Query:      query,
		Kind:       kind,
	})
	if err != nil {
		return nil, fmt.Errorf("bff: timeline search: %w", err)
	}
	out := make([]TimelineEntry, 0, len(res.GetItems()))
	for _, it := range res.GetItems() {
		out = append(out, TimelineEntry{
			EventID:    it.GetEventId(),
			CustomerID: it.GetCustomerId(),
			Kind:       it.GetKind(),
			Title:      it.GetTitle(),
			Text:       it.GetText(),
			Meta:       it.GetMeta(),
			Ago:        int(it.GetAgo()),
		})
	}
	return out, nil
}

func toBFFEntries(items []timeline.Entry) []TimelineEntry {
	out := make([]TimelineEntry, 0, len(items))
	for _, e := range items {
		out = append(out, TimelineEntry{
			EventID:    e.EventID,
			CustomerID: e.CustomerID,
			Kind:       e.Kind,
			Title:      e.Title,
			Text:       e.Text,
			Meta:       e.Meta,
			Ago:        e.Ago,
		})
	}
	return out
}
