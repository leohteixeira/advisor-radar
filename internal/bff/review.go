package bff

import (
	"errors"
	"fmt"
	"sync"
)

const reviewThreshold = 0.85

// ErrInvalidIntent is returned when the corrected intent is not one of the eight labels.
var ErrInvalidIntent = errors.New("invalid intent")

// ErrReviewNotFound is returned when the review row id is unknown.
var ErrReviewNotFound = errors.New("review row not found")

// ValidIntents are the eight Portuguese intent labels from ux.md.
var ValidIntents = []string{
	"Operacional",
	"Câmbio",
	"Tributação",
	"Investimento",
	"Resgate",
	"Reclamação",
	"Encerramento",
	"Contato",
}

var validIntentSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(ValidIntents))
	for _, label := range ValidIntents {
		m[label] = struct{}{}
	}
	return m
}()

// ReviewRow is one analyst review queue item. Field names match REVIEW in mock-data.js.
type ReviewRow struct {
	ID        string             `json:"id"`
	Client    string             `json:"client"`
	Text      string             `json:"text"`
	Dist      map[string]float64 `json:"dist"`
	Ago       int                `json:"ago"`
	Fallback  bool               `json:"fallback,omitempty"`
	Intent    string             `json:"intent"`
	Corrected bool               `json:"corrected,omitempty"`
}

// ReviewQueue holds the in-memory review rows for one BFF process.
type ReviewQueue struct {
	mu    sync.Mutex
	items []ReviewRow
}

// NewReviewQueue returns the seed review rows r01–r07 under the 0.85 threshold.
func NewReviewQueue() *ReviewQueue {
	return &ReviewQueue{items: seedReview()}
}

// Items returns a copy of rows whose top intent probability is below 0.85.
func (q *ReviewQueue) Items() []ReviewRow {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]ReviewRow, 0, len(q.items))
	for _, row := range q.items {
		if topProbability(row.Dist) >= reviewThreshold {
			continue
		}
		out = append(out, copyReviewRow(row))
	}
	return out
}

// Correct updates the row label in memory and marks feedback as confirmed.
func (q *ReviewQueue) Correct(id, intent string) (ReviewRow, error) {
	if _, ok := validIntentSet[intent]; !ok {
		return ReviewRow{}, fmt.Errorf("bff: correct review %s: %w", id, ErrInvalidIntent)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.items {
		if q.items[i].ID != id {
			continue
		}
		if topProbability(q.items[i].Dist) >= reviewThreshold {
			return ReviewRow{}, fmt.Errorf("bff: correct review %s: %w", id, ErrReviewNotFound)
		}
		q.items[i].Intent = intent
		q.items[i].Corrected = true
		return copyReviewRow(q.items[i]), nil
	}
	return ReviewRow{}, fmt.Errorf("bff: correct review %s: %w", id, ErrReviewNotFound)
}

func copyReviewRow(row ReviewRow) ReviewRow {
	out := row
	if row.Dist != nil {
		out.Dist = make(map[string]float64, len(row.Dist))
		for k, v := range row.Dist {
			out.Dist[k] = v
		}
	}
	return out
}

func topProbability(dist map[string]float64) float64 {
	var top float64
	first := true
	for _, v := range dist {
		if first || v > top {
			top = v
			first = false
		}
	}
	return top
}

func topIntentLabel(dist map[string]float64) string {
	var label string
	var top float64
	first := true
	for k, v := range dist {
		if first || v > top {
			label = k
			top = v
			first = false
		}
	}
	return label
}

func seedReview() []ReviewRow {
	raw := []struct {
		id       string
		client   string
		text     string
		dist     map[string]float64
		ago      int
		fallback bool
	}{
		{
			id: "r01", client: "c05",
			text: "Preciso sacar 5 mil dólares e trazer de volta para o Brasil.",
			dist: map[string]float64{"Resgate": 0.54, "Câmbio": 0.31, "Operacional": 0.10, "Encerramento": 0.05},
			ago:  33,
		},
		{
			id: "r02", client: "c10",
			text: "Estou frustrado com a demora pra liberar a transferência. Alguém pode me explicar?",
			dist: map[string]float64{"Operacional": 0.58, "Reclamação": 0.35, "Contato": 0.07},
			ago:  18,
		},
		{
			id: "r03", client: "c21",
			text: "Quero mandar dinheiro pra minha filha que estuda fora.",
			dist: map[string]float64{"Câmbio": 0.41, "Operacional": 0.38, "Investimento": 0.12, "Resgate": 0.09},
			ago:  44,
		},
		{
			id: "r04", client: "c22",
			text: "Isso aqui não está batendo com o extrato.",
			dist: map[string]float64{"Operacional": 0.45, "Reclamação": 0.40, "Tributação": 0.15},
			ago:  71,
		},
		{
			id: "r05", client: "c01",
			text: "Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.",
			dist: map[string]float64{"Reclamação": 0.82, "Encerramento": 0.11, "Operacional": 0.04, "Resgate": 0.03},
			ago:  12,
		},
		{
			id: "r06", client: "c06",
			text: "Meu cartão foi recusado na viagem, o que eu faço?",
			dist: map[string]float64{"Operacional": 0.71, "Reclamação": 0.19, "Contato": 0.10},
			ago:  50, fallback: true,
		},
		{
			id: "r07", client: "c08",
			text: "Qual a cotação que vocês usam no câmbio? Está muito diferente do Google.",
			dist: map[string]float64{"Câmbio": 0.79, "Reclamação": 0.15, "Operacional": 0.06},
			ago:  120,
		},
		{
			id: "r85", client: "c01",
			text: "fixture at the review threshold",
			dist: map[string]float64{"Tributação": 0.85, "Operacional": 0.15},
			ago:  1,
		},
	}
	out := make([]ReviewRow, 0, len(raw))
	for _, row := range raw {
		if topProbability(row.dist) >= reviewThreshold {
			continue
		}
		out = append(out, ReviewRow{
			ID:       row.id,
			Client:   row.client,
			Text:     row.text,
			Dist:     row.dist,
			Ago:      row.ago,
			Fallback: row.fallback,
			Intent:   topIntentLabel(row.dist),
		})
	}
	return out
}
