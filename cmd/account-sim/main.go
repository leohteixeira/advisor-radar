package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/leohteixeira/advisor-radar/internal/outbox"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("service started", "service", "account-sim")

	if err := run(ctx, logger); err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	dsn := os.Getenv("ACCOUNT_SIM_DATABASE_URL")
	if dsn == "" {
		// No database in CI process tests; wait for signal only.
		<-ctx.Done()
		return nil
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("account-sim: connect database: %w", err)
	}
	defer pool.Close()

	store := outbox.NewPGXStore(pool)
	if err := store.EnsureSchema(ctx); err != nil {
		return err
	}
	if err := outbox.Fire(ctx, store); err != nil {
		return fmt.Errorf("account-sim: fire: %w", err)
	}
	logger.Info("market day fired", "service", "account-sim")

	brokerURL := os.Getenv("ACCOUNT_SIM_BROKER_URL")
	if brokerURL == "" {
		<-ctx.Done()
		return nil
	}

	broker, cleanup, err := newAMQPBroker(ctx, brokerURL)
	if err != nil {
		return err
	}
	defer cleanup()

	return outbox.RunPublisher(ctx, store, broker, time.Second)
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
