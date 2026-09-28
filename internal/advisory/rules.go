package advisory

import (
	"math"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// Card kinds and Portuguese rule strings from SIGNALS s11–s15.
const (
	KindSaque    = "saque"
	KindQueda    = "queda"
	KindAporte   = "aporte"
	KindSegmento = "segmento"
	KindContato  = "contato"

	RuleKeyWithdrawal = "withdrawal"
	RuleKeyDrop       = "drop"
	RuleKeyDeposit    = "deposit"
	RuleKeySegment    = "segment"
	RuleKeySilence    = "silence"

	RuleWithdrawal = "Saque acima de 20% do patrimônio em 24 horas"
	RuleDrop       = "Queda acima de 15% em 5 dias úteis"
	RuleDeposit    = "Aporte maior que o patrimônio anterior"
	RuleSegment    = "Patrimônio cruzou a faixa de US$ 10 mil"
	RuleSilence    = "Sem contato há mais de 90 dias"
)

// Decision is one firing rule ready to become alert.raised.
type Decision struct {
	RuleKey string
	Kind    string
	Rule    string
	Amount  float64
	Before  float64
	After   float64
	From    string
	To      string
	Days    int
}

// EvaluateAccount applies the four account-fact rules. One fact may yield two decisions.
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

func ruleDrop(p sim.AccountPayload) (Decision, bool) {
	if p.Before <= 0 {
		return Decision{}, false
	}
	if math.Abs(p.Amount)/p.Before <= 0.15 {
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
