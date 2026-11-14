package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/event"
)

func TestTriagedOutcome(t *testing.T) {
	t.Parallel()

	transient := errors.New("advisory unavailable")
	tests := []struct {
		name        string
		err         error
		ctxErr      error
		wantAck     bool
		wantRequeue bool
	}{
		{name: "success acks", wantAck: true},
		{name: "success during shutdown still acks", ctxErr: context.Canceled, wantAck: true},
		{
			name: "permanent dead-letters",
			err:  cases.PermanentDeliveryError{Err: cases.ErrUnknownCustomer},
		},
		{
			name: "transient exhausted with a live context dead-letters",
			err:  fmt.Errorf("cases: intake: %w", transient),
		},
		{
			name:        "canceled context requeues",
			err:         errors.Join(transient, context.Canceled),
			ctxErr:      context.Canceled,
			wantRequeue: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ack, requeue := triagedOutcome(tt.err, tt.ctxErr)
			if ack != tt.wantAck || requeue != tt.wantRequeue {
				t.Fatalf("triagedOutcome = (ack %v, requeue %v), want (%v, %v)", ack, requeue, tt.wantAck, tt.wantRequeue)
			}
		})
	}
}

// recordingChannel records what the topology declares and binds.
type recordingChannel struct {
	bindings []string
	queues   map[string]amqp.Table
}

func (c *recordingChannel) QueueDeclare(name string, _, _, _, _ bool, args amqp.Table) (amqp.Queue, error) {
	if c.queues == nil {
		c.queues = make(map[string]amqp.Table)
	}
	c.queues[name] = args
	return amqp.Queue{Name: name}, nil
}

func (c *recordingChannel) QueueBind(name, key, _ string, _ bool, _ amqp.Table) error {
	c.bindings = append(c.bindings, name+"<-"+key)
	return nil
}

// cases consumes its own SLA breaches, message.triaged, the advisor's
// open-case command, and alert.raised. Only the first two and the command can
// open a case: alert.raised is bound for the SLA recalculation of ADR 0012,
// and cases.Resegment (tested in internal/cases) never opens one, so no alert
// kind (perfil included) can.
func TestTopology_Bindings(t *testing.T) {
	t.Parallel()

	ch := &recordingChannel{}
	if err := declareSLATopology(ch, "advisor-radar"); err != nil {
		t.Fatalf("declareSLATopology: %v", err)
	}
	for _, q := range intakeQueues {
		if err := declareIntakeTopology(ch, "advisor-radar", q); err != nil {
			t.Fatalf("declareIntakeTopology %s: %v", q.name, err)
		}
	}
	want := []string{
		"cases.sla.breached<-" + cases.SLADelayArgs(60).DeadLetterRoutingKey,
		triagedQueue + "<-" + event.NameMessageTriaged,
		"cases.case.open.requested<-" + event.NameCaseOpenRequested,
		"cases.alert.raised<-" + event.NameAlertRaised,
	}
	if !slices.Equal(ch.bindings, want) {
		t.Fatalf("bindings = %v, want %v", ch.bindings, want)
	}
	for _, q := range intakeQueues {
		args := ch.queues[q.name]
		if args["x-dead-letter-routing-key"] != q.dlq || args["x-dead-letter-exchange"] != "" {
			t.Fatalf("%s dead-letters to %v via %q, want %s via the default exchange",
				q.name, args["x-dead-letter-routing-key"], args["x-dead-letter-exchange"], q.dlq)
		}
		if _, ok := ch.queues[q.dlq]; !ok {
			t.Fatalf("%s is not declared", q.dlq)
		}
	}
	// The legacy delay queue keeps its original arguments: redeclaring a
	// durable queue with other arguments fails, and its messages must keep
	// dead-lettering.
	legacy, ok := ch.queues[cases.LegacyDelayQueue]
	if !ok {
		t.Fatalf("%s is not declared", cases.LegacyDelayQueue)
	}
	if len(legacy) != 2 || legacy["x-dead-letter-exchange"] != "advisor-radar" ||
		legacy["x-dead-letter-routing-key"] != event.NameCaseSLABreached {
		t.Fatalf("legacy delay queue args = %v", legacy)
	}
}

// Each TTL has its own delay queue with the TTL on the queue, so a short SLA
// delay is never held behind a long one (RabbitMQ only expires the head).
func TestDeclareDelayQueue_OneQueuePerTTL(t *testing.T) {
	t.Parallel()

	ch := &recordingChannel{}
	hour := cases.SLADelayArgs(60).TTLMs
	day := cases.SLADelayArgs(1440).TTLMs
	for _, ttl := range []int{hour, day} {
		if err := declareDelayQueue(ch, "advisor-radar", ttl); err != nil {
			t.Fatalf("declareDelayQueue %d: %v", ttl, err)
		}
	}
	if cases.DelayQueue(hour) == cases.DelayQueue(day) || cases.DelayQueue(hour) == cases.LegacyDelayQueue {
		t.Fatalf("delay queues collide: %s, %s", cases.DelayQueue(hour), cases.DelayQueue(day))
	}
	for _, ttl := range []int{hour, day} {
		args, ok := ch.queues[cases.DelayQueue(ttl)]
		if !ok {
			t.Fatalf("%s is not declared", cases.DelayQueue(ttl))
		}
		if args["x-message-ttl"] != int64(ttl) {
			t.Fatalf("%s x-message-ttl = %v, want %d", cases.DelayQueue(ttl), args["x-message-ttl"], ttl)
		}
		if args["x-dead-letter-exchange"] != "advisor-radar" || args["x-dead-letter-routing-key"] != event.NameCaseSLABreached {
			t.Fatalf("%s dead-letter args = %v", cases.DelayQueue(ttl), args)
		}
	}
	if len(ch.bindings) != 0 {
		t.Fatalf("delay queues are reached through the default exchange, got bindings %v", ch.bindings)
	}
}
