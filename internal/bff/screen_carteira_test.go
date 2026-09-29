package bff_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	timelinev1 "github.com/leohteixeira/advisor-radar/gen/timeline/v1"
	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/outbox"
	"github.com/leohteixeira/advisor-radar/internal/sim"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

type statProps struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Tone  string `json:"tone"`
	Money bool   `json:"money"`
}

type portfolioSummaryProps struct {
	TotalLabel string      `json:"total_label"`
	Total      string      `json:"total"`
	Stats      []statProps `json:"stats"`
}

type breakdownProps struct {
	Title string `json:"title"`
	Rows  []struct {
		Class    string `json:"class"`
		Label    string `json:"label"`
		Value    string `json:"value"`
		Share    string `json:"share"`
		BarWidth int    `json:"bar_width"`
	} `json:"rows"`
}

type positionListProps struct {
	Title        string `json:"title"`
	Subtotal     string `json:"subtotal"`
	AppliedLabel string `json:"applied_label"`
	Items        []struct {
		ProductID  string `json:"product_id"`
		Name       string `json:"name"`
		Applied    string `json:"applied"`
		Value      string `json:"value"`
		Return     string `json:"return"`
		ReturnTone string `json:"return_tone"`
	} `json:"items"`
}

type historyProps struct {
	Title string `json:"title"`
	Items []struct {
		Icon  string `json:"icon"`
		Title string `json:"title"`
		Meta  string `json:"meta"`
		Value string `json:"value"`
		Tone  string `json:"tone"`
	} `json:"items"`
	EmptyText string `json:"empty_text"`
}

func (p positionListProps) ids() []string {
	out := make([]string, 0, len(p.Items))
	for _, it := range p.Items {
		out = append(out, it.ProductID)
	}
	return out
}

