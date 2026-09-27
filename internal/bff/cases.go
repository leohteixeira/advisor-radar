package bff

import "errors"

// CaseStates matches the board column labels by index.
var CaseStates = []string{"Aberto", "Em atendimento", "Aguardando cliente", "Resolvido"}

// ErrCaseNotFound is returned when a case id is unknown.
var ErrCaseNotFound = errors.New("case not found")

// ErrCustomerNotFound is returned when a customer id is unknown.
var ErrCustomerNotFound = errors.New("customer not found")

// CaseHistoryEntry is one line on a case card.
type CaseHistoryEntry struct {
	Ago  int    `json:"ago"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Case is one case card for the board columns.
type Case struct {
	ID        string             `json:"id"`
	Client    string             `json:"client"`
	Name      string             `json:"name,omitempty"`
	Segment   string             `json:"segment,omitempty"`
	Signal    string             `json:"signal"`
	State     int                `json:"state"`
	OpenedAgo int                `json:"openedAgo"`
	SlaTotal  int                `json:"slaTotal"`
	Escalated bool               `json:"escalated"`
	History   []CaseHistoryEntry `json:"history"`
}
