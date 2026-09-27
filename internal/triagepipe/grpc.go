package triagepipe

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

	triagev1 "github.com/leohteixeira/advisor-radar/gen/triage/v1"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

const reviewThreshold = 0.85

// ReviewReader loads and corrects triage.results.
type ReviewReader struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewReviewReader wraps a pool for review RPCs.
func NewReviewReader(pool *pgxpool.Pool) *ReviewReader {
	return &ReviewReader{pool: pool, now: time.Now}
}

// ReviewRow is one review queue item.
type ReviewRow struct {
	ID        string
	Client    string
	Text      string
	Dist      map[string]float64
	Ago       int
	Fallback  bool
	Intent    string
	Corrected bool
}

// ListReview returns rows whose top intent probability is below 0.85.
func (r *ReviewReader) ListReview(ctx context.Context) ([]ReviewRow, error) {
	const q = `
SELECT id, customer_id, intent, intent_prob, degraded, corrected_intent, created_at, payload
FROM results
WHERE intent_prob < $1
ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, reviewThreshold)
	if err != nil {
		return nil, fmt.Errorf("triage: list review: %w", err)
	}
	defer rows.Close()
	now := r.now()
	out := make([]ReviewRow, 0)
	for rows.Next() {
		var (
			id, customer, intent string
			prob                 float64
			degraded             bool
			corrected            *string
			created              time.Time
			payload              []byte
		)
		if err := rows.Scan(&id, &customer, &intent, &prob, &degraded, &corrected, &created, &payload); err != nil {
			return nil, fmt.Errorf("triage: scan review: %w", err)
		}
		dist, text := decodeReviewPayload(payload)
		label := intent
		isCorrected := false
		if corrected != nil && *corrected != "" {
			label = *corrected
			isCorrected = true
		}
		ago := int(now.Sub(created).Minutes())
		if ago < 0 {
			ago = -ago
		}
		out = append(out, ReviewRow{
			ID: id, Client: customer, Text: text, Dist: dist, Ago: ago,
			Fallback: degraded, Intent: label, Corrected: isCorrected,
		})
	}
	return out, rows.Err()
}

// CorrectIntent persists the corrected label on results.
func (r *ReviewReader) CorrectIntent(ctx context.Context, id, intent string) (ReviewRow, error) {
	const q = `
UPDATE results
SET corrected_intent = $2, intent = $2
WHERE id = $1 AND intent_prob < $3
RETURNING id, customer_id, intent, intent_prob, degraded, corrected_intent, created_at, payload`
	var (
		rid, customer, label string
		prob                 float64
		degraded             bool
		corrected            *string
		created              time.Time
		payload              []byte
	)
	err := r.pool.QueryRow(ctx, q, id, intent, reviewThreshold).Scan(
		&rid, &customer, &label, &prob, &degraded, &corrected, &created, &payload,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewRow{}, err
	}
	if err != nil {
		return ReviewRow{}, fmt.Errorf("triage: correct intent: %w", err)
	}
	dist, text := decodeReviewPayload(payload)
	ago := int(r.now().Sub(created).Minutes())
	if ago < 0 {
		ago = -ago
	}
	return ReviewRow{
		ID: rid, Client: customer, Text: text, Dist: dist, Ago: ago,
		Fallback: degraded, Intent: label, Corrected: true,
	}, nil
}

// IntentStats aggregates review and fallback percentages plus intent counts.
func (r *ReviewReader) IntentStats(ctx context.Context) (reviewPct, fallbackPct int, intents map[string]int, err error) {
	const totalQ = `SELECT COUNT(*) FROM results`
	var total int
	if err := r.pool.QueryRow(ctx, totalQ).Scan(&total); err != nil {
		return 0, 0, nil, fmt.Errorf("triage: count results: %w", err)
	}
	if total == 0 {
		return 0, 0, map[string]int{}, nil
	}
	var reviewN, fallbackN int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM results WHERE intent_prob < $1`, reviewThreshold).Scan(&reviewN); err != nil {
		return 0, 0, nil, fmt.Errorf("triage: count review: %w", err)
	}
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM results WHERE degraded`).Scan(&fallbackN); err != nil {
		return 0, 0, nil, fmt.Errorf("triage: count fallback: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
SELECT COALESCE(corrected_intent, intent), COUNT(*)
FROM results
GROUP BY 1`)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("triage: intent groups: %w", err)
	}
	defer rows.Close()
	intents = map[string]int{}
	for rows.Next() {
		var label string
		var n int
		if err := rows.Scan(&label, &n); err != nil {
			return 0, 0, nil, fmt.Errorf("triage: scan intent: %w", err)
		}
		intents[label] = n
	}
	return (reviewN * 100) / total, (fallbackN * 100) / total, intents, rows.Err()
}

