package bff

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	advisoryv1 "github.com/leohteixeira/advisor-radar/gen/advisory/v1"
	casesv1 "github.com/leohteixeira/advisor-radar/gen/cases/v1"
	triagev1 "github.com/leohteixeira/advisor-radar/gen/triage/v1"
	"github.com/leohteixeira/advisor-radar/internal/telemetry"
)

// QueueSource loads the demo/live queue from advisory.
type QueueSource interface {
	ListQueue(ctx context.Context) ([]Signal, error)
	GetCustomer(ctx context.Context, id string) (Customer, error)
	ListOperators(ctx context.Context) ([]Operator, error)
	ContactMetrics(ctx context.Context) (avgToday, avgYesterday int, err error)
	MomentFacts(ctx context.Context, customerID string) (MomentFacts, error)
	InvestorProfile(ctx context.Context, customerID string) (InvestorProfile, error)
}

// ReviewSource loads and corrects triage review rows.
type ReviewSource interface {
	ListReview(ctx context.Context) ([]ReviewRow, error)
	Correct(ctx context.Context, id, intent string) (ReviewRow, error)
	IntentStats(ctx context.Context) (reviewPct, fallbackPct int, intents map[string]int, err error)
}

// CaseSource loads cases and advances state.
type CaseSource interface {
	ListCases(ctx context.Context) ([]Case, []string, error)
	Advance(ctx context.Context, id string) (Case, error)
	ListAtRisk(ctx context.Context) ([]ManagerAtRisk, error)
	Backlog(ctx context.Context) ([]ManagerBacklog, error)
	// CustomerCases lists one customer's cases, of every state, with the
	// state labels their State indexes.
	CustomerCases(ctx context.Context, customerID string) ([]Case, []string, error)
}

// Customer is the GET /v1/customers/{id} JSON shape.
type Customer struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Segment string  `json:"segment"`
	AUM     float64 `json:"aum"`
	Advisor string  `json:"advisor"`
	Since   string  `json:"since"`
}

// MomentFacts are the home moment conditions advisory evaluated for one
// customer. Money is integer USD cents.
type MomentFacts struct {
	SegmentUpgraded    bool
	UpgradedSegment    string
	SegmentUpgradeNear bool
	UpgradeGapCents    int64
	IdleCash           bool
	CashCents          int64
	PatrimonyCents     int64
	PortfolioReview    bool
	PortfolioDrop      bool
}

// InvestorProfile is one customer's advisory investor profile. MaxRiskTable
// is the whole advisory max-risk table, in level order.
type InvestorProfile struct {
	Profile      string
	MaxRisk      int
	AssessedOn   time.Time
	MaxRiskTable []ProfileMaxRisk
}

// ProfileMaxRisk is one row of the advisory max-risk table.
type ProfileMaxRisk struct {
	Profile string
	MaxRisk int
}

// Operator is one advisory operator.
type Operator struct {
	ID   string
	Name string
}

// EmptyQueue is a no-op QueueSource used in unit tests.
type EmptyQueue struct{}

func (EmptyQueue) ListQueue(context.Context) ([]Signal, error) { return nil, nil }
func (EmptyQueue) GetCustomer(context.Context, string) (Customer, error) {
	return Customer{}, ErrCustomerNotFound
}
func (EmptyQueue) ListOperators(context.Context) ([]Operator, error) { return nil, nil }
func (EmptyQueue) ContactMetrics(context.Context) (int, int, error)  { return 0, 0, nil }
func (EmptyQueue) MomentFacts(context.Context, string) (MomentFacts, error) {
	return MomentFacts{}, ErrCustomerNotFound
}
func (EmptyQueue) InvestorProfile(context.Context, string) (InvestorProfile, error) {
	return InvestorProfile{}, ErrCustomerNotFound
}

// EmptyReview is a no-op ReviewSource.
type EmptyReview struct{}

