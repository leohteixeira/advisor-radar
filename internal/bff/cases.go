package bff

import "sync"

// CaseStates matches mock-data.js CASE_STATES by index.
var CaseStates = []string{"Aberto", "Em atendimento", "Aguardando cliente", "Resolvido"}

// CaseHistoryEntry is one line on a case card.
type CaseHistoryEntry struct {
	Ago  int    `json:"ago"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Case is one seed case card for the board columns.
type Case struct {
	ID        string             `json:"id"`
	Client    string             `json:"client"`
	Signal    string             `json:"signal"`
	State     int                `json:"state"`
	OpenedAgo int                `json:"openedAgo"`
	SlaTotal  int                `json:"slaTotal"`
	Escalated bool               `json:"escalated"`
	History   []CaseHistoryEntry `json:"history"`
}

// CaseBoard holds the in-memory seed cases for one BFF process.
type CaseBoard struct {
	mu    sync.Mutex
	items []Case
}

// NewCaseBoard returns the three design seed cases.
func NewCaseBoard() *CaseBoard {
	return &CaseBoard{items: seedCases()}
}

// Items returns a copy of the seed cases.
func (c *CaseBoard) Items() []Case {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Case, len(c.items))
	copy(out, c.items)
	for i := range out {
		if c.items[i].History != nil {
			out[i].History = make([]CaseHistoryEntry, len(c.items[i].History))
			copy(out[i].History, c.items[i].History)
		}
	}
	return out
}

// Advance moves a case to the next state when it is not Resolvido.
func (c *CaseBoard) Advance(id string) (Case, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.items {
		if c.items[i].ID != id {
			continue
		}
		if c.items[i].State < len(CaseStates)-1 {
			c.items[i].State++
		}
		out := c.items[i]
		if out.History != nil {
			out.History = make([]CaseHistoryEntry, len(c.items[i].History))
			copy(out.History, c.items[i].History)
		}
		return out, true
	}
	return Case{}, false
}

func seedCases() []Case {
	return []Case{
		{
			ID: "k1042", Client: "c02", Signal: "s02", State: 1, OpenedAgo: 25, SlaTotal: 120, Escalated: false,
			History: []CaseHistoryEntry{
				{Ago: 25, Kind: "caso", Text: "Caso aberto a partir de mensagem com reclamação"},
				{Ago: 21, Kind: "caso", Text: "Em atendimento · Ana Paula Ribeiro"},
				{Ago: 19, Kind: "nota", Text: "Problema é o bloqueio da transferência internacional desde 12/09. Compliance pediu comprovante de origem."},
			},
		},
		{
			ID: "k1038", Client: "c07", Signal: "s07", State: 2, OpenedAgo: 65, SlaTotal: 120, Escalated: true,
			History: []CaseHistoryEntry{
				{Ago: 65, Kind: "caso", Text: "Caso aberto a partir de pedido de encerramento"},
				{Ago: 60, Kind: "caso", Text: "Escalonado automaticamente: risco de saída alto em cliente Advance"},
				{Ago: 58, Kind: "telefone", Text: "Ligação · 9 min · cliente insatisfeita com taxas de câmbio"},
				{Ago: 55, Kind: "caso", Text: "Aguardando cliente · proposta de isenção enviada por e-mail"},
			},
		},
		{
			ID: "k1031", Client: "c09", Signal: "s09", State: 3, OpenedAgo: 400, SlaTotal: 60, Escalated: false,
			History: []CaseHistoryEntry{
				{Ago: 400, Kind: "caso", Text: "Caso aberto"},
				{Ago: 380, Kind: "telefone", Text: "Ligação · 14 min · sugestão de Treasuries 2028"},
				{Ago: 370, Kind: "caso", Text: "Resolvido"},
			},
		},
	}
}
