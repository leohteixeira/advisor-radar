package bff

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/screen"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// errPOVDisabled is the account source failure when the BFF runs without
// ACCOUNT_SIM_GRPC_TARGET.
var errPOVDisabled = errors.New("bff: account-sim is not configured")

// newScreenEngine composes screens from the handler's own account-sim,
// advisory, cases, and timeline clients, measuring relative times against
// now.
func newScreenEngine(pov POVSource, queue QueueSource, cases CaseSource, tl TimelineClient, now func() time.Time) *screen.Engine {
	return screen.MustNew(screen.Sources{
		Accounts:  screenAccounts{pov: pov},
		Customers: screenCustomers{queue: queue},
		Activity:  screenActivity{timeline: tl},
		Moments:   screenMoments{queue: queue},
		Profiles:  screenProfiles{queue: queue},
		Cases:     screenCases{cases: cases},
		Products:  screenProducts{pov: pov},
	}, screen.WithClock(now))
}

// screenProducts reads the product catalog over account/v1 through the POV
// source.
type screenProducts struct {
	pov POVSource
}

// Products implements screen.ProductSource. Without account-sim configured
// the catalog is a failed source.
func (p screenProducts) Products(ctx context.Context) ([]screen.Product, error) {
	products, err := p.pov.Products(ctx)
	if err != nil {
		return nil, fmt.Errorf("bff: screen catalog: %w", err)
	}
	out := make([]screen.Product, 0, len(products))
	for _, product := range products {
		out = append(out, screen.Product{
			ID:           product.ID,
			Name:         product.Name,
			AssetClass:   product.AssetClass,
			Risk:         product.Risk,
			ReturnLabel:  product.ReturnLabel,
			MinimumCents: product.MinimumCents,
		})
	}
	return out, nil
}

// screenAccounts reads the account over account/v1 through the POV source.
type screenAccounts struct {
	pov POVSource
}

// Account implements screen.AccountSource. account-sim NotFound, which the
// POV source maps to sim.ErrUnknownCustomer, becomes screen.ErrUnknownCustomer.
// Without account-sim configured, the account is a failed source, not an
// unknown customer.
func (a screenAccounts) Account(ctx context.Context, customerID string) (screen.Account, error) {
	if _, disabled := a.pov.(emptyPOV); disabled {
		return screen.Account{}, errPOVDisabled
	}
	account, err := a.pov.Get(ctx, customerID)
	if errors.Is(err, sim.ErrUnknownCustomer) {
		return screen.Account{}, fmt.Errorf("bff: screen account: %w: %w", screen.ErrUnknownCustomer, err)
	}
	if err != nil {
		return screen.Account{}, fmt.Errorf("bff: screen account: %w", err)
	}
	positions := make([]screen.Position, 0, len(account.Positions))
	for _, p := range account.Positions {
		positions = append(positions, screen.Position{
			ProductID:    p.ProductID,
			AssetClass:   p.AssetClass,
			AppliedCents: p.AppliedCents,
			ValueCents:   p.ValueCents,
		})
	}
	return screen.Account{
		Acoes:     account.Acoes,
		ETFs:      account.ETFs,
		RendaFixa: account.RendaFixa,
		Cash:      account.Caixa,
		Patrimony: account.Patrimony,
		Positions: positions,
	}, nil
}

// screenCustomers reads the advisory book row. Advisor is already the
// operator's display name: advisory joins it in GetCustomer.
type screenCustomers struct {
	queue QueueSource
}

// Customer implements screen.CustomerSource.
func (c screenCustomers) Customer(ctx context.Context, customerID string) (screen.Customer, error) {
	customer, err := c.queue.GetCustomer(ctx, customerID)
	if err != nil {
		return screen.Customer{}, fmt.Errorf("bff: screen customer: %w", err)
	}
	return screen.Customer{
		Name:    customer.Name,
		Segment: customer.Segment,
		Advisor: customer.Advisor,
		Since:   customer.Since,
	}, nil
}

// screenActivity reads the customer timeline. Ago is in minutes.
type screenActivity struct {
	timeline TimelineClient
}

// Activity implements screen.ActivitySource.
func (a screenActivity) Activity(ctx context.Context, customerID string) ([]screen.Activity, error) {
	rows, err := a.timeline.Search(ctx, customerID, "", "")
	if err != nil {
		return nil, fmt.Errorf("bff: screen activity: %w", err)
	}
	out := make([]screen.Activity, 0, len(rows))
	for _, row := range rows {
		out = append(out, screen.Activity{
			Kind:       row.Kind,
			Title:      row.Title,
			Source:     row.Source,
			OccurredAt: row.OccurredAt,
			Age:        time.Duration(max(row.Ago, 0)) * time.Minute,
		})
	}
	return out, nil
}

// screenMoments reads the advisory moment facts.
type screenMoments struct {
	queue QueueSource
}

