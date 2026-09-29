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