// TestGetScreen_Carteira serves Carteira from the real account-sim seed over
// bufconn: Mariana and Fernanda hold all three classes, Thiago no fixed
// income.
func TestGetScreen_Carteira(t *testing.T) {
	t.Parallel()
	h := startScreens(t, screenBook(), bff.EmptyTimeline{})
	tests := []struct {
		name      string
		id        string
		sections  []string
		total     string
		stats     []statProps
		positions map[string][]string
	}{
		{
			name: "mariana", id: sim.CustomerMariana,
			sections: []string{"summary", "allocation", "positions_stocks", "positions_etf", "positions_fixed_income", "history"},
			total:    "US$ 248.300,00",
			stats: []statProps{
				{Label: "Valor aplicado", Value: "US$ 168.900,00", Money: true},
				{Label: "Rentabilidade", Value: "+US$ 19.400,00 (+11,5%)", Tone: "pos", Money: true},
				{Label: "Caixa", Value: "US$ 60.000,00", Money: true},
				{Label: "Dia simulado", Value: "0"},
			},
			positions: map[string][]string{
				"positions_stocks":       {"cobalto", "farol"},
				"positions_etf":          {"acoesg", "renda"},
				"positions_fixed_income": {"corp"},
			},
		},
		{
			name: "thiago", id: sim.CustomerThiago,
			sections: []string{"summary", "allocation", "positions_stocks", "positions_etf", "history"},
			total:    "US$ 68.000,00",
			stats: []statProps{
				{Label: "Valor aplicado", Value: "US$ 7.100,00", Money: true},
				{Label: "Rentabilidade", Value: "+US$ 380,00 (+5,4%)", Tone: "pos", Money: true},
				{Label: "Caixa", Value: "US$ 60.520,00", Money: true},
				{Label: "Dia simulado", Value: "0"},
			},
			positions: map[string][]string{
				"positions_stocks": {"cobalto"},
				"positions_etf":    {"acoesg"},
			},
		},
		{
			name: "fernanda", id: sim.CustomerFernanda,
			sections: []string{"summary", "allocation", "positions_stocks", "positions_etf", "positions_fixed_income", "history"},
			total:    "US$ 8.200,00",
			stats: []statProps{
				{Label: "Valor aplicado", Value: "US$ 6.840,00", Money: true},
				{Label: "Rentabilidade", Value: "+US$ 212,00 (+3,1%)", Tone: "pos", Money: true},
				{Label: "Caixa", Value: "US$ 1.148,00", Money: true},
				{Label: "Dia simulado", Value: "0"},
			},
			positions: map[string][]string{
				"positions_stocks":       {"farol"},
				"positions_etf":          {"acoesg", "renda"},
				"positions_fixed_income": {"tbill"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rr, body := getScreen(t, h, tt.id, "carteira")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d %s", rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if body.Slug != "carteira" || body.Revision != "v1" || body.Title != "Carteira" || body.Subtitle != "Valores de mercado no dia simulado 0" {
				t.Errorf("envelope = %q %q %q / %q", body.Slug, body.Revision, body.Title, body.Subtitle)
			}
			if !slices.Equal(body.ids(), tt.sections) {
				t.Errorf("sections = %v, want %v", body.ids(), tt.sections)
			}
			if body.Omitted == nil || len(body.Omitted) != 0 {
				t.Errorf("omitted = %+v, want []", body.Omitted)
			}

			kind, raw := body.component(t, "summary")
			var summary portfolioSummaryProps
			decodeProps(t, raw, &summary)
			if kind != "portfolio_summary/default" || summary.TotalLabel != "Patrimônio total" || summary.Total != tt.total ||
				!slices.Equal(summary.Stats, tt.stats) {
				t.Errorf("summary = %s %+v", kind, summary)
			}

			kind, raw = body.component(t, "allocation")
			var alloc breakdownProps
			decodeProps(t, raw, &alloc)
			classes := make([]string, 0, len(alloc.Rows))
			sum := 0
			for _, row := range alloc.Rows {
				classes = append(classes, row.Class)
				sum += row.BarWidth
			}
			if kind != "allocation_breakdown/default" || alloc.Title != "Alocação" ||
				!slices.Equal(classes, []string{"stocks", "etfs", "fixed_income", "cash"}) || sum != 100 {
				t.Errorf("allocation = %s %+v", kind, alloc)
			}

			variants := map[string]string{"positions_stocks": "stocks", "positions_etf": "etf", "positions_fixed_income": "fixed_income"}
			for id, want := range tt.positions {
				kind, raw := body.component(t, id)
				var list positionListProps
				decodeProps(t, raw, &list)
				if kind != "position_list/"+variants[id] || list.AppliedLabel != "Aplicado" || !slices.Equal(list.ids(), want) {
					t.Errorf("%s = %s %+v, want ids %v", id, kind, list, want)
				}
			}

			kind, raw = body.component(t, "history")
			var history historyProps
			decodeProps(t, raw, &history)
			if kind != "activity_list/history" || history.Title != "Movimentações" || len(history.Items) != 0 ||
				history.EmptyText != "Nenhuma movimentação ainda." {
				t.Errorf("history = %s %+v", kind, history)
			}
		})
	}
}

// TestGetScreen_CarteiraMarianaPositions pins every prop of Mariana's stock
// list as the artboard draws it on day 0.
func TestGetScreen_CarteiraMarianaPositions(t *testing.T) {
	t.Parallel()
	h := startScreens(t, screenBook(), bff.EmptyTimeline{})
	_, body := getScreen(t, h, sim.CustomerMariana, "carteira")
	_, raw := body.component(t, "positions_stocks")
	var list positionListProps
	decodeProps(t, raw, &list)
	if list.Title != "Ações" || list.Subtotal != "US$ 90.900,00" || len(list.Items) != 2 {
		t.Fatalf("stocks = %+v", list)
	}
	cobalto := list.Items[0]
	if cobalto.Name != "Cobalto Semicondutores" || cobalto.Applied != "US$ 60.000,00" || cobalto.Value != "US$ 72.000,00" ||
		cobalto.Return != "+20,0%" || cobalto.ReturnTone != "pos" {
		t.Errorf("cobalto = %+v", cobalto)
	}
}

