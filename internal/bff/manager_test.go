package bff_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/bff"
)

// statsReview is a ReviewSource that only answers IntentStats.
type statsReview struct {
	bff.EmptyReview
	reviewPct   int
	fallbackPct int
	counts      map[string]int
}

func (s statsReview) IntentStats(context.Context) (int, int, map[string]int, error) {
	return s.reviewPct, s.fallbackPct, s.counts, nil
}

func managerSnapshot(t *testing.T, review bff.ReviewSource) bff.ManagerSnapshot {
	t.Helper()
	h := bff.NewHandler(bff.NewBoard(), nil, nil, nil, review, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/manager", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var snap bff.ManagerSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestHTTP_ManagerIntentsArePercentages(t *testing.T) {
	t.Parallel()

	snap := managerSnapshot(t, statsReview{
		reviewPct:   83,
		fallbackPct: 61,
		counts:      map[string]int{"Reclamação": 9, "Tributação": 6, "Operacional": 3, "Contato": 0},
	})

	if snap.ReviewPct != 83 || snap.FallbackPct != 61 {
		t.Errorf("review=%d fallback=%d, want 83 and 61", snap.ReviewPct, snap.FallbackPct)
	}
	expected := map[string]int{"Reclamação": 50, "Tributação": 33, "Operacional": 17}
	if len(snap.Intents) != len(expected) {
		t.Fatalf("intents = %v, want %v", snap.Intents, expected)
	}
	for label, pct := range expected {
		if snap.Intents[label] != pct {
			t.Errorf("%s = %d%%, want %d%%", label, snap.Intents[label], pct)
		}
	}
}

func TestHTTP_ManagerWithoutMessagesHasNoIntents(t *testing.T) {
	t.Parallel()

	snap := managerSnapshot(t, bff.EmptyReview{})

	if snap.Intents == nil || len(snap.Intents) != 0 {
		t.Errorf("intents = %v, want an empty object", snap.Intents)
	}
}
