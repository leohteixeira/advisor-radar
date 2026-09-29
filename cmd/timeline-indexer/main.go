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

	timelinev1 "github.com/leohteixeira/advisor-radar/gen/timeline/v1"
	"github.com/leohteixeira/advisor-radar/internal/envfile"
	"github.com/leohteixeira/advisor-radar/internal/telemetry"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

const amqpDialTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := envfile.Load(".env"); err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("service started", "service", "timeline-indexer")

	shutdown, err := telemetry.Setup(ctx, "timeline-indexer")
	if err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}
	runErr := run(ctx, logger)
	if err := telemetry.Stop(shutdown); err != nil {
		logger.Warn("telemetry shutdown failed", "service", "timeline-indexer", "error", err.Error())
	}
	if runErr != nil {
		logger.Error("service failed", "error", runErr.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	grpcAddr := os.Getenv("TIMELINE_GRPC_ADDR")
	brokerURL := os.Getenv("TIMELINE_BROKER_URL")
	esURL := os.Getenv("ELASTICSEARCH_URL")

	idx := timeline.NewIndex()
	var elastic *timeline.ElasticStore
	if esURL != "" {
		elastic = timeline.NewElasticStore(esURL)
	}

	var pools []*pgxpool.Pool
	for _, envKey := range []string{
		"ACCOUNT_SIM_DATABASE_URL",
		"ADVISORY_DATABASE_URL",
		"CASES_DATABASE_URL",
	} {
		dsn := os.Getenv(envKey)
		if dsn == "" {
			continue
		}
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return fmt.Errorf("timeline-indexer: connect %s: %w", envKey, err)
		}
		defer pool.Close()
		pools = append(pools, pool)
	}
	if len(pools) > 0 {
		if err := timeline.ReplayOutboxes(ctx, idx, elastic, pools...); err != nil {
			return fmt.Errorf("timeline-indexer: replay: %w", err)
		}
		logger.Info("outboxes replayed", "service", "timeline-indexer", "pools", len(pools))
	}

	if grpcAddr == "" && brokerURL == "" {
		<-ctx.Done()
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	workers := 0
	var grpcSrv *grpc.Server
	var amqpCleanup func()

	if grpcAddr != "" {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			return fmt.Errorf("timeline-indexer: listen: %w", err)
		}
		grpcSrv = grpc.NewServer(telemetry.GRPCServerOption())
		timelinev1.RegisterTimelineServiceServer(grpcSrv, timeline.NewGRPCServer(idx))
		workers++
		go func() {
			logger.Info("grpc listening", "service", "timeline-indexer", "addr", grpcAddr)
			err := grpcSrv.Serve(lis)
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				errCh <- fmt.Errorf("timeline-indexer: serve: %w", err)
				return
			}
			errCh <- nil
		}()
	}

	if brokerURL != "" {
		session, cleanup, err := dialAMQP(ctx, brokerURL)
		if err != nil {
			cancel()
			if grpcSrv != nil {
				grpcSrv.Stop()
			}
			return err
		}
		amqpCleanup = cleanup
		ch, err := session.conn.Channel()
		if err != nil {
			cleanup()
			cancel()
			if grpcSrv != nil {
				grpcSrv.Stop()
			}
			return fmt.Errorf("timeline-indexer: open channel: %w", err)
		}
		workers++
		go func() {
			errCh <- runConsumer(runCtx, idx, elastic, ch, session.exchange)
		}()
	}

	select {
	case <-ctx.Done():
		cancel()
		if grpcSrv != nil {
			stopped := make(chan struct{})
			go func() {
				grpcSrv.GracefulStop()
				close(stopped)
			}()
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				grpcSrv.Stop()
			}
		}
		if amqpCleanup != nil {
			amqpCleanup()
		}
		for i := 0; i < workers; i++ {
			<-errCh
		}
		return nil
	case err := <-errCh:
		cancel()
		if grpcSrv != nil {
			grpcSrv.Stop()
		}
		if amqpCleanup != nil {
			amqpCleanup()
		}
		for i := 1; i < workers; i++ {
			<-errCh
		}
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
}

type amqpSession struct {
	conn     *amqp.Connection
	exchange string
}

func dialAMQP(ctx context.Context, url string) (*amqpSession, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("timeline-indexer: broker setup: %w", err)
	}
	type dialResult struct {
		conn *amqp.Connection
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		conn, err := amqp.DialConfig(url, amqp.Config{
			Properties: amqp.Table{"connection_name": "timeline-indexer"},
			Dial:       amqp.DefaultDial(amqpDialTimeout),
		})
		ch <- dialResult{conn: conn, err: err}
	}()

	var conn *amqp.Connection
	select {
	case <-ctx.Done():
		go func() {
			r := <-ch
			if r.conn != nil {
				_ = r.conn.Close()
			}
		}()
		return nil, nil, fmt.Errorf("timeline-indexer: dial broker: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return nil, nil, fmt.Errorf("timeline-indexer: dial broker: %w", r.err)
		}
		conn = r.conn
	}

	setupCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("timeline-indexer: open channel: %w", err)
	}
	exchange := "advisor.events"
	if err := setupCh.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		_ = setupCh.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("timeline-indexer: declare exchange: %w", err)
	}
	_ = setupCh.Close()
	cleanup := func() { _ = conn.Close() }
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("timeline-indexer: broker setup: %w", err)
	}
	return &amqpSession{conn: conn, exchange: exchange}, cleanup, nil
}

func runConsumer(ctx context.Context, idx *timeline.Index, elastic *timeline.ElasticStore, ch *amqp.Channel, exchange string) error {
	const queueName = "timeline.indexer"
	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("timeline-indexer: declare queue: %w", err)
	}
	for _, key := range timeline.RoutingKeys() {
		if err := ch.QueueBind(queueName, key, exchange, false, nil); err != nil {
			return fmt.Errorf("timeline-indexer: bind %s: %w", key, err)
		}
	}
	deliveries, err := ch.Consume(queueName, "timeline-indexer", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("timeline-indexer: consume: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("timeline-indexer: deliveries channel closed")
			}
			applied, entry, err := idx.ApplyDelivery(ctx, d.RoutingKey, d.Body)
			if err != nil {
				_ = d.Nack(false, false)
				continue
			}
			if applied && elastic != nil {
				if err := elastic.IndexDoc(ctx, entry); err != nil {
					idx.Forget(entry.EventID)
					_ = d.Nack(false, true)
					continue
				}
			}
			_ = d.Ack(false)
		}
	}
}
