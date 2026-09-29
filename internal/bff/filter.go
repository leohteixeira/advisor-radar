package bff

import (
	"cmp"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Queue query labels match the Portuguese UI filter chips.
const (
	andamentoNovoSinal = "Novo sinal"

	slaNoPrazo   = "No prazo"
	slaVencendo  = "Vencendo"
	slaVencido   = "Vencido"
	slaConcluido = "Concluído"

	sinalRiscoSaida   = "Risco de saída"
	sinalPedidoHumano = "Pedido humano"
	sinalPrecisaRev   = "Precisa de revisão"
	sinalClassifSimp  = "Classificação simplificada"
)

var segmentSLA = map[string]int{
	"Essencial": 1440,
	"Advance":   240,
	"Singular":  60,
}

var alertLabels = map[string]string{
	"saque":    "Saque relevante",
	"queda":    "Queda de patrimônio",
	"aporte":   "Aporte grande",
	"segmento": "Mudança de segmento",
	"contato":  "Sem contato há muito tempo",
	"perfil":   "Compra acima do perfil",
	"risco":    "Mensagem com risco",
}

var intentOptions = []string{
	"Operacional", "Câmbio", "Tributação", "Investimento",
	"Resgate", "Reclamação", "Encerramento", "Contato",
}

var sinalOptions = []string{
	sinalRiscoSaida, sinalPedidoHumano, sinalPrecisaRev, sinalClassifSimp,
}

var slaOptions = []string{slaNoPrazo, slaVencendo, slaVencido}

var andamentoOptions = []string{
	andamentoNovoSinal, "Aberto", "Em atendimento", "Aguardando cliente", "Resolvido",
}

var segmentoOptions = []string{"Essencial", "Advance", "Singular"}

// ListFilters holds repeated query params for queue and cases list endpoints.
type ListFilters struct {
	Query     string
	Andamento []string
	SLA       []string
	Sinal     []string
	Motivo    []string
	Segmento  []string
}

// FacetCount is one option with a count that ignores that dimension's own filter.
type FacetCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// ListFacets groups facet counts by filter dimension.
type ListFacets struct {
	Andamento []FacetCount `json:"andamento"`
	SLA       []FacetCount `json:"sla"`
	Sinal     []FacetCount `json:"sinal"`
	Motivo    []FacetCount `json:"motivo"`
	Segmento  []FacetCount `json:"segmento"`
}

type filterDim string

const (
	dimAndamento filterDim = "andamento"
	dimSLA       filterDim = "sla"
	dimSinal     filterDim = "sinal"
	dimMotivo    filterDim = "motivo"
	dimSegmento  filterDim = "segmento"
)

type scoredSignal struct {
	signal      Signal
	name        string
	segment     string
	slaState    string
	typeLabel   string
	needsReview bool
	score       int
	rem         int
}

type scoredCase struct {
	caseItem Case
	name     string
	segment  string
	slaState string
	rem      int
}

func parseListFilters(q url.Values) ListFilters {
	return ListFilters{
		Query:     strings.TrimSpace(q.Get("q")),
		Andamento: q["andamento"],
		SLA:       q["sla"],
		Sinal:     q["sinal"],
		Motivo:    q["motivo"],
		Segmento:  q["segmento"],
	}
}

func (f ListFilters) has(dim filterDim) bool {
	switch dim {
	case dimAndamento:
		return len(f.Andamento) > 0
	case dimSLA:
		return len(f.SLA) > 0
	case dimSinal:
		return len(f.Sinal) > 0
	case dimMotivo:
		return len(f.Motivo) > 0
	case dimSegmento:
		return len(f.Segmento) > 0
	default:
		return false
	}
}

func ptrBool(p *bool) bool {
	return p != nil && *p
}

func ptrInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func confidenceOf(dist map[string]float64, fallback bool) string {
	if fallback {
		return "média"
	}
	if len(dist) == 0 {
		return ""
	}
	top := 0.0
	for _, v := range dist {
		if v > top {
			top = v
		}
	}
	switch {
	case top >= 0.75:
		return "alta"
	case top >= 0.5:
		return "média"
	default:
		return "baixa"
	}
}

func signalSLA(sig Signal, segment string) (rem int, state string) {
	total, ok := segmentSLA[segment]
	if !ok {
		total = segmentSLA["Essencial"]
	}
	frustration := ptrInt(sig.Frustration)
	if ptrBool(sig.Churn) || frustration >= 2 || ptrBool(sig.Human) || sig.Alert == "saque" {
		total = total / 2
	}
	rem = total - sig.Ago
	switch {
	case rem < 0:
		state = slaVencido
	case rem < max(total*33/100, 20):
		state = slaVencendo
	default:
		state = slaNoPrazo
	}
	return rem, state
}

func scoreSignal(sig Signal) scoredSignal {
	segment := sig.Segment
	if segment == "" {
		segment = "Essencial"
	}
	name := sig.Name
	if name == "" {
		name = sig.Client
	}
	rem, slaState := signalSLA(sig, segment)
	isMsg := sig.Kind == "message"
	frustration := ptrInt(sig.Frustration)
	conf := ""
	if isMsg {
		conf = confidenceOf(sig.Dist, sig.Fallback)
	}
	typeLabel := "Alerta"
	if isMsg {
		if ptrBool(sig.Churn) || frustration >= 2 {
			typeLabel = "Mensagem com risco"
		} else {
			typeLabel = "Mensagem"
		}
	} else if label, ok := alertLabels[sig.Alert]; ok {
		typeLabel = label
	}
	needsReview := isMsg && conf != "" && conf != "alta"
	score := 0
	if ptrBool(sig.Churn) {
		score += 100
	}
	if ptrBool(sig.Human) {
		score += 30
	}
	score += frustration * 15
	if rem < 0 {
		score += 60
	} else if slaState == slaVencendo {
		score += 40
	}
	switch segment {
	case "Singular":
		score += 20
	case "Advance":
		score += 10
	}
	if sig.Alert == "saque" {
		score += 25
	}
	return scoredSignal{
		signal: sig, name: name, segment: segment, slaState: slaState,
		typeLabel: typeLabel, needsReview: needsReview, score: score, rem: rem,
	}
}

func scoreCase(c Case) scoredCase {
	segment := c.Segment
	if segment == "" {
		segment = "Essencial"
	}
	name := c.Name
	if name == "" {
		name = c.Client
	}
	done := c.State == 3
	rem := c.SlaTotal - c.OpenedAgo
	var slaState string
	switch {
	case done:
		slaState = slaConcluido
	case rem < 0:
		slaState = slaVencido
	case rem < max(c.SlaTotal*33/100, 20):
		slaState = slaVencendo
	default:
		slaState = slaNoPrazo
	}
	return scoredCase{caseItem: c, name: name, segment: segment, slaState: slaState, rem: rem}
}

func sinalOn(row scoredSignal, option string) bool {
	switch option {
	case sinalRiscoSaida:
		return ptrBool(row.signal.Churn)
	case sinalPedidoHumano:
		return ptrBool(row.signal.Human)
	case sinalPrecisaRev:
		return row.needsReview
	default:
		return row.signal.Fallback
	}
}

func signalMatches(row scoredSignal, f ListFilters, skip filterDim) bool {
	q := strings.ToLower(strings.TrimSpace(f.Query))
	if q != "" && !strings.Contains(strings.ToLower(row.name), q) {
		return false
	}
	if skip != dimSinal && f.has(dimSinal) {
		ok := false
		for _, o := range f.Sinal {
			if sinalOn(row, o) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if skip != dimSLA && f.has(dimSLA) && !slices.Contains(f.SLA, row.slaState) {
		return false
	}
	if skip != dimMotivo && f.has(dimMotivo) {
		if !slices.Contains(f.Motivo, row.typeLabel) && !slices.Contains(f.Motivo, row.signal.Intent) {
			return false
		}
	}
	if skip != dimSegmento && f.has(dimSegmento) && !slices.Contains(f.Segmento, row.segment) {
		return false
	}
	return true
}

func caseMatches(row scoredCase, bySignal map[string]scoredSignal, f ListFilters, skip filterDim) bool {
	q := strings.ToLower(strings.TrimSpace(f.Query))
	if q != "" && !strings.Contains(strings.ToLower(row.name), q) {
		return false
	}
	if skip != dimSLA && f.has(dimSLA) && !slices.Contains(f.SLA, row.slaState) {
		return false
	}
	if skip != dimSegmento && f.has(dimSegmento) && !slices.Contains(f.Segmento, row.segment) {
		return false
	}
	sig, ok := bySignal[row.caseItem.Signal]
	if skip != dimSinal && f.has(dimSinal) {
		if !ok {
			return false
		}
		hit := false
		for _, o := range f.Sinal {
			if sinalOn(sig, o) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if skip != dimMotivo && f.has(dimMotivo) {
		if !ok {
			return false
		}
		if !slices.Contains(f.Motivo, sig.typeLabel) && !slices.Contains(f.Motivo, sig.signal.Intent) {
			return false
		}
	}
	return true
}

func andamentoAllowsSignal(f ListFilters) bool {
	if !f.has(dimAndamento) {
		return true
	}
	return slices.Contains(f.Andamento, andamentoNovoSinal)
}

func andamentoAllowsCase(f ListFilters, state int) bool {
	if !f.has(dimAndamento) {
		return state < 3
	}
	if state < 0 || state >= len(CaseStates) {
		return false
	}
	return slices.Contains(f.Andamento, CaseStates[state])
}

func motivoOptionLabels() []string {
	alertKinds := []string{"saque", "queda", "aporte", "segmento", "contato", "perfil"}
	out := make([]string, 0, len(alertKinds)+len(intentOptions))
	for _, k := range alertKinds {
		if label, ok := alertLabels[k]; ok {
			out = append(out, label)
		}
	}
	out = append(out, intentOptions...)
	return out
}

func filterAndSortSignals(items []Signal, f ListFilters) []Signal {
	scored := make([]scoredSignal, 0, len(items))
	for _, it := range items {
		row := scoreSignal(it)
		if !andamentoAllowsSignal(f) {
			continue
		}
		if !signalMatches(row, f, "") {
			continue
		}
		scored = append(scored, row)
	}
	slices.SortFunc(scored, func(a, b scoredSignal) int {
		return cmp.Compare(b.score, a.score)
	})
	out := make([]Signal, len(scored))
	for i, row := range scored {
		out[i] = row.signal
	}
	return out
}

func filterAndSortCases(items []Case, signals []Signal, f ListFilters) []Case {
	bySignal := make(map[string]scoredSignal, len(signals))
	for _, s := range signals {
		bySignal[s.ID] = scoreSignal(s)
	}
	scored := make([]scoredCase, 0, len(items))
	for _, it := range items {
		row := scoreCase(it)
		if !andamentoAllowsCase(f, row.caseItem.State) {
			continue
		}
		if !caseMatches(row, bySignal, f, "") {
			continue
		}
		scored = append(scored, row)
	}
	slices.SortFunc(scored, func(a, b scoredCase) int {
		return cmp.Compare(a.rem, b.rem)
	})
	out := make([]Case, len(scored))
	for i, row := range scored {
		out[i] = row.caseItem
	}
	return out
}

func buildSignalFacets(items []Signal, cases []Case, f ListFilters) ListFacets {
	scored := make([]scoredSignal, len(items))
	for i, it := range items {
		scored[i] = scoreSignal(it)
	}
	bySignal := make(map[string]scoredSignal, len(items))
	for _, row := range scored {
		bySignal[row.signal.ID] = row
	}
	scoredCases := make([]scoredCase, len(cases))
	for i, c := range cases {
		scoredCases[i] = scoreCase(c)
	}

	andamento := make([]FacetCount, 0, len(andamentoOptions))
	for i, label := range andamentoOptions {
		count := 0
		if i == 0 {
			for _, row := range scored {
				if signalMatches(row, f, dimAndamento) {
					count++
				}
			}
		} else {
			state := i - 1
			for _, row := range scoredCases {
				if row.caseItem.State == state && caseMatches(row, bySignal, f, dimAndamento) {
					count++
				}
			}
		}
		andamento = append(andamento, FacetCount{Label: label, Count: count})
	}

	sla := make([]FacetCount, 0, len(slaOptions))
	for _, label := range slaOptions {
		count := 0
		for _, row := range scored {
			if row.slaState == label && signalMatches(row, f, dimSLA) {
				count++
			}
		}
		sla = append(sla, FacetCount{Label: label, Count: count})
	}

	sinal := make([]FacetCount, 0, len(sinalOptions))
	for _, label := range sinalOptions {
		count := 0
		for _, row := range scored {
			if !sinalOn(row, label) {
				continue
			}
			if signalMatches(row, f, dimSinal) {
				count++
			}
		}
		sinal = append(sinal, FacetCount{Label: label, Count: count})
	}

	motivos := motivoOptionLabels()
	motivo := make([]FacetCount, 0, len(motivos))
	for _, label := range motivos {
		count := 0
		for _, row := range scored {
			if row.typeLabel != label && row.signal.Intent != label {
				continue
			}
			if signalMatches(row, f, dimMotivo) {
				count++
			}
		}
		motivo = append(motivo, FacetCount{Label: label, Count: count})
	}

	segmento := make([]FacetCount, 0, len(segmentoOptions))
	for _, label := range segmentoOptions {
		count := 0
		for _, row := range scored {
			if row.segment == label && signalMatches(row, f, dimSegmento) {
				count++
			}
		}
		segmento = append(segmento, FacetCount{Label: label, Count: count})
	}

	return ListFacets{
		Andamento: andamento,
		SLA:       sla,
		Sinal:     sinal,
		Motivo:    motivo,
		Segmento:  segmento,
	}
}

func filtersFromRequest(r *http.Request) ListFilters {
	return parseListFilters(r.URL.Query())
}
