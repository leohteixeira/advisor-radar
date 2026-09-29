package bff_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	timelinev1 "github.com/leohteixeira/advisor-radar/gen/timeline/v1"
	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/sim"
	"github.com/leohteixeira/advisor-radar/internal/timeline"
)

// screenBook is the advisory book row of the three seed clients.
func screenBook() stubQueue {
	return stubQueue{customers: map[string]bff.Customer{
		sim.CustomerFernanda: {ID: sim.CustomerFernanda, Name: "Fernanda Lima", Segment: "Essencial", AUM: 8200, Advisor: "Ana Paula Ribeiro", Since: "2024"},
		sim.CustomerThiago:   {ID: sim.CustomerThiago, Name: "Thiago Azevedo", Segment: "Advance", AUM: 68000, Advisor: "Ana Paula Ribeiro", Since: "2024"},
		sim.CustomerMariana:  {ID: sim.CustomerMariana, Name: "Mariana Costa", Segment: "Singular", AUM: 248300, Advisor: "Ana Paula Ribeiro", Since: "2021"},
	}}
}

// stubTimeline answers every customer with rows, or fails with err.
type stubTimeline struct {
	rows map[string][]bff.TimelineEntry
	err  error
}

func (s stubTimeline) Search(_ context.Context, customerID, _, _ string) ([]bff.TimelineEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.rows[customerID], nil
}

// failingPOV is an account-sim that is down for every call.
type failingPOV struct{}

var errAccountSimDown = errors.New("account-sim unavailable")

func (failingPOV) List(context.Context) ([]bff.POVAccount, error) { return nil, errAccountSimDown }
func (failingPOV) Get(context.Context, string) (bff.POVAccount, error) {
	return bff.POVAccount{}, errAccountSimDown
}
func (failingPOV) Apply(context.Context, bff.POVCommand) (bff.POVResult, error) {
	return bff.POVResult{}, errAccountSimDown
}

// syncBuffer is a bytes.Buffer safe for the concurrent writes of a logger.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startScreens serves the real account-sim server over bufconn, seeded with
// the day-0 positions, and returns the BFF wired to it the production way.
func startScreens(t *testing.T, queue bff.QueueSource, tl bff.TimelineClient) *bff.Server {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	accountv1.RegisterAccountServiceServer(gs, sim.NewGRPCServer(sim.NewMemory(), nil))
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := bff.DialAccountSim("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
	)
	if err != nil {
		t.Fatalf("DialAccountSim: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	pov := bff.NewGRPCPOV(accountv1.NewAccountServiceClient(conn))
	return bff.NewHandlerWithPOV(bff.NewBoard(), nil, tl, queue, nil, nil, pov, nil)
}

// screenBody is the envelope with props kept raw for per-type decoding.
type screenBody struct {
	SchemaVersion int    `json:"schema_version"`
	Slug          string `json:"slug"`
	Revision      string `json:"revision"`
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle"`
	Sections      []struct {
		ID         string `json:"id"`
		Components []struct {
			Type    string          `json:"type"`
			Variant string          `json:"variant"`
			Props   json.RawMessage `json:"props"`
		} `json:"components"`
	} `json:"sections"`
	Omitted []struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Reason string `json:"reason"`
	} `json:"omitted"`
}

func (b screenBody) ids() []string {
	out := make([]string, 0, len(b.Sections))
	for _, s := range b.Sections {
		out = append(out, s.ID)
	}
	return out
}

// component returns "type/variant" and the props of a section.
func (b screenBody) component(t *testing.T, id string) (string, json.RawMessage) {
	t.Helper()
	for _, s := range b.Sections {
		if s.ID == id {
			if len(s.Components) != 1 {
				t.Fatalf("section %q has %d components", id, len(s.Components))
			}
			c := s.Components[0]
			return c.Type + "/" + c.Variant, c.Props
		}
	}
	t.Fatalf("no section %q in %v", id, b.ids())
	return "", nil
}

func getScreen(t *testing.T, h http.Handler, customerID, slug string) (*httptest.ResponseRecorder, screenBody) {
	t.Helper()
	rr := getPath(t, h, t.Context(), "/v1/client-pov/customers/"+customerID+"/screens/"+slug)
	var body screenBody
	if rr.Code == http.StatusOK {
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode screen %q: %v", rr.Body.String(), err)
		}
	}
	return rr, body
}

