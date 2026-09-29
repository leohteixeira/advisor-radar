package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	advisoryv1 "github.com/leohteixeira/advisor-radar/gen/advisory/v1"
	"github.com/leohteixeira/advisor-radar/internal/advisory"
	"github.com/leohteixeira/advisor-radar/internal/envfile"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/sim"
	"github.com/leohteixeira/advisor-radar/internal/telemetry"
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

	logger.Info("service started", "service", "advisory")

	shutdown, err := telemetry.Setup(ctx, "advisory")
	if err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}
	runErr := run(ctx, logger)
	if err := telemetry.Stop(shutdown); err != nil {
		logger.Warn("telemetry shutdown failed", "service", "advisory", "error", err.Error())
	}
	if runErr != nil {
		logger.Error("service failed", "error", runErr.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	dsn := os.Getenv("ADVISORY_DATABASE_URL")
	httpAddr := os.Getenv("ADVISORY_HTTP_ADDR")
	grpcAddr := os.Getenv("ADVISORY_GRPC_ADDR")
	brokerURL := os.Getenv("ADVISORY_BROKER_URL")
	accountTarget := os.Getenv("ACCOUNT_SIM_GRPC_TARGET")

	var actionStore advisory.ActionStore = advisory.NewMemoryActionStore()
	var store *advisory.PGXStore
	var pool *pgxpool.Pool
	var reader *advisory.BookReader

	if dsn != "" {
		var err error
		pool, err = pgxpool.New(ctx, dsn)
		if err != nil {
			return fmt.Errorf("advisory: connect database: %w", err)
		}
		defer pool.Close()

		store = advisory.NewPGXStore(pool)
		actionStore = store
		reader = advisory.NewBookReader(pool)
	}

	if dsn == "" && httpAddr == "" && grpcAddr == "" {
		<-ctx.Done()
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 4)
	workers := 0
	var httpSrv *http.Server
	var grpcSrv *grpc.Server
	var amqpCleanup func()

	if httpAddr != "" {
		actions := advisory.NewActions(actionStore, nil)
		httpSrv = &http.Server{
			Addr:              httpAddr,
			Handler:           advisory.NewActionsHandler(actions),
			ReadHeaderTimeout: 5 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return runCtx },
		}
		workers++
		go func() {
			logger.Info("http listening", "service", "advisory", "addr", httpAddr)
			err := httpSrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("advisory: listen: %w", err)
				return
			}
			errCh <- nil
		}()
	}

	if grpcAddr != "" && reader != nil {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			cancel()
			return fmt.Errorf("advisory: grpc listen: %w", err)
		}
		var opts []advisory.ServerOption
		if accountTarget != "" {
			conn, err := grpc.NewClient(accountTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), telemetry.GRPCClientOption())
			if err != nil {
				_ = lis.Close()
				cancel()
				if httpSrv != nil {
					_ = httpSrv.Close()
				}
				return fmt.Errorf("advisory: account-sim client: %w", err)
			}
			defer func() { _ = conn.Close() }()
			opts = append(opts, advisory.WithAccountReader(advisory.NewAccountSim(accountv1.NewAccountServiceClient(conn))))
		} else {
			logger.Warn("moment facts unavailable: ACCOUNT_SIM_GRPC_TARGET is not set", "service", "advisory")
		}
		grpcSrv = grpc.NewServer(telemetry.GRPCServerOption())
		advisoryv1.RegisterAdvisoryServiceServer(grpcSrv, advisory.NewGRPCServer(reader, opts...))
		workers++
		go func() {
			logger.Info("grpc listening", "service", "advisory", "addr", grpcAddr)
			err := grpcSrv.Serve(lis)
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				errCh <- fmt.Errorf("advisory: grpc serve: %w", err)
				return
			}
			errCh <- nil
		}()
	}

	if store != nil && brokerURL != "" {
		session, cleanup, err := dialAMQP(ctx, brokerURL)
		if err != nil {
			cancel()
			if httpSrv != nil {
				_ = httpSrv.Close()
			}
			return err
		}
		amqpCleanup = cleanup

		pubCh, err := session.conn.Channel()
		if err != nil {
			cleanup()
			cancel()
			if httpSrv != nil {
				_ = httpSrv.Close()
			}
			return fmt.Errorf("advisory: open publish channel: %w", err)
		}
		consumeCh, err := session.conn.Channel()
		if err != nil {
			_ = pubCh.Close()
			cleanup()
			cancel()
			if httpSrv != nil {
				_ = httpSrv.Close()
			}
			return fmt.Errorf("advisory: open consume channel: %w", err)
		}

		publisher := &amqpPublisher{ch: pubCh, exchange: session.exchange}
		workers += 2
		go func() {
			errCh <- advisory.RunPublisher(runCtx, store, publisher, time.Second)
		}()
		go func() {
			errCh <- runConsumer(runCtx, store, consumeCh, session.exchange)
		}()
	}

	if workers == 0 {
		<-ctx.Done()
		return nil
	}

	select {
	case <-ctx.Done():
		cancel()
		if httpSrv != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = httpSrv.Shutdown(shutdownCtx)
		}
		if grpcSrv != nil {
			grpcSrv.GracefulStop()
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
		if httpSrv != nil {
			_ = httpSrv.Close()
		}
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

type amqpPublisher struct {
	ch       *amqp.Channel
	exchange string
}

// permanentDeliveryError marks decode/validate failures that must not requeue.
type permanentDeliveryError struct {
	err error
}

func (e permanentDeliveryError) Error() string { return e.err.Error() }
func (e permanentDeliveryError) Unwrap() error { return e.err }

func dialAMQP(ctx context.Context, url string) (*amqpSession, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("advisory: broker setup: %w", err)
	}

	type dialResult struct {
		conn *amqp.Connection
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		conn, err := amqp.DialConfig(url, amqp.Config{
			Properties: amqp.Table{"connection_name": "advisory"},
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
		return nil, nil, fmt.Errorf("advisory: dial broker: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return nil, nil, fmt.Errorf("advisory: dial broker: %w", r.err)
		}
		conn = r.conn
	}

	setupCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("advisory: open channel: %w", err)
	}

	exchange := "advisor.events"
	if err := setupCh.ExchangeDeclare(
		exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		_ = setupCh.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("advisory: declare exchange: %w", err)
	}
	_ = setupCh.Close()

	cleanup := func() {
		_ = conn.Close()
	}

	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("advisory: broker setup: %w", err)
	}

	return &amqpSession{conn: conn, exchange: exchange}, cleanup, nil
}