func (EmptyReview) ListReview(context.Context) ([]ReviewRow, error) { return nil, nil }
func (EmptyReview) Correct(context.Context, string, string) (ReviewRow, error) {
	return ReviewRow{}, ErrReviewNotFound
}
func (EmptyReview) IntentStats(context.Context) (int, int, map[string]int, error) {
	return 0, 0, map[string]int{}, nil
}

// EmptyCases is a no-op CaseSource.
type EmptyCases struct{}

func (EmptyCases) ListCases(context.Context) ([]Case, []string, error) {
	return nil, CaseStates, nil
}
func (EmptyCases) Advance(context.Context, string) (Case, error) {
	return Case{}, ErrCaseNotFound
}
func (EmptyCases) ListAtRisk(context.Context) ([]ManagerAtRisk, error) { return nil, nil }
func (EmptyCases) Backlog(context.Context) ([]ManagerBacklog, error)   { return nil, nil }
func (EmptyCases) CustomerCases(context.Context, string) ([]Case, []string, error) {
	return nil, CaseStates, nil
}

// GRPCQueue talks to advisory.
type GRPCQueue struct {
	client advisoryv1.AdvisoryServiceClient
}

// NewGRPCQueue wraps an AdvisoryService client.
func NewGRPCQueue(client advisoryv1.AdvisoryServiceClient) *GRPCQueue {
	return &GRPCQueue{client: client}
}

func (g *GRPCQueue) ListQueue(ctx context.Context) ([]Signal, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.ListQueue(callCtx, &advisoryv1.ListQueueRequest{})
	if err != nil {
		return nil, fmt.Errorf("bff: list queue: %w", err)
	}
	out := make([]Signal, 0, len(res.GetItems()))
	for _, it := range res.GetItems() {
		sig := Signal{ID: it.GetId(), Kind: it.GetKind(), Client: it.GetClient(), Ago: int(it.GetAgo())}
		_ = json.Unmarshal([]byte(it.GetPayloadJson()), &sig)
		sig.ID = it.GetId()
		sig.Kind = it.GetKind()
		sig.Client = it.GetClient()
		sig.Ago = int(it.GetAgo())
		sig.Name = it.GetName()
		sig.Segment = it.GetSegment()
		out = append(out, sig)
	}
	return out, nil
}

func (g *GRPCQueue) GetCustomer(ctx context.Context, id string) (Customer, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.GetCustomer(callCtx, &advisoryv1.GetCustomerRequest{CustomerId: id})
	if err != nil {
		return Customer{}, err
	}
	return Customer{
		ID: res.GetId(), Name: res.GetName(), Segment: res.GetSegment(),
		AUM: res.GetAum(), Advisor: res.GetAdvisor(), Since: res.GetSince(),
	}, nil
}

func (g *GRPCQueue) ListOperators(ctx context.Context) ([]Operator, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.ListOperators(callCtx, &advisoryv1.ListOperatorsRequest{})
	if err != nil {
		return nil, fmt.Errorf("bff: list operators: %w", err)
	}
	out := make([]Operator, 0, len(res.GetItems()))
	for _, o := range res.GetItems() {
		out = append(out, Operator{ID: o.GetId(), Name: o.GetName()})
	}
	return out, nil
}

func (g *GRPCQueue) ContactMetrics(ctx context.Context) (int, int, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.ContactMetrics(callCtx, &advisoryv1.ContactMetricsRequest{})
	if err != nil {
		return 0, 0, fmt.Errorf("bff: contact metrics: %w", err)
	}
	return int(res.GetAvgFirstContactMin()), int(res.GetAvgFirstContactYesterday()), nil
}

