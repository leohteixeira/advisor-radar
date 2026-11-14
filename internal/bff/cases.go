package bff

import "errors"

// CaseStates matches the board column labels by index.
var CaseStates = []string{"Aberto", "Em atendimento", "Aguardando cliente", "Resolvido"}

// ErrCaseNotFound is returned when a case id is unknown.
var ErrCaseNotFound = errors.New("case not found")

// ErrCustomerNotFound is returned when a customer id is unknown.
var ErrCustomerNotFound = errors.New("customer not found")

// CaseHistoryEntry is one line on a case card. Signal is the id of the queue
// signal the line is about, set when Abrir caso joined that signal to a case
// the client already had open.
type CaseHistoryEntry struct {
	Ago    int    `json:"ago"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Signal string `json:"signal,omitempty"`
}

// Case is one case card for the board columns. SlaTotal is the SLA in
// minutes and SlaRemaining the minutes left of it, negative once overdue, as
// cases computes them: the clock restarts when the client changes segment
// (ADR 0012), so SlaRemaining is not SlaTotal minus OpenedAgo. SlaState is
// cases' reading of that clock ("No prazo", "Vencendo", "Vencido" or
// "Concluído"); see withSLAState for an older cases binary. SlaHalved says the
// SLA is half the segment's base, so the card can explain why a Singular
// case counts 30 min next to "Singular · 1 h". Origin names the
// signal the case came from when the queue serves it, for example
// "mensagem · Reclamação"; the list routes fill it.
type Case struct {
	ID           string             `json:"id"`
	Client       string             `json:"client"`
	Name         string             `json:"name,omitempty"`
	Segment      string             `json:"segment,omitempty"`
	Signal       string             `json:"signal"`
	State        int                `json:"state"`
	OpenedAgo    int                `json:"openedAgo"`
	SlaTotal     int                `json:"slaTotal"`
	SlaRemaining int                `json:"slaRemaining"`
	SlaState     string             `json:"slaState"`
	SlaHalved    bool               `json:"slaHalved,omitempty"`
	Origin       string             `json:"origin,omitempty"`
	Escalated    bool               `json:"escalated"`
	History      []CaseHistoryEntry `json:"history"`
}
