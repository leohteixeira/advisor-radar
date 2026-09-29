package sim

import (
	"slices"
	"strings"
	"testing"
)

func TestAggregate_UnknownClassIsAnError(t *testing.T) {
	t.Parallel()
	_, err := aggregate(CustomerFernanda, 100, []Position{
		{ProductID: "tbill", AssetClass: ClassRendaFixa, ValueCents: 50},
		{ProductID: "mystery", AssetClass: "commodities", ValueCents: 70},
	})
	if err == nil || !strings.Contains(err.Error(), "commodities") {
		t.Fatalf("err = %v, want unknown asset class", err)
	}
}

// POVSeed order is the lock order ListAccounts relies on (see ResetPOV).
func TestPOVSeed_SortedByCustomerID(t *testing.T) {
	t.Parallel()
	seed := POVSeed()
	if !slices.IsSortedFunc(seed, func(a, b Account) int { return strings.Compare(a.CustomerID, b.CustomerID) }) {
		t.Fatal("POVSeed is not in byte-wise customer id order")
	}
}
