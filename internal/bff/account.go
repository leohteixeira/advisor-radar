package bff

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	"github.com/leohteixeira/advisor-radar/internal/sim"
	"github.com/leohteixeira/advisor-radar/internal/telemetry"
)

// account-sim client policy (ADR 0008). Retrying a command is safe because
// every command carries its idempotency key.
const (
	accountSimTimeout  = 2 * time.Second
	accountSimAttempts = 3
	accountSimBackoff  = 50 * time.Millisecond
)

// DialAccountSim opens the account-sim connection. Every unary call gets the
// caller's deadline, or accountSimTimeout when the caller has none, and is
// retried on Unavailable with jittered exponential backoff. The otelgrpc
// stats handler traces each attempt as its own client span. opts are appended
// to the defaults; tests use them to dial an in-memory listener.
func DialAccountSim(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	all := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		telemetry.GRPCClientOption(),
		grpc.WithChainUnaryInterceptor(
			defaultDeadline(accountSimTimeout),
			retryUnavailable(accountSimAttempts, accountSimBackoff),
		),
	}, opts...)
	conn, err := grpc.NewClient(target, all...)
	if err != nil {
		return nil, fmt.Errorf("bff: account-sim client: %w", err)
	}
	return conn, nil
}

// defaultDeadline bounds a call that arrives without a deadline. A caller
// deadline is kept as is and travels to the server in grpc-timeout.
func defaultDeadline(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// retryMarkKey carries a *bool that retryUnavailable sets when it sends a
// call again, so the caller can tell a replay of its own lost attempt.
type retryMarkKey struct{}

// withRetryMark returns ctx carrying retried, which the retry interceptor
// sets to true on its first retry.
func withRetryMark(ctx context.Context, retried *bool) context.Context {
	return context.WithValue(ctx, retryMarkKey{}, retried)
}

// retryUnavailable makes at most attempts calls, retrying only
// codes.Unavailable. The wait before retry n is drawn from
// [base·2ⁿ⁻¹/2, base·2ⁿ⁻¹] and ends early when ctx is done, in which case the
// last call's error is returned. A retry sets the ctx retry mark, if any.
func retryUnavailable(attempts int, base time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var err error
		for attempt := range attempts {
			if attempt > 0 {
				if !sleepCtx(ctx, backoff(base, attempt)) {
					return err
				}
				if retried, ok := ctx.Value(retryMarkKey{}).(*bool); ok {
					*retried = true
				}
			}
			err = invoker(ctx, method, req, reply, cc, opts...)
			if status.Code(err) != codes.Unavailable {
				return err
			}
		}
		return err
	}
}

// backoff is the equal-jitter wait before retry n (n ≥ 1).
func backoff(base time.Duration, n int) time.Duration {
	ceiling := base << (n - 1)
	half := ceiling / 2
	if half <= 0 {
		return ceiling
	}
	return half + rand.N(half+1)
}

// sleepCtx waits for d and reports false when ctx ends first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

var (
	_ ProductCatalog   = (*grpcPOV)(nil)
	_ SimulationSource = (*grpcPOV)(nil)
)

// grpcPOV is the POVSource over account/v1. It maps account-sim statuses back
// to the sim sentinels that the POV handlers translate to HTTP.
type grpcPOV struct {
	client accountv1.AccountServiceClient
}

// NewGRPCPOV returns the account-sim POVSource. Dial client's connection with
// DialAccountSim so calls carry a deadline and retry.
func NewGRPCPOV(client accountv1.AccountServiceClient) POVSource {
	return &grpcPOV{client: client}
}

// List returns the POV accounts that account-sim holds.
func (p *grpcPOV) List(ctx context.Context) ([]POVAccount, error) {
	res, err := p.client.ListAccounts(ctx, &accountv1.ListAccountsRequest{})
	if err != nil {
		return nil, accountSimError("list accounts", err)
	}
	out := make([]POVAccount, 0, len(res.GetAccounts()))
	for _, account := range res.GetAccounts() {
		out = append(out, povFromProto(account))
	}
	return out, nil
}

