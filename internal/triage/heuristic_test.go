package triage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/sim"
	"github.com/leohteixeira/advisor-radar/internal/triage"
)

// The complaint panel of the client app promises a case with a protocol for
// every preset it offers. The presets live in one file the panel imports, and
// each must qualify for a case in the cases intake when the model is down and
// the keyword heuristic classifies alone.
func TestHeuristicOpensCaseForEveryComplaintPreset(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "screens", "complaintPresets.json"))
	if err != nil {
		t.Fatal(err)
	}
	var presets []struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(raw, &presets); err != nil {
		t.Fatal(err)
	}
	if len(presets) == 0 {
		t.Fatal("no complaint presets")
	}

	for _, p := range presets {
		t.Run(p.Title, func(t *testing.T) {
			t.Parallel()

			r, err := triage.HeuristicClassifier{}.Classify(t.Context(), triage.Message{Channel: "chat", Text: p.Text})
			if err != nil {
				t.Fatal(err)
			}
			reason, ok := cases.Qualifies(cases.TriagedMessage{
				Origin:    sim.OriginClientApp,
				Intent:    string(r.Intent),
				ChurnRisk: r.ChurnRisk,
			})
			if !ok || reason != cases.ReasonComplaint {
				t.Fatalf("intent=%s churn=%v: reason=%v qualifies=%v, want a complaint case", r.Intent, r.ChurnRisk, reason, ok)
			}
		})
	}
}

// Wording close to a preset that is a question, not a complaint, keeps its
// subject intent.
func TestHeuristicKeepsNeutralTransferQuestionAsCambio(t *testing.T) {
	t.Parallel()

	r, err := triage.HeuristicClassifier{}.Classify(t.Context(), triage.Message{
		Text: "Mandei uma remessa ontem e ainda não caiu. Qual o prazo?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Intent != triage.IntentCambio {
		t.Fatalf("intent=%s, want cambio", r.Intent)
	}
}
