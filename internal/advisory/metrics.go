package advisory

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
)

// instrumentationName scopes the advisory meter.
const instrumentationName = "github.com/leohteixeira/advisor-radar/internal/advisory"

// metricSuitabilityAlerts counts suitability-mismatch ("perfil") alerts
// advisory committed. A redelivered source event raises nothing and counts
// nothing.
//
//	increase(advisory_suitability_alerts_total[1h])
const metricSuitabilityAlerts = "advisory_suitability_alerts_total"

// suitabilityAlertCounter is built on first use from the global meter
// provider, so a service that calls telemetry.Setup first exports it and one
// that does not records into the no-op provider.
var suitabilityAlertCounter = sync.OnceValue(func() metric.Int64Counter {
	c, err := otel.Meter(instrumentationName).Int64Counter(metricSuitabilityAlerts,
		metric.WithDescription("Suitability-mismatch alerts raised by advisory."))
	if err != nil {
		return metricnoop.Int64Counter{}
	}
	return c
})

// countRaised counts the committed decisions that feed a metric: today only
// the suitability-mismatch alerts.
func countRaised(ctx context.Context, decisions []Decision) {
	var suitability int64
	for _, d := range decisions {
		if d.RuleKey == RuleKeySuitability {
			suitability++
		}
	}
	if suitability > 0 {
		suitabilityAlertCounter().Add(ctx, suitability)
	}
}
