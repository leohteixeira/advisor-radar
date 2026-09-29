package advisory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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

// InvestorProfile reads the customer's investor profile and assessment date.
// A customer outside the book wraps ErrUnknownCustomer.
func (r *BookReader) InvestorProfile(ctx context.Context, customerID string) (InvestorProfile, error) {
	const q = `
SELECT investor_profile, profile_assessed_on
FROM book
WHERE customer_id = $1`
	var p InvestorProfile
	err := r.pool.QueryRow(ctx, q, customerID).Scan(&p.Profile, &p.AssessedOn)
	if errors.Is(err, pgx.ErrNoRows) {
		return InvestorProfile{}, fmt.Errorf("%w: %w", ErrUnknownCustomer, err)
	}
	if err != nil {
		return InvestorProfile{}, fmt.Errorf("advisory: investor profile: %w", err)
	}
	return p, nil
}

// MomentBook is the book side of the moment facts: the customer's segment,
// the segment alerts raised since a given time, and the latest revaluation
// (nil when there is none).
type MomentBook struct {
	Segment     string
	Alerts      []SegmentAlert
	Revaluation *Revaluation
}

// MomentBook reads the customer's segment, the segmento alerts raised after
// since, newest first, and the latest revaluation, from one read-only
// repeatable-read snapshot, so
// a concurrent Apply is seen whole or not at all. A customer outside the book
// wraps ErrUnknownCustomer.
func (r *BookReader) MomentBook(ctx context.Context, customerID string, since time.Time) (MomentBook, error) {
	var mb MomentBook
	opts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	err := pgx.BeginTxFunc(ctx, r.pool, opts, func(tx pgx.Tx) error {
		var err error
		mb, err = readMomentBook(ctx, tx, customerID, since)
		return err
	})
	if err != nil {
		return MomentBook{}, err
	}
	return mb, nil
}

// readMomentBook runs the three MomentBook reads inside tx.
func readMomentBook(ctx context.Context, tx pgx.Tx, customerID string, since time.Time) (MomentBook, error) {
	const segmentQ = `SELECT segment FROM book WHERE customer_id = $1`
	var mb MomentBook
	err := tx.QueryRow(ctx, segmentQ, customerID).Scan(&mb.Segment)
	if errors.Is(err, pgx.ErrNoRows) {
		return MomentBook{}, fmt.Errorf("%w: %w", ErrUnknownCustomer, err)
	}
	if err != nil {
		return MomentBook{}, fmt.Errorf("advisory: moment book segment: %w", err)
	}

	const alertsQ = `
SELECT COALESCE(payload->>'from', ''), COALESCE(payload->>'to', ''),
       COALESCE(source_schema_version, 0), raised_at
FROM alerts
WHERE customer_id = $1 AND kind = $2 AND raised_at > $3
ORDER BY raised_at DESC`
	rows, err := tx.Query(ctx, alertsQ, customerID, KindSegmento, since)
	if err != nil {
		return MomentBook{}, fmt.Errorf("advisory: moment book alerts: %w", err)
	}
	defer rows.Close()
	mb.Alerts = make([]SegmentAlert, 0)
	for rows.Next() {
		var a SegmentAlert
		if err := rows.Scan(&a.From, &a.To, &a.SchemaVersion, &a.RaisedAt); err != nil {
			return MomentBook{}, fmt.Errorf("advisory: scan segment alert: %w", err)
		}
		mb.Alerts = append(mb.Alerts, a)
	}
	if err := rows.Err(); err != nil {
		return MomentBook{}, fmt.Errorf("advisory: moment book alerts: %w", err)
	}
	rev, err := readRevaluation(ctx, tx, customerID)
	if err != nil {
		return MomentBook{}, err
	}
	mb.Revaluation = rev
	return mb, nil
}

// readRevaluation reads the customer's latest revaluation, nil when there is
// none.
func readRevaluation(ctx context.Context, tx pgx.Tx, customerID string) (*Revaluation, error) {
	const q = `
SELECT epoch::text, sim_day, amount_cents, before_cents, product_id, product_change_bp, source_event_id::text
FROM revaluation
WHERE customer_id = $1`
	var rev Revaluation
	err := tx.QueryRow(ctx, q, customerID).Scan(
		&rev.Epoch, &rev.SimDay, &rev.AmountCents, &rev.BeforeCents, &rev.ProductID, &rev.ProductChangeBP, &rev.SourceEventID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("advisory: moment book revaluation: %w", err)
	}
	return &rev, nil
}

// FactsReader reads the book rows behind GetInvestorProfile and
// GetMomentFacts. A customer outside the book wraps ErrUnknownCustomer.
type FactsReader interface {
	InvestorProfile(ctx context.Context, customerID string) (InvestorProfile, error)
	MomentBook(ctx context.Context, customerID string, since time.Time) (MomentBook, error)
}

// GRPCServer serves AdvisoryService.
type GRPCServer struct {
	advisoryv1.UnimplementedAdvisoryServiceServer
	reader   *BookReader
	facts    FactsReader
	accounts AccountReader
	now      func() time.Time
}

// ServerOption configures a GRPCServer.
type ServerOption func(*GRPCServer)

// WithAccountReader sets the account-sim reader GetMomentFacts uses. Without
// one, GetMomentFacts answers Unavailable. A nil reader is ignored.
func WithAccountReader(accounts AccountReader) ServerOption {
	return func(s *GRPCServer) {
		if accounts != nil {
			s.accounts = accounts
		}
	}
}

