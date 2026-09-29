package bff_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/sim"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

// fakeSim is a POVSource with the simulation port. Advancing is idempotent by
// key; err, when set, fails every call. retried marks the next reply as sent
// more than once by the retry interceptor.
type fakeSim struct {
	fakePOV
	mu      sync.Mutex
	day     int
	keys    map[string]bff.AdvanceResult
	calls   int
	err     error
	retried bool
}

func newFakeSim() *fakeSim {
	return &fakeSim{keys: map[string]bff.AdvanceResult{}}
}

func (f *fakeSim) AdvanceDay(_ context.Context, key, commandID string) (bff.AdvanceResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return bff.AdvanceResult{}, f.err
	}
	if commandID == "" {
		return bff.AdvanceResult{}, status.Error(codes.InvalidArgument, "no command id")
	}
	if res, ok := f.keys[key]; ok {
		res.Replay, res.Retried, f.retried = true, f.retried, false
		return res, nil
	}
	f.day++
	res := bff.AdvanceResult{SimDay: f.day, EventIDs: []string{"ev-" + strconv.Itoa(f.day) + "-a", "ev-" + strconv.Itoa(f.day) + "-b"}}
	f.keys[key] = res
	return res, nil
}

func (f *fakeSim) SimDay(context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	return f.day, nil
}

// commit advances under key as account-sim does when the reply is lost on
// the way back: the day moved, and the next call with key replays it.
func (f *fakeSim) commit(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.day++
	f.keys[key] = bff.AdvanceResult{SimDay: f.day, EventIDs: []string{"ev-" + strconv.Itoa(f.day) + "-a"}}
}

// retryNext marks the next reply as retried by the interceptor.
func (f *fakeSim) retryNext() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retried = true
}

func (f *fakeSim) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeSim) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// clock is a settable request clock.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func postAdvance(t *testing.T, h http.Handler, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/client-pov/simulation/advance-day", nil)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

type advanceBody struct {
	SimDay  int    `json:"sim_day"`
	EventID string `json:"event_id"`
}

func advanceBodyOf(t *testing.T, rr *httptest.ResponseRecorder) advanceBody {
	t.Helper()
	var body advanceBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	return body
}

