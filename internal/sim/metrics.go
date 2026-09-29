package sim

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// instrumentationName scopes the account-sim meter.
const instrumentationName = "github.com/leohteixeira/advisor-radar/internal/sim"

// metricPurchases counts purchases account-sim committed, by asset class.
// It is counted here, on commit, rather than in the BFF on 202: a replayed
// Idempotency-Key and a refused purchase commit nothing and count nothing,
// and the asset class comes from the catalog row the command used.
//
//	sum by (asset_class) (increase(pov_purchases_total[1h]))
const metricPurchases = "pov_purchases_total"

// assetClassOther is the asset_class label of a class outside the catalog,
// which keeps the label bounded.
const assetClassOther = "other"

// purchaseCounter is built on first use from the global meter provider, so a
// service that calls telemetry.Setup first exports it and one that does not
// records into the no-op provider.
var purchaseCounter = sync.OnceValue(func() metric.Int64Counter {
	c, err := otel.Meter(instrumentationName).Int64Counter(metricPurchases,
		metric.WithDescription("Purchases committed by account-sim, by asset class."))
	if err != nil {
		return metricnoop.Int64Counter{}
	}
	return c
})

// countPurchase counts one committed event when it is a purchase. It reads
// the committed event body, the published contract, rather than the command,
// so every producer of kind aplicacao is counted the same way. A body it
// cannot read is not a purchase it can count; counting is best effort and
// never fails the command.
func countPurchase(ctx context.Context, routing string, body []byte) {
	if routing != event.NameAccountEventRecorded {
		return
	}
	var env struct {
		Payload struct {
			Kind       string `json:"kind"`
			AssetClass string `json:"asset_class"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Payload.Kind != KindAplicacao {
		return
	}
	class := env.Payload.AssetClass
	if !slices.Contains([]string{ClassAcoes, ClassETFs, ClassRendaFixa}, class) {
		class = assetClassOther
	}
	purchaseCounter().Add(ctx, 1, metric.WithAttributes(attribute.String("asset_class", class)))
}
