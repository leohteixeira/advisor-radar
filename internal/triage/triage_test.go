package triage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/jev"
)

func TestJevClassifierMapsAnswers(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"model":"typesafe-ai/jev",
			"answers":{
				"intent":{"type":"choice","choice":"reclamacao","probabilities":{"reclamacao":0.9,"cambio":0.1}},
				"frustration":{"type":"score","score":2.6},
				"churn_risk":{"type":"boolean","probability":0.93},
				"wants_human":{"type":"boolean","probability":0.1}
			},
			"providerMetadata":{"gateway":{"cost":"0.000012"}}
		}`))
	}))
	defer srv.Close()

	c := NewJevClassifier(jev.New("k", jev.WithBaseURL(srv.URL)))
	r, err := c.Classify(context.Background(), Message{ID: "1", Text: "vou levar meu dinheiro para outra corretora"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Intent != IntentReclamacao || r.IntentProb != 0.9 || r.ChurnRisk != 0.93 || r.Frustration != 2.6 || r.CostUSD != "0.000012" {
		t.Fatalf("result = %+v", r)
	}
	if r.NeedsReview(ReviewIntentProb) {
		t.Fatal("0.9 should not need review at 0.85")
	}
}

func TestJevClassifierMissingIntentIsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"model":"typesafe-ai/jev",
			"answers":{
				"frustration":{"type":"score","score":0},
				"churn_risk":{"type":"boolean","probability":0.1},
				"wants_human":{"type":"boolean","probability":0.1}
			}
		}`))
	}))
	defer srv.Close()

	c := NewJevClassifier(jev.New("k", jev.WithBaseURL(srv.URL)))
	_, err := c.Classify(context.Background(), Message{ID: "1", Text: "ola"})
	if err == nil {
		t.Fatal("expected error for missing intent")
	}
}

func TestJevClassifierRedactsDigitsBeforeEvaluate(t *testing.T) {
	t.Parallel()

	var gotState map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jev.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		gotState, _ = req.State.(map[string]any)
		_, _ = w.Write([]byte(`{
			"model":"typesafe-ai/jev",
			"answers":{
				"intent":{"type":"choice","choice":"cambio","probabilities":{"cambio":0.9}},
				"frustration":{"type":"score","score":0},
				"churn_risk":{"type":"boolean","probability":0.1},
				"wants_human":{"type":"boolean","probability":0.1}
			}
		}`))
	}))
	defer srv.Close()

	c := NewJevClassifier(jev.New("k", jev.WithBaseURL(srv.URL)))
	msg := Message{ID: "1", Channel: "chat", Text: "20 mil em 12/09/2025", Previous: []string{"paguei 15 em 01/02/2024"}}
	if _, err := c.Classify(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	digit := regexp.MustCompile(`\d`)
	mensagem, _ := gotState["mensagem"].(string)
	if digit.MatchString(mensagem) {
		t.Fatalf("mensagem still has digits: %q", mensagem)
	}
	prev, _ := gotState["mensagens_anteriores"].([]any)
	if len(prev) != 1 {
		t.Fatalf("previous = %#v", prev)
	}
	if digit.MatchString(prev[0].(string)) {
		t.Fatalf("previous still has digits: %q", prev[0])
	}
}

func TestNeedsReviewIgnoresDegradedAlone(t *testing.T) {
	t.Parallel()

	r := Result{IntentProb: 0.9, Degraded: true}
	if r.NeedsReview(ReviewIntentProb) {
		t.Fatal("degraded alone must not force review")
	}
	r.IntentProb = 0.8
	if !r.NeedsReview(ReviewIntentProb) {
		t.Fatal("0.8 must need review")
	}
}

type slow struct{}

func (slow) Classify(ctx context.Context, _ Message) (Result, error) {
	<-ctx.Done()
	return Result{}, ctx.Err()
}

type failing struct{}

func (failing) Classify(context.Context, Message) (Result, error) {
	return Result{}, errors.New("gateway down")
}

func TestFallbackDegradesOnTimeoutAndError(t *testing.T) {
	t.Parallel()

	msg := Message{ID: "1", Text: "Quero falar com uma pessoa, vou para outra corretora!!"}
	for name, primary := range map[string]Classifier{"timeout": slow{}, "error": failing{}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var degraded bool
			f := Fallback{Primary: primary, Secondary: HeuristicClassifier{}, Timeout: 20 * time.Millisecond,
				OnDegrade: func(Message, error) { degraded = true }}
			r, err := f.Classify(context.Background(), msg)
			if err != nil {
				t.Fatal(err)
			}
			if !r.Degraded || !degraded || r.Classifier != "heuristic" {
				t.Fatalf("result = %+v", r)
			}
			if r.ChurnRisk < 0.5 || r.WantsHuman < 0.5 {
				t.Fatalf("heuristic missed signals: %+v", r)
			}
			// Heuristic match is 0.6, so product review still applies.
			if !r.NeedsReview(ReviewIntentProb) {
				t.Fatal("heuristic 0.6 must need review at 0.85")
			}
		})
	}
}

func TestFallbackRespectsCallerCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := Fallback{Primary: slow{}, Secondary: HeuristicClassifier{}, Timeout: time.Second}
	if _, err := f.Classify(ctx, Message{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestHeuristicHandlesAccents(t *testing.T) {
	t.Parallel()

	r, _ := HeuristicClassifier{}.Classify(context.Background(), Message{Text: "Qual o prazo da REMESSA de câmbio?"})
	if r.Intent != IntentCambio {
		t.Fatalf("intent = %s", r.Intent)
	}
}

func TestHeuristicMatchesLabeledSet(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("testdata", "messages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var set []struct {
		Message
		Expected struct {
			Intent string `json:"intent"`
		} `json:"expected"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	if len(set) != 16 {
		t.Fatalf("messages = %d, want 16", len(set))
	}

	h := HeuristicClassifier{}
	matched := 0
	for _, l := range set {
		r, err := h.Classify(context.Background(), l.Message)
		if err != nil {
			t.Fatal(err)
		}
		if string(r.Intent) == l.Expected.Intent {
			matched++
			continue
		}
		t.Errorf("%s intent = %s, want %s", l.ID, r.Intent, l.Expected.Intent)
	}
	if matched != 16 {
		t.Fatalf("heuristic intent matches = %d, want 16", matched)
	}
}

func TestHeuristicSeesOriginalTextWithDigits(t *testing.T) {
	t.Parallel()

	// Digits stay for the heuristic; "20 mil" alone is not a cambio keyword, but
	// "remessa" would be. Ensure Classify does not panic on digit-heavy text.
	r, err := HeuristicClassifier{}.Classify(context.Background(), Message{Text: "20 mil em 12/09/2025 remessa"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Intent != IntentCambio {
		t.Fatalf("intent = %s, want cambio (heuristic sees original)", r.Intent)
	}
}
