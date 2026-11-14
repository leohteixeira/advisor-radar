package triage

import "strings"

// intentLabels maps each canonical intent key to its Portuguese display
// label. It is the only key-to-label mapping: the classifiers and
// message.triaged carry keys, every screen shows labels.
var intentLabels = map[Intent]string{
	IntentOperacional:  "Operacional",
	IntentCambio:       "Câmbio",
	IntentTributacao:   "Tributação",
	IntentInvestimento: "Investimento",
	IntentResgate:      "Resgate",
	IntentReclamacao:   "Reclamação",
	IntentEncerramento: "Encerramento",
	IntentContato:      "Contato",
}

// Label returns the Portuguese display label of a known intent, and the raw
// value for an unknown one.
func (i Intent) Label() string {
	if label, ok := intentLabels[i]; ok {
		return label
	}
	return string(i)
}

// ParseIntent resolves a stored or submitted intent, written either as the
// canonical key ("reclamacao") or as its label ("Reclamação"), to the
// canonical key. It reports false for anything else.
func ParseIntent(s string) (Intent, bool) {
	intent := Intent(normalize(strings.TrimSpace(s)))
	_, ok := intentLabels[intent]
	return intent, ok
}

// IntentLabel returns the display label for a stored intent in either
// spelling. An unknown value comes back unchanged.
func IntentLabel(s string) string {
	if intent, ok := ParseIntent(s); ok {
		return intent.Label()
	}
	return s
}
