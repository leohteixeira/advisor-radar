package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	"github.com/leohteixeira/advisor-radar/internal/envfile"
	"github.com/leohteixeira/advisor-radar/internal/outbox"
	"github.com/leohteixeira/advisor-radar/internal/sim"
	"github.com/leohteixeira/advisor-radar/internal/telemetry"
)

// grpcStopTimeout bounds how long shutdown waits for in-flight RPCs.
const grpcStopTimeout = 5 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := envfile.Load(".env"); err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("service started", "service", "account-sim")

	shutdown, err := telemetry.Setup(ctx, "account-sim")
	if err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}
	runErr := run(ctx, logger)
	if err := telemetry.Stop(shutdown); err != nil {
		logger.Warn("telemetry shutdown failed", "service", "account-sim", "error", err.Error())
	}
	if runErr != nil {
		logger.Error("service failed", "error", runErr.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	dsn := os.Getenv("ACCOUNT_SIM_DATABASE_URL")
	grpcAddr := os.Getenv("ACCOUNT_SIM_GRPC_ADDR")
	brokerURL := os.Getenv("ACCOUNT_SIM_BROKER_URL")
	if dsn == "" {
		if grpcAddr != "" || brokerURL != "" {
			logger.Warn("database url unset; grpc and relay disabled", "service", "account-sim")
		}
		// No database in CI process tests; wait for signal only.
		<-ctx.Done()
		return nil
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("account-sim: connect database: %w", err)
	}
	defer pool.Close()
	// pgxpool connects lazily; fail fast instead of serving with no database.
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("account-sim: ping database: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Each worker sends exactly one value on errCh when it exits.
	errCh := make(chan error, 2)
	workers := 0

	var grpcSrv *grpc.Server
	if grpcAddr != "" {
		lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", grpcAddr)
		if err != nil {
			return fmt.Errorf("account-sim: grpc listen: %w", err)
		}
		grpcSrv = grpc.NewServer(telemetry.GRPCServerOption())
		accountv1.RegisterAccountServiceServer(grpcSrv, sim.NewGRPCServer(sim.NewPGXStore(pool), logger))
		workers++
		go func() {
			logger.Info("grpc listening", "service", "account-sim", "addr", lis.Addr().String())
			err := grpcSrv.Serve(lis)
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				errCh <- fmt.Errorf("account-sim: grpc serve: %w", err)
				return
			}
			errCh <- nil
		}()
	}

	if brokerURL != "" {
		broker, cleanup, err := newAMQPBroker(ctx, brokerURL)
		if err != nil {
			if grpcSrv != nil {
				stopGRPC(grpcSrv)
				<-errCh
			}
			return err
		}
		defer cleanup()
		workers++
		go func() {
			errCh <- outbox.RunPublisher(runCtx, outbox.NewPGXStore(pool), broker, time.Second)
		}()
	}

	if workers == 0 {
		<-ctx.Done()
		return nil
	}

	var firstErr error
	select {
	case <-ctx.Done():
	case firstErr = <-errCh:
		workers--
	}

	// runCtx is a child of the signal ctx, so on SIGINT/SIGTERM the relay is
	// already stopping while GracefulStop drains in-flight commands. Rows those
	// commands commit stay in the outbox until the next relay tick or start.
	// On a worker error, cancel stops the relay after the gRPC drain.
	if grpcSrv != nil {
		stopGRPC(grpcSrv)
	}
	cancel()
	for ; workers > 0; workers-- {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// stopGRPC drains in-flight RPCs and forces a stop after grpcStopTimeout.
func stopGRPC(srv *grpc.Server) {
	done := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(done)
	}()
	timer := time.NewTimer(grpcStopTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		srv.Stop()
		<-done
	}
}

type amqpBroker struct {
	ch       *amqp.Channel
	exchange string
}

func newAMQPBroker(ctx context.Context, url string) (*amqpBroker, func(), error) {
	conn, err := amqp.DialConfig(url, amqp.Config{
		Properties: amqp.Table{"connection_name": "account-sim"},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("account-sim: dial broker: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("account-sim: open channel: %w", err)
	}

	exchange := "advisor.events"
	if err := ch.ExchangeDeclare(
		exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("account-sim: declare exchange: %w", err)
	}

	cleanup := func() {
		_ = ch.Close()
		_ = conn.Close()
	}

	// Respect an already-canceled context before returning a live connection.
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("account-sim: broker setup: %w", err)
	}

	return &amqpBroker{ch: ch, exchange: exchange}, cleanup, nil
}

func (b *amqpBroker) Publish(ctx context.Context, routingKey string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := b.ch.PublishWithContext(
		ctx,
		b.exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("account-sim: broker publish: %w", err)
	}
	return nil
}
