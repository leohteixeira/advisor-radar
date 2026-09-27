package triage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/jev"
)

// Questions are atomic on purpose: one judgment each, combined in code.
var jevQuestions = map[string]jev.Question{
	"intent": jev.Choice(
		"Qual é o assunto principal da mensagem deste cliente de uma corretora de investimentos internacionais?",
		map[string]string{
			string(IntentOperacional):  "acesso ao app, senha, cadastro, documentos ou atualização de dados",
			string(IntentCambio):       "envio ou recebimento de dinheiro do Brasil, câmbio, remessa ou wire",
			string(IntentTributacao):   "imposto de renda, DARF, informe de rendimentos ou declaração",
			string(IntentInvestimento): "dúvida sobre ações, ETFs, renda fixa, fundos ou composição da carteira",
			string(IntentResgate):      "sacar, resgatar ou transferir saldo da conta",
			string(IntentReclamacao):   "insatisfação com o atendimento, uma cobrança, um erro ou demora",
			string(IntentEncerramento): "pedido explícito para encerrar ou fechar a conta",
			string(IntentContato):      "pede retorno, ligação ou contato do assessor sem dizer o assunto",
		},
	),
	"frustration": jev.Score(
		"Qual o nível de frustração demonstrado pelo cliente nesta mensagem?",
		"calmo: tom neutro ou cordial",
		"incomodado: demonstra impaciência leve",
		"frustrado: reclama de forma clara ou cita tentativas anteriores sem sucesso",
		"muito frustrado: tom agressivo, ameaças ou indignação forte",
	),
	"churn_risk": jev.BooleanWith(
		"O cliente menciona sair da empresa, levar o dinheiro para outra instituição ou encerrar a conta?",
		"cita explicitamente sair, trocar de corretora, transferir tudo para outro lugar ou encerrar a conta",
		"não menciona deixar a empresa",
	),
	"wants_human": jev.BooleanWith(
		"O cliente pede para falar com uma pessoa, com o assessor ou com um atendente humano?",
		"pede explicitamente atendimento humano, ligação ou o assessor",
		"não faz esse pedido",
	),
}

type JevClassifier struct {
	client *jev.Client
}

func NewJevClassifier(c *jev.Client) *JevClassifier { return &JevClassifier{client: c} }

func (j *JevClassifier) Classify(ctx context.Context, m Message) (Result, error) {
	state := map[string]any{
		"canal":    m.Channel,
		"mensagem": RedactForModel(m.Text),
	}
	if len(m.Previous) > 0 {
		prev := make([]string, len(m.Previous))
		for i, p := range m.Previous {
			prev[i] = RedactForModel(p)
		}
		state["mensagens_anteriores"] = prev
	}

	start := time.Now()
	resp, err := j.client.Evaluate(ctx, state, jevQuestions)
	if err != nil {
		return Result{}, err
	}
	res := Result{
		Classifier:   "jev",
		ModelVersion: resp.Model,
		Latency:      time.Since(start),
		CostUSD:      gatewayCost(resp.ProviderMetadata),
	}

	intent, ok := resp.Answers["intent"]
	if !ok || intent.Choice == "" {
		return Result{}, fmt.Errorf("triage: jev response without intent answer")
	}
	res.Intent = Intent(intent.Choice)
	res.IntentProb = intent.Probabilities[intent.Choice]

	if a, ok := resp.Answers["frustration"]; ok && a.Score != nil {
		res.Frustration = *a.Score
	}
	if a, ok := resp.Answers["churn_risk"]; ok && a.Probability != nil {
		res.ChurnRisk = *a.Probability
	}
	if a, ok := resp.Answers["wants_human"]; ok && a.Probability != nil {
		res.WantsHuman = *a.Probability
	}
	return res, nil
}

func gatewayCost(meta json.RawMessage) string {
	var m struct {
		Gateway struct {
			Cost string `json:"cost"`
		} `json:"gateway"`
	}
	if len(meta) == 0 || json.Unmarshal(meta, &m) != nil {
		return ""
	}
	return m.Gateway.Cost
}
