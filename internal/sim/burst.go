// Package sim holds account-sim payload types. Market-day cast lives in SQL seeds.
package sim

import (
	"errors"
	"fmt"
	"math"
)

// ErrMoneyScale marks Dollars failures for schema or non-integer cents.
var ErrMoneyScale = errors.New("sim: money scale")

// OriginClientApp marks a message the customer sent from the client app.
// Seeded and burst messages leave Origin empty.
const OriginClientApp = "client_app"

// MessagePayload is the outbox payload for a customer message.
type MessagePayload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
	Origin  string `json:"origin,omitempty"`
}

// AccountPayload is the outbox payload for a fictional account fact.
// Schema version 1 stores amount, before, and after as whole USD dollars.
// Schema version 2 stores those fields as integer USD cents.
type AccountPayload struct {
	Kind        string  `json:"kind"`
	Amount      float64 `json:"amount"`
	Before      float64 `json:"before"`
	After       float64 `json:"after"`
	Origin      string  `json:"origin,omitempty"`
	Destination string  `json:"destination,omitempty"`
}

// Dollars returns a copy whose amount, before, and after are whole USD dollars.
// Schema version 1 is unchanged. Schema version 2 divides integer cents by 100.
func (p AccountPayload) Dollars(schemaVersion int) (AccountPayload, error) {
	switch schemaVersion {
	case 1:
		return p, nil
	case 2:
		amount, err := centsToDollars(p.Amount, "amount")
		if err != nil {
			return AccountPayload{}, err
		}
		before, err := centsToDollars(p.Before, "before")
		if err != nil {
			return AccountPayload{}, err
		}
		after, err := centsToDollars(p.After, "after")
		if err != nil {
			return AccountPayload{}, err
		}
		out := p
		out.Amount = amount
		out.Before = before
		out.After = after
		return out, nil
	default:
		return AccountPayload{}, fmt.Errorf("%w: unsupported schema_version %d", ErrMoneyScale, schemaVersion)
	}
}

func centsToDollars(cents float64, field string) (float64, error) {
	if math.IsNaN(cents) || math.IsInf(cents, 0) {
		return 0, fmt.Errorf("%w: %s has non-finite cents", ErrMoneyScale, field)
	}
	if math.Trunc(cents) != cents {
		return 0, fmt.Errorf("%w: %s has fractional cents", ErrMoneyScale, field)
	}
	return cents / 100, nil
}