func decodeReviewPayload(payload []byte) (map[string]float64, string) {
	var raw struct {
		Text string             `json:"text"`
		Dist map[string]float64 `json:"dist"`
	}
	_ = json.Unmarshal(payload, &raw)
	if raw.Dist == nil {
		raw.Dist = map[string]float64{}
	}
	return raw.Dist, raw.Text
}

var validIntents = map[string]struct{}{
	"Operacional": {}, "Câmbio": {}, "Tributação": {}, "Investimento": {},
	"Resgate": {}, "Reclamação": {}, "Encerramento": {}, "Contato": {},
}

// GRPCServer serves TriageService.
type GRPCServer struct {
	triagev1.UnimplementedTriageServiceServer
	reader *ReviewReader
}

// NewGRPCServer returns the triage read/correct server.
func NewGRPCServer(reader *ReviewReader) *GRPCServer {
	return &GRPCServer{reader: reader}
}

// ListReview implements TriageService.
func (s *GRPCServer) ListReview(ctx context.Context, _ *triagev1.ListReviewRequest) (*triagev1.ListReviewResponse, error) {
	items, err := s.reader.ListReview(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list review: %v", err)
	}
	out := make([]*triagev1.ReviewItem, 0, len(items))
	for _, r := range items {
		dist, _ := json.Marshal(r.Dist)
		out = append(out, &triagev1.ReviewItem{
			Id: r.ID, Client: r.Client, Text: r.Text, DistJson: string(dist),
			Ago: int32(r.Ago), Fallback: r.Fallback, Intent: r.Intent, Corrected: r.Corrected,
		})
	}
	return &triagev1.ListReviewResponse{Items: out}, nil
}

// CorrectIntent implements TriageService.
func (s *GRPCServer) CorrectIntent(ctx context.Context, req *triagev1.CorrectIntentRequest) (*triagev1.ReviewItem, error) {
	id, err := identity.ParseV7(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	if _, ok := validIntents[req.GetIntent()]; !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid intent")
	}
	row, err := s.reader.CorrectIntent(ctx, id, req.GetIntent())
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, status.Error(codes.NotFound, "review row not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "correct intent: %v", err)
	}
	dist, _ := json.Marshal(row.Dist)
	return &triagev1.ReviewItem{
		Id: row.ID, Client: row.Client, Text: row.Text, DistJson: string(dist),
		Ago: int32(row.Ago), Fallback: row.Fallback, Intent: row.Intent, Corrected: row.Corrected,
	}, nil
}

// IntentStats implements TriageService.
func (s *GRPCServer) IntentStats(ctx context.Context, _ *triagev1.IntentStatsRequest) (*triagev1.IntentStatsResponse, error) {
	reviewPct, fallbackPct, intents, err := s.reader.IntentStats(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "intent stats: %v", err)
	}
	m := make(map[string]int32, len(intents))
	for k, v := range intents {
		m[k] = int32(v)
	}
	return &triagev1.IntentStatsResponse{
		ReviewPct: int32(reviewPct), FallbackPct: int32(fallbackPct), Intents: m,
	}, nil
}
