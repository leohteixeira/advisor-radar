package bff

import "errors"

// ErrInvalidIntent is returned when the corrected intent is not one of the eight labels.
var ErrInvalidIntent = errors.New("invalid intent")

// ErrReviewNotFound is returned when the review row id is unknown.
var ErrReviewNotFound = errors.New("review row not found")

// ValidIntents are the eight Portuguese intent labels.
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

// ReviewRow is one analyst review queue item.
type ReviewRow struct {
	ID        string             `json:"id"`
	Client    string             `json:"client"`
	Name      string             `json:"name,omitempty"`
	Segment   string             `json:"segment,omitempty"`
	Text      string             `json:"text"`
	Dist      map[string]float64 `json:"dist"`
	Ago       int                `json:"ago"`
	Fallback  bool               `json:"fallback,omitempty"`
	Intent    string             `json:"intent"`
	Corrected bool               `json:"corrected,omitempty"`
}