// MomentFacts reads the advisory moment facts of one customer.
func (g *GRPCQueue) MomentFacts(ctx context.Context, customerID string) (MomentFacts, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.GetMomentFacts(callCtx, &advisoryv1.GetMomentFactsRequest{CustomerId: customerID})
	if err != nil {
		return MomentFacts{}, fmt.Errorf("bff: moment facts: %w", err)
	}
	return MomentFacts{
		SegmentUpgraded:    res.GetSegmentUpgraded(),
		UpgradedSegment:    res.GetUpgradedSegment(),
		SegmentUpgradeNear: res.GetSegmentUpgradeNear(),
		UpgradeGapCents:    res.GetUpgradeGapCents(),
		IdleCash:           res.GetIdleCash(),
		CashCents:          res.GetCashCents(),
		PatrimonyCents:     res.GetPatrimonyCents(),
		PortfolioReview:    res.GetPortfolioReview(),
		PortfolioDrop:      res.GetPortfolioDrop(),
	}, nil
}

// InvestorProfile reads the advisory investor profile of one customer.
func (g *GRPCQueue) InvestorProfile(ctx context.Context, customerID string) (InvestorProfile, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.GetInvestorProfile(callCtx, &advisoryv1.GetInvestorProfileRequest{CustomerId: customerID})
	if err != nil {
		return InvestorProfile{}, fmt.Errorf("bff: investor profile: %w", err)
	}
	assessed, err := time.Parse(time.DateOnly, res.GetAssessedOn())
	if err != nil {
		return InvestorProfile{}, fmt.Errorf("bff: investor profile assessed_on: %w", err)
	}
	table := make([]ProfileMaxRisk, 0, len(res.GetMaxRiskTable()))
	for _, row := range res.GetMaxRiskTable() {
		table = append(table, ProfileMaxRisk{Profile: row.GetProfile(), MaxRisk: int(row.GetMaxRisk())})
	}
	return InvestorProfile{
		Profile:      res.GetProfile(),
		MaxRisk:      int(res.GetMaxRisk()),
		AssessedOn:   assessed,
		MaxRiskTable: table,
	}, nil
}

// GRPCReview talks to triage.
type GRPCReview struct {
	client triagev1.TriageServiceClient
}

// NewGRPCReview wraps a TriageService client.
func NewGRPCReview(client triagev1.TriageServiceClient) *GRPCReview {
	return &GRPCReview{client: client}
}

func (g *GRPCReview) ListReview(ctx context.Context) ([]ReviewRow, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.ListReview(callCtx, &triagev1.ListReviewRequest{})
	if err != nil {
		return nil, fmt.Errorf("bff: list review: %w", err)
	}
	out := make([]ReviewRow, 0, len(res.GetItems()))
	for _, it := range res.GetItems() {
		var dist map[string]float64
		_ = json.Unmarshal([]byte(it.GetDistJson()), &dist)
		out = append(out, ReviewRow{
			ID: it.GetId(), Client: it.GetClient(), Text: it.GetText(), Dist: dist,
			Ago: int(it.GetAgo()), Fallback: it.GetFallback(), Intent: it.GetIntent(), Corrected: it.GetCorrected(),
		})
	}
	return out, nil
}

func (g *GRPCReview) Correct(ctx context.Context, id, intent string) (ReviewRow, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	it, err := g.client.CorrectIntent(callCtx, &triagev1.CorrectIntentRequest{Id: id, Intent: intent})
	if err != nil {
		return ReviewRow{}, err
	}
	var dist map[string]float64
	_ = json.Unmarshal([]byte(it.GetDistJson()), &dist)
	return ReviewRow{
		ID: it.GetId(), Client: it.GetClient(), Text: it.GetText(), Dist: dist,
		Ago: int(it.GetAgo()), Fallback: it.GetFallback(), Intent: it.GetIntent(), Corrected: it.GetCorrected(),
	}, nil
}

func (g *GRPCReview) IntentStats(ctx context.Context) (int, int, map[string]int, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.IntentStats(callCtx, &triagev1.IntentStatsRequest{})
	if err != nil {
		return 0, 0, nil, fmt.Errorf("bff: intent stats: %w", err)
	}
	m := make(map[string]int, len(res.GetIntents()))
	for k, v := range res.GetIntents() {
		m[k] = int(v)
	}
	return int(res.GetReviewPct()), int(res.GetFallbackPct()), m, nil
}

