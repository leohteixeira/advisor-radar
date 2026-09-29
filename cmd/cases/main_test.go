package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
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

// recordingChannel records the routing keys the topology binds.
type recordingChannel struct {
	bindings []string
}

func (c *recordingChannel) QueueDeclare(name string, _, _, _, _ bool, _ amqp.Table) (amqp.Queue, error) {
	return amqp.Queue{Name: name}, nil
}

func (c *recordingChannel) QueueBind(name, key, _ string, _ bool, _ amqp.Table) error {
	c.bindings = append(c.bindings, name+"<-"+key)
	return nil
}

// cases consumes message.triaged and its own SLA delay only. It never binds
// alert.raised, so no alert kind (perfil included) can open a case.
func TestTopology_NeverBindsAlertRaised(t *testing.T) {
	t.Parallel()

	ch := &recordingChannel{}
	if err := declareSLATopology(ch, "advisor-radar"); err != nil {
		t.Fatalf("declareSLATopology: %v", err)
	}
	if err := declareTriagedTopology(ch, "advisor-radar"); err != nil {
		t.Fatalf("declareTriagedTopology: %v", err)
	}
	want := []string{
		"cases.sla.breached<-" + cases.SLADelayArgs(60).DeadLetterRoutingKey,
		triagedQueue + "<-" + event.NameMessageTriaged,
	}
	if !slices.Equal(ch.bindings, want) {
		t.Fatalf("bindings = %v, want %v", ch.bindings, want)
	}
	for _, b := range ch.bindings {
		if strings.HasSuffix(b, "<-"+event.NameAlertRaised) {
			t.Fatalf("cases binds %s", b)
		}
	}
}
