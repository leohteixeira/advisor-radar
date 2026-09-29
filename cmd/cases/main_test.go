package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/cases"
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
