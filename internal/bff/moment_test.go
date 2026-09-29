package bff_test

import (
	"context"
	"net"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	advisoryv1 "github.com/leohteixeira/advisor-radar/gen/advisory/v1"
	casesv1 "github.com/leohteixeira/advisor-radar/gen/cases/v1"
	"github.com/leohteixeira/advisor-radar/internal/advisory"
	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// serveBufconn serves register on an in-memory listener and returns a
// client connection to it.
func serveBufconn(t *testing.T, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	register(gs)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// seedFacts is the advisory book of the seed clients for the profile and
// moment reads, without PostgreSQL.
type seedFacts struct{}

func (seedFacts) InvestorProfile(_ context.Context, customerID string) (advisory.InvestorProfile, error) {
	switch customerID {
	case sim.CustomerFernanda:
		return advisory.InvestorProfile{Profile: advisory.ProfileConservador, AssessedOn: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)}, nil
	case sim.CustomerThiago:
		return advisory.InvestorProfile{Profile: advisory.ProfileArrojado, AssessedOn: time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)}, nil
	case sim.CustomerMariana:
		return advisory.InvestorProfile{Profile: advisory.ProfileModerado, AssessedOn: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)}, nil
	}
	return advisory.InvestorProfile{}, advisory.ErrUnknownCustomer
}

func (seedFacts) MomentBook(_ context.Context, customerID string, _ time.Time) (advisory.MomentBook, error) {
	switch customerID {
	case sim.CustomerFernanda:
		return advisory.MomentBook{Segment: "Essencial"}, nil
	case sim.CustomerThiago:
		return advisory.MomentBook{Segment: "Advance"}, nil
	case sim.CustomerMariana:
		return advisory.MomentBook{Segment: "Singular"}, nil
	}
	return advisory.MomentBook{}, advisory.ErrUnknownCustomer
}

// advisoryQueue is the seed book for the customer read and the real advisory
// gRPC client for the profile and moment reads.
type advisoryQueue struct {
	stubQueue
	grpc *bff.GRPCQueue
}

func (q advisoryQueue) MomentFacts(ctx context.Context, id string) (bff.MomentFacts, error) {
	return q.grpc.MomentFacts(ctx, id)
}

func (q advisoryQueue) InvestorProfile(ctx context.Context, id string) (bff.InvestorProfile, error) {
	return q.grpc.InvestorProfile(ctx, id)
}

// TestGetScreen_MomentsFromAdvisory runs the moment chain over gRPC: the
// account-sim memory store, advisory reading balances from it, and the BFF
// reading the facts and the profile from advisory.
func TestGetScreen_MomentsFromAdvisory(t *testing.T) {
	t.Parallel()
	accountConn := serveBufconn(t, func(s *grpc.Server) {
		accountv1.RegisterAccountServiceServer(s, sim.NewGRPCServer(sim.NewMemory(), nil))
	})
	adv := advisory.NewGRPCServer(nil,
		advisory.WithFactsReader(seedFacts{}),
		advisory.WithAccountReader(advisory.NewAccountSim(accountv1.NewAccountServiceClient(accountConn))),
	)
	advisoryConn := serveBufconn(t, func(s *grpc.Server) { advisoryv1.RegisterAdvisoryServiceServer(s, adv) })
	queue := advisoryQueue{stubQueue: screenBook(), grpc: bff.NewGRPCQueue(advisoryv1.NewAdvisoryServiceClient(advisoryConn))}
	pov := bff.NewGRPCPOV(accountv1.NewAccountServiceClient(accountConn))
	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, bff.EmptyTimeline{}, queue, nil, nil, pov, nil)

	tests := []struct {
		id       string
		expected string
	}{
		{sim.CustomerFernanda, "moment_card/segment_upgrade_near | gold | Fernanda, faltam US$ 1.800,00 para o Advance | " +
			"A partir de US$ 10.000,00 você vira cliente Advance, com resposta da assessoria em até 4 h. |  | panel deposit Depositar"},
		{sim.CustomerThiago, "moment_card/idle_cash | info | Thiago, 89% do seu patrimônio está em caixa | " +
			"US$ 60.520,00 parados em caixa. Veja produtos para o seu perfil arrojado. |  | navigate investir Ver produtos"},
		{sim.CustomerMariana, "moment_card/portfolio_review | neutral | Mariana, sua revisão de carteira está disponível | " +
			"A Ana Paula Ribeiro separou 30 minutos nesta semana para revisar a carteira com você. |  | panel message Conversar"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			rr, body := getScreen(t, h, tt.id, "home")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d %s", rr.Code, rr.Body.String())
			}
			kind, raw := body.component(t, "moment")
			var moment momentProps
			decodeProps(t, raw, &moment)
			if got := moment.line(kind); got != tt.expected {
				t.Errorf("moment = %q, want %q", got, tt.expected)
			}
		})
	}

	profile, err := queue.InvestorProfile(t.Context(), sim.CustomerThiago)
	if err != nil {
		t.Fatalf("InvestorProfile: %v", err)
	}
	if profile.Profile != "arrojado" || profile.MaxRisk != 5 || !profile.AssessedOn.Equal(time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("profile = %+v", profile)
	}
	if !slices.Equal(profile.MaxRiskTable, maxRiskTable()) {
		t.Errorf("max risk table = %+v, want %+v", profile.MaxRiskTable, maxRiskTable())
	}

	// Perfil reads every level's max_risk from the same advisory gRPC chain.
	rr, body := getScreen(t, h, sim.CustomerFernanda, "perfil")
	if rr.Code != http.StatusOK {
		t.Fatalf("perfil status = %d %s", rr.Code, rr.Body.String())
	}
	for _, o := range body.Omitted {
		if o.ID == "suitability" {
			t.Errorf("suitability omitted, reason %q", o.Reason)
		}
	}
	_, raw := body.component(t, "suitability")
	var scale scaleProps
	decodeProps(t, raw, &scale)
	var risks []int
	for _, level := range scale.Levels {
		risks = append(risks, level.MaxRisk)
	}
	if !slices.Equal(risks, []int{2, 3, 5}) {
		t.Errorf("perfil level max_risk = %v, want [2 3 5]", risks)
	}
	if _, err := queue.MomentFacts(t.Context(), "01a0e3a4-9a44-7000-8000-000000000001"); status.Code(err) != codes.NotFound {
		t.Errorf("MomentFacts(unknown) = %v, want NotFound", err)
	}
}

