// Package triage classifies inbound customer messages for the advisory team.
// The Classifier port has a Jev adapter, a keyword heuristic adapter and a
// Fallback decorator that degrades to the heuristic when Jev is unavailable.
package triage

import (
	"context"
	"regexp"
	"time"
)

type Intent string

const (
	IntentOperacional  Intent = "operacional"  // cadastro, acesso, app, documentos
	IntentCambio       Intent = "cambio"       // remessa, câmbio, wire in/out
	IntentTributacao   Intent = "tributacao"   // IR, DARF, informe de rendimentos
	IntentInvestimento Intent = "investimento" // dúvida sobre ativos e carteira
	IntentResgate      Intent = "resgate"      // saque, resgate, transferir saldo
	IntentReclamacao   Intent = "reclamacao"   // insatisfação com serviço ou cobrança
	IntentEncerramento Intent = "encerramento" // quer encerrar a conta
	IntentContato      Intent = "contato"      // pede retorno ou contato sem assunto definido
)

// ReviewIntentProb is the product gate for human review.
const ReviewIntentProb = 0.85

// Message is the state sent for classification. Only redacted text goes to the
// model; numbers and dates stay in our code (Jev is documented as weak on both).
type Message struct {
	ID       string   `json:"id"`
	Channel  string   `json:"channel"`
	Text     string   `json:"text"`
	Previous []string `json:"previous,omitempty"` // últimas mensagens do mesmo cliente
}

type Result struct {
	Intent       Intent
	IntentProb   float64 // probabilidade da intenção escolhida
	Frustration  float64 // 0 (calmo) a 3 (muito frustrado), interpolado
	ChurnRisk    float64 // P(cliente fala em sair ou levar o dinheiro)
	WantsHuman   float64 // P(cliente pede atendimento humano)
	Classifier   string  // "jev" ou "heuristic"
	ModelVersion string
	Degraded     bool
	Latency      time.Duration
	CostUSD      string
}

// NeedsReview decides whether a person must confirm the routing. Review is
// intent probability alone; a degraded flag does not force review.
func (r Result) NeedsReview(minIntentProb float64) bool {
	return r.IntentProb < minIntentProb
}

type Classifier interface {
	Classify(ctx context.Context, m Message) (Result, error)
}

var (
	slashDateRE = regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{2,4}`)
	digitRunRE  = regexp.MustCompile(`\d+`)
)

// RedactForModel strips slash-dates and digit runs so they never leave the process.
func RedactForModel(s string) string {
	s = slashDateRE.ReplaceAllString(s, "")
	return digitRunRE.ReplaceAllString(s, "")
}