func decodeProps(t *testing.T, raw json.RawMessage, into any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		t.Fatalf("decode props %s: %v", raw, err)
	}
}

type wealthProps struct {
	TotalLabel string `json:"total_label"`
	Total      string `json:"total"`
	CashLabel  string `json:"cash_label"`
	Cash       string `json:"cash"`
	CashCents  int64  `json:"cash_cents"`
	Allocation []struct {
		Class    string `json:"class"`
		Label    string `json:"label"`
		Share    string `json:"share"`
		BarWidth int    `json:"bar_width"`
	} `json:"allocation"`
}

type advisorProps struct {
	Kicker   string `json:"kicker"`
	Name     string `json:"name"`
	Initials string `json:"initials"`
	Meta     string `json:"meta"`
	Action   struct {
		Type   string `json:"type"`
		Label  string `json:"label"`
		Target string `json:"target"`
	} `json:"action"`
}

type momentProps struct {
	Kicker string `json:"kicker"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Tone   string `json:"tone"`
	Icon   string `json:"icon"`
}

type activityProps struct {
	Title string `json:"title"`
	Items []struct {
		Icon  string `json:"icon"`
		Title string `json:"title"`
		Meta  string `json:"meta"`
	} `json:"items"`
	EmptyText string `json:"empty_text"`
}

type actionsProps struct {
	Items []struct {
		Label  string `json:"label"`
		Icon   string `json:"icon"`
		Action struct {
			Type   string `json:"type"`
			Label  string `json:"label"`
			Target string `json:"target"`
		} `json:"action"`
	} `json:"items"`
}

func TestGetScreen_SeedClients(t *testing.T) {
	t.Parallel()
	tl := stubTimeline{rows: map[string][]bff.TimelineEntry{
		sim.CustomerThiago: {
			{Kind: "nota", Title: "Nota do assessor", Source: "advisory.note.recorded", Ago: 1},
			{Kind: "segmento", Title: "Segmento", Source: "alert.raised", Ago: 4},
			{Kind: "aporte", Title: "Aporte", Source: "account.event.recorded", Ago: 5},
		},
		sim.CustomerMariana: {
			{Kind: "mensagem", Title: "Mensagem · e-mail", Source: "message.triaged", Ago: 30 * 60},
			{Kind: "mensagem", Title: "Mensagem · e-mail", Source: "message.received", Ago: 30 * 60},
			{Kind: "caso", Title: "Caso k1 aberto", Source: "case.opened", Ago: 20},
		},
	}}
	h := startScreens(t, screenBook(), tl)
	tests := []struct {
		name       string
		id         string
		title      string
		subtitle   string
		total      string
		cash       string
		cashCents  int64
		shares     []string
		advisor    string
		kicker     string
		meta       string
		activity   string
		activityN  int
		firstTitle string
	}{
		{
			name: "fernanda", id: sim.CustomerFernanda,
			title: "Olá, Fernanda", subtitle: "Cliente Essencial desde 2024",
			total: "US$ 8.200,00", cash: "US$ 1.148,00", cashCents: 114_800,
			shares:  []string{"stocks 20%", "etfs 45%", "fixed_income 21%", "cash 14%"},
			advisor: "advisor_card/default", kicker: "Sua assessora", meta: "Resposta em até 24 h · cliente Essencial",
			activity: "activity_list/empty",
		},
		{
			name: "thiago", id: sim.CustomerThiago,
			title: "Olá, Thiago", subtitle: "Cliente Advance desde 2024",
			total: "US$ 68.000,00", cash: "US$ 60.520,00", cashCents: 6_052_000,
			shares:  []string{"stocks 3%", "etfs 8%", "cash 89%"},
			advisor: "advisor_card/default", kicker: "Sua assessora", meta: "Resposta em até 4 h · cliente Advance",
			activity: "activity_list/recent", activityN: 1, firstTitle: "Aporte",
		},
		{
			name: "mariana", id: sim.CustomerMariana,
			title: "Olá, Mariana", subtitle: "Cliente Singular desde 2021",
			total: "US$ 248.300,00", cash: "US$ 60.000,00", cashCents: 6_000_000,
			shares:  []string{"stocks 37%", "etfs 24%", "fixed_income 15%", "cash 24%"},
			advisor: "advisor_card/dedicated", kicker: "Sua assessora dedicada", meta: "Resposta em até 1 h · cliente Singular",
			activity: "activity_list/recent", activityN: 1, firstTitle: "Mensagem · e-mail",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rr, body := getScreen(t, h, tt.id, "home")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d %s", rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if got := rr.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q", got)
			}
			if body.SchemaVersion != 1 || body.Slug != "home" || body.Revision != "v1" {
				t.Errorf("envelope = %d %q %q", body.SchemaVersion, body.Slug, body.Revision)
			}
			if body.Title != tt.title || body.Subtitle != tt.subtitle {
				t.Errorf("heading = %q / %q", body.Title, body.Subtitle)
			}
			if want := []string{"moment", "wealth", "actions", "advisor", "activity"}; !slices.Equal(body.ids(), want) {
				t.Errorf("sections = %v, want %v", body.ids(), want)
			}
			if body.Omitted == nil || len(body.Omitted) != 0 {
				t.Errorf("omitted = %+v, want []", body.Omitted)
			}

			kind, raw := body.component(t, "moment")
			var moment momentProps
			decodeProps(t, raw, &moment)
			if kind != "moment_card/welcome" || moment.Title != tt.title+". Sua conta está em dia." || moment.Tone != "neutral" || moment.Kicker != "Tudo em dia" {
				t.Errorf("moment = %s %+v", kind, moment)
			}

			kind, raw = body.component(t, "wealth")
			var wealth wealthProps
			decodeProps(t, raw, &wealth)
			if kind != "wealth_summary/default" || wealth.Total != tt.total || wealth.Cash != tt.cash || wealth.CashCents != tt.cashCents {
				t.Errorf("wealth = %s %+v", kind, wealth)
			}
			shares := make([]string, 0, len(wealth.Allocation))
			for _, row := range wealth.Allocation {
				shares = append(shares, row.Class+" "+row.Share)
			}
			if !slices.Equal(shares, tt.shares) {
				t.Errorf("allocation = %v, want %v", shares, tt.shares)
			}

			kind, raw = body.component(t, "actions")
			var actions actionsProps
			decodeProps(t, raw, &actions)
			targets := make([]string, 0, len(actions.Items))
			for _, it := range actions.Items {
				targets = append(targets, it.Label+"→"+it.Action.Type+" "+it.Action.Target)
			}
			if want := []string{"Depositar→panel deposit", "Sacar→panel withdraw", "Mensagem→panel message", "Reclamar→panel complaint"}; kind != "action_grid/default" || !slices.Equal(targets, want) {
				t.Errorf("actions = %s %v", kind, targets)
			}

			kind, raw = body.component(t, "advisor")
			var advisor advisorProps
			decodeProps(t, raw, &advisor)
			if kind != tt.advisor || advisor.Kicker != tt.kicker || advisor.Meta != tt.meta ||
				advisor.Name != "Ana Paula Ribeiro" || advisor.Initials != "AP" || advisor.Action.Target != "message" {
				t.Errorf("advisor = %s %+v", kind, advisor)
			}

			kind, raw = body.component(t, "activity")
			var activity activityProps
			decodeProps(t, raw, &activity)
			if kind != tt.activity || len(activity.Items) != tt.activityN {
				t.Errorf("activity = %s %+v", kind, activity)
			}
			if tt.activityN > 0 && activity.Items[0].Title != tt.firstTitle {
				t.Errorf("first activity = %+v, want title %q", activity.Items[0], tt.firstTitle)
			}
			if tt.activityN == 0 && activity.EmptyText != "Suas movimentações aparecem aqui assim que acontecerem." {
				t.Errorf("empty text = %q", activity.EmptyText)
			}
		})
	}
}

func TestGetScreen_Status(t *testing.T) {
	t.Parallel()
	h := startScreens(t, screenBook(), bff.EmptyTimeline{})
	tests := []struct {
		name     string
		id       string
		slug     string
		expected int
	}{
		{name: "non-v7 id", id: "not-a-uuid", slug: "home", expected: http.StatusBadRequest},
		{name: "investir not served yet", id: sim.CustomerThiago, slug: "investir", expected: http.StatusNotFound},
		{name: "unknown slug", id: sim.CustomerThiago, slug: "x", expected: http.StatusNotFound},
		{name: "unknown customer", id: "01a0e3a4-9a44-7000-8000-000000000001", slug: "home", expected: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rr, _ := getScreen(t, h, tt.id, tt.slug)
			if rr.Code != tt.expected {
				t.Errorf("status = %d, want %d", rr.Code, tt.expected)
			}
			if got := rr.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}

func TestGetScreen_SourceFailures(t *testing.T) {
	t.Parallel()
	thiagoAccount := bff.POVAccount{
		CustomerID: sim.CustomerThiago,
		Acoes:      204_000, ETFs: 544_000, Caixa: 6_052_000, Patrimony: 6_800_000,
	}
	tests := []struct {
		name     string
		pov      bff.POVSource
		queue    bff.QueueSource
		timeline bff.TimelineClient
		sections []string
		omitted  string
		title    string
		subtitle string
	}{
		{
			name:     "timeline down",
			pov:      &fakePOV{accounts: []bff.POVAccount{thiagoAccount}},
			queue:    screenBook(),
			timeline: stubTimeline{err: errors.New("timeline unavailable")},
			sections: []string{"moment", "wealth", "actions", "advisor"},
			omitted:  "activity/activity_list/timeline",
			title:    "Olá, Thiago", subtitle: "Cliente Advance desde 2024",
		},
		{
			name:     "advisory down",
			pov:      &fakePOV{accounts: []bff.POVAccount{thiagoAccount}},
			queue:    bff.EmptyQueue{},
			timeline: bff.EmptyTimeline{},
			sections: []string{"moment", "wealth", "actions", "activity"},
			omitted:  "advisor/advisor_card/advisory",
			title:    "Olá", subtitle: "",
		},
		{
			name:     "account down",
			pov:      failingPOV{},
			queue:    screenBook(),
			timeline: bff.EmptyTimeline{},
			sections: []string{"moment", "actions", "advisor", "activity"},
			omitted:  "wealth/wealth_summary/account-sim",
			title:    "Olá, Thiago", subtitle: "Cliente Advance desde 2024",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, tt.timeline, tt.queue, nil, nil, tt.pov, nil)
			rr, body := getScreen(t, h, sim.CustomerThiago, "home")
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
			if !slices.Equal(omitted, []string{tt.omitted}) {
				t.Errorf("omitted = %v, want [%s]", omitted, tt.omitted)
			}
			if body.Title != tt.title || body.Subtitle != tt.subtitle {
				t.Errorf("heading = %q / %q, want %q / %q", body.Title, body.Subtitle, tt.title, tt.subtitle)
			}
			if strings.Contains(rr.Body.String(), `"subtitle"`) != (tt.subtitle != "") {
				t.Errorf("subtitle presence wrong in %s", rr.Body.String())
			}
		})
	}
}

// TestGetScreen_LogsNoCopyOrValues checks the info and warn logs carry ids,
// sources, and reasons, never rendered copy, names, or account values.
func TestGetScreen_LogsNoCopyOrValues(t *testing.T) {
	t.Parallel()
	var buf syncBuffer
	h := startScreens(t, screenBook(), stubTimeline{err: errors.New("timeline unavailable")})
	h.SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	rr, _ := getScreen(t, h, sim.CustomerMariana, "home")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	logs := buf.String()
	for _, want := range []string{`"msg":"screen served"`, `"omitted":"activity:timeline"`, `"msg":"screen part failed"`, `"source":"timeline"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs lack %s: %s", want, logs)
		}
	}
	for _, leak := range []string{"US$", "Olá", "Mariana", "Ana Paula", "248.300", "Singular", "Resposta"} {
		if strings.Contains(logs, leak) {
			t.Errorf("logs contain %q: %s", leak, logs)
		}
	}
}