// recordingCases is a cases server that answers ListCases with fixed cases
// and records each request's customer_id.
type recordingCases struct {
	casesv1.UnimplementedCasesServiceServer

	mu   sync.Mutex
	seen []string
}

func (r *recordingCases) ListCases(_ context.Context, req *casesv1.ListCasesRequest) (*casesv1.ListCasesResponse, error) {
	r.mu.Lock()
	r.seen = append(r.seen, req.GetCustomerId())
	r.mu.Unlock()
	return &casesv1.ListCasesResponse{
		Items:  []*casesv1.Case{{Id: "k1", Client: req.GetCustomerId(), State: 1, OpenedAgo: 12}},
		States: bff.CaseStates,
	}, nil
}

func (r *recordingCases) requests() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

func TestGRPCCases_CustomerCases(t *testing.T) {
	t.Parallel()
	srv := &recordingCases{}
	conn := serveBufconn(t, func(s *grpc.Server) { casesv1.RegisterCasesServiceServer(s, srv) })
	client := bff.NewGRPCCases(casesv1.NewCasesServiceClient(conn))

	items, states, err := client.CustomerCases(t.Context(), sim.CustomerMariana)
	if err != nil {
		t.Fatalf("CustomerCases: %v", err)
	}
	if len(items) != 1 || items[0].ID != "k1" || items[0].Client != sim.CustomerMariana || items[0].State != 1 || items[0].OpenedAgo != 12 {
		t.Errorf("items = %+v", items)
	}
	if len(states) != len(bff.CaseStates) {
		t.Errorf("states = %v", states)
	}
	if _, _, err := client.ListCases(t.Context()); err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	if got := srv.requests(); len(got) != 2 || got[0] != sim.CustomerMariana || got[1] != "" {
		t.Errorf("customer_id sent = %q, want the customer, then empty for the board", got)
	}
}

// factsAdvisory answers GetMomentFacts with fixed facts.
type factsAdvisory struct {
	advisoryv1.UnimplementedAdvisoryServiceServer
	facts *advisoryv1.MomentFacts
}

func (f factsAdvisory) GetMomentFacts(context.Context, *advisoryv1.GetMomentFactsRequest) (*advisoryv1.MomentFacts, error) {
	return f.facts, nil
}

func TestGRPCQueue_MomentFacts(t *testing.T) {
	t.Parallel()
	facts := &advisoryv1.MomentFacts{
		SegmentUpgraded: true, UpgradedSegment: "Advance",
		SegmentUpgradeNear: true, UpgradeGapCents: 180_000,
		IdleCash: true, CashCents: 1_114_800, PatrimonyCents: 1_820_000,
		PortfolioReview: true, PortfolioDrop: true,
		DropBp: 1551, DropProductId: "cobalto", DropProductBp: -5350, DropDay: 3,
	}
	conn := serveBufconn(t, func(s *grpc.Server) {
		advisoryv1.RegisterAdvisoryServiceServer(s, factsAdvisory{facts: facts})
	})
	got, err := bff.NewGRPCQueue(advisoryv1.NewAdvisoryServiceClient(conn)).MomentFacts(t.Context(), sim.CustomerFernanda)
	if err != nil {
		t.Fatalf("MomentFacts: %v", err)
	}
	want := bff.MomentFacts{
		SegmentUpgraded: true, UpgradedSegment: "Advance",
		SegmentUpgradeNear: true, UpgradeGapCents: 180_000,
		IdleCash: true, CashCents: 1_114_800, PatrimonyCents: 1_820_000,
		PortfolioReview: true, PortfolioDrop: true,
		DropBP: 1551, DropProductID: "cobalto", DropProductBP: -5350, DropDay: 3,
	}
	if got != want {
		t.Errorf("MomentFacts = %+v, want %+v", got, want)
	}
}
