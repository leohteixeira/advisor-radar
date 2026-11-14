package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// brokerTestEnv names a RabbitMQ URL for the purge integration test. Unset,
// the test skips. The test only touches queues it creates, under a random
// prefix, and deletes them after.
const brokerTestEnv = "DB_TEST_BROKER_URL"

// TestPurgeQueues_RabbitMQ runs the purge against a real broker: a plain
// queue, a TTL delay queue with a dead-letter route (as the SLA delays use),
// and a name the broker does not know.
func TestPurgeQueues_RabbitMQ(t *testing.T) {
	t.Parallel()
	url := os.Getenv(brokerTestEnv)
	if url == "" {
		t.Skipf("%s is not set", brokerTestEnv)
	}
	conn, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(brokerDialTimeout)})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	open := func() (queueChannel, error) { return conn.Channel() }

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("random suffix: %v", err)
	}
	prefix := "dbtest." + hex.EncodeToString(suffix)
	plain, delay, dead, missing := prefix+".plain", prefix+".delay", prefix+".dead", prefix+".missing"

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	t.Cleanup(func() {
		cleanup, err := conn.Channel()
		if err != nil {
			t.Errorf("cleanup channel: %v", err)
			return
		}
		for _, name := range []string{plain, delay, dead} {
			if _, err := cleanup.QueueDelete(name, false, false, false); err != nil {
				t.Errorf("delete %s: %v", name, err)
			}
		}
	})
	if err := ch.Confirm(false); err != nil {
		t.Fatalf("confirm mode: %v", err)
	}
	declare := func(name string, args amqp.Table) {
		t.Helper()
		if _, err := ch.QueueDeclare(name, true, false, false, false, args); err != nil {
			t.Fatalf("declare %s: %v", name, err)
		}
	}
	declare(plain, nil)
	declare(dead, nil)
	// A purged delay must not dead-letter: it would arrive as an SLA breach.
	declare(delay, amqp.Table{
		"x-message-ttl":             int64(time.Hour / time.Millisecond),
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": dead,
	})
	publish := func(queue string, n int) {
		t.Helper()
		for range n {
			confirm, err := ch.PublishWithDeferredConfirmWithContext(t.Context(), "", queue, true, false,
				amqp.Publishing{DeliveryMode: amqp.Persistent, Body: []byte("pre-seed")})
			if err != nil {
				t.Fatalf("publish to %s: %v", queue, err)
			}
			if ok, err := confirm.WaitContext(t.Context()); err != nil || !ok {
				t.Fatalf("publish to %s not confirmed: %v, %v", queue, ok, err)
			}
		}
	}
	depth := func(queue string) int {
		t.Helper()
		state, exists, err := inspectQueue(open, queue)
		if err != nil || !exists {
			t.Fatalf("inspect %s = exists %v, %v", queue, exists, err)
		}
		return state.Messages
	}
	publish(plain, 3)
	publish(delay, 2)

	var out strings.Builder
	if err := purgeQueues(context.Background(), open, []string{plain, missing, delay}, &out); err != nil {
		t.Fatalf("purgeQueues: %v", err)
	}
	for queue, want := range map[string]int{plain: 0, delay: 0, dead: 0} {
		if got := depth(queue); got != want {
			t.Errorf("%s holds %d messages after the purge, want %d", queue, got, want)
		}
	}
	for _, want := range []string{
		"emptied " + plain + " (3 messages)",
		"emptied " + delay + " (2 messages)",
		missing + " is not declared",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output misses %q:\n%s", want, out.String())
		}
	}
	// The absent queue was inspected, not created.
	if _, exists, err := inspectQueue(open, missing); err != nil || exists {
		t.Errorf("%s after the purge = exists %v, %v, want absent", missing, exists, err)
	}

	// A consumer means a service is running: nothing is purged.
	publish(plain, 2)
	publish(delay, 1)
	consumerCh, err := conn.Channel()
	if err != nil {
		t.Fatalf("consumer channel: %v", err)
	}
	// Prefetch 1 leaves the second message ready, so the depth shows it.
	if err := consumerCh.Qos(1, 0, false); err != nil {
		t.Fatalf("qos: %v", err)
	}
	if _, err := consumerCh.Consume(plain, "service", false, false, false, false, nil); err != nil {
		t.Fatalf("consume: %v", err)
	}
	err = purgeQueues(context.Background(), open, []string{delay, plain}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), plain+" (1)") {
		t.Fatalf("purgeQueues with a consumer = %v, want an error naming %s", err, plain)
	}
	if got := depth(delay); got != 1 {
		t.Errorf("%s holds %d messages after the refused purge, want 1", delay, got)
	}
	if err := consumerCh.Close(); err != nil {
		t.Fatalf("close consumer: %v", err)
	}
}
