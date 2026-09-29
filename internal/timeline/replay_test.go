package timeline_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

func outboxRow(t *testing.T, routingKey, customerID string, payload map[string]any) timeline.OutboxRow {
	t.Helper()
	eventID := identity.MustNewV7()
	body, err := json.Marshal(map[string]any{
		"event_id": eventID, "occurred_at": time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC),
		"customer_id": customerID, "schema_version": 1,
		"payload": payload,
	})
	if err != nil {
		t.Fatalf("marshal %s: %v", routingKey, err)
	}
	return timeline.OutboxRow{EventID: eventID, RoutingKey: routingKey, Payload: body}
}

// A cases outbox holds case.sla.breached, which the timeline does not index.
// The replay skips it, as the live consumer never binds it, and indexes the
// rows around it.
func TestReplayRows_SkipsEventsTheTimelineDoesNotIndex(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	cust := identity.MustNewV7()
	rows := []timeline.OutboxRow{
		outboxRow(t, event.NameCaseOpened, cust, map[string]any{"case_id": "k1", "state": "Aberto"}),
		outboxRow(t, event.NameCaseSLABreached, cust, map[string]any{"case_id": "k1", "state": "Aberto", "escalated": true}),
		outboxRow(t, event.NameCaseStatusChanged, cust, map[string]any{"case_id": "k1", "state": "Resolvido"}),
	}

	if err := timeline.ReplayRows(context.Background(), idx, nil, rows); err != nil {
		t.Fatalf("ReplayRows: %v", err)
	}

	got, err := idx.Search(context.Background(), cust, "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var sources []string
	for _, e := range got {
		sources = append(sources, e.Source)
	}
	slices.Sort(sources)
	want := []string{event.NameCaseOpened, event.NameCaseStatusChanged}
	if !slices.Equal(sources, want) {
		t.Fatalf("indexed sources = %v, want %v", sources, want)
	}
}

// A row the timeline indexes but cannot decode still stops the replay.
func TestReplayRows_IndexedRowErrorStops(t *testing.T) {
	t.Parallel()
	rows := []timeline.OutboxRow{{EventID: "bad", RoutingKey: event.NameCaseOpened, Payload: []byte("{")}}

	err := timeline.ReplayRows(context.Background(), timeline.NewIndex(), nil, rows)
	if err == nil || !strings.Contains(err.Error(), "timeline: replay bad") {
		t.Fatalf("ReplayRows error = %v, want the replay error for row bad", err)
	}
}

func TestReplayRows_StopsOnCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rows := []timeline.OutboxRow{outboxRow(t, event.NameCaseOpened, identity.MustNewV7(), map[string]any{"case_id": "k1"})}

	if err := timeline.ReplayRows(ctx, timeline.NewIndex(), nil, rows); err != context.Canceled {
		t.Fatalf("ReplayRows error = %v, want context.Canceled", err)
	}
}

// Every routing key the consumer binds is one the index can apply, and
// case.sla.breached is not among them.
func TestRoutingKeys_AreIndexable(t *testing.T) {
	t.Parallel()
	keys := timeline.RoutingKeys()
	if slices.Contains(keys, event.NameCaseSLABreached) {
		t.Fatalf("RoutingKeys = %v, must not bind %s", keys, event.NameCaseSLABreached)
	}
	for _, key := range keys {
		row := outboxRow(t, key, identity.MustNewV7(), map[string]any{})
		if _, _, err := timeline.NewIndex().ApplyDelivery(context.Background(), key, row.Payload); err != nil {
			t.Errorf("ApplyDelivery(%s): %v", key, err)
		}
	}
	keys[0] = "mutated"
	if timeline.RoutingKeys()[0] == "mutated" {
		t.Fatal("RoutingKeys returns the shared slice")
	}
}
