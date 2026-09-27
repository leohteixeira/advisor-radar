package bff_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

type stubQueue struct {
	bff.EmptyQueue
	customers map[string]bff.Customer
	ops       []bff.Operator
	queue     []bff.Signal
}

func (s stubQueue) ListQueue(context.Context) ([]bff.Signal, error) { return s.queue, nil }
func (s stubQueue) GetCustomer(_ context.Context, id string) (bff.Customer, error) {
	c, ok := s.customers[id]
	if !ok {
		return bff.Customer{}, bff.ErrCustomerNotFound
	}
	return c, nil
}
func (s stubQueue) ListOperators(context.Context) ([]bff.Operator, error) { return s.ops, nil }

type stubCases struct {
	bff.EmptyCases
	items   []bff.Case
	atRisk  []bff.ManagerAtRisk
	backlog []bff.ManagerBacklog
}

func (s stubCases) ListCases(context.Context) ([]bff.Case, []string, error) {
	return s.items, bff.CaseStates, nil
}
func (s stubCases) ListAtRisk(context.Context) ([]bff.ManagerAtRisk, error) { return s.atRisk, nil }
func (s stubCases) Backlog(context.Context) ([]bff.ManagerBacklog, error)   { return s.backlog, nil }

type stubReview struct {
	bff.EmptyReview
	items []bff.ReviewRow
}

func (s stubReview) ListReview(context.Context) ([]bff.ReviewRow, error) { return s.items, nil }

func TestHTTP_QueueIncludesDisplayFields(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	sigID := identity.MustNewV7()
	q := stubQueue{
		customers: map[string]bff.Customer{
			cust: {ID: cust, Name: "Mariana Costa", Segment: "Singular"},
		},
		queue: []bff.Signal{
			{ID: sigID, Kind: "alert", Client: cust, Name: "Mariana Costa", Segment: "Singular", Ago: 5},
		},
	}
	h := bff.NewHandler(bff.NewBoard(), nil, nil, q, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/queue", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Items []bff.Signal `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items = %d", len(body.Items))
	}
	got := body.Items[0]
	if got.Client != cust {
		t.Fatalf("client = %q, want uuid", got.Client)
	}
	if got.Name != "Mariana Costa" || got.Segment != "Singular" {
		t.Fatalf("display = %+v", got)
	}
}

func TestHTTP_QueueEnrichesLiveBoardFromBook(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	sigID := identity.MustNewV7()
	board := bff.NewBoard()
	body, _ := json.Marshal(map[string]any{
		"event_id": sigID, "occurred_at": time.Now().UTC(),
		"customer_id": cust, "schema_version": 1,
		"payload": map[string]any{"kind": "saque", "rule": "r1", "amount": 1000.0},
	})
	if err := board.ApplyDelivery(context.Background(), "alert.raised", body); err != nil {
		t.Fatal(err)
	}
	q := stubQueue{
		customers: map[string]bff.Customer{
			cust: {ID: cust, Name: "Ana Souza", Segment: "Advance"},
		},
	}
	h := bff.NewHandler(board, nil, nil, q, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/queue", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var out struct {
		Items []bff.Signal `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].Name != "Ana Souza" || out.Items[0].Segment != "Advance" {
		t.Fatalf("items = %+v", out.Items)
	}
	if out.Items[0].Client != cust {
		t.Fatalf("client mutated: %q", out.Items[0].Client)
	}
}

func TestHTTP_CasesAndReviewIncludeDisplayFields(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	caseID := identity.MustNewV7()
	revID := identity.MustNewV7()
	q := stubQueue{
		customers: map[string]bff.Customer{
			cust: {ID: cust, Name: "Pedro Lima", Segment: "Essencial"},
		},
	}
	cases := stubCases{items: []bff.Case{{
		ID: caseID, Client: cust, Signal: "s1", State: 0, OpenedAgo: 10, SlaTotal: 60, History: []bff.CaseHistoryEntry{},
	}}}
	review := stubReview{items: []bff.ReviewRow{{
		ID: revID, Client: cust, Text: "preciso resgatar", Dist: map[string]float64{"Resgate": 0.4}, Ago: 3, Intent: "Resgate",
	}}}
	h := bff.NewHandler(bff.NewBoard(), nil, nil, q, review, cases)

	req := httptest.NewRequest(http.MethodGet, "/v1/cases", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var casesBody struct {
		Items []bff.Case `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &casesBody); err != nil {
		t.Fatal(err)
	}
	if len(casesBody.Items) != 1 || casesBody.Items[0].Name != "Pedro Lima" || casesBody.Items[0].Segment != "Essencial" {
		t.Fatalf("cases = %+v", casesBody.Items)
	}
	if casesBody.Items[0].Client != cust {
		t.Fatalf("case client = %q", casesBody.Items[0].Client)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/review", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var reviewBody struct {
		Items []bff.ReviewRow `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &reviewBody); err != nil {
		t.Fatal(err)
	}
	if len(reviewBody.Items) != 1 || reviewBody.Items[0].Name != "Pedro Lima" || reviewBody.Items[0].Segment != "Essencial" {
		t.Fatalf("review = %+v", reviewBody.Items)
	}
}

func TestHTTP_ManagerAdvisorIsOperatorName(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	opID := identity.MustNewV7()
	caseID := identity.MustNewV7()
	q := stubQueue{
		customers: map[string]bff.Customer{
			cust: {ID: cust, Name: "Mariana Costa", Segment: "Singular"},
		},
		ops: []bff.Operator{{ID: opID, Name: "Carla Mendes"}},
	}
	cases := stubCases{
		backlog: []bff.ManagerBacklog{{Advisor: opID, Open: 2, Risk: 1, Overdue: 0}},
		atRisk:  []bff.ManagerAtRisk{{ID: caseID, Client: cust, Advisor: opID, Remaining: 40}},
	}
	h := bff.NewHandler(bff.NewBoard(), nil, nil, q, nil, cases)
	req := httptest.NewRequest(http.MethodGet, "/v1/manager", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var snap bff.ManagerSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Backlog) != 1 || snap.Backlog[0].Advisor != "Carla Mendes" {
		t.Fatalf("backlog = %+v", snap.Backlog)
	}
	if len(snap.AtRisk) != 1 {
		t.Fatalf("atRisk = %+v", snap.AtRisk)
	}
	got := snap.AtRisk[0]
	if got.Advisor != "Carla Mendes" || got.Client != "Mariana Costa" || got.Segment != "Singular" {
		t.Fatalf("atRisk = %+v", got)
	}
}
