package cases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	casesv1 "github.com/leohteixeira/advisor-radar/gen/cases/v1"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// CaseStates matches the board column labels by index.
var CaseStates = []string{StateAberto, StateEmAtendimento, StateAguardandoCliente, StateResolvido}

// CaseReader loads cases and history for the BFF.
type CaseReader struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewCaseReader wraps a pool for cases read RPCs.
func NewCaseReader(pool *pgxpool.Pool) *CaseReader {
	return &CaseReader{pool: pool, now: time.Now}
}

// HistoryEntry is one case_history row.
type HistoryEntry struct {
	Ago  int
	Kind string
	Text string
}

// CaseView is one board case with computed fields.
type CaseView struct {
	Row       CaseRow
	History   []HistoryEntry
	StateIdx  int
	OpenedAgo int
	Remaining int
}

func stateIndex(state string) int {
	for i, s := range CaseStates {
		if s == state {
			return i
		}
	}
	return 0
}

// ListCases returns every case with history.
func (r *CaseReader) ListCases(ctx context.Context) ([]CaseView, error) {
	const q = `
SELECT id, customer_id, COALESCE(signal_id::text, ''), advisor_id, state, sla_total_minutes, escalated, opened_at
FROM cases
ORDER BY opened_at DESC`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("cases: list: %w", err)
	}
	defer rows.Close()
	now := r.now()
	out := make([]CaseView, 0)
	for rows.Next() {
		var row CaseRow
		if err := rows.Scan(
			&row.ID, &row.CustomerID, &row.SignalID, &row.AdvisorID,
			&row.State, &row.SLATotalMinutes, &row.Escalated, &row.OpenedAt,
		); err != nil {
			return nil, fmt.Errorf("cases: scan: %w", err)
		}
		hist, err := r.history(ctx, row.ID, now)
		if err != nil {
			return nil, err
		}
		openedAgo := int(now.Sub(row.OpenedAt).Minutes())
		if openedAgo < 0 {
			openedAgo = -openedAgo
		}
		out = append(out, CaseView{
			Row: row, History: hist, StateIdx: stateIndex(row.State),
			OpenedAgo: openedAgo, Remaining: RemainingMinutes(row, now),
		})
	}
	return out, rows.Err()
}

func (r *CaseReader) history(ctx context.Context, caseID string, now time.Time) ([]HistoryEntry, error) {
	const q = `
SELECT kind, text, occurred_at
FROM case_history
WHERE case_id = $1
ORDER BY occurred_at DESC`
	rows, err := r.pool.Query(ctx, q, caseID)
	if err != nil {
		return nil, fmt.Errorf("cases: history: %w", err)
	}
	defer rows.Close()
	out := make([]HistoryEntry, 0)
	for rows.Next() {
		var kind, text string
		var at time.Time
		if err := rows.Scan(&kind, &text, &at); err != nil {
			return nil, fmt.Errorf("cases: scan history: %w", err)
		}
		ago := int(now.Sub(at).Minutes())
		if ago < 0 {
			ago = -ago
		}
		out = append(out, HistoryEntry{Ago: ago, Kind: kind, Text: text})
	}
	return out, rows.Err()
}

// Advance moves one case forward when possible.
func (r *CaseReader) Advance(ctx context.Context, store Store, id string) (CaseView, error) {
	row, ok, err := store.GetCase(ctx, id)
	if err != nil {
		return CaseView{}, err
	}
	if !ok {
		return CaseView{}, pgx.ErrNoRows
	}
	next, ok := nextState(row.State)
	if !ok {
		// Already resolved: return current view.
		return r.viewOf(ctx, row)
	}
	if err := Advance(ctx, store, id, next); err != nil {
		return CaseView{}, err
	}
	row, ok, err = store.GetCase(ctx, id)
	if err != nil {
		return CaseView{}, err
	}
	if !ok {
		return CaseView{}, pgx.ErrNoRows
	}
	return r.viewOf(ctx, row)
}

func (r *CaseReader) viewOf(ctx context.Context, row CaseRow) (CaseView, error) {
	now := r.now()
	hist, err := r.history(ctx, row.ID, now)
	if err != nil {
		return CaseView{}, err
	}
	openedAgo := int(now.Sub(row.OpenedAt).Minutes())
	if openedAgo < 0 {
		openedAgo = -openedAgo
	}
	return CaseView{
		Row: row, History: hist, StateIdx: stateIndex(row.State),
		OpenedAgo: openedAgo, Remaining: RemainingMinutes(row, now),
	}, nil
}

