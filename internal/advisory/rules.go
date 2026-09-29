package advisory

import (
	"errors"
	"fmt"
	"math"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// Card kinds and Portuguese rule strings from SIGNALS s11–s15, plus the
// phase-3 suitability rule.
const (
	KindSaque    = "saque"
	KindQueda    = "queda"
	KindAporte   = "aporte"
	KindSegmento = "segmento"
	KindContato  = "contato"
	KindPerfil   = "perfil"

	RuleKeyWithdrawal  = "withdrawal"
	RuleKeyDrop        = "drop"
	RuleKeyDeposit     = "deposit"
	RuleKeySegment     = "segment"
	RuleKeySilence     = "silence"
	RuleKeySuitability = "suitability"

	RuleWithdrawal  = "Saque acima de 20% do patrimônio em 24 horas"
	RuleDrop        = "Queda acima de 15% em 5 dias úteis"
	RuleDeposit     = "Aporte maior que o patrimônio anterior"
	RuleSegment     = "Patrimônio cruzou a faixa de US$ 10 mil"
	RuleSilence     = "Sem contato há mais de 90 dias"
	RuleSuitability = "Compra acima do perfil de investidor"
)

// Decision is one firing rule ready to become alert.raised. The product
// fields are set only by the suitability rule.
type Decision struct {
	RuleKey    string
	Kind       string
	Rule       string
	Amount     float64
	Before     float64
	After      float64
	From       string
	To         string
	Days       int
	ProductID  string
	AssetClass string
	Risk       int
	Profile    string
	MaxRisk    int
}

// EvaluateAccount applies the four account-fact rules. One fact may yield two
// decisions. The drop rule runs on the phase-1 asset_drop and on a negative
// reavaliacao (the daily revaluation of the simulated market); a gain never
// fires it.
func EvaluateAccount(p sim.AccountPayload) []Decision {
	out := make([]Decision, 0, 2)

	switch p.Kind {
	case "withdrawal", "saque":
		if d, ok := ruleWithdrawal(p); ok {
			out = append(out, d)
		}
	case "asset_drop":
		if d, ok := ruleDrop(p); ok {
			out = append(out, d)
		}
	case sim.KindReavaliacao:
		if p.Amount >= 0 {
			break
		}
		if d, ok := ruleDrop(p); ok {
			out = append(out, d)
		}
	case "deposit", "aporte":
		if d, ok := ruleDeposit(p); ok {
			out = append(out, d)
		}
	}

	if d, ok := ruleSegment(p); ok {
		out = append(out, d)
	}
	return out
}

// ErrInvalidPurchase marks an aplicacao that cannot be evaluated: no
// product_id, a risk outside 1–5, or a schema_version below 3. Redelivery
// cannot fix it.
var ErrInvalidPurchase = errors.New("advisory: invalid purchase")

// EvaluateSuitability is the suitability rule: an aplicacao (purchase) whose
// product risk exceeds MaxRisk(profile) raises a perfil alert that names the
// product and the amount. It only alerts; no case follows from it. Any other
// kind never fires. A purchase without product_id or with a risk outside 1–5
// wraps ErrInvalidPurchase; an unknown profile wraps ErrUnknownProfile. Neither
// is ever a silent pass.
func EvaluateSuitability(p sim.AccountPayload, profile string) (Decision, bool, error) {
	if p.Kind != sim.KindAplicacao {
		return Decision{}, false, nil
	}
	if p.ProductID == "" {
		return Decision{}, false, fmt.Errorf("advisory: suitability: %w: product_id is required", ErrInvalidPurchase)
	}
	if p.Risk < 1 || p.Risk > 5 {
		return Decision{}, false, fmt.Errorf("advisory: suitability: %w: risk %d outside 1-5", ErrInvalidPurchase, p.Risk)
	}
	limit, err := MaxRisk(profile)
	if err != nil {
		return Decision{}, false, fmt.Errorf("advisory: suitability: %w", err)
	}
	if p.Risk <= limit {
		return Decision{}, false, nil
	}
	return Decision{
		RuleKey:    RuleKeySuitability,
		Kind:       KindPerfil,
		Rule:       RuleSuitability,
		Amount:     p.Amount,
		Before:     p.Before,
		After:      p.After,
		ProductID:  p.ProductID,
		AssetClass: p.AssetClass,
		Risk:       p.Risk,
		Profile:    profile,
		MaxRisk:    limit,
	}, true, nil
}

// EvaluateSilence fires when recorded days with no contact are greater than 90.
func EvaluateSilence(days int, before, after float64) (Decision, bool) {
	if days <= 90 {
		return Decision{}, false
	}
	return Decision{
		RuleKey: RuleKeySilence,
		Kind:    KindContato,
		Rule:    RuleSilence,
		Before:  before,
		After:   after,
		Days:    days,
	}, true
}

func ruleWithdrawal(p sim.AccountPayload) (Decision, bool) {
	if p.Before <= 0 {
		return Decision{}, false
	}
	if p.Amount/p.Before <= 0.20 {
		return Decision{}, false
	}
	return Decision{
		RuleKey: RuleKeyWithdrawal,
		Kind:    KindSaque,
		Rule:    RuleWithdrawal,
		Amount:  p.Amount,
		Before:  p.Before,
		After:   p.After,
	}, true
}

// ruleDrop raises queda when the move is more than 15% of before. p is in
// dollars; the comparison runs in exact integer cents (dropAboveThreshold),
// the same predicate the portfolio_drop moment uses.
func ruleDrop(p sim.AccountPayload) (Decision, bool) {
	loss, okLoss := dollarsToCents(math.Abs(p.Amount))
	before, okBefore := dollarsToCents(p.Before)
	if !okLoss || !okBefore || !dropAboveThreshold(loss, before) {
		return Decision{}, false
	}
	return Decision{
		RuleKey: RuleKeyDrop,
		Kind:    KindQueda,
		Rule:    RuleDrop,
		Amount:  p.Amount,
		Before:  p.Before,
		After:   p.After,
	}, true
}

// dropAboveThreshold reports whether a loss of lossCents is more than 15% of
// beforeCents: loss/before > 0.15 is loss·20 > before·3, exact in integers
// for any amount up to 2^53 cents. A before of zero or less never holds.
func dropAboveThreshold(lossCents, beforeCents int64) bool {
	return beforeCents > 0 && lossCents*20 > beforeCents*3
}

// dollarsToCents rounds a dollar amount to whole cents. It reports false
// when the result is not finite or beyond 2^53 cents, where cents stop being
// exact.
func dollarsToCents(dollars float64) (int64, bool) {
	cents := math.Round(dollars * 100)
	if math.IsNaN(cents) || math.Abs(cents) > maxExactCents {
		return 0, false
	}
	return int64(cents), true
}

func ruleDeposit(p sim.AccountPayload) (Decision, bool) {
	if p.Amount <= p.Before {
		return Decision{}, false
	}
	return Decision{
		RuleKey: RuleKeyDeposit,
		Kind:    KindAporte,
		Rule:    RuleDeposit,
		Amount:  p.Amount,
		Before:  p.Before,
		After:   p.After,
	}, true
}

func ruleSegment(p sim.AccountPayload) (Decision, bool) {
	from := book.SegmentFromAssets(p.Before)
	to := book.SegmentFromAssets(p.After)
	if from == to {
		return Decision{}, false
	}
	return Decision{
		RuleKey: RuleKeySegment,
		Kind:    KindSegmento,
		Rule:    RuleSegment,
		Before:  p.Before,
		After:   p.After,
		From:    from,
		To:      to,
	}, true
}