// startTimeline serves a real timeline index over bufconn and returns the
// BFF client for it.
func startTimeline(t *testing.T, idx *timeline.Index) bff.TimelineClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	timelinev1.RegisterTimelineServiceServer(gs, timeline.NewGRPCServer(idx))
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial timeline: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return bff.NewGRPCTimeline(timelinev1.NewTimelineServiceClient(conn))
}

// TestGetScreen_CarteiraAfterThiagoBuysEverything buys all of Thiago's cash
// in acoesg through the BFF, feeds the outbox row account-sim wrote for it
// to the timeline index, as the relay and timeline-indexer would, and reads
// Carteira: the ETF subtotal holds the purchase and the history names it.
func TestGetScreen_CarteiraAfterThiagoBuysEverything(t *testing.T) {
	t.Parallel()
	idx := timeline.NewIndex()
	mem := sim.NewMemory()
	h := startScreensOn(t, mem, screenBook(), startTimeline(t, idx))

	rr := postPOV(t, h, t.Context(), sim.CustomerThiago, "purchases", "tudo", `{"product_id":"acoesg","amount_cents":6052000}`)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("purchase = %d %s", rr.Code, rr.Body.String())
	}
	var accepted struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &accepted); err != nil || accepted.EventID == "" {
		t.Fatalf("purchase body = %s (%v)", rr.Body.String(), err)
	}
	var written []outbox.Row
	for _, row := range mem.PendingOutbox() {
		if row.EventID == accepted.EventID {
			written = append(written, row)
		}
	}
	if len(written) != 1 {
		t.Fatalf("outbox rows for %s = %d, want 1", accepted.EventID, len(written))
	}
	if _, _, err := idx.ApplyDelivery(t.Context(), written[0].RoutingKey, written[0].Payload); err != nil {
		t.Fatalf("index the purchase: %v", err)
	}

	rr, body := getScreen(t, h, sim.CustomerThiago, "carteira")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rr.Code, rr.Body.String())
	}
	_, raw := body.component(t, "positions_etf")
	var etf positionListProps
	decodeProps(t, raw, &etf)
	if etf.Subtotal != "US$ 65.960,00" || len(etf.Items) != 1 || etf.Items[0].Applied != "US$ 65.720,00" {
		t.Errorf("etf = %+v, want the purchase in acoesg", etf)
	}
	_, raw = body.component(t, "history")
	var history historyProps
	decodeProps(t, raw, &history)
	if len(history.Items) != 1 {
		t.Fatalf("history = %+v, want the purchase", history)
	}
	row := history.Items[0]
	if row.Title != "Compra · Maré Ações Globais ETF" || row.Value != "−US$ 60.520,00" || row.Icon != "out" || row.Meta != "agora" {
		t.Errorf("purchase row = %+v", row)
	}
	_, raw = body.component(t, "summary")
	var summary portfolioSummaryProps
	decodeProps(t, raw, &summary)
	if summary.Total != "US$ 68.000,00" || summary.Stats[2].Value != "US$ 0,00" {
		t.Errorf("summary = %+v, want the patrimony unchanged and no cash", summary)
	}
}

