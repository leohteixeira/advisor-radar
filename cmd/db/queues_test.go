package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

// fakeBroker is a broker whose queues are a map from name to state. Like
// RabbitMQ, it answers a queue it does not know with a 404 channel error.
type fakeBroker struct {
	queues map[string]*fakeQueue
	// openErr fails every channel open; purgeErr fails every purge.
	openErr  error
	purgeErr error
	// declareErr fails the passive declare of the named queue with an error
	// that is not "not found".
	declareErr map[string]error
	purged     []string
	opened     int
	closed     int
}

type fakeQueue struct {
	messages  int
	consumers int
}

type fakeChannel struct{ broker *fakeBroker }

func (b *fakeBroker) open() (queueChannel, error) {
	if b.openErr != nil {
		return nil, b.openErr
	}
	b.opened++
	return fakeChannel{broker: b}, nil
}

func (c fakeChannel) QueueDeclarePassive(name string, _, _, _, _ bool, _ amqp.Table) (amqp.Queue, error) {
	if err := c.broker.declareErr[name]; err != nil {
		return amqp.Queue{}, err
	}
	queue, ok := c.broker.queues[name]
	if !ok {
		return amqp.Queue{}, &amqp.Error{Code: amqp.NotFound, Reason: "NOT_FOUND - no queue '" + name + "'"}
	}
	return amqp.Queue{Name: name, Messages: queue.messages, Consumers: queue.consumers}, nil
}

func (c fakeChannel) QueuePurge(name string, _ bool) (int, error) {
	if c.broker.purgeErr != nil {
		return 0, c.broker.purgeErr
	}
	queue := c.broker.queues[name]
	removed := queue.messages
	queue.messages = 0
	c.broker.purged = append(c.broker.purged, name)
	return removed, nil
}

func (c fakeChannel) Close() error {
	c.broker.closed++
	return nil
}

func TestPurgeQueues(t *testing.T) {
	t.Parallel()
	errBoom := errors.New("boom")
	tests := []struct {
		name   string
		broker *fakeBroker
		queues []string
		// wantErr is a text the error holds; empty means no error.
		wantErr    string
		wantPurged []string
		// wantOut are texts the output holds.
		wantOut []string
		// wantLeft is the messages left per queue.
		wantLeft map[string]int
	}{
		{
			name: "empties every queue that exists and skips the ones that do not",
			broker: &fakeBroker{queues: map[string]*fakeQueue{
				"timeline.indexer":         {messages: 3},
				"cases.sla.delay.14400000": {messages: 1},
				"cases.message.triaged":    {},
			}},
			queues:     []string{"timeline.indexer", "cases.alert.raised", "cases.sla.delay.14400000", "cases.message.triaged"},
			wantPurged: []string{"timeline.indexer", "cases.sla.delay.14400000", "cases.message.triaged"},
			wantOut: []string{
				"queues: cases.alert.raised is not declared, nothing to empty",
				"queues: emptied timeline.indexer (3 messages)",
				"queues: emptied cases.sla.delay.14400000 (1 messages)",
				"queues: emptied cases.message.triaged (0 messages)",
			},
			wantLeft: map[string]int{"timeline.indexer": 0, "cases.sla.delay.14400000": 0, "cases.message.triaged": 0},
		},
		{
			name:   "no queue exists yet",
			broker: &fakeBroker{queues: map[string]*fakeQueue{}},
			queues: []string{"timeline.indexer", "bff.queue.signals"},
		},
		{
			name: "a running service stops the seed before anything is purged",
			broker: &fakeBroker{queues: map[string]*fakeQueue{
				"timeline.indexer":  {messages: 3},
				"bff.queue.signals": {messages: 2, consumers: 1},
				"cases.sla.breached": {
					consumers: 2,
				},
			}},
			queues:   []string{"timeline.indexer", "bff.queue.signals", "cases.sla.breached"},
			wantErr:  "queues with consumers: bff.queue.signals (1), cases.sla.breached (2)",
			wantLeft: map[string]int{"timeline.indexer": 3, "bff.queue.signals": 2},
		},
		{
			name:    "the broker refuses a channel",
			broker:  &fakeBroker{openErr: errBoom},
			queues:  []string{"timeline.indexer"},
			wantErr: "open broker channel: boom",
		},
		{
			name: "an inspection that fails for another reason is not read as an absent queue",
			broker: &fakeBroker{
				queues:     map[string]*fakeQueue{"timeline.indexer": {messages: 3}},
				declareErr: map[string]error{"timeline.indexer": &amqp.Error{Code: amqp.AccessRefused, Reason: "ACCESS_REFUSED"}},
			},
			queues:   []string{"timeline.indexer"},
			wantErr:  "inspect queue timeline.indexer",
			wantLeft: map[string]int{"timeline.indexer": 3},
		},
		{
			name:     "a purge that fails",
			broker:   &fakeBroker{queues: map[string]*fakeQueue{"timeline.indexer": {messages: 3}}, purgeErr: errBoom},
			queues:   []string{"timeline.indexer"},
			wantErr:  "purge queue timeline.indexer: boom",
			wantLeft: map[string]int{"timeline.indexer": 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out strings.Builder
			err := purgeQueues(context.Background(), tt.broker.open, tt.queues, &out)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("purgeQueues: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("purgeQueues error = %v, want one holding %q", err, tt.wantErr)
			}
			if !slices.Equal(tt.broker.purged, tt.wantPurged) {
				t.Errorf("purged = %v, want %v", tt.broker.purged, tt.wantPurged)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out.String(), want+"\n") {
					t.Errorf("output misses %q:\n%s", want, out.String())
				}
			}
			for name, want := range tt.wantLeft {
				if got := tt.broker.queues[name].messages; got != want {
					t.Errorf("%s holds %d messages, want %d", name, got, want)
				}
			}
			if tt.broker.opened != tt.broker.closed {
				t.Errorf("opened %d channels, closed %d", tt.broker.opened, tt.broker.closed)
			}
		})
	}
}

