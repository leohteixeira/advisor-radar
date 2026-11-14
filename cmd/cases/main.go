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
	"google.golang.org/grpc/credentials/insecure"

	advisoryv1 "github.com/leohteixeira/advisor-radar/gen/advisory/v1"
	casesv1 "github.com/leohteixeira/advisor-radar/gen/cases/v1"
	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/envfile"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/telemetry"
)

const amqpDialTimeout = 10 * time.Second

// intakeQueue is one event queue cases consumes with the intake policy:
// prefetch 1, transient errors retried with backoff, and its own dead-letter
// queue. apply handles one delivery body.
type intakeQueue struct {
	name       string
	dlq        string
	routingKey string
	consumer   string
	apply      func(ctx context.Context, store cases.IntakeStore, lookup cases.CustomerLookup, body []byte) (cases.IntakeResult, error)
}

// Triaged intake queue and its dead-letter queue.
const (
	triagedQueue = "cases.message.triaged"
	triagedDLQ   = "cases.message.triaged.dlq"
)

// intakeQueues are the three event queues: triaged messages open or join
// cases, the advisor's open-case command does the same for a queue signal
// (ADR 0013), and alert.raised only recalculates the SLA of an open case
// after a segment change (ADR 0012).
var intakeQueues = []intakeQueue{
	{
		name: triagedQueue, dlq: triagedDLQ,
		routingKey: event.NameMessageTriaged, consumer: "cases-intake", apply: cases.Intake,
	},
	{
		name: "cases.case.open.requested", dlq: "cases.case.open.requested.dlq",
		routingKey: event.NameCaseOpenRequested, consumer: "cases-open-command", apply: cases.OpenRequested,
	},
	{
		name: "cases.alert.raised", dlq: "cases.alert.raised.dlq",
		routingKey: event.NameAlertRaised, consumer: "cases-resegment", apply: cases.Resegment,
	},
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := envfile.Load(".env"); err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("service started", "service", "cases")

	shutdown, err := telemetry.Setup(ctx, "cases")
	if err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}
	runErr := run(ctx, logger)
	if err := telemetry.Stop(shutdown); err != nil {
		logger.Warn("telemetry shutdown failed", "service", "cases", "error", err.Error())
	}
	if runErr != nil {
		logger.Error("service failed", "error", runErr.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	dsn := os.Getenv("CASES_DATABASE_URL")
	grpcAddr := os.Getenv("CASES_GRPC_ADDR")
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
	reader := cases.NewCaseReader(pool)

	brokerURL := os.Getenv("CASES_BROKER_URL")
	advisoryTarget := os.Getenv("ADVISORY_GRPC_TARGET")
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 3+len(intakeQueues))
	workers := 0
	var grpcSrv *grpc.Server
	var amqpCleanup func()

	if grpcAddr != "" {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			return fmt.Errorf("cases: grpc listen: %w", err)
		}
		grpcSrv = grpc.NewServer(telemetry.GRPCServerOption())
		casesv1.RegisterCasesServiceServer(grpcSrv, cases.NewGRPCServer(reader, store))
		workers++
		go func() {
			logger.Info("grpc listening", "service", "cases", "addr", grpcAddr)
			err := grpcSrv.Serve(lis)
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				errCh <- fmt.Errorf("cases: grpc serve: %w", err)
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
			return err
		}
	}

	session, cleanup, err := dialAMQP(ctx, brokerURL)
	if err != nil {
		cancel()
		if grpcSrv != nil {
			grpcSrv.Stop()
		}
		return err
	}
	amqpCleanup = cleanup
	defer cleanup()

	// abort stops the gRPC server and drains it when broker setup fails, so
	// no worker outlives run.
	abort := func(err error) error {
		cancel()
		if grpcSrv != nil {
			grpcSrv.Stop()
		}
		for i := 0; i < workers; i++ {
			<-errCh
		}
		return err
	}

	pubCh, err := session.conn.Channel()
	if err != nil {
		return abort(fmt.Errorf("cases: open publish channel: %w", err))
	}
	defer pubCh.Close()

	consumeCh, err := session.conn.Channel()
	if err != nil {
		return abort(fmt.Errorf("cases: open consume channel: %w", err))
	}
	defer consumeCh.Close()

	if err := declareSLATopology(consumeCh, session.exchange); err != nil {
		return abort(err)
	}

	// Intake setup completes before any broker worker starts.
	var intakeChs []*amqp.Channel
	var lookup cases.CustomerLookup
	if advisoryTarget == "" {
		logger.Warn("event intake disabled: ADVISORY_GRPC_TARGET is not set", "service", "cases")
	} else {
		advisoryConn, err := grpc.NewClient(advisoryTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), telemetry.GRPCClientOption())
		if err != nil {
			return abort(fmt.Errorf("cases: dial advisory: %w", err))
		}
		// Closed when run returns, after every worker (the intake included) exited.
		defer advisoryConn.Close()

		for _, q := range intakeQueues {
			ch, err := session.conn.Channel()
			if err != nil {
				return abort(fmt.Errorf("cases: open intake channel: %w", err))
			}
			defer ch.Close()
			if err := declareIntakeTopology(ch, session.exchange, q); err != nil {
				return abort(err)
			}
			intakeChs = append(intakeChs, ch)
		}
		lookup = cases.NewAdvisoryLookup(advisoryv1.NewAdvisoryServiceClient(advisoryConn))
	}

	publisher := &amqpPublisher{ch: pubCh, exchange: session.exchange, delayQueues: map[int]struct{}{}}

	workers += 2
	go func() {
		errCh <- cases.RunPublisher(runCtx, store, publisher, time.Second)
	}()
	go func() {
		errCh <- runBreachConsumer(runCtx, store, consumeCh)
	}()
	for i, ch := range intakeChs {
		workers++
		go func() {
			errCh <- runIntakeConsumer(runCtx, logger, store, lookup, ch, intakeQueues[i])
		}()
	}

	select {
	case <-ctx.Done():
		cancel()
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

type amqpSession struct {
	conn     *amqp.Connection
	exchange string
}

// amqpPublisher publishes events and SLA delays on one channel. It is used
// by the single publisher goroutine, so delayQueues needs no lock.
type amqpPublisher struct {
	ch       *amqp.Channel
	exchange string
	// delayQueues are the TTLs whose delay queue this run already declared.
	delayQueues map[int]struct{}
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

// topologyChannel is the part of *amqp.Channel the topology declarations use.
type topologyChannel interface {
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
}

// declareSLATopology declares the breach queue and the legacy delay queue.
// The legacy queue is kept, with its original arguments, only so the delays a
// previous version published into it keep dead-lettering; new delays go to
// the per-TTL queues declareDelayQueue creates (ADR 0012).
func declareSLATopology(ch topologyChannel, exchange string) error {
	const breachQueue = "cases.sla.breached"

	delayArgs := cases.SLADelayArgs(60)
	if _, err := ch.QueueDeclare(
		cases.LegacyDelayQueue,
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

// declareDelayQueue declares the delay queue of one TTL. The TTL is on the
// queue, not on the message, so the queue is in expiry order and every
// message dead-letters to exchange as case.sla.breached when its own TTL
// ends.
func declareDelayQueue(ch topologyChannel, exchange string, ttlMs int) error {
	if _, err := ch.QueueDeclare(
		cases.DelayQueue(ttlMs),
		true,
		false,
		false,
		false,
		amqp.Table{
			"x-message-ttl":             int64(ttlMs),
			"x-dead-letter-exchange":    exchange,
			"x-dead-letter-routing-key": cases.SLADelayArgs(0).DeadLetterRoutingKey,
		},
	); err != nil {
		return fmt.Errorf("cases: declare delay queue %d ms: %w", ttlMs, err)
	}
	return nil
}

// PublishDelay sends one SLA delay to the queue of its TTL, declaring the
// queue the first time this run uses that TTL.
func (b *amqpPublisher) PublishDelay(ctx context.Context, caseID string, ttlMs int, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, declared := b.delayQueues[ttlMs]; !declared {
		if err := declareDelayQueue(b.ch, b.exchange, ttlMs); err != nil {
			return err
		}
		b.delayQueues[ttlMs] = struct{}{}
	}
	err := b.ch.PublishWithContext(
		ctx,
		"",
		cases.DelayQueue(ttlMs),
		false,
		false,
		amqp.Publishing{
			ContentType:  "text/plain",
			DeliveryMode: amqp.Persistent,
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

// declareIntakeTopology declares one intake queue bound to its event and its
// own dead-letter queue through the default exchange.
func declareIntakeTopology(ch topologyChannel, exchange string, q intakeQueue) error {
	if _, err := ch.QueueDeclare(q.dlq, true, false, false, false, nil); err != nil {
		return fmt.Errorf("cases: declare %s: %w", q.dlq, err)
	}
	if _, err := ch.QueueDeclare(
		q.name,
		true,
		false,
		false,
		false,
		amqp.Table{
			"x-dead-letter-exchange":    "",
			"x-dead-letter-routing-key": q.dlq,
		},
	); err != nil {
		return fmt.Errorf("cases: declare %s: %w", q.name, err)
	}
	if err := ch.QueueBind(q.name, q.routingKey, exchange, false, nil); err != nil {
		return fmt.Errorf("cases: bind %s: %w", q.name, err)
	}
	return nil
}

// runIntakeConsumer applies the deliveries of one intake queue one at a time
// (prefetch 1, one goroutine) until ctx is canceled.
func runIntakeConsumer(
	ctx context.Context,
	logger *slog.Logger,
	store *cases.PGXStore,
	lookup cases.CustomerLookup,
	ch *amqp.Channel,
	q intakeQueue,
) error {
	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("cases: %s qos: %w", q.name, err)
	}
	deliveries, err := ch.Consume(q.name, q.consumer, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("cases: consume %s: %w", q.name, err)
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
				return fmt.Errorf("cases: %s deliveries channel closed", q.name)
			}
			handleIntake(ctx, logger, store, lookup, q, d)
		}
	}
}

// handleIntake retries transient errors with backoff, then acks, requeues on
// shutdown, or dead-letters. Logs carry ids and the decision, never text.
func handleIntake(
	ctx context.Context,
	logger *slog.Logger,
	store *cases.PGXStore,
	lookup cases.CustomerLookup,
	q intakeQueue,
	d amqp.Delivery,
) {
	var res cases.IntakeResult
	err := cases.RetryTransient(ctx, cases.IntakeAttempts, cases.IntakeBaseBackoff, func(ctx context.Context) error {
		var err error
		res, err = q.apply(ctx, store, lookup, d.Body)
		return err
	})
	ack, requeue := triagedOutcome(err, ctx.Err())
	switch {
	case ack:
		logger.Info("event applied",
			"service", "cases",
			"queue", q.name,
			"event_id", res.EventID,
			"customer_id", res.CustomerID,
			"case_id", res.CaseID,
			"decision", string(res.Decision),
		)
		if ackErr := d.Ack(false); ackErr != nil {
			logger.Warn("event ack failed", "service", "cases", "queue", q.name, "event_id", res.EventID, "error", ackErr.Error())
		}
		return
	case !requeue:
		var perm cases.PermanentDeliveryError
		logger.Warn("event dead-lettered",
			"service", "cases",
			"queue", q.name,
			"event_id", res.EventID,
			"customer_id", res.CustomerID,
			"permanent", errors.As(err, &perm),
			"error", err.Error(),
		)
	}
	if nackErr := d.Nack(false, requeue); nackErr != nil {
		logger.Warn("event nack failed",
			"service", "cases", "queue", q.name, "event_id", res.EventID, "requeue", requeue, "error", nackErr.Error())
	}
}

// triagedOutcome maps the intake result onto the delivery settlement. A
// success acks. On shutdown (ctxErr set) nothing was committed, so the message
// is requeued for another run. Any other failure, permanent or transient after
// the retries, goes to the dead-letter queue.
func triagedOutcome(err, ctxErr error) (ack, requeue bool) {
	switch {
	case err == nil:
		return true, false
	case ctxErr != nil:
		return false, true
	default:
		return false, false
	}
}