// TestGetScreen_ActivityFromIndexer feeds the real timeline index a mix of
// client and team events and reads the home through the gRPC timeline client:
// only the account event and the received message reach the client, aged
// from occurred_at against the request clock.
func TestGetScreen_ActivityFromIndexer(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	idx := timeline.NewIndex()
	rows := []struct {
		key     string
		ago     time.Duration
		payload map[string]any
	}{
		{event.NameAccountEventRecorded, 2 * time.Hour, map[string]any{"kind": "aporte", "amount": 1_000_000}},
		{event.NameAlertRaised, 2 * time.Hour, map[string]any{"kind": "aporte", "reason": "Aporte grande", "rule": "Regra de aporte"}},
		{event.NameMessageReceived, 10 * time.Minute, map[string]any{"channel": "chat", "text": "Quero sacar tudo"}},
		{event.NameMessageTriaged, 9 * time.Minute, map[string]any{"channel": "chat", "text": "Quero sacar tudo", "intent": "encerramento"}},
		{event.NameCaseOpened, 8 * time.Minute, map[string]any{"case_id": "k1", "state": "aberto", "text": "Risco de saída"}},
		{"advisory.note.recorded", 5 * time.Minute, map[string]any{"kind": "nota", "title": "Nota do assessor", "text": "Ligar amanhã"}},
	}
	for _, r := range rows {
		body, _ := json.Marshal(map[string]any{
			"event_id": identity.MustNewV7(), "occurred_at": now.Add(-r.ago),
			"customer_id": sim.CustomerThiago, "schema_version": 2, "payload": r.payload,
		})
		if _, _, err := idx.ApplyDelivery(t.Context(), r.key, body); err != nil {
			t.Fatalf("apply %s: %v", r.key, err)
		}
	}
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
	tl := bff.NewGRPCTimeline(timelinev1.NewTimelineServiceClient(conn))
	pov := &fakePOV{accounts: []bff.POVAccount{{CustomerID: sim.CustomerThiago, Caixa: 100, Patrimony: 100}}}
	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, tl, screenBook(), nil, nil, pov, func() time.Time { return now })

	rr, body := getScreen(t, h, sim.CustomerThiago, "home")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rr.Code, rr.Body.String())
	}
	kind, raw := body.component(t, "activity")
	var activity activityProps
	decodeProps(t, raw, &activity)
	got := make([]string, 0, len(activity.Items))
	for _, it := range activity.Items {
		got = append(got, it.Icon+" "+it.Title+" "+it.Meta)
	}
	want := []string{"msg Mensagem · chat há 10 min", "in Aporte há 2 h"}
	if kind != "activity_list/recent" || !slices.Equal(got, want) {
		t.Errorf("activity = %s %v, want %v", kind, got, want)
	}
}

