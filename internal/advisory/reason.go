package advisory

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/leohteixeira/advisor-radar/internal/screen"
)

// Reason is the one-line Portuguese description of a fired rule, built from
// the facts of the decision only: what happened, with the amount, the share
// of the assets or the segments involved. It is what the advisor reads on the
// queue card and on the customer timeline. A decision whose facts cannot be
// stated (a kind this function does not know, an amount that is not finite)
// has no reason rather than a vague one.
func Reason(d Decision) string {
	switch d.Kind {
	case KindSaque:
		amount, ok := reasonMoney(d.Amount)
		if !ok {
			return ""
		}
		if share, ok := percentOf(d.Amount, d.Before); ok {
			return fmt.Sprintf("Saque de %s, %s do patrimônio", amount, share)
		}
		return "Saque de " + amount
	case KindQueda:
		share, okShare := dropPercent(d.Amount, d.Before)
		before, okBefore := reasonMoney(d.Before)
		after, okAfter := reasonMoney(d.After)
		if !okShare || !okBefore || !okAfter {
			return ""
		}
		return fmt.Sprintf("Patrimônio caiu %s, de %s para %s", share, before, after)
	case KindAporte:
		amount, ok := reasonMoney(d.Amount)
		if !ok {
			return ""
		}
		if d.Before > 0 {
			return fmt.Sprintf("Aporte de %s, %s× o patrimônio", amount, ratio(d.Amount/d.Before))
		}
		return "Aporte de " + amount
	case KindSegmento:
		if d.From == "" || d.To == "" {
			return ""
		}
		verb := "Subiu"
		if d.After < d.Before {
			verb = "Desceu"
		}
		return fmt.Sprintf("%s de %s para %s", verb, d.From, d.To)
	case KindContato:
		return fmt.Sprintf("Último contato há %d dias", d.Days)
	case KindPerfil:
		amount, ok := reasonMoney(d.Amount)
		if !ok {
			return ""
		}
		return fmt.Sprintf("Compra de %s em produto de risco %d. Perfil %s vai até risco %d.",
			amount, d.Risk, d.Profile, d.MaxRisk)
	default:
		return ""
	}
}

// reasonMoney formats a dollar magnitude as "US$ 1.234,56".
func reasonMoney(dollars float64) (string, bool) {
	cents, ok := dollarsToCents(math.Abs(dollars))
	if !ok {
		return "", false
	}
	return screen.Money(cents), true
}

// percentOf is |part| as a whole percentage of whole, "30%". It reports
// false when whole is not positive.
func percentOf(part, whole float64) (string, bool) {
	if whole <= 0 {
		return "", false
	}
	pct := math.Round(math.Abs(part) / whole * 100)
	if math.IsNaN(pct) || math.IsInf(pct, 0) {
		return "", false
	}
	return strconv.FormatFloat(pct, 'f', 0, 64) + "%", true
}

// dropPercent is |part| as a percentage of whole with one decimal, "15,5%".
// It is the formatting the client home uses for the same drop
// (screen.ChangePercent, without its sign), so the queue card and the client
// read one number. It reports false when whole is not positive or either
// amount cannot be stated in cents.
func dropPercent(part, whole float64) (string, bool) {
	partCents, okPart := dollarsToCents(math.Abs(part))
	wholeCents, okWhole := dollarsToCents(whole)
	if !okPart || !okWhole || wholeCents <= 0 {
		return "", false
	}
	return strings.TrimLeft(screen.ChangePercent(partCents, wholeCents), "+"), true
}

// ratio formats a multiple with one decimal and a pt-BR comma, without a
// trailing ",0": 7.5 is "7,5" and 2 is "2".
func ratio(x float64) string {
	s := strconv.FormatFloat(x, 'f', 1, 64)
	s = strings.TrimSuffix(s, ".0")
	return strings.Replace(s, ".", ",", 1)
}
