// Package sim builds the fictional market-day burst from the design seed.
package sim

import (
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// MarketDayOccurredAt is the fixed occurred_at for every STREAM row.
var MarketDayOccurredAt = time.Date(2026, 9, 27, 15, 0, 0, 0, time.UTC)

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

// MarketDay returns the six STREAM envelopes in arrival order (n01–n06).
func MarketDay() []event.Envelope {
	return []event.Envelope{
		{
			Name:          event.NameMessageReceived,
			EventID:       "md-n01",
			OccurredAt:    MarketDayOccurredAt,
			CustomerID:    "c18",
			SchemaVersion: event.SchemaVersionMVP,
			Payload: MessagePayload{
				Channel: "email",
				Text:    "Ninguém me responde há dois dias. Vou abrir reclamação no Reclame Aqui.",
			},
		},
		{
			Name:          event.NameAccountEventRecorded,
			EventID:       "md-n02",
			OccurredAt:    MarketDayOccurredAt,
			CustomerID:    "c19",
			SchemaVersion: event.SchemaVersionMVP,
			Payload: AccountPayload{
				Kind:   "withdrawal",
				Amount: 55000,
				Before: 196000,
				After:  141000,
			},
		},
		{
			Name:          event.NameMessageReceived,
			EventID:       "md-n03",
			OccurredAt:    MarketDayOccurredAt,
			CustomerID:    "c17",
			SchemaVersion: event.SchemaVersionMVP,
			Payload: MessagePayload{
				Channel: "chat",
				Text:    "Bom dia! Como faço para ver o informe de rendimentos de 2025?",
			},
		},
		{
			Name:          event.NameMessageReceived,
			EventID:       "md-n04",
			OccurredAt:    MarketDayOccurredAt,
			CustomerID:    "c20",
			SchemaVersion: event.SchemaVersionMVP,
			Payload: MessagePayload{
				Channel: "chat",
				Text:    "Preciso falar com uma pessoa, não com robô.",
			},
		},
		{
			Name:          event.NameAccountEventRecorded,
			EventID:       "md-n05",
			OccurredAt:    MarketDayOccurredAt,
			CustomerID:    "c22",
			SchemaVersion: event.SchemaVersionMVP,
			Payload: AccountPayload{
				Kind:   "asset_drop",
				Amount: -12000,
				Before: 79000,
				After:  67000,
			},
		},
		{
			Name:          event.NameMessageReceived,
			EventID:       "md-n06",
			OccurredAt:    MarketDayOccurredAt,
			CustomerID:    "c21",
			SchemaVersion: event.SchemaVersionMVP,
			Payload: MessagePayload{
				Channel: "email",
				Text:    "Como declaro os dividendos recebidos em dólar?",
			},
		},
	}
}