func TestGetScreen_CarteiraSourceFailures(t *testing.T) {
	t.Parallel()
	thiago := bff.POVAccount{
		CustomerID: sim.CustomerThiago, Acoes: 204_000, ETFs: 544_000, Caixa: 6_052_000, Patrimony: 6_800_000,
		Positions: []bff.POVPosition{
			{ProductID: "cobalto", AssetClass: "acoes", AppliedCents: 190_000, ValueCents: 204_000},
			{ProductID: "acoesg", AssetClass: "etfs", AppliedCents: 520_000, ValueCents: 544_000},
		},
	}
	catalog := make([]bff.POVProduct, 0, len(sim.Catalog()))
	for _, p := range sim.Catalog() {
		catalog = append(catalog, bff.POVProduct{
			ID: p.ID, Name: p.Name, AssetClass: p.AssetClass, Risk: p.Risk, ReturnLabel: p.ReturnLabel, MinimumCents: p.MinimumCents,
		})
	}
	tests := []struct {
		name     string
		pov      bff.POVSource
		tl       bff.TimelineClient
		sections []string
		omitted  []string
		subtitle string
	}{
		{
			name:     "timeline down",
			pov:      &fakePOV{accounts: []bff.POVAccount{thiago}, products: catalog},
			tl:       stubTimeline{err: errors.New("timeline unavailable")},
			sections: []string{"summary", "allocation", "positions_stocks", "positions_etf"},
			omitted:  []string{"history/activity_list/timeline"},
			subtitle: "Valores de mercado no dia simulado 0",
		},
		{
			name:     "catalog down",
			pov:      &fakePOV{accounts: []bff.POVAccount{thiago}, productsErr: errors.New("account-sim unavailable")},
			tl:       bff.EmptyTimeline{},
			sections: []string{"summary", "allocation", "history"},
			omitted: []string{
				"positions_stocks/position_list/catalog", "positions_etf/position_list/catalog",
				"positions_fixed_income/position_list/catalog",
			},
			subtitle: "Valores de mercado no dia simulado 0",
		},
		{
			name:     "account-sim down",
			pov:      failingPOV{},
			tl:       bff.EmptyTimeline{},
			sections: []string{"history"},
			omitted: []string{
				"summary/portfolio_summary/account-sim", "allocation/allocation_breakdown/account-sim",
				"positions_stocks/position_list/account-sim", "positions_etf/position_list/account-sim",
				"positions_fixed_income/position_list/account-sim",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, tt.tl, screenBook(), nil, nil, tt.pov, nil)
			rr, body := getScreen(t, h, sim.CustomerThiago, "carteira")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d %s", rr.Code, rr.Body.String())
			}
			if !slices.Equal(body.ids(), tt.sections) {
				t.Errorf("sections = %v, want %v", body.ids(), tt.sections)
			}
			omitted := make([]string, 0, len(body.Omitted))
			for _, o := range body.Omitted {
				omitted = append(omitted, o.ID+"/"+o.Type+"/"+o.Reason)
			}
			if !slices.Equal(omitted, tt.omitted) {
				t.Errorf("omitted = %v, want %v", omitted, tt.omitted)
			}
			if body.Title != "Carteira" || body.Subtitle != tt.subtitle {
				t.Errorf("heading = %q / %q, want Carteira / %q", body.Title, body.Subtitle, tt.subtitle)
			}
		})
	}
}

// The screen timeline adapter hands product and amount to the shared
// mapping, so the home activity names a purchase too.
func TestGetScreen_HomeActivityNamesAPurchase(t *testing.T) {
	t.Parallel()
	tl := stubTimeline{rows: map[string][]bff.TimelineEntry{
		sim.CustomerThiago: {{
			EventID: identity.MustNewV7(), Kind: "aplicacao", Title: "Aplicação", Ago: 3,
			Source: event.NameAccountEventRecorded, ProductID: "cobalto", AmountCents: 25_000,
		}},
	}}
	h := startScreens(t, screenBook(), tl)
	rr, body := getScreen(t, h, sim.CustomerThiago, "home")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	_, raw := body.component(t, "activity")
	var activity historyProps
	decodeProps(t, raw, &activity)
	if len(activity.Items) != 1 || activity.Items[0].Title != "Compra · Cobalto Semicondutores" ||
		activity.Items[0].Value != "−US$ 250,00" || !strings.HasPrefix(activity.Items[0].Meta, "há 3 min") {
		t.Errorf("activity = %+v", activity)
	}
}
