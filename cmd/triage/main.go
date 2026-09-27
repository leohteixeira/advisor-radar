package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"

	triagev1 "github.com/leohteixeira/advisor-radar/gen/triage/v1"
	"github.com/leohteixeira/advisor-radar/internal/envfile"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/jev"
	"github.com/leohteixeira/advisor-radar/internal/resilience"
	"github.com/leohteixeira/advisor-radar/internal/triage"
	"github.com/leohteixeira/advisor-radar/internal/triagepipe"
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

	logger.Info("service started", "service", "triage")

	if err := run(ctx, logger); err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	dsn := os.Getenv("TRIAGE_DATABASE_URL")
	grpcAddr := os.Getenv("TRIAGE_GRPC_ADDR")
	if dsn == "" {
		// No database in CI process tests; wait for signal only.
		<-ctx.Done()
		return nil
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("triage: connect database: %w", err)
	}
	defer pool.Close()

	store := triagepipe.NewPGXStore(pool)
	reviewer := triagepipe.NewReviewReader(pool)

	brokerURL := os.Getenv("TRIAGE_BROKER_URL")
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 3)
	workers := 0
	var grpcSrv *grpc.Server

	if grpcAddr != "" {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			return fmt.Errorf("triage: grpc listen: %w", err)
		}
		grpcSrv = grpc.NewServer()
		triagev1.RegisterTriageServiceServer(grpcSrv, triagepipe.NewGRPCServer(reviewer))
		workers++
		go func() {
			logger.Info("grpc listening", "service", "triage", "addr", grpcAddr)
			err := grpcSrv.Serve(lis)
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				errCh <- fmt.Errorf("triage: grpc serve: %w", err)
				return
			}
			errCh <- nil
		}()
	}

	if brokerURL == "" {
		if workers == 0 {
			<-ctx.Done()
			return nil
		}
		<-ctx.Done()
		cancel()
		if grpcSrv != nil {
			grpcSrv.GracefulStop()
		}
		for i := 0; i < workers; i++ {
			<-errCh
		}
		return nil
	}

	key := os.Getenv("AI_GATEWAY_API_KEY")
	classifier := buildClassifier(key, logger)

	session, cleanup, err := dialAMQP(ctx, brokerURL)
	if err != nil {
		cancel()
		if grpcSrv != nil {
			grpcSrv.Stop()
		}
		return err
	}
	defer cleanup()

	pubCh, err := session.conn.Channel()
	if err != nil {
		return fmt.Errorf("triage: open publish channel: %w", err)
	}
	defer pubCh.Close()

	consumeCh, err := session.conn.Channel()
	if err != nil {
		return fmt.Errorf("triage: open consume channel: %w", err)
	}
	defer consumeCh.Close()

	prefetch := envInt("TRIAGE_PREFETCH", 10)
	if err := consumeCh.Qos(prefetch, 0, false); err != nil {
		return fmt.Errorf("triage: qos: %w", err)
	}

	publisher := &amqpPublisher{ch: pubCh, exchange: session.exchange}

	workers += 2
	go func() {
		errCh <- triagepipe.RunPublisher(runCtx, store, publisher, time.Second)
	}()
	go func() {
		errCh <- runConsumer(runCtx, store, classifier, consumeCh, session.exchange)
	}()

	select {
	case <-ctx.Done():
		cancel()
		if grpcSrv != nil {
			grpcSrv.GracefulStop()
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
		for i := 1; i < workers; i++ {
			<-errCh
		}
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
}

func buildClassifier(apiKey string, logger *slog.Logger) triage.Classifier {
	prefetch := envInt("TRIAGE_PREFETCH", 10)
	rps := envFloat("TRIAGE_RATE_LIMIT", 5)
	primary := triage.NewJevClassifier(jev.New(apiKey, jev.WithZeroDataRetention()))
	return resilience.New(primary, triage.HeuristicClassifier{}, resilience.Config{
		Bulkhead: prefetch,
		Limiter:  rate.NewLimiter(rate.Limit(rps), max(1, prefetch)),
		Logger:   logger,
		OnDegrade: func(_ triage.Message, err error) {
			logger.Warn("triage degraded to heuristic", "error", err.Error())
		},
	})
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

type amqpSession struct {
	conn     *amqp.Connection
	exchange string
}

type amqpPublisher struct {
	ch       *amqp.Channel
	exchange string
}

type permanentDeliveryError struct {
	err error
}

func (e permanentDeliveryError) Error() string { return e.err.Error() }
func (e permanentDeliveryError) Unwrap() error { return e.err }

func dialAMQP(ctx context.Context, url string) (*amqpSession, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("triage: broker setup: %w", err)
	}

	type dialResult struct {
		conn *amqp.Connection
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		conn, err := amqp.DialConfig(url, amqp.Config{
			Properties: amqp.Table{"connection_name": "triage"},
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
		return nil, nil, fmt.Errorf("triage: dial broker: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return nil, nil, fmt.Errorf("triage: dial broker: %w", r.err)
		}
		conn = r.conn
	}

	setupCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("triage: open channel: %w", err)
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
		return nil, nil, fmt.Errorf("triage: declare exchange: %w", err)
	}
	_ = setupCh.Close()

	cleanup := func() {
		_ = conn.Close()
	}

	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("triage: broker setup: %w", err)
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
		return fmt.Errorf("triage: broker publish: %w", err)
	}
	return nil
}

func runConsumer(
	ctx context.Context,
	store *triagepipe.PGXStore,
	classifier triage.Classifier,
	ch *amqp.Channel,
	exchange string,
) error {
	const queueName = "triage.message.received"

	_, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("triage: declare queue: %w", err)
	}
	if err := ch.QueueBind(
		queueName,
		event.NameMessageReceived,
		exchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("triage: bind queue: %w", err)
	}

	deliveries, err := ch.Consume(queueName, "triage", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("triage: consume: %w", err)
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
				return fmt.Errorf("triage: deliveries channel closed")
			}
			if err := handleDelivery(ctx, store, classifier, d.Body); err != nil {
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

func handleDelivery(ctx context.Context, store *triagepipe.PGXStore, classifier triage.Classifier, body []byte) error {
	var raw struct {
		EventID       string          `json:"event_id"`
		OccurredAt    time.Time       `json:"occurred_at"`
		CustomerID    string          `json:"customer_id"`
		SchemaVersion int             `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return permanentDeliveryError{err: fmt.Errorf("triage: decode delivery: %w", err)}
	}

	if len(raw.Payload) == 0 {
		return permanentDeliveryError{err: fmt.Errorf("triage: payload is required")}
	}
	var payload any
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		return permanentDeliveryError{err: fmt.Errorf("triage: decode payload: %w", err)}
	}

	env := event.Envelope{
		Name:          event.NameMessageReceived,
		EventID:       raw.EventID,
		OccurredAt:    raw.OccurredAt,
		CustomerID:    raw.CustomerID,
		SchemaVersion: raw.SchemaVersion,
		Payload:       payload,
	}
	if err := env.Validate(); err != nil {
		return permanentDeliveryError{err: fmt.Errorf("triage: validate delivery: %w", err)}
	}
	if err := triagepipe.Apply(ctx, store, classifier, env); err != nil {
		return err
	}
	return nil
}
