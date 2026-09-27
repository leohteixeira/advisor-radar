package bff_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

func TestHTTP_QueueSegmentFilterAndScoreOrder(t *testing.T) {
	t.Parallel()
	c1 := identity.MustNewV7()
	c2 := identity.MustNewV7()
	c3 := identity.MustNewV7()
	s1 := identity.MustNewV7()
	s2 := identity.MustNewV7()
	s3 := identity.MustNewV7()
	churn := true
	q := stubQueue{
		customers: map[string]bff.Customer{
			c1: {ID: c1, Name: "A Singular", Segment: "Singular"},
			c2: {ID: c2, Name: "B Advance", Segment: "Advance"},
			c3: {ID: c3, Name: "C Singular Low", Segment: "Singular"},
		},
		queue: []bff.Signal{
			{ID: s3, Kind: "message", Client: c3, Name: "C Singular Low", Segment: "Singular", Ago: 5, Intent: "Operacional"},
			{ID: s1, Kind: "message", Client: c1, Name: "A Singular", Segment: "Singular", Ago: 5, Intent: "Reclamação", Churn: &churn, Frustration: intPtr(3)},
			{ID: s2, Kind: "message", Client: c2, Name: "B Advance", Segment: "Advance", Ago: 5, Intent: "Operacional"},
		},
	}
	h := bff.NewHandler(bff.NewBoard(), nil, nil, q, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/queue?segmento=Singular", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Items  []bff.Signal   `json:"items"`
		Facets bff.ListFacets `json:"facets"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %d, want 2 Singular", len(body.Items))
	}
	if body.Items[0].ID != s1 {
		t.Fatalf("want higher score first: got %s then %s", body.Items[0].ID, body.Items[1].ID)
	}
	for _, it := range body.Items {
		if it.Segment != "Singular" {
			t.Fatalf("unexpected segment %q", it.Segment)
		}
	}
	var singularCount, advanceCount int
	for _, f := range body.Facets.Segmento {
		switch f.Label {
		case "Singular":
			singularCount = f.Count
		case "Advance":
			advanceCount = f.Count
		}
	}
	if singularCount != 2 || advanceCount != 1 {
		t.Fatalf("segment facets Singular=%d Advance=%d (counts ignore own filter)", singularCount, advanceCount)
	}
}

func TestHTTP_CasesSegmentFilterAndRemOrder(t *testing.T) {
	t.Parallel()
	c1 := identity.MustNewV7()
	c2 := identity.MustNewV7()
	c3 := identity.MustNewV7()
	k1 := identity.MustNewV7()
	k2 := identity.MustNewV7()
	k3 := identity.MustNewV7()
	q := stubQueue{
		customers: map[string]bff.Customer{
			c1: {ID: c1, Name: "Case A", Segment: "Advance"},
			c2: {ID: c2, Name: "Case B", Segment: "Advance"},
			c3: {ID: c3, Name: "Case C", Segment: "Essencial"},
		},
	}
	cases := stubCases{items: []bff.Case{
		{ID: k2, Client: c2, Name: "Case B", Segment: "Advance", Signal: "x", State: 0, OpenedAgo: 10, SlaTotal: 120, History: []bff.CaseHistoryEntry{}},
		{ID: k1, Client: c1, Name: "Case A", Segment: "Advance", Signal: "y", State: 0, OpenedAgo: 100, SlaTotal: 120, History: []bff.CaseHistoryEntry{}},
		{ID: k3, Client: c3, Name: "Case C", Segment: "Essencial", Signal: "z", State: 0, OpenedAgo: 5, SlaTotal: 60, History: []bff.CaseHistoryEntry{}},
	}}
	h := bff.NewHandler(bff.NewBoard(), nil, nil, q, nil, cases)
	req := httptest.NewRequest(http.MethodGet, "/v1/cases?segmento=Advance", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Items []bff.Case `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %d, want 2 Advance", len(body.Items))
	}
	// rem = slaTotal - openedAgo: k1 has 20, k2 has 110 → k1 (smaller rem) first
	if body.Items[0].ID != k1 || body.Items[1].ID != k2 {
		t.Fatalf("want rem ascending: got %+v", body.Items)
	}
}

func intPtr(n int) *int { return &n }
