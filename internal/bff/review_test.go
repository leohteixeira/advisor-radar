package bff

import "testing"

func TestReviewQueue_ExcludesExactThreshold(t *testing.T) {
	t.Parallel()
	q := NewReviewQueue()
	q.mu.Lock()
	q.items = append(q.items, ReviewRow{
		ID:     "r85-live",
		Client: "c01",
		Text:   "live fixture",
		Dist:   map[string]float64{"Tributação": 0.85},
		Intent: "Tributação",
	})
	q.mu.Unlock()

	for _, row := range q.Items() {
		if row.ID == "r85" || row.ID == "r85-live" {
			t.Fatalf("row %s at 0.85 must be absent", row.ID)
		}
		if topProbability(row.Dist) >= reviewThreshold {
			t.Fatalf("%s top = %v, want below 0.85", row.ID, topProbability(row.Dist))
		}
	}
	if len(q.Items()) != 7 {
		t.Fatalf("items = %d, want 7", len(q.Items()))
	}
}
