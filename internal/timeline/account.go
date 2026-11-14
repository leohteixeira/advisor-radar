package timeline

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Kinds of account.event.recorded as live events name them.
const (
	kindSaque       = "saque"
	kindAporte      = "aporte"
	kindQueda       = "queda"
	kindAplicacao   = "aplicacao"
	kindReavaliacao = "reavaliacao"
)

// legacyAccountKinds maps the phase-1 English kinds, which the seeded
// market-day events still carry, to the kinds live events use. advisory's
// rules read both spellings as the same fact.
var legacyAccountKinds = map[string]string{
	"deposit":    kindAporte,
	"withdrawal": kindSaque,
	"asset_drop": kindQueda,
}

// accountEntry maps one account.event.recorded payload to its row. The kind
// is the live Portuguese one whatever the event spelled. An event that brings
// its own title, text, or meta keeps them; otherwise the row gets the kind's
// title and a text written from the event's facts, so it reads and searches
// by amount and product.
func (idx *Index) accountEntry(schemaVersion int, payload json.RawMessage) (Entry, error) {
	var p struct {
		Kind            string `json:"kind"`
		Title           string `json:"title"`
		Text            string `json:"text"`
		Meta            string `json:"meta"`
		Amount          any    `json:"amount"`
		ProductID       string `json:"product_id"`
		SimDay          int    `json:"sim_day"`
		ProductChangeBP int    `json:"product_change_bp"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return Entry{}, fmt.Errorf("timeline: decode account payload: %w", err)
	}
	kind := p.Kind
	if kind == "" {
		kind = kindSaque
	}
	if live, ok := legacyAccountKinds[kind]; ok {
		kind = live
	}
	cents := amountCents(p.Amount, schemaVersion)
	title := p.Title
	if title == "" {
		title = kindTitle(kind)
	}
	text := p.Text
	if text == "" {
		text = accountText(kind, cents, idx.products[p.ProductID], p.SimDay)
	}
	return Entry{
		Kind:            kind,
		Title:           title,
		Text:            text,
		Meta:            p.Meta,
		ProductID:       p.ProductID,
		AmountCents:     cents,
		SimDay:          p.SimDay,
		ProductChangeBP: p.ProductChangeBP,
	}, nil
}

// accountText writes the text of an account row from its facts:
//
//	aporte, saque  "US$ 10.000,00"
//	aplicacao      "Compra de US$ 1.000,00 em Cobalto Semicondutores"
//	queda          "−US$ 12.000,00"
//	reavaliacao    "−US$ 38.520,00 no dia simulado 3"
//
// A part whose fact is missing is left out: a purchase of an unknown product
// reads "Compra de US$ 1.000,00", and a row without an amount has no amount.
// Any other kind has no text.
func accountText(kind string, cents int64, product string, simDay int) string {
	switch kind {
	case kindAporte, kindSaque:
		if cents == 0 {
			return ""
		}
		return money(magnitude(cents))
	case kindAplicacao:
		text := "Compra"
		if cents != 0 {
			text += " de " + money(magnitude(cents))
		}
		if product != "" {
			text += " em " + product
		}
		return text
	case kindQueda:
		if cents == 0 {
			return ""
		}
		return signedMoney(cents)
	case kindReavaliacao:
		text := signedMoney(cents)
		if simDay > 0 {
			text += " no dia simulado " + strconv.Itoa(simDay)
		}
		return text
	default:
		return ""
	}
}

// signedMoney formats integer USD cents with an explicit sign, "+US$ 12,00"
// or "−US$ 38.520,00" with U+2212, and zero without one.
func signedMoney(cents int64) string {
	switch {
	case cents > 0:
		return "+" + money(magnitude(cents))
	case cents < 0:
		return "−" + money(magnitude(cents))
	default:
		return money(0)
	}
}

// magnitude is |n|. amountCents keeps n within 2^53, so the negation is exact.
func magnitude(n int64) uint64 {
	if n < 0 {
		return uint64(-n)
	}
	return uint64(n)
}

// money formats USD cents as the product writes them, "US$ 1.234,56": "."
// between groups of three digits and "," before the cents.
func money(cents uint64) string {
	digits := strconv.FormatUint(cents/100, 10)
	var b strings.Builder
	b.WriteString("US$ ")
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(d)
	}
	fmt.Fprintf(&b, ",%02d", cents%100)
	return b.String()
}