func TestPurgeQueues_CanceledContext(t *testing.T) {
	t.Parallel()
	broker := &fakeBroker{queues: map[string]*fakeQueue{"timeline.indexer": {messages: 3}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := purgeQueues(ctx, broker.open, []string{"timeline.indexer"}, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("purgeQueues error = %v, want context.Canceled", err)
	}
	if len(broker.purged) != 0 {
		t.Fatalf("purged = %v, want nothing", broker.purged)
	}
}

// The delay queues are named by cases, one per SLA total (ADR 0012).
func TestSeedQueues_ListsEverySLADelayQueue(t *testing.T) {
	t.Parallel()
	queues := seedQueues()
	for _, want := range []string{
		"cases.sla.delay",
		"cases.sla.delay.86400000", "cases.sla.delay.43200000",
		"cases.sla.delay.14400000", "cases.sla.delay.7200000",
		"cases.sla.delay.3600000", "cases.sla.delay.1800000",
	} {
		if !slices.Contains(queues, want) {
			t.Errorf("seedQueues misses %s", want)
		}
	}
	seen := map[string]struct{}{}
	for _, name := range queues {
		if _, dup := seen[name]; dup {
			t.Errorf("seedQueues lists %s twice", name)
		}
		seen[name] = struct{}{}
	}
}

// queueLiteral matches a string literal that names something of one service,
// as every queue under cmd/ does ("cases.alert.raised.dlq").
var queueLiteral = regexp.MustCompile(`"((?:advisory|triage|cases|timeline|bff|account-sim)\.[a-z0-9_.-]+)"`)

// A queue a service declares and the seed does not empty keeps its messages
// across a reseed and delivers the walk the seed removed. Every service
// names its queues with string literals under cmd/, so each such literal must
// be in seedQueues. A new literal of that shape that is not a queue belongs
// in notQueues below.
func TestSeedQueues_CoverDeclaredQueues(t *testing.T) {
	t.Parallel()
	// Events whose names start like a queue's.
	notQueues := timeline.RoutingKeys()

	files, err := filepath.Glob(filepath.Join("..", "*", "*.go"))
	if err != nil {
		t.Fatalf("glob cmd: %v", err)
	}
	queues := seedQueues()
	found := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || filepath.Base(filepath.Dir(file)) == "db" {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, match := range queueLiteral.FindAllStringSubmatch(string(body), -1) {
			name := match[1]
			if slices.Contains(notQueues, name) {
				continue
			}
			found++
			if !slices.Contains(queues, name) {
				t.Errorf("%s declares %q, which seedQueues does not empty", file, name)
			}
		}
	}
	// advisory, triage, timeline, bff, and the cases queues.
	if found < 8 {
		t.Fatalf("found %d queue literals under cmd/, want at least 8: the scan no longer sees the declarations", found)
	}
}

// Not parallel: it sets the process environment.
func TestBrokerURL(t *testing.T) {
	for _, key := range brokerEnvKeys {
		t.Setenv(key, "")
	}
	if _, _, err := brokerURL(); err == nil || !strings.Contains(err.Error(), "-skip-queues") || !strings.Contains(err.Error(), "BROKER_URL") {
		t.Fatalf("brokerURL without a broker = %v, want an error naming the variables and -skip-queues", err)
	}

	t.Setenv("BFF_BROKER_URL", "amqp://bff")
	t.Setenv("CASES_BROKER_URL", "amqp://cases")
	if key, url, err := brokerURL(); err != nil || key != "CASES_BROKER_URL" || url != "amqp://cases" {
		t.Fatalf("brokerURL = %q, %q, %v, want the first service variable set", key, url, err)
	}

	t.Setenv("BROKER_URL", "amqp://one")
	if key, url, err := brokerURL(); err != nil || key != "BROKER_URL" || url != "amqp://one" {
		t.Fatalf("brokerURL = %q, %q, %v, want BROKER_URL first", key, url, err)
	}
}

// Not parallel: it sets the process environment.
func TestSeed_WithoutBrokerChangesNothing(t *testing.T) {
	for _, key := range brokerEnvKeys {
		t.Setenv(key, "")
	}
	// A database URL that would fail loudly if the seed got that far.
	for _, key := range []string{"ACCOUNT_SIM_DATABASE_URL", "ADVISORY_DATABASE_URL", "TRIAGE_DATABASE_URL", "CASES_DATABASE_URL"} {
		t.Setenv(key, "")
	}
	var out strings.Builder
	err := seed(context.Background(), t.TempDir(), false, &out)
	if err == nil || !strings.Contains(err.Error(), "no broker is configured") {
		t.Fatalf("seed error = %v, want the missing broker", err)
	}
	if out.Len() != 0 {
		t.Fatalf("seed wrote %q before failing", out.String())
	}
}
