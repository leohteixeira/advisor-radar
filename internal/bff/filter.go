package bff

import (
	"cmp"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Queue query labels match the Portuguese UI filter chips.
const (
	andamentoNovoSinal = "Novo sinal"
	andamentoAdiados   = "Adiados"

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

// andamentoOptions is the Andamento menu: new signals, snoozed signals, then
// the case states in CaseStates order.
var andamentoOptions = []string{
	andamentoNovoSinal, andamentoAdiados, "Aberto", "Em atendimento", "Aguardando cliente", "Resolvido",
}

// caseStateResolved is the index of "Resolvido" in CaseStates.
const caseStateResolved = 3

var segmentoOptions = []string{"Essencial", "Advance", "Singular"}

// ListFilters holds repeated query params for queue and cases list endpoints.
// Query is folded (see fold), so the name search ignores case and accents.
type ListFilters struct {
	Query     string
	Andamento []string
	SLA       []string
	Sinal     []string
	Motivo    []string
	Segmento  []string
}

// FacetCount is one filter value and the number of cards, signals plus cases,
// the board shows when that value is the only one picked in its dimension and
// every other filter stays as requested.
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

// card is one board card, a signal or a case, reduced to what the filters
// compare. Both list routes filter and count the same cards, so a facet count
// is the number of cards its filter shows.
type card struct {
	andamento string
	name      string // folded
	segment   string
	slaState  string
	// origin is the signal the card is about: the signal itself, or the one a
	// case was opened from. It is nil for a case whose signal the queue does
	// not serve.
	origin *scoredSignal
}

// listing is what GET /v1/queue and GET /v1/cases filter and count: the
// signals the queue lists (snoozed ones included, marked by SnoozedUntil) and
// every case.
type listing struct {
	signals []scoredSignal
	cases   []scoredCase
	origins map[string]scoredSignal
}

func parseListFilters(q url.Values) ListFilters {
	return ListFilters{
		Query:     fold(strings.TrimSpace(q.Get("q"))),
		Andamento: q["andamento"],
		SLA:       q["sla"],
		Sinal:     q["sinal"],
		Motivo:    q["motivo"],
		Segmento:  q["segmento"],
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

// fold lowers s and strips the accents of Portuguese letters, so "sergio"
// matches "Sérgio". Combining marks are dropped too, for names that arrive
// decomposed.
func fold(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'á', 'à', 'â', 'ã', 'ä':
			return 'a'
		case 'é', 'è', 'ê', 'ë':
			return 'e'
		case 'í', 'ì', 'î', 'ï':
			return 'i'
		case 'ó', 'ò', 'ô', 'õ', 'ö':
			return 'o'
		case 'ú', 'ù', 'û', 'ü':
			return 'u'
		case 'ç':
			return 'c'
		case 'ñ':
			return 'n'
		}
		if r >= 0x0300 && r <= 0x036F {
			return -1
		}
		return r
	}, strings.ToLower(s))
}

// withSLAState returns c with its SLA state set. cases decides the state; an
// older cases binary sends none, and only then the BFF applies the same
// formula to the remaining minutes cases reported.
func withSLAState(c Case) Case {
	if c.SlaState != "" {
		return c
	}
	switch {
	case c.State == caseStateResolved:
		c.SlaState = slaConcluido
	case c.SlaRemaining < 0:
		c.SlaState = slaVencido
	case c.SlaRemaining < max(c.SlaTotal*33/100, 20):
		c.SlaState = slaVencendo
	default:
		c.SlaState = slaNoPrazo
	}
	return c
}

func scoreCase(c Case) scoredCase {
	c = withSLAState(c)
	segment := c.Segment
	if segment == "" {
		segment = "Essencial"
	}
	name := c.Name
	if name == "" {
		name = c.Client
	}
	return scoredCase{caseItem: c, name: name, segment: segment, slaState: c.SlaState, rem: c.SlaRemaining}
}

// originLabel is the line a case card shows for the signal it came from, for
// example "mensagem · Reclamação" or "saque relevante".
func originLabel(row scoredSignal) string {
	if row.signal.Kind != "message" {
		return strings.ToLower(row.typeLabel)
	}
	if row.signal.Intent == "" {
		return "mensagem"
	}
	return "mensagem · " + row.signal.Intent
}

// newListing joins the signals the queue serves with the advisor's actions
// and the cases.
//
// A signal that opened or joined a case is represented by that case and is
// not listed: any case, of any state, whose signal or history names the
// signal's id or its source message id hides it. A snoozed signal stays
// listed with SnoozedUntil set, so only the Adiados filter shows it.
func newListing(signals []Signal, actions []SignalAction, cases []Case, now time.Time) listing {
	inCase := make(map[string]struct{}, len(cases))
	for _, c := range cases {
		inCase[c.Signal] = struct{}{}
		for _, line := range c.History {
			if line.Signal != "" {
				inCase[line.Signal] = struct{}{}
			}
		}
	}
	delete(inCase, "")
	actionOf := make(map[string]SignalAction, len(actions))
	for _, a := range actions {
		actionOf[a.SignalID] = a
	}

	out := listing{
		signals: make([]scoredSignal, 0, len(signals)),
		cases:   make([]scoredCase, 0, len(cases)),
		origins: make(map[string]scoredSignal, len(signals)),
	}
	for _, sig := range signals {
		if a, ok := actionOf[sig.ID]; ok {
			sig.ContactedAt = a.ContactedAt
			if a.SnoozedUntil != nil && a.SnoozedUntil.After(now) {
				sig.SnoozedUntil = a.SnoozedUntil
			}
		}
		row := scoreSignal(sig)
		out.origins[sig.ID] = row
		_, byID := inCase[sig.ID]
		_, bySource := inCase[sig.SourceID]
		if sig.SourceID != "" {
			out.origins[sig.SourceID] = row
		}
		if byID || bySource {
			continue
		}
		out.signals = append(out.signals, row)
	}
	for _, c := range cases {
		row := scoreCase(c)
		if origin, ok := out.origins[c.Signal]; ok {
			row.caseItem.Origin = originLabel(origin)
		}
		out.cases = append(out.cases, row)
	}
	return out
}

func (l listing) signalCard(row scoredSignal) card {
	andamento := andamentoNovoSinal
	if row.signal.SnoozedUntil != nil {
		andamento = andamentoAdiados
	}
	return card{andamento: andamento, name: fold(row.name), segment: row.segment, slaState: row.slaState, origin: &row}
}

func (l listing) caseCard(row scoredCase) card {
	c := card{name: fold(row.name), segment: row.segment, slaState: row.slaState}
	if state := row.caseItem.State; state >= 0 && state < len(CaseStates) {
		c.andamento = CaseStates[state]
	}
	if origin, ok := l.origins[row.caseItem.Signal]; ok {
		c.origin = &origin
	}
	return c
}

func (l listing) cards() []card {
	out := make([]card, 0, len(l.signals)+len(l.cases))
	for _, row := range l.signals {
		out = append(out, l.signalCard(row))
	}
	for _, row := range l.cases {
		out = append(out, l.caseCard(row))
	}
	return out
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

// has reports whether the card carries value label in dimension dim.
func (c card) has(dim filterDim, label string) bool {
	switch dim {
	case dimAndamento:
		return c.andamento == label
	case dimSLA:
		return c.slaState == label
	case dimSegmento:
		return c.segment == label
	case dimSinal:
		return c.origin != nil && sinalOn(*c.origin, label)
	case dimMotivo:
		return c.origin != nil && (c.origin.typeLabel == label || c.origin.signal.Intent == label)
	default:
		return false
	}
}

func (f ListFilters) values(dim filterDim) []string {
	switch dim {
	case dimAndamento:
		return f.Andamento
	case dimSLA:
		return f.SLA
	case dimSinal:
		return f.Sinal
	case dimMotivo:
		return f.Motivo
	case dimSegmento:
		return f.Segmento
	default:
		return nil
	}
}

var filterDims = []filterDim{dimAndamento, dimSLA, dimSinal, dimMotivo, dimSegmento}

// matches reports whether the board shows the card under f. skip leaves one
// dimension out, which is how a facet counts its own values. Without an
// Andamento filter the board shows new signals and open cases: snoozed
// signals and resolved cases appear only when asked for.
func (c card) matches(f ListFilters, skip filterDim) bool {
	if f.Query != "" && !strings.Contains(c.name, f.Query) {
		return false
	}
	for _, dim := range filterDims {
		if dim == skip {
			continue
		}
		picked := f.values(dim)
		if len(picked) == 0 {
			if dim == dimAndamento && (c.andamento == andamentoAdiados || c.andamento == CaseStates[caseStateResolved]) {
				return false
			}
			continue
		}
		if !slices.ContainsFunc(picked, func(label string) bool { return c.has(dim, label) }) {
			return false
		}
	}
	return true
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

// filteredSignals is the GET /v1/queue list: highest score first.
func (l listing) filteredSignals(f ListFilters) []Signal {
	rows := make([]scoredSignal, 0, len(l.signals))
	for _, row := range l.signals {
		if l.signalCard(row).matches(f, "") {
			rows = append(rows, row)
		}
	}
	slices.SortStableFunc(rows, func(a, b scoredSignal) int {
		return cmp.Compare(b.score, a.score)
	})
	out := make([]Signal, len(rows))
	for i, row := range rows {
		out[i] = row.signal
	}
	return out
}

// filteredCases is the GET /v1/cases list: least time left first.
func (l listing) filteredCases(f ListFilters) []Case {
	rows := make([]scoredCase, 0, len(l.cases))
	for _, row := range l.cases {
		if l.caseCard(row).matches(f, "") {
			rows = append(rows, row)
		}
	}
	slices.SortStableFunc(rows, func(a, b scoredCase) int {
		return cmp.Compare(a.rem, b.rem)
	})
	out := make([]Case, len(rows))
	for i, row := range rows {
		out[i] = row.caseItem
	}
	return out
}

// facets counts, for every filter value, the cards the board shows with that
// value picked. Both list routes answer the same facets.
func (l listing) facets(f ListFilters) ListFacets {
	cards := l.cards()
	count := func(dim filterDim, labels []string) []FacetCount {
		out := make([]FacetCount, 0, len(labels))
		for _, label := range labels {
			n := 0
			for _, c := range cards {
				if c.has(dim, label) && c.matches(f, dim) {
					n++
				}
			}
			out = append(out, FacetCount{Label: label, Count: n})
		}
		return out
	}
	return ListFacets{
		Andamento: count(dimAndamento, andamentoOptions),
		SLA:       count(dimSLA, slaOptions),
		Sinal:     count(dimSinal, sinalOptions),
		Motivo:    count(dimMotivo, motivoOptionLabels()),
		Segmento:  count(dimSegmento, segmentoOptions),
	}
}

func filtersFromRequest(r *http.Request) ListFilters {
	return parseListFilters(r.URL.Query())
}
