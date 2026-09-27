package proc_test

import (
	"context"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/proc"
)

func TestRun_CancelReturnsNil(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- proc.Run(ctx, "advisory")
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after cancel")
	}
}

func TestRun_BlankNameRejected(t *testing.T) {
	t.Parallel()

	err := proc.Run(context.Background(), "")
	if err == nil {
		t.Fatal("Run() error = nil, want rejection for blank name")
	}
}