// ListAtRisk returns non-resolved cases within the at-risk window.
func (r *CaseReader) ListAtRisk(ctx context.Context) ([]CaseView, error) {
	all, err := r.ListCases(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CaseView, 0)
	for _, c := range all {
		if IsAtRisk(c.Row, r.now()) {
			out = append(out, c)
		}
	}
	return out, nil
}

// Backlog aggregates open/risk/overdue counts by advisor_id.
func (r *CaseReader) Backlog(ctx context.Context) (map[string]struct{ Open, Risk, Overdue int }, error) {
	all, err := r.ListCases(ctx)
	if err != nil {
		return nil, err
	}
	now := r.now()
	out := map[string]struct{ Open, Risk, Overdue int }{}
	for _, c := range all {
		if c.Row.State == StateResolvido {
			continue
		}
		b := out[c.Row.AdvisorID]
		b.Open++
		rem := RemainingMinutes(c.Row, now)
		if rem <= 0 {
			b.Overdue++
		}
		if IsAtRisk(c.Row, now) {
			b.Risk++
		}
		out[c.Row.AdvisorID] = b
	}
	return out, nil
}

// GRPCServer serves CasesService.
type GRPCServer struct {
	casesv1.UnimplementedCasesServiceServer
	reader *CaseReader
	store  Store
}

// NewGRPCServer returns the cases read/advance server.
func NewGRPCServer(reader *CaseReader, store Store) *GRPCServer {
	return &GRPCServer{reader: reader, store: store}
}

func toProto(c CaseView) *casesv1.Case {
	hist := make([]*casesv1.CaseHistoryEntry, 0, len(c.History))
	for _, h := range c.History {
		hist = append(hist, &casesv1.CaseHistoryEntry{Ago: int32(h.Ago), Kind: h.Kind, Text: h.Text})
	}
	return &casesv1.Case{
		Id: c.Row.ID, Client: c.Row.CustomerID, Signal: c.Row.SignalID,
		State: int32(c.StateIdx), OpenedAgo: int32(c.OpenedAgo), SlaTotal: int32(c.Row.SLATotalMinutes),
		Escalated: c.Row.Escalated, History: hist, AdvisorId: c.Row.AdvisorID,
		RemainingMinutes: int32(c.Remaining),
	}
}

// ListCases implements CasesService.
func (s *GRPCServer) ListCases(ctx context.Context, _ *casesv1.ListCasesRequest) (*casesv1.ListCasesResponse, error) {
	items, err := s.reader.ListCases(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list cases: %v", err)
	}
	out := make([]*casesv1.Case, 0, len(items))
	for _, c := range items {
		out = append(out, toProto(c))
	}
	return &casesv1.ListCasesResponse{Items: out, States: CaseStates}, nil
}

// AdvanceCase implements CasesService.
func (s *GRPCServer) AdvanceCase(ctx context.Context, req *casesv1.AdvanceCaseRequest) (*casesv1.Case, error) {
	id, err := identity.ParseV7(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	view, err := s.reader.Advance(ctx, s.store, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "case not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "advance: %v", err)
	}
	return toProto(view), nil
}

// ListAtRisk implements CasesService.
func (s *GRPCServer) ListAtRisk(ctx context.Context, _ *casesv1.ListAtRiskRequest) (*casesv1.ListAtRiskResponse, error) {
	items, err := s.reader.ListAtRisk(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list at risk: %v", err)
	}
	out := make([]*casesv1.AtRiskCase, 0, len(items))
	for _, c := range items {
		out = append(out, &casesv1.AtRiskCase{
			Id: c.Row.ID, ClientId: c.Row.CustomerID, AdvisorId: c.Row.AdvisorID, Remaining: int32(c.Remaining),
		})
	}
	return &casesv1.ListAtRiskResponse{Items: out}, nil
}

// BacklogByAdvisor implements CasesService.
func (s *GRPCServer) BacklogByAdvisor(ctx context.Context, _ *casesv1.BacklogByAdvisorRequest) (*casesv1.BacklogByAdvisorResponse, error) {
	m, err := s.reader.Backlog(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "backlog: %v", err)
	}
	out := make([]*casesv1.AdvisorBacklog, 0, len(m))
	for id, b := range m {
		out = append(out, &casesv1.AdvisorBacklog{
			AdvisorId: id, Open: int32(b.Open), Risk: int32(b.Risk), Overdue: int32(b.Overdue),
		})
	}
	return &casesv1.BacklogByAdvisorResponse{Items: out}, nil
}