// Get returns one POV account, or sim.ErrUnknownCustomer.
func (p *grpcPOV) Get(ctx context.Context, customerID string) (POVAccount, error) {
	account, err := p.client.GetAccount(ctx, &accountv1.GetAccountRequest{CustomerId: customerID})
	if err != nil {
		return POVAccount{}, accountSimError("get account", err)
	}
	return povFromProto(account), nil
}

// Products returns the account-sim product catalog.
func (p *grpcPOV) Products(ctx context.Context) ([]POVProduct, error) {
	res, err := p.client.ListProducts(ctx, &accountv1.ListProductsRequest{})
	if err != nil {
		return nil, accountSimError("list products", err)
	}
	out := make([]POVProduct, 0, len(res.GetProducts()))
	for _, product := range res.GetProducts() {
		out = append(out, POVProduct{
			ID:           product.GetId(),
			Name:         product.GetName(),
			AssetClass:   product.GetAssetClass(),
			Risk:         int(product.GetRisk()),
			ReturnLabel:  product.GetReturnLabel(),
			MinimumCents: product.GetMinimumCents(),
		})
	}
	return out, nil
}

// Registration returns one customer's registration data, or
// sim.ErrUnknownCustomer.
func (p *grpcPOV) Registration(ctx context.Context, customerID string) (POVRegistration, error) {
	res, err := p.client.GetRegistration(ctx, &accountv1.GetRegistrationRequest{CustomerId: customerID})
	if err != nil {
		return POVRegistration{}, accountSimError("get registration", err)
	}
	return POVRegistration{
		Email:         res.GetEmail(),
		Phone:         res.GetPhone(),
		City:          res.GetCity(),
		AccountNumber: res.GetAccountNumber(),
	}, nil
}

// Preferences returns one customer's stored preferences, or
// sim.ErrUnknownCustomer.
func (p *grpcPOV) Preferences(ctx context.Context, customerID string) (POVPreferences, error) {
	res, err := p.client.GetPreferences(ctx, &accountv1.GetPreferencesRequest{CustomerId: customerID})
	if err != nil {
		return POVPreferences{}, accountSimError("get preferences", err)
	}
	return POVPreferences{Channel: res.GetChannel(), Beta: res.GetBeta()}, nil
}

// UpdatePreferences stores the preferences. A bad channel is sim.ErrCommand
// and an unknown customer sim.ErrUnknownCustomer. The retry interceptor may
// resend it on Unavailable, which is safe: the update is idempotent.
func (p *grpcPOV) UpdatePreferences(ctx context.Context, customerID string, prefs POVPreferences) (POVPreferences, error) {
	res, err := p.client.UpdatePreferences(ctx, &accountv1.UpdatePreferencesRequest{
		CustomerId: customerID,
		Channel:    prefs.Channel,
		Beta:       prefs.Beta,
	})
	if err != nil {
		return POVPreferences{}, accountSimError("update preferences", err)
	}
	return POVPreferences{Channel: res.GetChannel(), Beta: res.GetBeta()}, nil
}

// AdvanceDay advances the global simulated day through account-sim, which
// writes the day and one reavaliacao outbox row per account in one
// transaction. The key makes a retry safe; Retried is set when the
// interceptor sent the command more than once.
func (p *grpcPOV) AdvanceDay(ctx context.Context, idempotencyKey, commandID string) (AdvanceResult, error) {
	var retried bool
	ctx = withRetryMark(ctx, &retried)
	reply, err := p.client.AdvanceDay(ctx, &accountv1.AdvanceDayRequest{
		IdempotencyKey: idempotencyKey,
		CommandId:      commandID,
	})
	if err != nil {
		return AdvanceResult{}, accountSimError("advance day", err)
	}
	return AdvanceResult{
		SimDay:   int(reply.GetSimDay()),
		EventIDs: reply.GetEventIds(),
		Replay:   reply.GetReplay(),
		Retried:  retried,
	}, nil
}

// SimDay reads the current global simulated day.
func (p *grpcPOV) SimDay(ctx context.Context) (int, error) {
	res, err := p.client.GetSimulation(ctx, &accountv1.GetSimulationRequest{})
	if err != nil {
		return 0, accountSimError("get simulation", err)
	}
	return int(res.GetSimDay()), nil
}

