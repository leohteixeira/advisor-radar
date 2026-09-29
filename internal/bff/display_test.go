package bff_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
	moments   map[string]bff.MomentFacts
	profiles  map[string]bff.InvestorProfile
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
func (s stubQueue) MomentFacts(_ context.Context, id string) (bff.MomentFacts, error) {
	f, ok := s.moments[id]
	if !ok {
		return bff.MomentFacts{}, bff.ErrCustomerNotFound
	}
	return f, nil
}
func (s stubQueue) InvestorProfile(_ context.Context, id string) (bff.InvestorProfile, error) {
	p, ok := s.profiles[id]
	if !ok {
		return bff.InvestorProfile{}, bff.ErrCustomerNotFound
	}
	return p, nil
}

type stubCases struct {
	bff.EmptyCases
	items       []bff.Case
	atRisk      []bff.ManagerAtRisk
	backlog     []bff.ManagerBacklog
	customerErr error
}

func (s stubCases) ListCases(context.Context) ([]bff.Case, []string, error) {
	return s.items, bff.CaseStates, nil
}

// CustomerCases filters items by client, as the cases service filters by
// customer_id.
func (s stubCases) CustomerCases(_ context.Context, customerID string) ([]bff.Case, []string, error) {
	if s.customerErr != nil {
		return nil, nil, s.customerErr
	}
	var out []bff.Case
	for _, c := range s.items {
		if c.Client == customerID {
			out = append(out, c)
		}
	}
	return out, bff.CaseStates, nil
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

// perfilQueue answers one perfil card from the advisory queue.
func perfilQueue(t *testing.T) stubQueue {
	t.Helper()
	return stubQueue{
		customers: map[string]bff.Customer{
			"01a0e3a4-9a44-757a-ac8f-dab7db5eb068": {Name: "Fernanda Lima", Segment: "Essencial"},
		},
		queue: []bff.Signal{{
			ID: identity.MustNewV7(), Kind: "alert", Client: "01a0e3a4-9a44-757a-ac8f-dab7db5eb068",
			Alert: "perfil", Rule: "Compra acima do perfil de investidor", Amount: 1_000, Before: 8_200, After: 8_200,
			ProductID: "cobalto", Risk: 5, Profile: "conservador", MaxRisk: 2,
		}},
	}
}

func queueItems(t *testing.T, h http.Handler) ([]bff.Signal, bff.ListFacets) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/queue", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("queue = %d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Items  []bff.Signal   `json:"items"`
		Facets bff.ListFacets `json:"facets"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Items, body.Facets
}

// The perfil card body names the product from the account-sim catalog, its
// risk, and the profile's limit; the queue labels it "Compra acima do perfil".
func TestQueue_PerfilCardReason(t *testing.T) {
	t.Parallel()
	pov := &catalogPOV{products: []bff.POVProduct{{ID: "cobalto", Name: "Cobalto Semicondutores"}}}
	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, perfilQueue(t), nil, nil, pov, nil)
	items, facets := queueItems(t, h)
	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
	want := "Compra de US$ 1.000,00 em Cobalto Semicondutores, risco 5. Perfil conservador vai até risco 2."
	if items[0].Alert != "perfil" || items[0].Reason != want || items[0].Name != "Fernanda Lima" {
		t.Fatalf("card = %+v, want reason %q", items[0], want)
	}
	found := false
	for _, f := range facets.Motivo {
		if f.Label == "Compra acima do perfil" {
			found = f.Count == 1
		}
	}
	if !found {
		t.Fatalf("motivo facets = %+v, want Compra acima do perfil with 1", facets.Motivo)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/queue?motivo=Compra+acima+do+perfil", nil))
	if !strings.Contains(rr.Body.String(), `"alert":"perfil"`) {
		t.Fatalf("motivo filter dropped the perfil card: %s", rr.Body.String())
	}
}

// Without the catalog, the reason names the product id; a card that already
// carries a reason keeps it.
func TestQueue_PerfilCardReasonWithoutCatalog(t *testing.T) {
	t.Parallel()
	queue := perfilQueue(t)
	seeded := queue.queue[0]
	seeded.ID = identity.MustNewV7()
	seeded.Reason = "Texto do seed"
	queue.queue = append(queue.queue, seeded)
	for _, pov := range []bff.POVSource{nil, &catalogPOV{err: errors.New("account-sim down")}} {
		h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, queue, nil, nil, pov, nil)
		items, _ := queueItems(t, h)
		reasons := map[string]bool{}
		for _, it := range items {
			reasons[it.Reason] = true
		}
		if !reasons["Compra de US$ 1.000,00 em cobalto, risco 5. Perfil conservador vai até risco 2."] || !reasons["Texto do seed"] {
			t.Fatalf("reasons = %v", reasons)
		}
	}
}

// catalogPOV is a POVSource that also lends the product catalog. The first
// failures calls fail; block waits for the caller's deadline and records it.
type catalogPOV struct {
	bff.POVSource
	products []bff.POVProduct
	err      error
	failures int32
	block    bool
	calls    atomic.Int32
	budget   atomic.Int64
}

func (c *catalogPOV) Products(ctx context.Context) ([]bff.POVProduct, error) {
	n := c.calls.Add(1)
	if c.block {
		if deadline, ok := ctx.Deadline(); ok {
			c.budget.Store(int64(time.Until(deadline)))
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if n <= c.failures {
		return nil, errors.New("account-sim down")
	}
	return c.products, c.err
}

// The catalog is fixed: after the first successful read the names are
// cached, while a failed read is retried on the next render.
func TestQueue_PerfilCatalogIsCachedAfterSuccess(t *testing.T) {
	t.Parallel()
	pov := &catalogPOV{products: []bff.POVProduct{{ID: "cobalto", Name: "Cobalto Semicondutores"}}, failures: 1}
	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, perfilQueue(t), nil, nil, pov, nil)

	const byName = "Compra de US$ 1.000,00 em Cobalto Semicondutores, risco 5. Perfil conservador vai até risco 2."
	wants := []string{
		"Compra de US$ 1.000,00 em cobalto, risco 5. Perfil conservador vai até risco 2.",
		byName,
		byName,
		byName,
	}
	for i, want := range wants {
		items, _ := queueItems(t, h)
		if len(items) != 1 || items[0].Reason != want {
			t.Fatalf("render %d: items = %+v, want reason %q", i, items, want)
		}
	}
	if got := pov.calls.Load(); got != 2 {
		t.Fatalf("catalog calls = %d, want 2 (one failure, one success)", got)
	}
}

// A slow catalog is cut at 300 ms and the card names the product id.
func TestQueue_PerfilCatalogReadIsBounded(t *testing.T) {
	t.Parallel()
	pov := &catalogPOV{block: true}
	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, perfilQueue(t), nil, nil, pov, nil)
	items, _ := queueItems(t, h)
	if len(items) != 1 || items[0].Reason != "Compra de US$ 1.000,00 em cobalto, risco 5. Perfil conservador vai até risco 2." {
		t.Fatalf("items = %+v", items)
	}
	if budget := time.Duration(pov.budget.Load()); budget <= 0 || budget > 300*time.Millisecond {
		t.Fatalf("catalog deadline budget = %v, want within 300ms", budget)
	}
}

// Without a product name or id the card says "produto"; without a risk it
// leaves the risk out.
func TestQueue_PerfilCardReasonFallbacks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		productID string
		risk      int
		want      string
	}{
		{name: "no product", risk: 5, want: "Compra de US$ 1.000,00 em produto, risco 5. Perfil conservador vai até risco 2."},
		{name: "no risk", productID: "cobalto", want: "Compra de US$ 1.000,00 em cobalto. Perfil conservador vai até risco 2."},
		{name: "neither", want: "Compra de US$ 1.000,00 em produto. Perfil conservador vai até risco 2."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			queue := perfilQueue(t)
			queue.queue[0].ProductID = tt.productID
			queue.queue[0].Risk = tt.risk
			h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, queue, nil, nil, nil, nil)
			items, _ := queueItems(t, h)
			if len(items) != 1 || items[0].Reason != tt.want {
				t.Fatalf("items = %+v, want reason %q", items, tt.want)
			}
		})
	}
}
