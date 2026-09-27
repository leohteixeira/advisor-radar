package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/leohteixeira/advisor-radar/internal/proc"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := proc.Run(ctx, "cases"); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("service failed", "error", err.Error())
		os.Exit(1)
	}
}