func simDayOf(t *testing.T, h http.Handler) int {
	t.Helper()
	rr := getPath(t, h, t.Context(), "/v1/client-pov/simulation")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET simulation = %d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		SimDay int `json:"sim_day"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rr.Body.String(), err)
	}
	return body.SimDay
}

func TestAdvanceDay_Answers(t *testing.T) {
	t.Parallel()
	src := newFakeSim()
	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, nil, nil, nil, src, nil)

	if rr := postAdvance(t, h, ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("no key = %d, want 400", rr.Code)
	}
	rr := postAdvance(t, h, "k1")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("advance = %d %s, want 202", rr.Code, rr.Body.String())
	}
	if got := advanceBodyOf(t, rr); got != (advanceBody{SimDay: 1, EventID: "ev-1-a"}) {
		t.Fatalf("body = %+v, want day 1 and the first event id", got)
	}
	if got := advanceBodyOf(t, postAdvance(t, h, "k1")); got != (advanceBody{SimDay: 1, EventID: "ev-1-a"}) {
		t.Fatalf("replay body = %+v, want the original reply", got)
	}
	if day := simDayOf(t, h); day != 1 {
		t.Fatalf("GET simulation = %d, want 1", day)
	}

	src.fail(status.Error(codes.Internal, "boom"))
	if rr := postAdvance(t, h, "k2"); rr.Code != http.StatusBadGateway {
		t.Fatalf("failed advance = %d, want 502", rr.Code)
	}
	if rr := getPath(t, h, t.Context(), "/v1/client-pov/simulation"); rr.Code != http.StatusBadGateway {
		t.Fatalf("failed GET simulation = %d, want 502", rr.Code)
	}
}

// Without the simulation port, as with account-sim unset, both routes are
// upstream failures.
func TestAdvanceDay_WithoutSimulation(t *testing.T) {
	t.Parallel()
	for _, pov := range []bff.POVSource{&fakePOV{}, nil} {
		h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, nil, nil, nil, pov, nil)
		if rr := postAdvance(t, h, "k"); rr.Code != http.StatusBadGateway {
			t.Errorf("advance = %d, want 502", rr.Code)
		}
		if rr := getPath(t, h, t.Context(), "/v1/client-pov/simulation"); rr.Code != http.StatusBadGateway {
			t.Errorf("GET simulation = %d, want 502", rr.Code)
		}
	}
}

// The advance budget is global: 20 in any 10 minutes. A known key replays
// for free; an Unavailable failure refunds, any other failure spends.
func TestAdvanceDay_GlobalLimit(t *testing.T) {
	t.Parallel()
	src := newFakeSim()
	clk := &clock{now: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, nil, nil, nil, src, clk.Now)

	src.fail(status.Error(codes.Unavailable, "down"))
	if rr := postAdvance(t, h, "lost"); rr.Code != http.StatusBadGateway {
		t.Fatalf("unavailable = %d, want 502", rr.Code)
	}
	src.fail(nil)
	for i := range 19 {
		if rr := postAdvance(t, h, "k"+strconv.Itoa(i)); rr.Code != http.StatusAccepted {
			t.Fatalf("advance %d = %d, want 202", i, rr.Code)
		}
		clk.add(10 * time.Second)
	}
	src.fail(status.Error(codes.DeadlineExceeded, "slow"))
	if rr := postAdvance(t, h, "uncertain"); rr.Code != http.StatusBadGateway {
		t.Fatalf("deadline = %d, want 502", rr.Code)
	}
	src.fail(nil)
	calls := src.callCount()
	rr := postAdvance(t, h, "k19")
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("21st advance = %d, want 429", rr.Code)
	}
	if got := src.callCount(); got != calls {
		t.Fatalf("account-sim calls = %d after a 429, want %d: a 429 never calls it", got, calls)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body["error"] != "ten_minutes" {
		t.Fatalf("429 body = %s, want {\"error\":\"ten_minutes\"}", rr.Body.String())
	}
	if rr := postAdvance(t, h, "k3"); rr.Code != http.StatusAccepted {
		t.Fatalf("replay over the limit = %d, want 202", rr.Code)
	}
	// The first advance leaves the window 10 minutes after it was sent.
	clk.add(10*time.Minute - 191*time.Second)
	if rr := postAdvance(t, h, "k19"); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("advance at the window edge = %d, want 429", rr.Code)
	}
	clk.add(time.Second)
	if rr := postAdvance(t, h, "k19"); rr.Code != http.StatusAccepted {
		t.Fatalf("advance after the window = %d, want 202", rr.Code)
	}
	if day := simDayOf(t, h); day != 20 {
		t.Fatalf("day = %d, want 20", day)
	}
}

// The two replay answers of one request: a replay that answers this request's
// own retry is its first commit and keeps the budget; a replay of a key whose
// earlier request failed uncertainly refunds this request's spend.
func TestAdvanceDay_ReplaySpend(t *testing.T) {
	t.Parallel()
	fill := func(t *testing.T, h http.Handler, src *fakeSim, n int) {
		t.Helper()
		for i := range n {
			if rr := postAdvance(t, h, "fill-"+strconv.Itoa(i)); rr.Code != http.StatusAccepted {
				t.Fatalf("fill %d = %d, want 202", i, rr.Code)
			}
		}
		calls := src.callCount()
		if rr := postAdvance(t, h, "over"); rr.Code != http.StatusTooManyRequests {
			t.Fatalf("advance over the budget = %d, want 429", rr.Code)
		}
		if got := src.callCount(); got != calls {
			t.Fatalf("account-sim calls = %d after a 429, want %d", got, calls)
		}
	}

	t.Run("a lost reply retried into a replay keeps the budget spent", func(t *testing.T) {
		t.Parallel()
		src := newFakeSim()
		h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, nil, nil, nil, src, nil)
		// account-sim commits, the reply is lost, the interceptor retries
		// through an Unavailable, and the last attempt replays the commit.
		src.commit("lost-reply")
		src.retryNext()
		rr := postAdvance(t, h, "lost-reply")
		if rr.Code != http.StatusAccepted || advanceBodyOf(t, rr).SimDay != 1 {
			t.Fatalf("retried advance = %d %s, want 202 day 1", rr.Code, rr.Body.String())
		}
		fill(t, h, src, 19)
	})

	t.Run("a replay after an uncertain 502 refunds its spend", func(t *testing.T) {
		t.Parallel()
		src := newFakeSim()
		h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, nil, nil, nil, src, nil)
		src.fail(status.Error(codes.DeadlineExceeded, "slow"))
		if rr := postAdvance(t, h, "uncertain"); rr.Code != http.StatusBadGateway {
			t.Fatalf("deadline = %d, want 502", rr.Code)
		}
		// The deadline hit after account-sim committed; the same key replays.
		src.fail(nil)
		src.commit("uncertain")
		rr := postAdvance(t, h, "uncertain")
		if rr.Code != http.StatusAccepted || advanceBodyOf(t, rr).SimDay != 1 {
			t.Fatalf("replay = %d %s, want 202 day 1", rr.Code, rr.Body.String())
		}
		// Only the uncertain 502 spent: 19 more fit.
		fill(t, h, src, 19)
	})
}

// TestGRPCPOV_Simulation drives the simulation routes through the gRPC
// adapter and the real account-sim server.
func TestGRPCPOV_Simulation(t *testing.T) {
	t.Parallel()
	h := startSim(t)
	if day := simDayOf(t, h); day != 0 {
		t.Fatalf("seed day = %d, want 0", day)
	}
	var first advanceBody
	for want := 1; want <= 3; want++ {
		rr := postAdvance(t, h, "day-"+strconv.Itoa(want))
		if rr.Code != http.StatusAccepted {
			t.Fatalf("advance to %d = %d %s", want, rr.Code, rr.Body.String())
		}
		got := advanceBodyOf(t, rr)
		if got.SimDay != want || got.EventID == "" {
			t.Fatalf("advance body = %+v, want day %d with an event id", got, want)
		}
		if want == 1 {
			first = got
		}
	}
	if got := advanceBodyOf(t, postAdvance(t, h, "day-1")); got != first {
		t.Fatalf("replay = %+v, want %+v", got, first)
	}
	if day := simDayOf(t, h); day != 3 {
		t.Fatalf("day = %d, want 3", day)
	}
}

// indexRevaluation indexes one of Mariana's reavaliacao events the way
// timeline-indexer does.
func indexRevaluation(t *testing.T, idx *timeline.Index, eventID string, day int, amount, before int64, productID string, productBP int) {
	t.Helper()
	delivery, err := json.Marshal(map[string]any{
		"event_id": eventID, "occurred_at": time.Now().UTC(),
		"customer_id": sim.CustomerMariana, "schema_version": 3,
		"payload": map[string]any{
			"kind": "reavaliacao", "amount": amount, "before": before, "after": before + amount,
			"sim_day": day, "product_id": productID, "product_change_bp": productBP,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := idx.ApplyDelivery(t.Context(), event.NameAccountEventRecorded, delivery); err != nil {
		t.Fatalf("index the revaluation: %v", err)
	}
}

// TestGetScreen_ShockDay advances the real account-sim to day 3, indexes the
// revaluations, and serves Mariana's home and Carteira as advisory reports
// the drop.
func TestGetScreen_ShockDay(t *testing.T) {
	t.Parallel()
	book := screenBook()
	book.moments[sim.CustomerMariana] = bff.MomentFacts{
		PortfolioReview: true, CashCents: 6_000_000, PatrimonyCents: 20_978_000,
		PortfolioDrop: true, DropBP: 1551, DropProductID: "cobalto", DropProductBP: -5350, DropDay: 3,
	}
	idx := timeline.NewIndex()
	h := startScreens(t, book, startTimeline(t, idx))
	for day := 1; day <= 3; day++ {
		rr := postAdvance(t, h, "shock-"+strconv.Itoa(day))
		if rr.Code != http.StatusAccepted {
			t.Fatalf("advance to %d = %d %s", day, rr.Code, rr.Body.String())
		}
		if day < 3 {
			indexRevaluation(t, idx, identity.MustNewV7(), day, 0, 24_830_000, "", 0)
		} else {
			indexRevaluation(t, idx, identity.MustNewV7(), day, -3_852_000, 24_830_000, "cobalto", -5350)
		}
	}

	rr, home := getScreen(t, h, sim.CustomerMariana, "home")
	if rr.Code != http.StatusOK {
		t.Fatalf("home = %d", rr.Code)
	}
	kind, raw := home.component(t, "moment")
	var moment momentProps
	decodeProps(t, raw, &moment)
	want := "moment_card/portfolio_drop | neg | Mariana, sua carteira caiu 15,5% hoje | " +
		"Cobalto Semicondutores recuou 53,5% no dia simulado 3. A Ana Paula Ribeiro já foi avisada e vai falar com você. |  | " +
		"navigate carteira Ver carteira"
	if got := moment.line(kind); got != want {
		t.Errorf("moment = %q\nwant      %q", got, want)
	}
	kind, raw = home.component(t, "wealth")
	var wealth struct {
		wealthProps
		DayChange     string `json:"day_change"`
		DayChangeTone string `json:"day_change_tone"`
	}
	decodeProps(t, raw, &wealth)
	if kind != "wealth_summary/with_day_change" || wealth.Total != "US$ 209.780,00" ||
		wealth.DayChange != "−US$ 38.520,00 (−15,5%) no dia 3" || wealth.DayChangeTone != "neg" {
		t.Errorf("wealth = %s %+v", kind, wealth)
	}

	rr, carteira := getScreen(t, h, sim.CustomerMariana, "carteira")
	if rr.Code != http.StatusOK {
		t.Fatalf("carteira = %d", rr.Code)
	}
	if carteira.Subtitle != "Valores de mercado no dia simulado 3" {
		t.Errorf("carteira subtitle = %q", carteira.Subtitle)
	}
	kind, raw = carteira.component(t, "summary")
	var summary struct {
		portfolioSummaryProps
		DayChange     string `json:"day_change"`
		DayChangeTone string `json:"day_change_tone"`
	}
	decodeProps(t, raw, &summary)
	if kind != "portfolio_summary/with_day_change" || summary.DayChange != "−US$ 38.520,00 (−15,5%) no dia 3" ||
		summary.DayChangeTone != "neg" || summary.Stats[len(summary.Stats)-1] != (statProps{Label: "Dia simulado", Value: "3"}) {
		t.Errorf("summary = %s %+v", kind, summary)
	}
	_, raw = carteira.component(t, "history")
	var history historyProps
	decodeProps(t, raw, &history)
	if len(history.Items) != 1 || history.Items[0].Title != "Reavaliação diária" ||
		history.Items[0].Meta != "dia simulado 3 · Cobalto Semicondutores −53,5%" ||
		history.Items[0].Value != "−US$ 38.520,00" || history.Items[0].Tone != "neg" || history.Items[0].Icon != "drop" {
		t.Errorf("history = %+v", history)
	}

	// Day 4 is flat: no pill.
	if rr := postAdvance(t, h, "shock-4"); rr.Code != http.StatusAccepted {
		t.Fatalf("advance to 4 = %d", rr.Code)
	}
	_, home = getScreen(t, h, sim.CustomerMariana, "home")
	if kind, _ := home.component(t, "wealth"); kind != "wealth_summary/default" {
		t.Errorf("day 4 wealth = %s, want the default", kind)
	}
}
