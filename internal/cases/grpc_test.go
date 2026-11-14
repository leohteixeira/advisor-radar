package cases_test

import (
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	casesv1 "github.com/leohteixeira/advisor-radar/gen/cases/v1"
	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

func TestGRPCServer_ListCases_InvalidCustomer(t *testing.T) {
	t.Parallel()
	srv := cases.NewGRPCServer(nil, nil)
	for _, id := range []string{"x", "not-a-uuid", "6f1c2a4e-3b7d-4c1e-9a2b-1d2e3f4a5b6c"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			_, err := srv.ListCases(t.Context(), &casesv1.ListCasesRequest{CustomerId: id})
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("ListCases(%q) = %v, want InvalidArgument", id, err)
			}
		})
	}
}

// TestGRPCServer_ListCases_CustomerFilter opens a case for two customers on
// PostgreSQL: a customer filter returns only that customer's cases, and an
// empty request still lists every case, as the Bastidores board sends it.
func TestGRPCServer_ListCases_CustomerFilter(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)
	ctx := t.Context()
	lookup := singular(identity.MustNewV7())

	mariana, other := identity.MustNewV7(), identity.MustNewV7()
	opened := map[string]string{}
	for _, customerID := range []string{mariana, other} {
		res, err := cases.Intake(ctx, store, lookup, complaintBody(t, customerID))
		if err != nil || res.Decision != cases.DecisionOpened {
			t.Fatalf("open for %s = %+v, %v", customerID, res, err)
		}
		opened[customerID] = res.CaseID
	}
	srv := cases.NewGRPCServer(cases.NewCaseReader(pool), store)

	tests := []struct {
		name       string
		customerID string
		expected   []string
	}{
		{name: "one customer", customerID: mariana, expected: []string{opened[mariana]}},
		{name: "uppercase id", customerID: strings.ToUpper(mariana), expected: []string{opened[mariana]}},
		{name: "customer without cases", customerID: identity.MustNewV7(), expected: []string{}},
		{name: "empty lists every case", customerID: "", expected: []string{opened[mariana], opened[other]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := srv.ListCases(ctx, &casesv1.ListCasesRequest{CustomerId: tt.customerID})
			if err != nil {
				t.Fatalf("ListCases: %v", err)
			}
			got := make([]string, 0, len(res.GetItems()))
			for _, c := range res.GetItems() {
				got = append(got, c.GetId())
				if tt.customerID != "" && c.GetClient() != mariana {
					t.Errorf("case %s belongs to %s, want %s", c.GetId(), c.GetClient(), mariana)
				}
			}
			slices.Sort(got)
			want := slices.Sorted(slices.Values(tt.expected))
			if !slices.Equal(got, want) {
				t.Errorf("cases = %v, want %v", got, want)
			}
			if !slices.Equal(res.GetStates(), cases.CaseStates) {
				t.Errorf("states = %v", res.GetStates())
			}
		})
	}
}

// A case reports whether its SLA is the halved one, so the card can explain
// a Singular case counting 30 min next to the segment's 1 h.
func TestGRPCServer_ListCases_SLAHalved(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)
	ctx := t.Context()
	lookup := singular(identity.MustNewV7())

	tests := []struct {
		name      string
		msg       cases.TriagedMessage
		wantTotal int32
		want      bool
	}{
		{name: "churn risk halves", msg: cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "reclamacao", ChurnRisk: 0.8}, wantTotal: 30, want: true},
		{name: "plain complaint keeps the base", msg: cases.TriagedMessage{Origin: cases.OriginClientApp, Intent: "reclamacao"}, wantTotal: 60, want: false},
	}
	srv := cases.NewGRPCServer(cases.NewCaseReader(pool), store)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customerID := identity.MustNewV7()
			tt.msg.SourceEventID = identity.MustNewV7()
			if _, err := cases.Intake(ctx, store, lookup, triagedBody(t, identity.MustNewV7(), customerID, tt.msg)); err != nil {
				t.Fatalf("open: %v", err)
			}
			res, err := srv.ListCases(ctx, &casesv1.ListCasesRequest{CustomerId: customerID})
			if err != nil || len(res.GetItems()) != 1 {
				t.Fatalf("ListCases = %d items, %v", len(res.GetItems()), err)
			}
			got := res.GetItems()[0]
			if got.GetSlaTotal() != tt.wantTotal || got.GetSlaHalved() != tt.want {
				t.Fatalf("total %d halved %v, want %d and %v", got.GetSlaTotal(), got.GetSlaHalved(), tt.wantTotal, tt.want)
			}
		})
	}
}