// GRPCCases talks to cases.
type GRPCCases struct {
	client casesv1.CasesServiceClient
}

// NewGRPCCases wraps a CasesService client.
func NewGRPCCases(client casesv1.CasesServiceClient) *GRPCCases {
	return &GRPCCases{client: client}
}

func (g *GRPCCases) ListCases(ctx context.Context) ([]Case, []string, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.ListCases(callCtx, &casesv1.ListCasesRequest{})
	if err != nil {
		return nil, nil, fmt.Errorf("bff: list cases: %w", err)
	}
	out := make([]Case, 0, len(res.GetItems()))
	for _, it := range res.GetItems() {
		out = append(out, protoCase(it))
	}
	return out, res.GetStates(), nil
}

// CustomerCases lists one customer's cases through the ListCases filter.
func (g *GRPCCases) CustomerCases(ctx context.Context, customerID string) ([]Case, []string, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.ListCases(callCtx, &casesv1.ListCasesRequest{CustomerId: customerID})
	if err != nil {
		return nil, nil, fmt.Errorf("bff: list customer cases: %w", err)
	}
	out := make([]Case, 0, len(res.GetItems()))
	for _, it := range res.GetItems() {
		out = append(out, protoCase(it))
	}
	return out, res.GetStates(), nil
}

func (g *GRPCCases) Advance(ctx context.Context, id string) (Case, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	it, err := g.client.AdvanceCase(callCtx, &casesv1.AdvanceCaseRequest{Id: id})
	if err != nil {
		return Case{}, err
	}
	return protoCase(it), nil
}

func (g *GRPCCases) ListAtRisk(ctx context.Context) ([]ManagerAtRisk, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.ListAtRisk(callCtx, &casesv1.ListAtRiskRequest{})
	if err != nil {
		return nil, fmt.Errorf("bff: list at risk: %w", err)
	}
	out := make([]ManagerAtRisk, 0, len(res.GetItems()))
	for _, it := range res.GetItems() {
		out = append(out, ManagerAtRisk{
			ID: it.GetId(), Client: it.GetClientId(), Advisor: it.GetAdvisorId(), Remaining: int(it.GetRemaining()),
		})
	}
	return out, nil
}

func (g *GRPCCases) Backlog(ctx context.Context) ([]ManagerBacklog, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := g.client.BacklogByAdvisor(callCtx, &casesv1.BacklogByAdvisorRequest{})
	if err != nil {
		return nil, fmt.Errorf("bff: backlog: %w", err)
	}
	out := make([]ManagerBacklog, 0, len(res.GetItems()))
	for _, it := range res.GetItems() {
		out = append(out, ManagerBacklog{
			Advisor: it.GetAdvisorId(), Open: int(it.GetOpen()), Risk: int(it.GetRisk()), Overdue: int(it.GetOverdue()),
		})
	}
	return out, nil
}

func protoCase(it *casesv1.Case) Case {
	hist := make([]CaseHistoryEntry, 0, len(it.GetHistory()))
	for _, h := range it.GetHistory() {
		hist = append(hist, CaseHistoryEntry{Ago: int(h.GetAgo()), Kind: h.GetKind(), Text: h.GetText()})
	}
	return Case{
		ID: it.GetId(), Client: it.GetClient(), Signal: it.GetSignal(), State: int(it.GetState()),
		OpenedAgo: int(it.GetOpenedAgo()), SlaTotal: int(it.GetSlaTotal()), Escalated: it.GetEscalated(), History: hist,
	}
}

// DialGRPC opens an insecure client connection traced with the otelgrpc
// stats handler, so each call is a client span that carries the trace
// context to the server.
func DialGRPC(target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		telemetry.GRPCClientOption(),
	)
}
