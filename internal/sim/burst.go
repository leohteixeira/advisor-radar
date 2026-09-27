// Package sim holds account-sim payload types. Market-day cast lives in SQL seeds.
package sim

// MessagePayload is the outbox payload for a customer message.
type MessagePayload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

// AccountPayload is the outbox payload for a fictional account fact.
type AccountPayload struct {
	Kind   string  `json:"kind"`
	Amount float64 `json:"amount"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
}
