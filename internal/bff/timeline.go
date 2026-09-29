package bff

import (
	"context"
	"fmt"
	"slices"
	"time"

	timelinev1 "github.com/leohteixeira/advisor-radar/gen/timeline/v1"
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
	// Source is the routing key of the event the row came from.
	Source string `json:"source,omitempty"`
	// OccurredAt is the event time, zero when the indexer did not send it.
	OccurredAt time.Time `json:"occurred_at,omitzero"`
	// ProductID is the catalog product an account event names, such as the
	// one an aplicacao bought.
	ProductID string `json:"product_id,omitempty"`
	// AmountCents is an account event's amount in integer USD cents.
	AmountCents int64 `json:"amount_cents,omitempty"`
}

// TimelineClient reads the customer 360. The BFF never stores the rows.
type TimelineClient interface {
	Search(ctx context.Context, customerID, query, kind string) ([]TimelineEntry, error)
}

// EmptyTimeline returns no rows (used when TIMELINE_GRPC_TARGET is unset).
type EmptyTimeline struct{}

// Search implements TimelineClient.
func (EmptyTimeline) Search(context.Context, string, string, string) ([]TimelineEntry, error) {
	return []TimelineEntry{}, nil
}

// GRPCTimeline dials the timeline-indexer. It does not keep a local copy of rows.
type GRPCTimeline struct {
	client timelinev1.TimelineServiceClient
}

// NewGRPCTimeline wraps an existing TimelineService client (tests use bufconn).
func NewGRPCTimeline(client timelinev1.TimelineServiceClient) *GRPCTimeline {
	return &GRPCTimeline{client: client}
}

// NewTimelineClient returns EmptyTimeline when target is empty, otherwise a gRPC client.
func NewTimelineClient(target string) (TimelineClient, func(), error) {
	if target == "" {
		return EmptyTimeline{}, func() {}, nil
	}
	conn, err := DialGRPC(target)
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
		// An empty or unreadable occurred_at stays zero; readers then use Ago.
		occurredAt, _ := time.Parse(time.RFC3339Nano, it.GetOccurredAt())
		out = append(out, TimelineEntry{
			EventID:     it.GetEventId(),
			CustomerID:  it.GetCustomerId(),
			Kind:        it.GetKind(),
			Title:       it.GetTitle(),
			Text:        it.GetText(),
			Meta:        it.GetMeta(),
			Ago:         int(it.GetAgo()),
			Source:      it.GetSource(),
			OccurredAt:  occurredAt,
			ProductID:   it.GetProductId(),
			AmountCents: it.GetAmountCents(),
		})
	}
	return out, nil
}

// parseTimelineOrder returns "asc" or "desc". Missing or invalid values mean asc.
func parseTimelineOrder(raw string) string {
	if raw == "desc" {
		return "desc"
	}
	return "asc"
}

// reverseTimeline reverses items in place (used for order=desc after ascending search).
func reverseTimeline(items []TimelineEntry) {
	slices.Reverse(items)
}
