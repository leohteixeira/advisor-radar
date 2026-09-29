package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	advisoryv1 "github.com/leohteixeira/advisor-radar/gen/advisory/v1"
	casesv1 "github.com/leohteixeira/advisor-radar/gen/cases/v1"
	triagev1 "github.com/leohteixeira/advisor-radar/gen/triage/v1"
	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/envfile"
	"github.com/leohteixeira/advisor-radar/internal/event"
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

	logger.Info("service started", "service", "bff")

	shutdown, err := telemetry.Setup(ctx, "bff")
	if err != nil {
		logger.Error("service failed", "error", err.Error())
		os.Exit(1)
	}
	runErr := run(ctx, logger)
	if err := telemetry.Stop(shutdown); err != nil {
		logger.Warn("telemetry shutdown failed", "service", "bff", "error", err.Error())
	}
	if runErr != nil {
		logger.Error("service failed", "error", runErr.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	board := bff.NewBoard()
	addr := os.Getenv("BFF_HTTP_ADDR")
	brokerURL := os.Getenv("BFF_BROKER_URL")
	actions := bff.NewActionsClient(os.Getenv("ADVISORY_HTTP_URL"))
	tl, tlCleanup, err := bff.NewTimelineClient(os.Getenv("TIMELINE_GRPC_TARGET"))
	if err != nil {
		return err
	}
	defer tlCleanup()

	var queue bff.QueueSource = bff.EmptyQueue{}
	var review bff.ReviewSource = bff.EmptyReview{}
	var casesSrc bff.CaseSource = bff.EmptyCases{}
	var pov bff.POVSource // nil keeps the handler's empty POV source
	var cleanups []func()
	defer func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}()

	if target := os.Getenv("ADVISORY_GRPC_TARGET"); target != "" {
		conn, err := bff.DialGRPC(target)
		if err != nil {
			return fmt.Errorf("bff: dial advisory: %w", err)
		}
		cleanups = append(cleanups, func() { _ = conn.Close() })
		queue = bff.NewGRPCQueue(advisoryv1.NewAdvisoryServiceClient(conn))
	}
	if target := os.Getenv("TRIAGE_GRPC_TARGET"); target != "" {
		conn, err := bff.DialGRPC(target)
		if err != nil {
			return fmt.Errorf("bff: dial triage: %w", err)
		}
		cleanups = append(cleanups, func() { _ = conn.Close() })
		review = bff.NewGRPCReview(triagev1.NewTriageServiceClient(conn))
	}
	if target := os.Getenv("CASES_GRPC_TARGET"); target != "" {
		conn, err := bff.DialGRPC(target)
		if err != nil {
			return fmt.Errorf("bff: dial cases: %w", err)
		}
		cleanups = append(cleanups, func() { _ = conn.Close() })
		casesSrc = bff.NewGRPCCases(casesv1.NewCasesServiceClient(conn))
	}
	if target := os.Getenv("ACCOUNT_SIM_GRPC_TARGET"); target != "" {
		conn, err := bff.DialAccountSim(target)
		if err != nil {
			return fmt.Errorf("bff: dial account-sim: %w", err)
		}
		cleanups = append(cleanups, func() { _ = conn.Close() })
		pov = bff.NewGRPCPOV(accountv1.NewAccountServiceClient(conn))
	} else {
		logger.Warn("client POV disabled: ACCOUNT_SIM_GRPC_TARGET is not set", "service", "bff")
	}

	if addr == "" && brokerURL == "" {
		<-ctx.Done()
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	var httpSrv *http.Server
	var amqpCleanup func()

	var session *amqpSession
	if brokerURL != "" {
		opened, cleanup, err := dialAMQP(ctx, brokerURL)
		if err != nil {
			return err
		}
		amqpCleanup = cleanup
		session = opened
	}

	server := bff.NewHandlerWithPOV(board, actions, tl, queue, review, casesSrc, pov, nil)
	server.SetLogger(logger)
	if addr != "" {
		httpSrv = &http.Server{
			Addr:              addr,
			Handler:           server,
			ReadHeaderTimeout: 5 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return runCtx },
		}
		go func() {
			logger.Info("http listening", "service", "bff", "addr", addr)
			err := httpSrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("bff: listen: %w", err)
				return
			}
			errCh <- nil
		}()
	}

	if session != nil {
		consumeCh, err := session.conn.Channel()
		if err != nil {
			amqpCleanup()
			cancel()
			if httpSrv != nil {
				_ = httpSrv.Close()
			}
			return fmt.Errorf("bff: open consume channel: %w", err)
		}
		go func() {
			errCh <- runConsumer(runCtx, board, server, consumeCh, session.exchange)
		}()
	}

	workers := 0
	if addr != "" {
		workers++
	}
	if brokerURL != "" {
		workers++
	}

	select {
	case <-ctx.Done():
		cancel()
		board.Drain()
		if httpSrv != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = httpSrv.Shutdown(shutdownCtx)
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
		board.Drain()
		if httpSrv != nil {
			_ = httpSrv.Close()
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
		return nil, nil, fmt.Errorf("bff: broker setup: %w", err)
	}

	type dialResult struct {
		conn *amqp.Connection
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		conn, err := amqp.DialConfig(url, amqp.Config{
			Properties: amqp.Table{"connection_name": "bff"},
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
		return nil, nil, fmt.Errorf("bff: dial broker: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return nil, nil, fmt.Errorf("bff: dial broker: %w", r.err)
		}
		conn = r.conn
	}

	setupCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("bff: open channel: %w", err)
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
		return nil, nil, fmt.Errorf("bff: declare exchange: %w", err)
	}
	_ = setupCh.Close()

	cleanup := func() {
		_ = conn.Close()
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("bff: broker setup: %w", err)
	}
	return &amqpSession{conn: conn, exchange: exchange}, cleanup, nil
}

func runConsumer(ctx context.Context, board *bff.Board, server *bff.Server, ch *amqp.Channel, exchange string) error {
	const queueName = "bff.queue.signals"

	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		return fmt.Errorf("bff: declare queue: %w", err)
	}
	for _, key := range []string{
		event.NameAlertRaised,
		event.NameMessageTriaged,
		event.NameAccountEventRecorded,
		event.NameMessageReceived,
	} {
		if err := ch.QueueBind(queueName, key, exchange, false, nil); err != nil {
			return fmt.Errorf("bff: bind %s: %w", key, err)
		}
	}

	deliveries, err := ch.Consume(queueName, "bff", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("bff: consume: %w", err)
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
				return fmt.Errorf("bff: deliveries channel closed")
			}
			server.ObservePOV(d.RoutingKey, d.Body)
			if d.RoutingKey == event.NameAlertRaised || d.RoutingKey == event.NameMessageTriaged {
				if err := board.ApplyDelivery(ctx, d.RoutingKey, d.Body); err != nil {
					requeue := !bff.IsPermanent(err)
					_ = d.Nack(false, requeue)
					continue
				}
			}
			_ = d.Ack(false)
		}
	}
}
