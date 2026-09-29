package screen

import (
	"slices"
	"testing"
	"time"
)

// TestActivityItems checks the one mapping the home activity and the Carteira
// history share, on purchase, aporte, saque, and other rows.
func TestActivityItems(t *testing.T) {
	t.Parallel()
	withRows := func(rows ...Activity) Snapshot {
		snap := thiagoFixture().snapshot()
		snap.Activity.Value = rows
		return snap
	}
	noCatalog := withRows(purchaseRow())
	noCatalog.Products = Fetched[[]Product]{Err: errDown}
	notListed := purchaseRow()
	notListed.ProductID = "gone"
	noAmount := purchaseRow()
	noAmount.AmountCents = 0
	tests := []struct {
		name     string
		snap     Snapshot
		expected []ActivityItem
	}{
		{
			name:     "purchase names the product and the amount that left caixa",
			snap:     withRows(purchaseRow()),
			expected: []ActivityItem{{Icon: "out", Title: "Compra · Maré Ações Globais ETF", Meta: "há 2 min", Value: "−US$ 60.520,00"}},
		},
		{
			name:     "catalog down leaves the product out",
			snap:     noCatalog,
			expected: []ActivityItem{{Icon: "out", Title: "Compra", Meta: "há 2 min", Value: "−US$ 60.520,00"}},
		},
		{
			name:     "product the catalog does not list",
			snap:     withRows(notListed),
			expected: []ActivityItem{{Icon: "out", Title: "Compra", Meta: "há 2 min", Value: "−US$ 60.520,00"}},
		},
		{
			name:     "a row without an amount has no value",
			snap:     withRows(noAmount),
			expected: []ActivityItem{{Icon: "out", Title: "Compra · Maré Ações Globais ETF", Meta: "há 2 min"}},
		},
		{
			name:     "aporte adds its amount, positive",
			snap:     withRows(Activity{Kind: "aporte", Title: "Aporte", Source: "account.event.recorded", AmountCents: 6000000}),
			expected: []ActivityItem{{Icon: "in", Title: "Aporte", Meta: "agora", Value: "+US$ 60.000,00", Tone: "pos"}},
		},
		{
			name:     "saque takes its amount out, negative",
			snap:     withRows(Activity{Kind: "saque", Title: "Saque", Source: "account.event.recorded", AmountCents: 2000000}),
			expected: []ActivityItem{{Icon: "out", Title: "Saque", Meta: "agora", Value: "−US$ 20.000,00", Tone: "neg"}},
		},
		{
			name:     "aporte and saque without an amount have no value",
			snap:     withRows(Activity{Kind: "aporte", Title: "Aporte", Source: "account.event.recorded"}, Activity{Kind: "saque", Title: "Saque", Source: "account.event.recorded", Age: 1}),
			expected: []ActivityItem{{Icon: "in", Title: "Aporte", Meta: "agora"}, {Icon: "out", Title: "Saque", Meta: "agora"}},
		},
		{
			name:     "other kinds keep the indexed title and no value",
			snap:     withRows(Activity{Kind: "mensagem", Title: "Mensagem · chat", Source: "message.received", AmountCents: 100}),
			expected: []ActivityItem{{Icon: "msg", Title: "Mensagem · chat", Meta: "agora"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := activityItems(tt.snap, embedded(t), historyLimit)
			if err != nil {
				t.Fatalf("activityItems error = %v", err)
			}
			if !slices.Equal(got, tt.expected) {
				t.Errorf("items = %+v, want %+v", got, tt.expected)
			}
		})
	}

	t.Run("missing copy is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := activityItems(withRows(purchaseRow()), Catalog{}, historyLimit); err == nil {
			t.Error("activityItems without the row copy returned no error")
		}
	})
}

// The home activity reads the same mapping and keeps its limit of five.
func TestRecentActivity_Purchase(t *testing.T) {
	t.Parallel()
	p := props[ActivityList](t, build(t, recentActivity{}, thiagoAfterTudo().snapshot()), "activity_list", "recent")
	expected := []ActivityItem{
		{Icon: "out", Title: "Compra · Maré Ações Globais ETF", Meta: "há 2 min", Value: "−US$ 60.520,00"},
		{Icon: "in", Title: "Aporte", Meta: "há 4 dias"},
	}
	if !slices.Equal(p.Items, expected) {
		t.Errorf("items = %+v, want %+v", p.Items, expected)
	}
	if _, err := (recentActivity{}).Build(thiagoAfterTudo().snapshot(), Catalog{}); err == nil {
		t.Error("recent activity without its copy returned no error")
	}
}

