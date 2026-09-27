// Package proc provides shared process startup without business work.
package proc

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// Run logs that the named service started and returns when ctx is canceled.
func Run(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("proc: service name is required")
	}
	if ctx == nil {
		return fmt.Errorf("proc: context is required")
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("service started", "service", name)

	<-ctx.Done()
	return nil
}