// Apply sends one client command to account-sim, which writes the account
// state and the outbox row in one transaction. Retried is set when the
// interceptor sent the command more than once.
func (p *grpcPOV) Apply(ctx context.Context, cmd POVCommand) (POVResult, error) {
	var (
		reply   *accountv1.CommandReply
		err     error
		retried bool
	)
	ctx = withRetryMark(ctx, &retried)
	switch cmd.Kind {
	case sim.CmdDeposit:
		reply, err = p.client.Deposit(ctx, &accountv1.DepositRequest{
			CustomerId:     cmd.CustomerID,
			IdempotencyKey: cmd.IdempotencyKey,
			AmountCents:    cmd.Amount,
			Origin:         cmd.Origin,
		})
	case sim.CmdWithdrawal:
		reply, err = p.client.Withdraw(ctx, &accountv1.WithdrawRequest{
			CustomerId:     cmd.CustomerID,
			IdempotencyKey: cmd.IdempotencyKey,
			AmountCents:    cmd.Amount,
			Destination:    cmd.Destination,
		})
	case sim.CmdMessage:
		reply, err = p.client.SendMessage(ctx, &accountv1.SendMessageRequest{
			CustomerId:     cmd.CustomerID,
			IdempotencyKey: cmd.IdempotencyKey,
			Channel:        cmd.Channel,
			Text:           cmd.Text,
		})
	case sim.CmdComplaint:
		reply, err = p.client.FileComplaint(ctx, &accountv1.FileComplaintRequest{
			CustomerId:     cmd.CustomerID,
			IdempotencyKey: cmd.IdempotencyKey,
			Text:           cmd.Text,
		})
	case sim.CmdPurchase:
		reply, err = p.client.Purchase(ctx, &accountv1.PurchaseRequest{
			CustomerId:     cmd.CustomerID,
			IdempotencyKey: cmd.IdempotencyKey,
			ProductId:      cmd.ProductID,
			AmountCents:    cmd.Amount,
			CommandId:      cmd.CommandID,
		})
	default:
		// A kind the handlers never send is a BFF bug, not a client refusal.
		return POVResult{}, fmt.Errorf("bff: unknown pov command %q", cmd.Kind)
	}
	if err != nil {
		return POVResult{}, accountSimError(cmd.Kind, err)
	}
	return POVResult{EventID: reply.GetEventId(), Replay: reply.GetReplay(), Retried: retried}, nil
}

// accountSimError maps a refusal status to its sim sentinel and keeps the
// status in the chain. Any other code stays an upstream failure.
func accountSimError(op string, err error) error {
	switch status.Code(err) {
	case codes.FailedPrecondition:
		return fmt.Errorf("bff: account-sim %s: %w: %w", op, sim.ErrInsufficient, err)
	case codes.NotFound:
		return fmt.Errorf("bff: account-sim %s: %w: %w", op, sim.ErrUnknownCustomer, err)
	case codes.InvalidArgument:
		return fmt.Errorf("bff: account-sim %s: %w: %w", op, sim.ErrCommand, err)
	default:
		return fmt.Errorf("bff: account-sim %s: %w", op, err)
	}
}

func povFromProto(a *accountv1.Account) POVAccount {
	positions := make([]POVPosition, 0, len(a.GetPositions()))
	for _, p := range a.GetPositions() {
		positions = append(positions, POVPosition{
			ProductID:    p.GetProductId(),
			AssetClass:   p.GetAssetClass(),
			AppliedCents: p.GetAppliedCents(),
			ValueCents:   p.GetValueCents(),
		})
	}
	return POVAccount{
		CustomerID: a.GetCustomerId(),
		Acoes:      a.GetAcoesCents(),
		ETFs:       a.GetEtfsCents(),
		RendaFixa:  a.GetRendaFixaCents(),
		Caixa:      a.GetCaixaCents(),
		Patrimony:  a.GetPatrimonyCents(),
		Positions:  positions,
		SimDay:     int(a.GetSimDay()),
		DayChange:  a.GetDayChangeCents(),
	}
}
