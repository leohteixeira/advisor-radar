package bff_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

func TestHTTP_QueueSegmentFilterAndScoreOrder(t *testing.T) {
	t.Parallel()
	c1 := identity.MustNewV7()
	c2 := identity.MustNewV7()
	c3 := identity.MustNewV7()
	s1 := identity.MustNewV7()
	s2 := identity.MustNewV7()
	s3 := identity.MustNewV7()
	churn := true
	q := stubQueue{
		customers: map[string]bff.Customer{
			c1: {ID: c1, Name: "A Singular", Segment: "Singular"},
			c2: {ID: c2, Name: "B Advance", Segment: "Advance"},
			c3: {ID: c3, Name: "C Singular Low", Segment: "Singular"},
		},
		queue: []bff.Signal{
			{ID: s3, Kind: "message", Client: c3, Name: "C Singular Low", Segment: "Singular", Ago: 5, Intent: "Operacional"},
			{ID: s1, Kind: "message", Client: c1, Name: "A Singular", Segment: "Singular", Ago: 5, Intent: "Reclamação", Churn: &churn, Frustration: intPtr(3)},
			{ID: s2, Kind: "message", Client: c2, Name: "B Advance", Segment: "Advance", Ago: 5, Intent: "Operacional"},
		},
	}
	h := bff.NewHandler(bff.NewBoard(), nil, nil, q, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/queue?segmento=Singular", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Items  []bff.Signal   `json:"items"`
		Facets bff.ListFacets `json:"facets"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %d, want 2 Singular", len(body.Items))
	}
	if body.Items[0].ID != s1 {
		t.Fatalf("want higher score first: got %s then %s", body.Items[0].ID, body.Items[1].ID)
	}
	for _, it := range body.Items {
		if it.Segment != "Singular" {
			t.Fatalf("unexpected segment %q", it.Segment)
		}
	}
	var singularCount, advanceCount int
	for _, f := range body.Facets.Segmento {
		switch f.Label {
		case "Singular":
			singularCount = f.Count
		case "Advance":
			advanceCount = f.Count
		}
	}
	if singularCount != 2 || advanceCount != 1 {
		t.Fatalf("segment facets Singular=%d Advance=%d (counts ignore own filter)", singularCount, advanceCount)
	}
}

func TestHTTP_CasesSegmentFilterAndRemOrder(t *testing.T) {
	t.Parallel()
	c1 := identity.MustNewV7()
	c2 := identity.MustNewV7()
	c3 := identity.MustNewV7()
	k1 := identity.MustNewV7()
	k2 := identity.MustNewV7()
	k3 := identity.MustNewV7()
	q := stubQueue{
		customers: map[string]bff.Customer{
			c1: {ID: c1, Name: "Case A", Segment: "Advance"},
			c2: {ID: c2, Name: "Case B", Segment: "Advance"},
			c3: {ID: c3, Name: "Case C", Segment: "Essencial"},
		},
	}
	cases := stubCases{items: []bff.Case{
		{ID: k2, Client: c2, Name: "Case B", Segment: "Advance", Signal: "x", State: 0, OpenedAgo: 10, SlaTotal: 120, SlaRemaining: 110, History: []bff.CaseHistoryEntry{}},
		{ID: k1, Client: c1, Name: "Case A", Segment: "Advance", Signal: "y", State: 0, OpenedAgo: 100, SlaTotal: 120, SlaRemaining: 20, History: []bff.CaseHistoryEntry{}},
		{ID: k3, Client: c3, Name: "Case C", Segment: "Essencial", Signal: "z", State: 0, OpenedAgo: 5, SlaTotal: 60, SlaRemaining: 55, History: []bff.CaseHistoryEntry{}},
	}}
	h := bff.NewHandler(bff.NewBoard(), nil, nil, q, nil, cases)
	req := httptest.NewRequest(http.MethodGet, "/v1/cases?segmento=Advance", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Items []bff.Case `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %d, want 2 Advance", len(body.Items))
	}
	// k1 has 20 minutes left and k2 has 110: the smaller remainder comes first.
	if body.Items[0].ID != k1 || body.Items[1].ID != k2 {
		t.Fatalf("want rem ascending: got %+v", body.Items)
	}
}