// WithFactsReader replaces the BookReader as the source of the profile and
// moment reads. A nil reader is ignored.
func WithFactsReader(facts FactsReader) ServerOption {
	return func(s *GRPCServer) {
		if facts != nil {
			s.facts = facts
		}
	}
}

// WithServerClock replaces time.Now as the clock the 24 h upgrade window is
// measured against. A nil clock is ignored.
func WithServerClock(now func() time.Time) ServerOption {
	return func(s *GRPCServer) {
		if now != nil {
			s.now = now
		}
	}
}

// NewGRPCServer returns the advisory read server. A nil reader serves only
// the profile and moment reads of WithFactsReader; the book reads answer
// Unavailable.
func NewGRPCServer(reader *BookReader, opts ...ServerOption) *GRPCServer {
	s := &GRPCServer{reader: reader, accounts: noAccountSim{}, now: time.Now}
	if reader != nil {
		s.facts = reader
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// errNoBook answers the book reads of a server built without a BookReader.
var errNoBook = status.Error(codes.Unavailable, "book is not configured")

// GetCustomer implements AdvisoryService.
func (s *GRPCServer) GetCustomer(ctx context.Context, req *advisoryv1.GetCustomerRequest) (*advisoryv1.Customer, error) {
	id, err := identity.ParseV7(req.GetCustomerId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid customer id")
	}
	if s.reader == nil {
		return nil, errNoBook
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
		AdvisorId: c.AdvisorID,
	}, nil
}

// ListOperators implements AdvisoryService.
func (s *GRPCServer) ListOperators(ctx context.Context, _ *advisoryv1.ListOperatorsRequest) (*advisoryv1.ListOperatorsResponse, error) {
	if s.reader == nil {
		return nil, errNoBook
	}
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
	if s.reader == nil {
		return nil, errNoBook
	}
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
	if s.reader == nil {
		return nil, errNoBook
	}
	today, yesterday, err := s.reader.ContactMetrics(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "contact metrics: %v", err)
	}
	return &advisoryv1.ContactMetricsResponse{
		AvgFirstContactMin:       int32(today),
		AvgFirstContactYesterday: int32(yesterday),
	}, nil
}

// GetInvestorProfile implements AdvisoryService. max_risk comes from the
// advisory max-risk table, which max_risk_table carries whole.
func (s *GRPCServer) GetInvestorProfile(ctx context.Context, req *advisoryv1.GetInvestorProfileRequest) (*advisoryv1.InvestorProfile, error) {
	id, err := identity.ParseV7(req.GetCustomerId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid customer id")
	}
	if s.facts == nil {
		return nil, errNoBook
	}
	p, err := s.facts.InvestorProfile(ctx, id)
	if err != nil && ctx.Err() != nil {
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if errors.Is(err, ErrUnknownCustomer) {
		return nil, status.Error(codes.NotFound, "customer not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "investor profile: %v", err)
	}
	risk, err := MaxRisk(p.Profile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "investor profile: %v", err)
	}
	table := MaxRiskTable()
	rows := make([]*advisoryv1.ProfileMaxRisk, 0, len(table))
	for _, row := range table {
		rows = append(rows, &advisoryv1.ProfileMaxRisk{Profile: row.Profile, MaxRisk: int32(row.MaxRisk)})
	}
	return &advisoryv1.InvestorProfile{
		Profile:      p.Profile,
		MaxRisk:      int32(risk), // risk is 2, 3, or 5
		AssessedOn:   p.AssessedOn.Format(time.DateOnly),
		MaxRiskTable: rows,
	}, nil
}

// GetMomentFacts implements AdvisoryService. The book decides the customer
// (NotFound outside it); the balance comes from account-sim, and any failure
// to read it is Unavailable.
func (s *GRPCServer) GetMomentFacts(ctx context.Context, req *advisoryv1.GetMomentFactsRequest) (*advisoryv1.MomentFacts, error) {
	id, err := identity.ParseV7(req.GetCustomerId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid customer id")
	}
	if s.facts == nil {
		return nil, errNoBook
	}
	now := s.now()
	mb, err := s.facts.MomentBook(ctx, id, now.Add(-upgradeWindow))
	if err != nil && ctx.Err() != nil {
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if errors.Is(err, ErrUnknownCustomer) {
		return nil, status.Error(codes.NotFound, "customer not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "moment facts: %v", err)
	}
	balance, err := s.accounts.Balance(ctx, id)
	if err != nil && ctx.Err() != nil {
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "moment facts: %v", err)
	}
	f := EvaluateMoments(MomentInput{
		Segment:     mb.Segment,
		Balance:     balance,
		Alerts:      mb.Alerts,
		Revaluation: mb.Revaluation,
		Now:         now,
	})
	return &advisoryv1.MomentFacts{
		SegmentUpgraded:    f.SegmentUpgraded,
		UpgradedSegment:    f.UpgradedSegment,
		SegmentUpgradeNear: f.SegmentUpgradeNear,
		UpgradeGapCents:    f.UpgradeGapCents,
		IdleCash:           f.IdleCash,
		CashCents:          f.CashCents,
		PatrimonyCents:     f.PatrimonyCents,
		PortfolioReview:    f.PortfolioReview,
		PortfolioDrop:      f.PortfolioDrop,
		DropBp:             clampInt32(f.DropBP),
		DropProductId:      f.DropProductID,
		DropProductBp:      clampInt32(f.DropProductBP),
		DropDay:            clampInt32(f.DropDay),
	}, nil
}

// clampInt32 narrows n to the int32 range of the proto fields.
func clampInt32(n int) int32 {
	return int32(max(min(n, math.MaxInt32), math.MinInt32))
}
