package advisory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	advisoryv1 "github.com/leohteixeira/advisor-radar/gen/advisory/v1"
	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// BookReader loads book, operators, and queue cards from advisory Postgres.
type BookReader struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewBookReader wraps a pool for read RPCs.
func NewBookReader(pool *pgxpool.Pool) *BookReader {
	return &BookReader{pool: pool, now: time.Now}
}

// GetCustomer returns one book row with the live operator name.
func (r *BookReader) GetCustomer(ctx context.Context, customerID string) (book.Client, error) {
	const q = `
SELECT b.customer_id, b.name, b.segment, b.aum, b.advisor_id, o.name, b.since
FROM book b
JOIN operators o ON o.id = b.advisor_id
WHERE b.customer_id = $1`
	var c book.Client
	err := r.pool.QueryRow(ctx, q, customerID).Scan(
		&c.ID, &c.Name, &c.Segment, &c.AUM, &c.AdvisorID, &c.Advisor, &c.Since,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return book.Client{}, err
	}
	if err != nil {
		return book.Client{}, fmt.Errorf("advisory: get customer: %w", err)
	}
	return c, nil
}

// ListOperators returns every operator.
func (r *BookReader) ListOperators(ctx context.Context) ([]Operator, error) {
	const q = `SELECT id, name FROM operators ORDER BY name`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("advisory: list operators: %w", err)
	}
	defer rows.Close()
	out := make([]Operator, 0)
	for rows.Next() {
		var o Operator
		if err := rows.Scan(&o.ID, &o.Name); err != nil {
			return nil, fmt.Errorf("advisory: scan operator: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Operator is one advisory.operators row.
type Operator struct {
	ID   string
	Name string
}

// QueueCard is one queue_signals row for the BFF.
type QueueCard struct {
	ID         string
	CustomerID string
	Kind       string
	RaisedAt   time.Time
	Payload    json.RawMessage
	Name       string
	Segment    string
}

// ListQueue returns queue cards joined with the book, newest first.
func (r *BookReader) ListQueue(ctx context.Context) ([]QueueCard, error) {
	const q = `
SELECT q.id, q.customer_id, q.kind, q.raised_at, q.payload, b.name, b.segment
FROM queue_signals q
JOIN book b ON b.customer_id = q.customer_id
ORDER BY q.raised_at DESC`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("advisory: list queue: %w", err)
	}
	defer rows.Close()
	out := make([]QueueCard, 0)
	for rows.Next() {
		var c QueueCard
		if err := rows.Scan(&c.ID, &c.CustomerID, &c.Kind, &c.RaisedAt, &c.Payload, &c.Name, &c.Segment); err != nil {
			return nil, fmt.Errorf("advisory: scan queue: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ContactMetrics returns average first-contact minutes from signal_action.
func (r *BookReader) ContactMetrics(ctx context.Context) (avgToday, avgYesterday int, err error) {
	const q = `
SELECT
  COALESCE(AVG(EXTRACT(EPOCH FROM (a.contacted_at - q.raised_at)) / 60.0)
    FILTER (WHERE a.contacted_at IS NOT NULL AND a.contacted_at::date = CURRENT_DATE), 0),
  COALESCE(AVG(EXTRACT(EPOCH FROM (a.contacted_at - q.raised_at)) / 60.0)
    FILTER (WHERE a.contacted_at IS NOT NULL AND a.contacted_at::date = CURRENT_DATE - 1), 0)
FROM signal_action a
JOIN queue_signals q ON q.id = a.signal_id`
	var today, yesterday float64
	if err := r.pool.QueryRow(ctx, q).Scan(&today, &yesterday); err != nil {
		return 0, 0, fmt.Errorf("advisory: contact metrics: %w", err)
	}
	return int(today + 0.5), int(yesterday + 0.5), nil
}

// GRPCServer serves AdvisoryService.
type GRPCServer struct {
	advisoryv1.UnimplementedAdvisoryServiceServer
	reader *BookReader
}

// NewGRPCServer returns the advisory read server.
func NewGRPCServer(reader *BookReader) *GRPCServer {
	return &GRPCServer{reader: reader}
}

// GetCustomer implements AdvisoryService.
func (s *GRPCServer) GetCustomer(ctx context.Context, req *advisoryv1.GetCustomerRequest) (*advisoryv1.Customer, error) {
	id, err := identity.ParseV7(req.GetCustomerId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid customer id")
	}
	c, err := s.reader.GetCustomer(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "customer not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get customer: %v", err)
	}
	return &advisoryv1.Customer{
		Id: c.ID, Name: c.Name, Segment: c.Segment, Aum: c.AUM, Advisor: c.Advisor, Since: c.Since,
	}, nil
}

// ListOperators implements AdvisoryService.
func (s *GRPCServer) ListOperators(ctx context.Context, _ *advisoryv1.ListOperatorsRequest) (*advisoryv1.ListOperatorsResponse, error) {
	items, err := s.reader.ListOperators(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list operators: %v", err)
	}
	out := make([]*advisoryv1.Operator, 0, len(items))
	for _, o := range items {
		out = append(out, &advisoryv1.Operator{Id: o.ID, Name: o.Name})
	}
	return &advisoryv1.ListOperatorsResponse{Items: out}, nil
}

// ListQueue implements AdvisoryService.
func (s *GRPCServer) ListQueue(ctx context.Context, _ *advisoryv1.ListQueueRequest) (*advisoryv1.ListQueueResponse, error) {
	items, err := s.reader.ListQueue(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list queue: %v", err)
	}
	now := s.reader.now()
	out := make([]*advisoryv1.QueueSignal, 0, len(items))
	for _, c := range items {
		ago := int32(now.Sub(c.RaisedAt).Minutes())
		if ago < 0 {
			ago = -ago
		}
		out = append(out, &advisoryv1.QueueSignal{
			Id: c.ID, Kind: c.Kind, Client: c.CustomerID, Ago: ago, PayloadJson: string(c.Payload),
			Name: c.Name, Segment: c.Segment,
		})
	}
	return &advisoryv1.ListQueueResponse{Items: out}, nil
}

// ContactMetrics implements AdvisoryService.
func (s *GRPCServer) ContactMetrics(ctx context.Context, _ *advisoryv1.ContactMetricsRequest) (*advisoryv1.ContactMetricsResponse, error) {
	today, yesterday, err := s.reader.ContactMetrics(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "contact metrics: %v", err)
	}
	return &advisoryv1.ContactMetricsResponse{
		AvgFirstContactMin:       int32(today),
		AvgFirstContactYesterday: int32(yesterday),
	}, nil
}