func (b *amqpPublisher) Publish(ctx context.Context, routingKey string, body []byte) error {
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
		return fmt.Errorf("advisory: broker publish: %w", err)
	}
	return nil
}

func runConsumer(ctx context.Context, store *advisory.PGXStore, ch *amqp.Channel, exchange string) error {
	const queueName = "advisory.account.event.recorded"

	_, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("advisory: declare queue: %w", err)
	}
	if err := ch.QueueBind(
		queueName,
		event.NameAccountEventRecorded,
		exchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("advisory: bind queue: %w", err)
	}

	deliveries, err := ch.Consume(queueName, "advisory", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("advisory: consume: %w", err)
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
				return fmt.Errorf("advisory: deliveries channel closed")
			}
			if err := handleDelivery(ctx, store, d.Body); err != nil {
				requeue := true
				var perm permanentDeliveryError
				if errors.As(err, &perm) {
					requeue = false
				}
				_ = d.Nack(false, requeue)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

func handleDelivery(ctx context.Context, store *advisory.PGXStore, body []byte) error {
	var raw struct {
		EventID       string          `json:"event_id"`
		OccurredAt    time.Time       `json:"occurred_at"`
		CustomerID    string          `json:"customer_id"`
		SchemaVersion int             `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return permanentDeliveryError{err: fmt.Errorf("advisory: decode delivery: %w", err)}
	}

	var payload any
	if len(raw.Payload) > 0 {
		if err := json.Unmarshal(raw.Payload, &payload); err != nil {
			return permanentDeliveryError{err: fmt.Errorf("advisory: decode payload: %w", err)}
		}
	}

	env := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       raw.EventID,
		OccurredAt:    raw.OccurredAt,
		CustomerID:    raw.CustomerID,
		SchemaVersion: raw.SchemaVersion,
		Payload:       payload,
	}
	if err := env.Validate(); err != nil {
		return permanentDeliveryError{err: fmt.Errorf("advisory: validate delivery: %w", err)}
	}
	if err := advisory.Apply(ctx, store, env); err != nil {
		return classifyApplyError(err)
	}
	return nil
}

// classifyApplyError marks failures that redelivery cannot fix as permanent so
// they do not requeue: a money-scale error, a customer outside the book, an
// unknown investor profile, and an invalid purchase.
func classifyApplyError(err error) error {
	switch {
	case errors.Is(err, sim.ErrMoneyScale),
		errors.Is(err, advisory.ErrUnknownCustomer),
		errors.Is(err, advisory.ErrUnknownProfile),
		errors.Is(err, advisory.ErrInvalidPurchase):
		return permanentDeliveryError{err: err}
	}
	return err
}
