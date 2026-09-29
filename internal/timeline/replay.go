package timeline

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// routingKeys are the events the customer 360 indexes. The live consumer
// binds exactly these, and the startup replay skips every other outbox row,
// such as case.sla.breached, so both paths index the same events.
var routingKeys = []string{
	event.NameAccountEventRecorded,
	event.NameMessageReceived,
	event.NameMessageTriaged,
	event.NameAlertRaised,
	event.NameCaseOpened,
	event.NameCaseStatusChanged,
	"advisory.note.recorded",
}

// RoutingKeys returns the routing keys the timeline indexes, for the live
// consumer to bind.
func RoutingKeys() []string {
	return slices.Clone(routingKeys)
}

// OutboxRow is one outbox record to replay into the index.
type OutboxRow struct {
	EventID    string
	RoutingKey string
	Payload    []byte
}

// ReplayOutboxes reads every outbox row from the given databases and applies
// the ones the timeline indexes. Elasticsearch document id is event_id (via
// IndexDoc).
func ReplayOutboxes(ctx context.Context, idx *Index, elastic *ElasticStore, pools ...*pgxpool.Pool) error {
	for _, pool := range pools {
		if pool == nil {
			continue
		}
		rows, err := listOutbox(ctx, pool)
		if err != nil {
			return err
		}
		if err := replayRows(ctx, idx, elastic, rows); err != nil {
			return err
		}
	}
	return nil
}

// replayRows applies outbox rows in order. A row whose routing key the
// timeline does not index is skipped, as the live consumer never receives it.
func replayRows(ctx context.Context, idx *Index, elastic *ElasticStore, rows []OutboxRow) error {
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !slices.Contains(routingKeys, row.RoutingKey) {
			continue
		}
		applied, entry, err := idx.ApplyDelivery(ctx, row.RoutingKey, row.Payload)
		if err != nil {
			return fmt.Errorf("timeline: replay %s: %w", row.EventID, err)
		}
		if applied && elastic != nil {
			if err := elastic.IndexDoc(ctx, entry); err != nil {
				idx.Forget(entry.EventID)
				return fmt.Errorf("timeline: index %s: %w", row.EventID, err)
			}
		}
	}
	return nil
}

func listOutbox(ctx context.Context, pool *pgxpool.Pool) ([]OutboxRow, error) {
	const q = `SELECT event_id::text, routing_key, payload FROM outbox ORDER BY id`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("timeline: list outbox: %w", err)
	}
	defer rows.Close()
	out := make([]OutboxRow, 0)
	for rows.Next() {
		var row OutboxRow
		if err := rows.Scan(&row.EventID, &row.RoutingKey, &row.Payload); err != nil {
			return nil, fmt.Errorf("timeline: scan outbox: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