func TestGetScreen_AccountSimDisabled(t *testing.T) {
	t.Parallel()
	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, bff.EmptyTimeline{}, screenBook(), nil, nil, nil, nil)
	rr, body := getScreen(t, h, sim.CustomerThiago, "home")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with wealth omitted", rr.Code)
	}
	if want := []string{"moment", "actions", "advisor", "activity"}; !slices.Equal(body.ids(), want) {
		t.Errorf("sections = %v, want %v", body.ids(), want)
	}
	if len(body.Omitted) != 1 || body.Omitted[0].ID != "wealth" || body.Omitted[0].Reason != "account-sim" {
		t.Errorf("omitted = %+v", body.Omitted)
	}
}

// TestGetScreen_BuildErrorLogsNoValues gives Thiago a segment with no SLA in
// the catalog: the advisor card fails to build, and the log names the section
// and error class without the segment.
func TestGetScreen_BuildErrorLogsNoValues(t *testing.T) {
	t.Parallel()
	book := screenBook()
	thiago := book.customers[sim.CustomerThiago]
	thiago.Segment = "Privatissimo"
	book.customers = map[string]bff.Customer{sim.CustomerThiago: thiago}
	var buf syncBuffer
	h := startScreens(t, book, bff.EmptyTimeline{})
	h.SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	rr, body := getScreen(t, h, sim.CustomerThiago, "home")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if len(body.Omitted) != 1 || body.Omitted[0].ID != "advisor" || body.Omitted[0].Reason != "build_error" {
		t.Errorf("omitted = %+v, want advisor build_error", body.Omitted)
	}
	logs := buf.String()
	for _, want := range []string{`"section":"advisor"`, `"error":"build_error"`, `"omitted":"advisor:build_error"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs lack %s: %s", want, logs)
		}
	}
	if strings.Contains(logs, "Privatissimo") {
		t.Errorf("logs contain the segment: %s", logs)
	}
}

func TestGetScreen_ClientGone(t *testing.T) {
	t.Parallel()
	var buf syncBuffer
	h := startScreens(t, screenBook(), stubTimeline{err: errors.New("timeline unavailable")})
	h.SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rr := getPath(t, h, ctx, "/v1/client-pov/customers/"+sim.CustomerThiago+"/screens/home")
	if rr.Body.Len() != 0 {
		t.Errorf("body = %q, want nothing written", rr.Body.String())
	}
	if logs := buf.String(); logs != "" {
		t.Errorf("logs for a gone client: %s", logs)
	}
}