// Moments implements screen.MomentSource.
func (m screenMoments) Moments(ctx context.Context, customerID string) (screen.MomentFacts, error) {
	f, err := m.queue.MomentFacts(ctx, customerID)
	if err != nil {
		return screen.MomentFacts{}, fmt.Errorf("bff: screen moments: %w", err)
	}
	return screen.MomentFacts{
		SegmentUpgraded:    f.SegmentUpgraded,
		UpgradedSegment:    f.UpgradedSegment,
		SegmentUpgradeNear: f.SegmentUpgradeNear,
		UpgradeGapCents:    f.UpgradeGapCents,
		IdleCash:           f.IdleCash,
		CashCents:          f.CashCents,
		PatrimonyCents:     f.PatrimonyCents,
		PortfolioReview:    f.PortfolioReview,
		PortfolioDrop:      f.PortfolioDrop,
	}, nil
}

// screenProfiles reads the advisory investor profile.
type screenProfiles struct {
	queue QueueSource
}

// Profile implements screen.ProfileSource.
func (p screenProfiles) Profile(ctx context.Context, customerID string) (screen.InvestorProfile, error) {
	ip, err := p.queue.InvestorProfile(ctx, customerID)
	if err != nil {
		return screen.InvestorProfile{}, fmt.Errorf("bff: screen profile: %w", err)
	}
	return screen.InvestorProfile{Profile: ip.Profile, MaxRisk: ip.MaxRisk, AssessedOn: ip.AssessedOn}, nil
}

// caseResolved is the state label of a closed case.
const caseResolved = "Resolvido"

// screenCases reads the customer's cases through the cases filter and keeps
// the ones that are not resolved. OpenedAgo is in minutes.
type screenCases struct {
	cases CaseSource
}

// OpenCases implements screen.CaseSource. A case whose state index has no
// label counts as open, so an unknown state never hides a case.
func (c screenCases) OpenCases(ctx context.Context, customerID string) ([]screen.OpenCase, error) {
	items, states, err := c.cases.CustomerCases(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("bff: screen cases: %w", err)
	}
	out := make([]screen.OpenCase, 0, len(items))
	for _, it := range items {
		state := ""
		if it.State >= 0 && it.State < len(states) {
			state = states[it.State]
		}
		if state == caseResolved {
			continue
		}
		out = append(out, screen.OpenCase{
			ID:    it.ID,
			State: state,
			Age:   time.Duration(max(it.OpenedAgo, 0)) * time.Minute,
		})
	}
	return out, nil
}

// getScreen serves GET /v1/client-pov/customers/{id}/screens/{slug}. A bad id
// is 400; an unknown slug or an account-sim NotFound is 404; any other source
// failure omits sections and still answers 200. When the client is gone
// before the screen is built, nothing is written or logged.
func (h *Handler) getScreen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := identity.ParseV7(r.PathValue("id"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	res, err := h.screens.Build(r.Context(), r.PathValue("slug"), id)
	switch {
	case err != nil && r.Context().Err() != nil:
		return
	case errors.Is(err, screen.ErrUnknownScreen), errors.Is(err, screen.ErrUnknownCustomer):
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	case err != nil:
		h.logger.LogAttrs(r.Context(), slog.LevelError, "screen build failed",
			slog.String("service", "bff"),
			slog.String("customer_id", id),
			slog.String("error", err.Error()),
		)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.logScreen(r, id, res)
	writeJSON(w, http.StatusOK, res.Page)
}

// logScreen records a served screen and each failure behind it. It logs ids,
// source names, gRPC codes, and omitted reasons; never rendered copy,
// registration data, or account values.
func (h *Handler) logScreen(r *http.Request, customerID string, res screen.Result) {
	ctx := r.Context()
	for _, f := range res.Failures {
		attrs := []slog.Attr{
			slog.String("service", "bff"),
			slog.String("slug", res.Page.Slug),
			slog.String("customer_id", customerID),
		}
		if f.Source != "" {
			attrs = append(attrs,
				slog.String("source", string(f.Source)),
				slog.String("code", status.Code(f.Err).String()),
				slog.Bool("deadline", errors.Is(f.Err, context.DeadlineExceeded) || status.Code(f.Err) == codes.DeadlineExceeded),
			)
		} else {
			attrs = append(attrs, slog.String("section", f.Section), slog.String("error", failureClass(f.Err)))
		}
		h.logger.LogAttrs(ctx, slog.LevelWarn, "screen part failed", attrs...)
	}
	omitted := make([]string, 0, len(res.Page.Omitted))
	for _, o := range res.Page.Omitted {
		omitted = append(omitted, o.ID+":"+o.Reason)
	}
	h.logger.LogAttrs(ctx, slog.LevelInfo, "screen served",
		slog.String("service", "bff"),
		slog.String("slug", res.Page.Slug),
		slog.String("revision", res.Page.Revision),
		slog.String("customer_id", customerID),
		slog.Int("sections", len(res.Page.Sections)),
		slog.String("omitted", strings.Join(omitted, ",")),
	)
}

// failureClass is the fixed log label of a build or heading failure. The
// error text itself may carry copy fields or a panic value, so it is never
// logged.
func failureClass(err error) string {
	if errors.Is(err, screen.ErrPanic) {
		return "panic"
	}
	return screen.ReasonBuildError
}
