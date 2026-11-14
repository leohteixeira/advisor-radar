package triage_test

import (
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/triage"
)

func TestIntentLabelAcceptsKeyAndLabel(t *testing.T) {
	t.Parallel()

	for stored, want := range map[string]string{
		"reclamacao":   "Reclamação",
		"Reclamação":   "Reclamação",
		"tributacao":   "Tributação",
		"Tributação":   "Tributação",
		"operacional":  "Operacional",
		" Câmbio ":     "Câmbio",
		"encerramento": "Encerramento",
		"desconhecida": "desconhecida",
	} {
		if got := triage.IntentLabel(stored); got != want {
			t.Errorf("IntentLabel(%q) = %q, want %q", stored, got, want)
		}
	}
	if _, ok := triage.ParseIntent("Outra"); ok {
		t.Error("ParseIntent accepted an unknown intent")
	}
	if intent, ok := triage.ParseIntent("Reclamação"); !ok || intent != triage.IntentReclamacao {
		t.Errorf("ParseIntent(Reclamação) = %q, %v", intent, ok)
	}
}
