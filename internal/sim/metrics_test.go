package sim

import (
	"context"
	"errors"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// metricReader collects what this package records through the global meter
// provider, which TestMain installs before any test builds an instrument.
var metricReader = sdkmetric.NewManualReader()

func TestMain(m *testing.M) {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader)))
	os.Exit(m.Run())
}

// purchasesByClass reads pov_purchases_total per asset_class.
func purchasesByClass(t *testing.T) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := metricReader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect error = %v", err)
	}
	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != metricPurchases {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || !sum.IsMonotonic {
				t.Fatalf("%s is %T, want a monotonic int64 sum", metricPurchases, m.Data)
			}
			for _, dp := range sum.DataPoints {
				class, _ := dp.Attributes.Value(attribute.Key("asset_class"))
				out[class.AsString()] = dp.Value
			}
		}
	}
	return out
}

// TestCountPurchase is not parallel: it reads deltas of one global counter.
func TestCountPurchase(t *testing.T) {
	purchase := func(class string) []byte {
		return []byte(`{"event_id":"e","occurred_at":"2026-09-29T12:00:00Z","customer_id":"c","schema_version":3,` +
			`"payload":{"kind":"aplicacao","amount":3000000,"before":6800000,"after":6800000,` +
			`"product_id":"acoesg","asset_class":"` + class + `","risk":3}}`)
	}
	tests := []struct {
		name    string
		routing string
		body    []byte
		class   string // empty when nothing is counted
	}{
		{name: "purchase", routing: event.NameAccountEventRecorded, body: purchase(ClassETFs), class: ClassETFs},
		{name: "class outside the catalog", routing: event.NameAccountEventRecorded, body: purchase("crypto"), class: assetClassOther},
		{
			name:    "deposit",
			routing: event.NameAccountEventRecorded,
			body:    []byte(`{"schema_version":2,"payload":{"kind":"aporte","amount":100,"before":1,"after":101}}`),
		},
		{name: "other routing key", routing: event.NameMessageReceived, body: purchase(ClassAcoes)},
		{name: "unreadable body", routing: event.NameAccountEventRecorded, body: []byte(`{`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := purchasesByClass(t)
			countPurchase(t.Context(), tt.routing, tt.body)
			after := purchasesByClass(t)
			for class, n := range after {
				delta := n - before[class]
				want := int64(0)
				if class == tt.class {
					want = 1
				}
				if delta != want {
					t.Errorf("asset_class %q grew by %d, want %d", class, delta, want)
				}
			}
			if tt.class != "" && after[tt.class] == 0 {
				t.Errorf("asset_class %q was not counted", tt.class)
			}
		})
	}
}

// TestApply_CountsPurchaseByClass is not parallel: it reads deltas of one
// global counter, and parallel tests only start after it ends.
func TestApply_CountsPurchaseByClass(t *testing.T) {
	store := NewMemory()
	buy := func(key string, amount int64) error {
		_, err := Apply(t.Context(), store, Command{
			CustomerID:     CustomerThiago,
			IdempotencyKey: key,
			Kind:           CmdPurchase,
			ProductID:      "acoesg",
			Amount:         amount,
		})
		return err
	}
	tests := []struct {
		name    string
		key     string
		amount  int64
		wantErr error
		want    int64 // growth of asset_class=etfs
	}{
		{name: "purchase", key: "buy-1", amount: 3_000_000, want: 1},
		{name: "replay of the same key", key: "buy-1", amount: 3_000_000, want: 0},
		{name: "over cash", key: "buy-2", amount: 50_000_000, wantErr: ErrInsufficient, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := purchasesByClass(t)
			if err := buy(tt.key, tt.amount); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Apply error = %v, want %v", err, tt.wantErr)
			}
			after := purchasesByClass(t)
			if got := after[ClassETFs] - before[ClassETFs]; got != tt.want {
				t.Errorf("asset_class=etfs grew by %d, want %d", got, tt.want)
			}
			for class, n := range after {
				if class != ClassETFs && n != before[class] {
					t.Errorf("asset_class=%q grew by %d, want 0", class, n-before[class])
				}
			}
		})
	}
}

func TestApply_CountsNoPurchaseForOtherCommands(t *testing.T) {
	before := purchasesByClass(t)
	store := NewMemory()
	if _, err := Apply(t.Context(), store, Command{
		CustomerID:     CustomerThiago,
		IdempotencyKey: "deposit-1",
		Kind:           CmdDeposit,
		Amount:         10_000,
		Origin:         "conta corrente",
	}); err != nil {
		t.Fatalf("Apply error = %v", err)
	}
	after := purchasesByClass(t)
	for class, n := range after {
		if n != before[class] {
			t.Errorf("a deposit counted a %q purchase", class)
		}
	}
}
