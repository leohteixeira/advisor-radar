package triage

import (
	"context"
	"errors"
	"strings"
	"time"
)

// HeuristicClassifier is a keyword baseline. It serves as fallback when Jev is
// down and as the baseline the Jev results are compared against.
type HeuristicClassifier struct{}

var intentKeywords = []struct {
	intent Intent
	words  []string
}{
	// order matters: stronger signals first
	{IntentEncerramento, []string{"encerrar", "fechar minha conta", "cancelar a conta", "encerramento"}},
	{IntentReclamacao, []string{"absurdo", "pessimo", "reclamacao", "reclame aqui", "descaso", "cobrado duas", "cobranca indevida", "ninguem resolve", "outra corretora", "trocar de corretora", "transferir tudo", "nao entende"}},
	{IntentTributacao, []string{"imposto", "darf", "informe de rendimentos", "declaracao", "receita federal", " ir "}},
	{IntentCambio, []string{"remessa", "cambio", "wire", "enviar dinheiro", "mandar dinheiro", "transferencia internacional", "spread", "nao caiu"}},
	{IntentResgate, []string{"sacar", "saque", "resgatar", "resgate", "retirar"}},
	{IntentInvestimento, []string{"acao", "acoes", "etf", "carteira", "renda fixa", "treasury", "fundo", "dividendo", "reit"}},
	{IntentOperacional, []string{"senha", "login", "acesso", "aplicativo", "app", "cadastro", "documento", "token"}},
	{IntentContato, []string{"me ligar", "me liga", "retorno", "entrar em contato"}},
}

var (
	churnWords       = []string{"outra corretora", "trocar de corretora", "outro banco", "vou sair", "levar meu dinheiro", "tirar tudo", "encerrar", "concorrente"}
	humanWords       = []string{"falar com alguem", "falar com uma pessoa", "atendente", "humano", "meu assessor", "me liga", "ligacao"}
	frustrationWords = []string{"absurdo", "pessimo", "vergonha", "de novo", "ninguem", "cansado", "!!", "descaso", "ate agora"}
)

func (HeuristicClassifier) Classify(_ context.Context, m Message) (Result, error) {
	start := time.Now()
	text := " " + normalize(m.Text) + " "

	res := Result{Intent: IntentOperacional, IntentProb: 0.3, Classifier: "heuristic", ModelVersion: "keywords-v1"}
	for _, k := range intentKeywords {
		if containsAny(text, k.words) {
			res.Intent, res.IntentProb = k.intent, 0.6
			break
		}
	}
	if containsAny(text, churnWords) {
		res.ChurnRisk = 0.8
	}
	if containsAny(text, humanWords) {
		res.WantsHuman = 0.8
	}
	res.Frustration = min(3, float64(countAny(text, frustrationWords)))
	res.Latency = time.Since(start)
	return res, nil
}

// Fallback decorates a primary classifier and degrades to the secondary on any
// error. When Timeout is positive it bounds only the primary call.
type Fallback struct {
	Primary   Classifier
	Secondary Classifier
	Timeout   time.Duration
	OnDegrade func(m Message, err error) // metrics and logs hook
}

func (f Fallback) Classify(ctx context.Context, m Message) (Result, error) {
	pctx := ctx
	cancel := func() {}
	if f.Timeout > 0 {
		pctx, cancel = context.WithTimeout(ctx, f.Timeout)
	}
	defer cancel()

	res, err := f.Primary.Classify(pctx, m)
	if err == nil {
		return res, nil
	}
	if ctx.Err() != nil { // caller gave up, not the provider
		return Result{}, ctx.Err()
	}
	if f.OnDegrade != nil {
		f.OnDegrade(m, err)
	}
	res, err2 := f.Secondary.Classify(ctx, m)
	if err2 != nil {
		return Result{}, errors.Join(err, err2)
	}
	res.Degraded = true
	return res, nil
}

var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "ê", "e", "è", "e",
	"í", "i", "î", "i",
	"ó", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ü", "u",
	"ç", "c",
)

// normalize lowercases and strips Portuguese diacritics with the stdlib only.
func normalize(s string) string {
	return accents.Replace(strings.ToLower(s))
}

func containsAny(text string, words []string) bool { return countAny(text, words) > 0 }

func countAny(text string, words []string) int {
	n := 0
	for _, w := range words {
		if strings.Contains(text, w) {
			n++
		}
	}
	return n
}
