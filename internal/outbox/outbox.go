// Package outbox stages and publishes account-sim events transactionally.
package outbox

import (
	"context"
	"fmt"
	"time"
)

// Row is one unpublished or published outbox record.
type Row struct {
	EventID    string
	RoutingKey string
	Payload    []byte
}

// Tx is the write side of a single insert transaction.
type Tx interface {
	Insert(ctx context.Context, row Row) error
}

// Store persists outbox rows. Declared here for the Publish consumers.
type Store interface {
	WithTx(ctx context.Context, fn func(Tx) error) error
	ListUnpublished(ctx context.Context) ([]Row, error)
	MarkPublished(ctx context.Context, eventID string, at time.Time) error
}

// Broker accepts one event body under a routing key.
type Broker interface {
	Publish(ctx context.Context, routingKey string, body []byte) error
}

// Publish sends each unpublished row once. A broker error leaves that row
// and later rows unpublished. The row is marked published only after ack.
func Publish(ctx context.Context, store Store, broker Broker) error {
	if store == nil {
		return fmt.Errorf("outbox: store is required")
	}
	if broker == nil {
		return fmt.Errorf("outbox: broker is required")
	}

	rows, err := store.ListUnpublished(ctx)
	if err != nil {
		return fmt.Errorf("outbox: list unpublished: %w", err)
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("outbox: publish: %w", err)
		}
		if err := broker.Publish(ctx, row.RoutingKey, row.Payload); err != nil {
			return fmt.Errorf("outbox: publish %s: %w", row.EventID, err)
		}
		if err := store.MarkPublished(ctx, row.EventID, time.Now().UTC()); err != nil {
			return fmt.Errorf("outbox: mark published %s: %w", row.EventID, err)
		}
	}
	return nil
}

// RunPublisher polls unpublished rows until ctx is canceled.
// Failed publishes leave rows unpublished and are retried on the next tick.
func RunPublisher(ctx context.Context, store Store, broker Broker, every time.Duration) error {
	if every <= 0 {
		every = time.Second
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		if err := Publish(ctx, store, broker); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// Retry on the next tick; do not mark the failed row published.
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