func TestProductNames(t *testing.T) {
	t.Parallel()
	names := productNames(thiagoFixture().snapshot())
	if len(names) != len(testCatalog()) || names["acoesg"] != "Maré Ações Globais ETF" {
		t.Errorf("names = %v", names)
	}
	if got := productNames(Snapshot{Products: Fetched[[]Product]{Err: errDown}}); got == nil || len(got) != 0 {
		t.Errorf("names without the catalog = %v, want empty", got)
	}
}

// TestActivityItems_Revaluation checks the reavaliacao row the home activity
// and the Carteira history share.
func TestActivityItems_Revaluation(t *testing.T) {
	t.Parallel()
	withRows := func(rows ...Activity) Snapshot {
		snap := marianaShocked().snapshot()
		snap.Activity.Value = rows
		return snap
	}
	noCatalog := withRows(revaluationRow())
	noCatalog.Products = Fetched[[]Product]{Err: errDown}
	gain := revaluationRow()
	gain.AmountCents, gain.ProductID, gain.ProductChangeBP, gain.SimDay = 500_000, "acoesg", 1234, 5
	flat := revaluationRow()
	flat.AmountCents, flat.ProductID, flat.ProductChangeBP, flat.SimDay = 0, "", 0, 1
	tests := []struct {
		name     string
		snap     Snapshot
		expected []ActivityItem
	}{
		{
			name: "a loss names the day and the product that moved the most",
			snap: withRows(revaluationRow()),
			expected: []ActivityItem{{
				Icon: "drop", Title: "Reavaliação diária", Meta: "dia simulado 3 · Cobalto Semicondutores −53,5%",
				Value: "−US$ 38.520,00", Tone: ToneNeg,
			}},
		},
		{
			name: "a gain",
			snap: withRows(gain),
			expected: []ActivityItem{{
				Title: "Reavaliação diária", Meta: "dia simulado 5 · Maré Ações Globais ETF +12,3%",
				Value: "+US$ 5.000,00", Tone: TonePos,
			}},
		},
		{
			name: "catalog down leaves the product out",
			snap: noCatalog,
			expected: []ActivityItem{{
				Icon: "drop", Title: "Reavaliação diária", Meta: "dia simulado 3", Value: "−US$ 38.520,00", Tone: ToneNeg,
			}},
		},
		{
			name:     "a zero-change revaluation is not shown",
			snap:     withRows(flat, purchaseRow()),
			expected: []ActivityItem{{Icon: "out", Title: "Compra · Maré Ações Globais ETF", Meta: "há 2 min", Value: "−US$ 60.520,00"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := activityItems(tt.snap, embedded(t), historyLimit)
			if err != nil {
				t.Fatalf("activityItems error = %v", err)
			}
			if !slices.Equal(got, tt.expected) {
				t.Errorf("items = %+v, want %+v", got, tt.expected)
			}
		})
	}

	t.Run("missing copy is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := activityItems(withRows(revaluationRow()), Catalog{}, historyLimit); err == nil {
			t.Error("activityItems without the row copy returned no error")
		}
	})
}

// Zero-change revaluations do not take the home's five places: they are
// dropped before the limit.
func TestVisibleActivity_DropsFlatRevaluationsBeforeTheLimit(t *testing.T) {
	t.Parallel()
	rows := make([]Activity, 0, 7)
	for day := range 6 {
		rows = append(rows, Activity{Kind: "reavaliacao", Source: "account.event.recorded", Age: time.Duration(day) * time.Minute, SimDay: day + 1})
	}
	rows = append(rows, Activity{Kind: "aporte", Title: "Aporte", Source: "account.event.recorded", Age: time.Hour})
	got := visibleActivity(rows, time.Time{}, activityLimit)
	if len(got) != 1 || got[0].Kind != "aporte" {
		t.Errorf("visible = %+v, want only the aporte", got)
	}
	snap := thiagoFixture().snapshot()
	snap.Activity.Value = rows[:6]
	if (recentActivity{}).Matches(snap) {
		t.Error("recent activity matches with only flat revaluations")
	}
}
