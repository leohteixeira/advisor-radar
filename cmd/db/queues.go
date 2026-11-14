package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/leohteixeira/advisor-radar/internal/cases"
)

const brokerDialTimeout = 10 * time.Second

// brokerEnvKeys name the broker, first one set wins. BROKER_URL is for a
// deployment that hands this tool one variable; the others are the services'
// own, which a local .env already holds. Every service publishes to and
// consumes from the same advisor.events exchange, so they all name one
// broker.
var brokerEnvKeys = []string{
	"BROKER_URL",
	"ACCOUNT_SIM_BROKER_URL",
	"ADVISORY_BROKER_URL",
	"TRIAGE_BROKER_URL",
	"CASES_BROKER_URL",
	"TIMELINE_BROKER_URL",
	"BFF_BROKER_URL",
}

// seedQueues is every durable queue the services declare. A message left in
// one of them was published before the seed: an event of the walk the seed
// removes, a retry in a dead-letter queue, or the SLA delay of a case that no
// longer exists. Delivered after the seed it would put that walk back on the
// queue, the timeline, or a case. A queue declared under cmd/ must be listed
// here; TestSeedQueues_CoverDeclaredQueues fails when one is missing.
func seedQueues() []string {
	queues := []string{
		"advisory.account.event.recorded",
		"triage.message.received",
		"cases.message.triaged",
		"cases.message.triaged.dlq",
		"cases.case.open.requested",
		"cases.case.open.requested.dlq",
		"cases.alert.raised",
		"cases.alert.raised.dlq",
		"cases.sla.breached",
		cases.LegacyDelayQueue,
		"timeline.indexer",
		"bff.queue.signals",
	}
	// One SLA delay queue per TTL (ADR 0012): each segment's base and its
	// half. cases declares them on first use, so some may not exist.
	for _, base := range []int{cases.BaseEssencial, cases.BaseAdvance, cases.BaseSingular} {
		for _, total := range []int{base, base / 2} {
			queues = append(queues, cases.DelayQueue(cases.SLADelayArgs(total).TTLMs))
		}
	}
	return queues
}

// queueChannel is the part of *amqp.Channel the purge uses.
type queueChannel interface {
	QueueDeclarePassive(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueuePurge(name string, noWait bool) (int, error)
	Close() error
}

// brokerURL returns the first broker variable that is set and its value.
func brokerURL() (key, url string, err error) {
	for _, key := range brokerEnvKeys {
		if url := os.Getenv(key); url != "" {
			return key, url, nil
		}
	}
	return "", "", fmt.Errorf(
		"db: seed empties the broker queues and no broker is configured: set one of %s, or pass -skip-queues to load the seed and leave the queues as they are",
		strings.Join(brokerEnvKeys, ", "))
}

// purgeBroker empties the system's queues on the configured broker.
func purgeBroker(ctx context.Context, out io.Writer) error {
	key, url, err := brokerURL()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("db: purge queues: %w", err)
	}
	conn, err := amqp.DialConfig(url, amqp.Config{
		Properties: amqp.Table{"connection_name": "db-seed"},
		Dial:       amqp.DefaultDial(brokerDialTimeout),
	})
	if err != nil {
		return fmt.Errorf("db: dial broker (%s): %w", key, err)
	}
	defer func() { _ = conn.Close() }()

	fmt.Fprintf(out, "queues: emptying on the broker of %s\n", key)
	open := func() (queueChannel, error) { return conn.Channel() }
	return purgeQueues(ctx, open, seedQueues(), out)
}

// purgeQueues empties the given queues. A queue that does not exist is
// skipped: its service has not declared it yet, so it holds nothing.
//
// Every queue is inspected before any is emptied. A queue with a consumer
// means a service is running, and then a seed cannot leave a consistent demo:
// that service holds messages it has not acknowledged, the timeline index
// and the BFF board keep in memory what the seed is about to remove, and a
// purge would discard events the running services still need. In that case
// nothing is purged and the error names the queues.
//
// open returns a new channel; the broker closes a channel on any error, such
// as a queue that does not exist, so each operation uses its own.
func purgeQueues(ctx context.Context, open func() (queueChannel, error), queues []string, out io.Writer) error {
	var present, busy []string
	for _, name := range queues {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("db: inspect queues: %w", err)
		}
		queue, exists, err := inspectQueue(open, name)
		switch {
		case err != nil:
			return err
		case !exists:
			fmt.Fprintf(out, "queues: %s is not declared, nothing to empty\n", name)
		case queue.Consumers > 0:
			busy = append(busy, fmt.Sprintf("%s (%d)", name, queue.Consumers))
		default:
			present = append(present, name)
		}
	}
	if len(busy) > 0 {
		return fmt.Errorf(
			"db: services are still consuming, nothing was changed: stop every service, seed, then start them again; queues with consumers: %s",
			strings.Join(busy, ", "))
	}
	for _, name := range present {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("db: purge queues: %w", err)
		}
		removed, err := purgeQueue(open, name)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "queues: emptied %s (%d messages)\n", name, removed)
	}
	return nil
}

// inspectQueue reads a queue's state without creating it. exists is false
// when the broker does not know the queue.
func inspectQueue(open func() (queueChannel, error), name string) (queue amqp.Queue, exists bool, err error) {
	ch, err := open()
	if err != nil {
		return amqp.Queue{}, false, fmt.Errorf("db: open broker channel: %w", err)
	}
	defer func() { _ = ch.Close() }()
	// Passive: only the name is checked, the other arguments are ignored.
	queue, err = ch.QueueDeclarePassive(name, true, false, false, false, nil)
	if err != nil {
		var amqpErr *amqp.Error
		if errors.As(err, &amqpErr) && amqpErr.Code == amqp.NotFound {
			return amqp.Queue{}, false, nil
		}
		return amqp.Queue{}, false, fmt.Errorf("db: inspect queue %s: %w", name, err)
	}
	return queue, true, nil
}

func purgeQueue(open func() (queueChannel, error), name string) (int, error) {
	ch, err := open()
	if err != nil {
		return 0, fmt.Errorf("db: open broker channel: %w", err)
	}
	defer func() { _ = ch.Close() }()
	removed, err := ch.QueuePurge(name, false)
	if err != nil {
		return 0, fmt.Errorf("db: purge queue %s: %w", name, err)
	}
	return removed, nil
}