// The SLA state and the order of a case come from the remaining minutes cases
// reports, not from slaTotal minus openedAgo: after a segment change the
// clock counts from the change (ADR 0012).
func TestHTTP_CasesSLAStateFollowsRemainingMinutes(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	upgraded, downgraded, due := identity.MustNewV7(), identity.MustNewV7(), identity.MustNewV7()
	q := stubQueue{customers: map[string]bff.Customer{cust: {ID: cust, Name: "Thiago", Segment: "Advance"}}}
	// The handler writes name and segment into the cases it lists, so every
	// subtest gets its own slice.
	newHandler := func() http.Handler {
		return bff.NewHandler(bff.NewBoard(), nil, nil, q, nil, stubCases{items: []bff.Case{
			// Opened 20 h ago as Essencial, Advance for the last hour: 3 h left,
			// although 240 − 1200 is long overdue.
			{ID: upgraded, Client: cust, State: 0, OpenedAgo: 1200, SlaTotal: 240, SlaRemaining: 180, History: []bff.CaseHistoryEntry{}},
			// Opened 30 min ago as Singular, Advance since then, already past the
			// new deadline by cases' count.
			{ID: downgraded, Client: cust, State: 1, OpenedAgo: 30, SlaTotal: 240, SlaRemaining: -5, History: []bff.CaseHistoryEntry{}},
			{ID: due, Client: cust, State: 0, OpenedAgo: 30, SlaTotal: 240, SlaRemaining: 10, History: []bff.CaseHistoryEntry{}},
		}})
	}

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "least time left first", query: "", want: []string{downgraded, due, upgraded}},
		{name: "on time", query: "?sla=No+prazo", want: []string{upgraded}},
		{name: "due soon", query: "?sla=Vencendo", want: []string{due}},
		{name: "overdue", query: "?sla=Vencido", want: []string{downgraded}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rr := httptest.NewRecorder()
			newHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/cases"+tt.query, nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d", rr.Code)
			}
			var body struct {
				Items []struct {
					ID           string `json:"id"`
					SlaRemaining *int   `json:"slaRemaining"`
				} `json:"items"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(body.Items))
			for _, it := range body.Items {
				if it.SlaRemaining == nil {
					t.Fatalf("case %s has no slaRemaining", it.ID)
				}
				got = append(got, it.ID)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("cases = %v, want %v", got, tt.want)
			}
		})
	}
}

// stubActions is advisory's actions port answering a fixed list.
type stubActions struct {
	bff.UnavailableActions
	items []bff.SignalAction
}

func (s stubActions) List(context.Context) ([]bff.SignalAction, error) { return s.items, nil }

// boardFixture is a queue with every kind of card: listed, contacted and
// snoozed signals, signals a case represents, and cases of every state.
type boardFixture struct {
	h http.Handler

	sergioMsg, patriciaAlert, biaSnoozed, biaContacted string
	anaMsg, patriciaMsg, biaJoined                     string
	anaCase, patriciaResolved, sergioCase, biaCase     string
}

var fixtureNow = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

func newBoardFixture(t *testing.T) boardFixture {
	t.Helper()
	sergio, patricia, ana, bia := identity.MustNewV7(), identity.MustNewV7(), identity.MustNewV7(), identity.MustNewV7()
	f := boardFixture{
		sergioMsg: identity.MustNewV7(), patriciaAlert: identity.MustNewV7(),
		biaSnoozed: identity.MustNewV7(), biaContacted: identity.MustNewV7(),
		anaMsg: identity.MustNewV7(), patriciaMsg: identity.MustNewV7(), biaJoined: identity.MustNewV7(),
		anaCase: identity.MustNewV7(), patriciaResolved: identity.MustNewV7(),
		sergioCase: identity.MustNewV7(), biaCase: identity.MustNewV7(),
	}
	yes := true
	queue := stubQueue{
		customers: map[string]bff.Customer{
			sergio:   {ID: sergio, Name: "Sérgio Cardoso", Segment: "Singular"},
			patricia: {ID: patricia, Name: "Patrícia Gomes", Segment: "Advance"},
			ana:      {ID: ana, Name: "Ana Beatriz Oliveira", Segment: "Advance"},
			bia:      {ID: bia, Name: "Beatriz Santana", Segment: "Essencial"},
		},
		queue: []bff.Signal{
			{ID: f.sergioMsg, Kind: "message", Client: sergio, Ago: 5, Intent: "Reclamação", Churn: &yes},
			{ID: f.patriciaAlert, Kind: "alert", Client: patricia, Ago: 5, Alert: "saque"},
			{ID: f.biaSnoozed, Kind: "message", Client: bia, Ago: 5, Intent: "Operacional", Fallback: true},
			{ID: f.biaContacted, Kind: "alert", Client: bia, Ago: 2000, Alert: "queda"},
			// The three below opened or joined a case.
			{ID: f.anaMsg, Kind: "message", Client: ana, Ago: 40, Intent: "Encerramento", Churn: &yes},
			{ID: f.patriciaMsg, Kind: "message", Client: patricia, Ago: 400, Intent: "Reclamação", Human: &yes},
			{ID: f.biaJoined, Kind: "alert", Client: bia, Ago: 30, Alert: "aporte"},
		},
	}
	until, contacted := fixtureNow.Add(30*time.Minute), fixtureNow.Add(-time.Hour)
	actions := stubActions{items: []bff.SignalAction{
		{SignalID: f.biaSnoozed, SnoozedUntil: &until},
		{SignalID: f.biaContacted, ContactedAt: &contacted},
	}}
	cases := stubCases{items: []bff.Case{
		{ID: f.anaCase, Client: ana, Signal: f.anaMsg, State: 0, SlaTotal: 120, SlaRemaining: 80, SlaState: "No prazo", History: []bff.CaseHistoryEntry{}},
		{ID: f.patriciaResolved, Client: patricia, Signal: f.patriciaMsg, State: 3, SlaTotal: 120, SlaRemaining: 10, SlaState: "Concluído", History: []bff.CaseHistoryEntry{}},
		{ID: f.sergioCase, Client: sergio, Signal: identity.MustNewV7(), State: 1, SlaTotal: 30, SlaRemaining: -5, SlaState: "Vencido", History: []bff.CaseHistoryEntry{}},
		{ID: f.biaCase, Client: bia, Signal: identity.MustNewV7(), State: 2, SlaTotal: 1440, SlaRemaining: 100, SlaState: "Vencendo", History: []bff.CaseHistoryEntry{
			{Ago: 50, Kind: "open", Text: "Caso aberto"},
			{Ago: 30, Kind: "signal", Text: "Aporte grande adicionado ao caso", Signal: f.biaJoined},
		}},
	}}
	f.h = bff.NewHandlerWithPOV(bff.NewBoard(), actions, nil, queue, nil, cases, nil, func() time.Time { return fixtureNow })
	return f
}

type listBody struct {
	Signals []bff.Signal
	Cases   []bff.Case
	Facets  bff.ListFacets
}

// readBoard reads both list routes with the same filters and fails when
// their facets differ.
func readBoard(t *testing.T, h http.Handler, query url.Values) listBody {
	t.Helper()
	get := func(path string, into any) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path+"?"+query.Encode(), nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, rr.Code, rr.Body.String())
		}
		if err := json.Unmarshal(rr.Body.Bytes(), into); err != nil {
			t.Fatal(err)
		}
	}
	var queue struct {
		Items  []bff.Signal   `json:"items"`
		Facets bff.ListFacets `json:"facets"`
	}
	var cases struct {
		Items  []bff.Case     `json:"items"`
		Facets bff.ListFacets `json:"facets"`
	}
	get("/v1/queue", &queue)
	get("/v1/cases", &cases)
	if !reflect.DeepEqual(queue.Facets, cases.Facets) {
		t.Fatalf("facets differ for %q:\nqueue %+v\ncases %+v", query.Encode(), queue.Facets, cases.Facets)
	}
	return listBody{Signals: queue.Items, Cases: cases.Items, Facets: queue.Facets}
}

func facetCount(t *testing.T, rows []bff.FacetCount, label string) int {
	t.Helper()
	for _, row := range rows {
		if row.Label == label {
			return row.Count
		}
	}
	t.Fatalf("no facet %q in %+v", label, rows)
	return 0
}

// For every filter value, on both routes, the facet count is the number of
// cards, signals plus cases, the board shows with that value picked.
func TestHTTP_FacetCountEqualsCardsShown(t *testing.T) {
	t.Parallel()
	f := newBoardFixture(t)

	bases := []struct {
		name  string
		query url.Values
	}{
		{name: "no filter", query: url.Values{}},
		{name: "search", query: url.Values{"q": {"beatriz"}}},
		{name: "one segment", query: url.Values{"segmento": {"Advance"}}},
		{name: "signals and open cases", query: url.Values{"andamento": {"Novo sinal", "Aberto"}}},
		{name: "snoozed only", query: url.Values{"andamento": {"Adiados"}}},
		{name: "two filters", query: url.Values{"sla": {"No prazo"}, "sinal": {"Risco de saída"}}},
	}
	for _, base := range bases {
		t.Run(base.name, func(t *testing.T) {
			t.Parallel()
			facets := readBoard(t, f.h, base.query).Facets
			dims := map[string][]bff.FacetCount{
				"andamento": facets.Andamento, "sla": facets.SLA, "sinal": facets.Sinal,
				"motivo": facets.Motivo, "segmento": facets.Segmento,
			}
			for dim, rows := range dims {
				if len(rows) == 0 {
					t.Fatalf("no %s facets", dim)
				}
				for _, row := range rows {
					picked := url.Values{}
					for k, v := range base.query {
						picked[k] = v
					}
					picked[dim] = []string{row.Label}
					shown := readBoard(t, f.h, picked)
					if got := len(shown.Signals) + len(shown.Cases); got != row.Count {
						t.Errorf("%s=%q: facet says %d, the board shows %d cards (%d signals, %d cases)",
							dim, row.Label, row.Count, got, len(shown.Signals), len(shown.Cases))
					}
				}
			}
		})
	}
}

// The counts of the unfiltered board: cases count next to signals, a signal a
// case represents counts once (as the case), and a snoozed signal counts only
// under Adiados.
func TestHTTP_FacetCountsOfTheDefaultBoard(t *testing.T) {
	t.Parallel()
	f := newBoardFixture(t)
	facets := readBoard(t, f.h, url.Values{}).Facets

	tests := []struct {
		name  string
		rows  []bff.FacetCount
		label string
		want  int
	}{
		{name: "listed signals", rows: facets.Andamento, label: "Novo sinal", want: 3},
		{name: "snoozed signal", rows: facets.Andamento, label: "Adiados", want: 1},
		{name: "open case", rows: facets.Andamento, label: "Aberto", want: 1},
		{name: "resolved case", rows: facets.Andamento, label: "Resolvido", want: 1},
		{name: "on time: two signals and a case", rows: facets.SLA, label: "No prazo", want: 3},
		{name: "overdue: a signal and a case", rows: facets.SLA, label: "Vencido", want: 2},
		{name: "churn risk: a signal and the case of a hidden one", rows: facets.Sinal, label: "Risco de saída", want: 2},
		{name: "human request only on a resolved case", rows: facets.Sinal, label: "Pedido humano", want: 0},
		{name: "motivo of a case comes from its signal", rows: facets.Motivo, label: "Encerramento", want: 1},
		{name: "snoozed signal is not counted by motivo", rows: facets.Motivo, label: "Operacional", want: 0},
		{name: "segment: a signal and a case", rows: facets.Segmento, label: "Advance", want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := facetCount(t, tt.rows, tt.label); got != tt.want {
				t.Fatalf("%s = %d, want %d", tt.label, got, tt.want)
			}
		})
	}
}

// The name search ignores case and accents on both routes.
func TestHTTP_SearchIgnoresAccents(t *testing.T) {
	t.Parallel()
	f := newBoardFixture(t)

	tests := []struct {
		name        string
		q           string
		wantSignals []string
		wantCases   []string
	}{
		{name: "plain letters find an accented name", q: "sergio", wantSignals: []string{f.sergioMsg}, wantCases: []string{f.sergioCase}},
		{name: "upper case with the accent", q: "SÉRGIO", wantSignals: []string{f.sergioMsg}, wantCases: []string{f.sergioCase}},
		{name: "capitalised without the accent", q: "Patricia", wantSignals: []string{f.patriciaAlert}, wantCases: []string{}},
		{name: "accent typed where the name has none", q: "gômes", wantSignals: []string{f.patriciaAlert}, wantCases: []string{}},
		{name: "decomposed accent", q: "Se\u0301rgio", wantSignals: []string{f.sergioMsg}, wantCases: []string{f.sergioCase}},
		{name: "no match", q: "zuleica", wantSignals: []string{}, wantCases: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := readBoard(t, f.h, url.Values{"q": {tt.q}})
			signals := make([]string, 0, len(got.Signals))
			for _, s := range got.Signals {
				signals = append(signals, s.ID)
			}
			cases := make([]string, 0, len(got.Cases))
			for _, c := range got.Cases {
				cases = append(cases, c.ID)
			}
			if !slices.Equal(signals, tt.wantSignals) || !slices.Equal(cases, tt.wantCases) {
				t.Fatalf("q=%q: signals %v cases %v, want %v and %v", tt.q, signals, cases, tt.wantSignals, tt.wantCases)
			}
		})
	}
}

// A snoozed signal is off the default board and listed, with snoozed_until,
// by the Adiados filter; a contacted one stays listed with contacted_at.
func TestHTTP_QueueSnoozedAndContactedSignals(t *testing.T) {
	t.Parallel()
	f := newBoardFixture(t)

	byID := func(items []bff.Signal) map[string]bff.Signal {
		out := make(map[string]bff.Signal, len(items))
		for _, s := range items {
			out[s.ID] = s
		}
		return out
	}

	board := byID(readBoard(t, f.h, url.Values{}).Signals)
	if _, ok := board[f.biaSnoozed]; ok {
		t.Fatal("the default board lists a snoozed signal")
	}
	if s := board[f.biaContacted]; s.ContactedAt == nil || s.SnoozedUntil != nil {
		t.Fatalf("contacted signal = %+v, want contacted_at and no snoozed_until", s)
	}
	if s := board[f.sergioMsg]; s.ContactedAt != nil || s.SnoozedUntil != nil {
		t.Fatalf("untouched signal = %+v, want no action fields", s)
	}

	snoozed := readBoard(t, f.h, url.Values{"andamento": {"Adiados"}})
	if len(snoozed.Signals) != 1 || snoozed.Signals[0].ID != f.biaSnoozed || len(snoozed.Cases) != 0 {
		t.Fatalf("Adiados lists %+v and %d cases, want only the snoozed signal", snoozed.Signals, len(snoozed.Cases))
	}
	if until := snoozed.Signals[0].SnoozedUntil; until == nil || !until.Equal(fixtureNow.Add(30*time.Minute)) {
		t.Fatalf("snoozed_until = %v", until)
	}

	both := readBoard(t, f.h, url.Values{"andamento": {"Adiados", "Novo sinal"}})
	if len(both.Signals) != 4 {
		t.Fatalf("Adiados + Novo sinal lists %d signals, want 4", len(both.Signals))
	}
}

// A snooze that ran out is a new signal again.
func TestHTTP_QueueExpiredSnoozeIsListed(t *testing.T) {
	t.Parallel()
	cust, sig := identity.MustNewV7(), identity.MustNewV7()
	past := fixtureNow.Add(-time.Minute)
	h := bff.NewHandlerWithPOV(
		bff.NewBoard(),
		stubActions{items: []bff.SignalAction{{SignalID: sig, SnoozedUntil: &past}}},
		nil,
		stubQueue{queue: []bff.Signal{{ID: sig, Kind: "alert", Client: cust, Alert: "aporte"}}},
		nil, nil, nil,
		func() time.Time { return fixtureNow },
	)
	got := readBoard(t, h, url.Values{})
	if len(got.Signals) != 1 || got.Signals[0].SnoozedUntil != nil {
		t.Fatalf("signals = %+v, want the signal listed without snoozed_until", got.Signals)
	}
	if n := facetCount(t, got.Facets.Andamento, "Adiados"); n != 0 {
		t.Fatalf("Adiados = %d, want 0", n)
	}
}

// A signal that opened or joined a case is represented by the case: the queue
// does not list it, whatever the state of the case, and matches a live
// message by its triaged id or by the id of the message it was triaged from.
func TestHTTP_QueueHidesSignalsACaseRepresents(t *testing.T) {
	t.Parallel()

	cust := identity.MustNewV7()
	seeded := identity.MustNewV7()
	triaged, source := identity.MustNewV7(), identity.MustNewV7()
	other := identity.MustNewV7()

	tests := []struct {
		name     string
		cases    []bff.Case
		wantList []string
	}{
		{name: "no case", cases: nil, wantList: []string{seeded, triaged}},
		{
			name:     "case opened from the signal",
			cases:    []bff.Case{{ID: identity.MustNewV7(), Client: cust, Signal: seeded}},
			wantList: []string{triaged},
		},
		{
			name:     "resolved case opened from the signal",
			cases:    []bff.Case{{ID: identity.MustNewV7(), Client: cust, Signal: seeded, State: 3}},
			wantList: []string{triaged},
		},
		{
			name:     "case opened by the intake from the source message",
			cases:    []bff.Case{{ID: identity.MustNewV7(), Client: cust, Signal: source}},
			wantList: []string{seeded},
		},
		{
			name:     "case opened by the advisor from the live signal",
			cases:    []bff.Case{{ID: identity.MustNewV7(), Client: cust, Signal: triaged}},
			wantList: []string{seeded},
		},
		{
			name: "signal joined to the history of an open case",
			cases: []bff.Case{{ID: identity.MustNewV7(), Client: cust, Signal: other, History: []bff.CaseHistoryEntry{
				{Kind: "signal", Text: "joined", Signal: seeded},
			}}},
			wantList: []string{triaged},
		},
		{
			name: "later message in the history, by its source id",
			cases: []bff.Case{{ID: identity.MustNewV7(), Client: cust, Signal: other, History: []bff.CaseHistoryEntry{
				{Kind: "message", Text: "later message", Signal: source},
			}}},
			wantList: []string{seeded},
		},
		{
			name: "history lines without a signal hide nothing",
			cases: []bff.Case{{ID: identity.MustNewV7(), Client: cust, Signal: other, History: []bff.CaseHistoryEntry{
				{Kind: "state", Text: "Caso passou para Em atendimento"},
			}}},
			wantList: []string{seeded, triaged},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := bff.NewBoard()
			live := envelopeJSON(t, triaged, cust, map[string]any{
				"source_event_id": source, "text": "oi", "intent": "operacional", "intent_prob": 0.9, "channel": "chat",
			})
			if err := board.ApplyDelivery(t.Context(), "message.triaged", live); err != nil {
				t.Fatal(err)
			}
			queue := stubQueue{queue: []bff.Signal{{ID: seeded, Kind: "alert", Client: cust, Alert: "saque"}}}
			h := bff.NewHandler(board, nil, nil, queue, nil, stubCases{items: tt.cases})

			got := readBoard(t, h, url.Values{})
			ids := make([]string, 0, len(got.Signals))
			for _, s := range got.Signals {
				ids = append(ids, s.ID)
			}
			slices.Sort(ids)
			want := slices.Sorted(slices.Values(tt.wantList))
			if !slices.Equal(ids, want) {
				t.Fatalf("queue lists %v, want %v", ids, want)
			}
			if n := facetCount(t, got.Facets.Andamento, "Novo sinal"); n != len(tt.wantList) {
				t.Fatalf("Novo sinal = %d, want %d", n, len(tt.wantList))
			}
		})
	}
}

// A case names the signal it came from, also when that signal is no longer
// listed and when the case stores the source message id of a live message.
func TestHTTP_CaseOrigin(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()
	message, alert := identity.MustNewV7(), identity.MustNewV7()
	triaged, source := identity.MustNewV7(), identity.MustNewV7()
	fromMessage, fromAlert, fromLive, unknown := identity.MustNewV7(), identity.MustNewV7(), identity.MustNewV7(), identity.MustNewV7()

	board := bff.NewBoard()
	live := envelopeJSON(t, triaged, cust, map[string]any{
		"source_event_id": source, "text": "oi", "intent": "cambio", "intent_prob": 0.9, "channel": "chat",
	})
	if err := board.ApplyDelivery(t.Context(), "message.triaged", live); err != nil {
		t.Fatal(err)
	}
	queue := stubQueue{queue: []bff.Signal{
		{ID: message, Kind: "message", Client: cust, Intent: "Reclamação"},
		{ID: alert, Kind: "alert", Client: cust, Alert: "saque"},
	}}
	cases := stubCases{items: []bff.Case{
		{ID: fromMessage, Client: cust, Signal: message},
		{ID: fromAlert, Client: cust, Signal: alert},
		{ID: fromLive, Client: cust, Signal: source},
		{ID: unknown, Client: cust, Signal: identity.MustNewV7()},
	}}
	h := bff.NewHandler(board, nil, nil, queue, nil, cases)

	want := map[string]string{
		fromMessage: "mensagem · Reclamação",
		fromAlert:   "saque relevante",
		fromLive:    "mensagem · Câmbio",
		unknown:     "",
	}
	got := readBoard(t, h, url.Values{})
	if len(got.Cases) != len(want) {
		t.Fatalf("cases = %d, want %d", len(got.Cases), len(want))
	}
	for _, c := range got.Cases {
		if c.Origin != want[c.ID] {
			t.Errorf("case %s origin = %q, want %q", c.ID, c.Origin, want[c.ID])
		}
	}
	// The Motivo filter reaches the case through the same link.
	byMotivo := readBoard(t, h, url.Values{"motivo": {"Câmbio"}})
	if len(byMotivo.Cases) != 1 || byMotivo.Cases[0].ID != fromLive {
		t.Fatalf("motivo=Câmbio cases = %+v, want the case of the live message", byMotivo.Cases)
	}
}

// advancingCases is a cases port whose Advance answers one case.
type advancingCases struct {
	stubCases
	advanced bff.Case
}

func (a advancingCases) Advance(context.Context, string) (bff.Case, error) { return a.advanced, nil }

// cases decides the SLA state and the BFF passes it on, filters and counts by
// it. Only a case without one, from an older cases binary, gets the state
// from the same formula over the remaining minutes.
func TestHTTP_CaseSLAState(t *testing.T) {
	t.Parallel()
	cust := identity.MustNewV7()

	tests := []struct {
		name string
		in   bff.Case
		want string
	}{
		// 200 of 240 minutes left would be "No prazo" by the formula.
		{name: "state from cases wins over the formula", in: bff.Case{SlaTotal: 240, SlaRemaining: 200, SlaState: "Vencendo"}, want: "Vencendo"},
		{name: "resolved from cases", in: bff.Case{State: 3, SlaTotal: 240, SlaRemaining: -9, SlaState: "Concluído"}, want: "Concluído"},
		{name: "older cases: on time", in: bff.Case{SlaTotal: 240, SlaRemaining: 80}, want: "No prazo"},
		{name: "older cases: below a third", in: bff.Case{SlaTotal: 240, SlaRemaining: 78}, want: "Vencendo"},
		{name: "older cases: below the 20 minute floor", in: bff.Case{SlaTotal: 30, SlaRemaining: 19}, want: "Vencendo"},
		{name: "older cases: at the floor", in: bff.Case{SlaTotal: 30, SlaRemaining: 20}, want: "No prazo"},
		{name: "older cases: overdue", in: bff.Case{SlaTotal: 30, SlaRemaining: -1}, want: "Vencido"},
		{name: "older cases: resolved", in: bff.Case{State: 3, SlaTotal: 30, SlaRemaining: -1}, want: "Concluído"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := tt.in
			in.ID, in.Client = identity.MustNewV7(), cust
			h := bff.NewHandler(bff.NewBoard(), nil, nil, nil, nil, advancingCases{stubCases: stubCases{items: []bff.Case{in}}, advanced: in})

			listed := readBoard(t, h, url.Values{"andamento": {"Aberto", "Resolvido"}})
			if len(listed.Cases) != 1 || listed.Cases[0].SlaState != tt.want {
				t.Fatalf("listed = %+v, want slaState %q", listed.Cases, tt.want)
			}
			bySLA := readBoard(t, h, url.Values{"andamento": {"Aberto", "Resolvido"}, "sla": {tt.want}})
			if len(bySLA.Cases) != 1 {
				t.Fatalf("sla=%q lists %d cases, want 1", tt.want, len(bySLA.Cases))
			}

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/cases/"+in.ID+"/advance", nil))
			var advanced bff.Case
			if err := json.Unmarshal(rr.Body.Bytes(), &advanced); err != nil || rr.Code != http.StatusOK {
				t.Fatalf("advance = %d %s (%v)", rr.Code, rr.Body.String(), err)
			}
			if advanced.SlaState != tt.want {
				t.Fatalf("advanced slaState = %q, want %q", advanced.SlaState, tt.want)
			}
		})
	}
}

func intPtr(n int) *int { return &n }

// "Contato" (a message asking to be contacted) and "Sem contato há muito
// tempo" (an alert) are two motives: each filter value returns exactly the
// cards its facet counts, never the other's.
func TestHTTP_MotivoContatoIsNotSemContato(t *testing.T) {
	t.Parallel()
	fernanda, camila := identity.MustNewV7(), identity.MustNewV7()
	msgA, msgB, alert := identity.MustNewV7(), identity.MustNewV7(), identity.MustNewV7()
	q := stubQueue{
		queue: []bff.Signal{
			{ID: msgA, Kind: "message", Client: fernanda, Intent: "Contato", Text: "Consegue pedir para o meu assessor me ligar?"},
			{ID: msgB, Kind: "message", Client: fernanda, Intent: "Contato", Text: "Pode me ligar?"},
			{ID: alert, Kind: "alert", Client: camila, Alert: "contato"},
		},
		customers: map[string]bff.Customer{
			fernanda: {ID: fernanda, Name: "Fernanda Lima", Segment: "Essencial"},
			camila:   {ID: camila, Name: "Camila Rodrigues", Segment: "Advance"},
		},
	}
	h := bff.NewHandler(bff.NewBoard(), nil, nil, q, nil, stubCases{})

	tests := []struct {
		motivo string
		want   []string
	}{
		{motivo: "Contato", want: []string{msgA, msgB}},
		{motivo: "Sem contato há muito tempo", want: []string{alert}},
	}
	for _, tt := range tests {
		t.Run(tt.motivo, func(t *testing.T) {
			t.Parallel()
			got := readBoard(t, h, url.Values{"motivo": {tt.motivo}})
			ids := make([]string, 0, len(got.Signals))
			for _, s := range got.Signals {
				ids = append(ids, s.ID)
			}
			slices.Sort(ids)
			want := slices.Sorted(slices.Values(tt.want))
			if !slices.Equal(ids, want) {
				t.Fatalf("motivo=%q lists %v, want %v", tt.motivo, ids, want)
			}
			if n := facetCount(t, got.Facets.Motivo, tt.motivo); n != len(tt.want) {
				t.Fatalf("facet %q = %d, want %d", tt.motivo, n, len(tt.want))
			}
		})
	}
}
