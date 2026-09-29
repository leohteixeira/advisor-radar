package advisory

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// metricReader collects what this package records through the global meter
// provider, which TestMain installs before any test builds an instrument.
var metricReader = sdkmetric.NewManualReader()

func TestMain(m *testing.M) {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader)))
	os.Exit(m.Run())
}

// suitabilityAlerts reads advisory_suitability_alerts_total.
func suitabilityAlerts(t *testing.T) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := metricReader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect error = %v", err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != metricSuitabilityAlerts {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || !sum.IsMonotonic {
				t.Fatalf("%s is %T, want a monotonic int64 sum", metricSuitabilityAlerts, m.Data)
			}
			for _, dp := range sum.DataPoints {
				if dp.Attributes.Len() != 0 {
					t.Errorf("%s has labels %v, want none", metricSuitabilityAlerts, dp.Attributes)
				}
				total += dp.Value
			}
		}
	}
	return total
}

// inboxStore is an in-memory Store whose inbox claims each event id once.
type inboxStore struct {
	mu      sync.Mutex
	claimed map[string]bool
}

func (s *inboxStore) WithTx(ctx context.Context, fn func(Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(inboxTx{s: s})
}

func (s *inboxStore) ListUnpublished(context.Context) ([]OutboxRow, error) { return nil, nil }

func (s *inboxStore) MarkPublished(context.Context, string, time.Time) error { return nil }

type inboxTx struct {
	s *inboxStore
}

func (tx inboxTx) ClaimInbox(_ context.Context, eventID string) (bool, error) {
	if tx.s.claimed[eventID] {
		return false, nil
	}
	tx.s.claimed[eventID] = true
	return true, nil
}

func (inboxTx) InsertAlert(context.Context, AlertRow) error { return nil }

func (inboxTx) InsertOutbox(context.Context, OutboxRow) error { return nil }

func (inboxTx) UpdateBook(context.Context, string, float64, string) error { return nil }

func (inboxTx) SaveRevaluation(context.Context, string, Revaluation) error { return nil }

func (inboxTx) InvestorProfile(context.Context, string) (string, error) {
	return ProfileConservador, nil
}

// TestRaise_CountsSuitabilityAlerts is not parallel: it reads deltas of one
// global counter.
func TestRaise_CountsSuitabilityAlerts(t *testing.T) {
	suitability := Decision{RuleKey: RuleKeySuitability, Kind: "perfil", Rule: "Compra acima do perfil de investidor"}
	segment := Decision{RuleKey: RuleKeySegment, Kind: KindSegmento, Rule: RuleSegment, From: "Essencial", To: "Advance"}
	// Cobalto is risk 5, above the conservador profile inboxTx reads.
	aboveProfile := &sim.AccountPayload{
		Kind:       sim.KindAplicacao,
		Amount:     1_000,
		Before:     8_200,
		After:      8_200,
		ProductID:  "cobalto",
		AssetClass: sim.ClassAcoes,
		Risk:       5,
	}
	store := &inboxStore{claimed: map[string]bool{}}
	tests := []struct {
		name      string
		sourceID  string
		decisions []Decision
		purchase  *sim.AccountPayload
		want      int64
	}{
		{name: "suitability alert", sourceID: "01a0e3a4-9a44-7000-8000-000000000001", decisions: []Decision{suitability, segment}, want: 1},
		{name: "redelivered event", sourceID: "01a0e3a4-9a44-7000-8000-000000000001", decisions: []Decision{suitability, segment}, want: 0},
		{name: "other rule", sourceID: "01a0e3a4-9a44-7000-8000-000000000002", decisions: []Decision{segment}, want: 0},
		{name: "purchase above the profile", sourceID: "01a0e3a4-9a44-7000-8000-000000000003", purchase: aboveProfile, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := suitabilityAlerts(t)
			err := raise(t.Context(), store, raiseInput{
				sourceEventID: tt.sourceID,
				customerID:    "01a0e3a4-9a44-757a-ac8f-dab7db5eb068",
				occurredAt:    time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
				decisions:     tt.decisions,
				purchase:      tt.purchase,
			})
			if err != nil {
				t.Fatalf("raise error = %v", err)
			}
			if got := suitabilityAlerts(t) - before; got != tt.want {
				t.Errorf("%s grew by %d, want %d", metricSuitabilityAlerts, got, tt.want)
			}
		})
	}
}
