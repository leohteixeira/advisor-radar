package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/leohteixeira/advisor-radar/internal/cases"
)

const amqpDialTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("service started", "service", "cases")

	if err := run(ctx, logger); err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	dsn := os.Getenv("CASES_DATABASE_URL")
	if dsn == "" {
		// No database in CI process tests; wait for signal only.
		<-ctx.Done()
		return nil
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("cases: connect database: %w", err)
	}
	defer pool.Close()

	store := cases.NewPGXStore(pool)
	if err := store.EnsureSchema(ctx); err != nil {
		return err
	}
	if err := cases.RaiseSeed(ctx, store); err != nil {
		return fmt.Errorf("cases: raise seed: %w", err)
	}
	logger.Info("seed cases raised", "service", "cases")

	brokerURL := os.Getenv("CASES_BROKER_URL")
	if brokerURL == "" {
		<-ctx.Done()
		return nil
	}

	session, cleanup, err := dialAMQP(ctx, brokerURL)
	if err != nil {
		return err
	}
	defer cleanup()

	pubCh, err := session.conn.Channel()
	if err != nil {
		return fmt.Errorf("cases: open publish channel: %w", err)
	}
	defer pubCh.Close()

	consumeCh, err := session.conn.Channel()
	if err != nil {
		return fmt.Errorf("cases: open consume channel: %w", err)
	}
	defer consumeCh.Close()

	if err := declareSLATopology(consumeCh, session.exchange); err != nil {
		return err
	}

	publisher := &amqpPublisher{ch: pubCh, exchange: session.exchange}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	go func() {
		errCh <- cases.RunPublisher(runCtx, store, publisher, time.Second)
	}()
	go func() {
		errCh <- runBreachConsumer(runCtx, store, consumeCh)
	}()

	select {
	case <-ctx.Done():
		cancel()
		<-errCh
		<-errCh
		return nil
	case err := <-errCh:
		cancel()
		<-errCh
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

func dialAMQP(ctx context.Context, url string) (*amqpSession, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("cases: broker setup: %w", err)
	}

	type dialResult struct {
		conn *amqp.Connection
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		conn, err := amqp.DialConfig(url, amqp.Config{
			Properties: amqp.Table{"connection_name": "cases"},
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
		return nil, nil, fmt.Errorf("cases: dial broker: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return nil, nil, fmt.Errorf("cases: dial broker: %w", r.err)
		}
		conn = r.conn
	}

	setupCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("cases: open channel: %w", err)
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
		return nil, nil, fmt.Errorf("cases: declare exchange: %w", err)
	}
	_ = setupCh.Close()

	cleanup := func() {
		_ = conn.Close()
	}

	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("cases: broker setup: %w", err)
	}

	return &amqpSession{conn: conn, exchange: exchange}, cleanup, nil
}

func declareSLATopology(ch *amqp.Channel, exchange string) error {
	const delayQueue = "cases.sla.delay"
	const breachQueue = "cases.sla.breached"

	// Per-message TTL is set on publish via SLADelayArgs; the delay queue
	// only wires the dead-letter path using the same routing key helper.
	delayArgs := cases.SLADelayArgs(60)
	if _, err := ch.QueueDeclare(
		delayQueue,
		true,
		false,
		false,
		false,
		amqp.Table{
			"x-dead-letter-exchange":    exchange,
			"x-dead-letter-routing-key": delayArgs.DeadLetterRoutingKey,
		},
	); err != nil {
		return fmt.Errorf("cases: declare delay queue: %w", err)
	}

	if _, err := ch.QueueDeclare(breachQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("cases: declare breach queue: %w", err)
	}
	if err := ch.QueueBind(
		breachQueue,
		delayArgs.DeadLetterRoutingKey,
		exchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("cases: bind breach queue: %w", err)
	}
	return nil
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
		return fmt.Errorf("cases: broker publish: %w", err)
	}
	return nil
}

func (b *amqpPublisher) PublishDelay(ctx context.Context, caseID string, ttlMs int, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := b.ch.PublishWithContext(
		ctx,
		"",
		"cases.sla.delay",
		false,
		false,
		amqp.Publishing{
			ContentType:  "text/plain",
			DeliveryMode: amqp.Persistent,
			Expiration:   strconv.Itoa(ttlMs),
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("cases: broker delay %s: %w", caseID, err)
	}
	return nil
}

func runBreachConsumer(ctx context.Context, store *cases.PGXStore, ch *amqp.Channel) error {
	const queueName = "cases.sla.breached"

	deliveries, err := ch.Consume(queueName, "cases", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("cases: consume: %w", err)
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
				return fmt.Errorf("cases: deliveries channel closed")
			}
			if err := cases.ApplyBreachDelivery(ctx, store, d.Body); err != nil {
				requeue := true
				var perm cases.PermanentDeliveryError
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
