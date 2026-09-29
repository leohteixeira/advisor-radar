package sim

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// GRPCServer serves account/v1 AccountService over a Store. Commands go
// through Apply, so state, idempotency key, and outbox row commit together.
type GRPCServer struct {
	accountv1.UnimplementedAccountServiceServer
	store  Store
	logger *slog.Logger
}

var _ accountv1.AccountServiceServer = (*GRPCServer)(nil)

// NewGRPCServer returns the account-sim command and query server. A nil
// logger discards the internal-failure logs.
func NewGRPCServer(store Store, logger *slog.Logger) *GRPCServer {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &GRPCServer{store: store, logger: logger}
}

// Deposit credits caixa and publishes account.event.recorded.
func (s *GRPCServer) Deposit(ctx context.Context, req *accountv1.DepositRequest) (*accountv1.CommandReply, error) {
	return s.apply(ctx, "Deposit", Command{
		CustomerID:     req.GetCustomerId(),
		IdempotencyKey: req.GetIdempotencyKey(),
		Kind:           CmdDeposit,
		Amount:         req.GetAmountCents(),
		Origin:         req.GetOrigin(),
	})
}

// Withdraw debits caixa and publishes account.event.recorded.
func (s *GRPCServer) Withdraw(ctx context.Context, req *accountv1.WithdrawRequest) (*accountv1.CommandReply, error) {
	return s.apply(ctx, "Withdraw", Command{
		CustomerID:     req.GetCustomerId(),
		IdempotencyKey: req.GetIdempotencyKey(),
		Kind:           CmdWithdrawal,
		Amount:         req.GetAmountCents(),
		Destination:    req.GetDestination(),
	})
}

// SendMessage publishes message.received on chat or e-mail.
func (s *GRPCServer) SendMessage(ctx context.Context, req *accountv1.SendMessageRequest) (*accountv1.CommandReply, error) {
	return s.apply(ctx, "SendMessage", Command{
		CustomerID:     req.GetCustomerId(),
		IdempotencyKey: req.GetIdempotencyKey(),
		Kind:           CmdMessage,
		Channel:        req.GetChannel(),
		Text:           req.GetText(),
	})
}

// FileComplaint publishes message.received on chat.
func (s *GRPCServer) FileComplaint(ctx context.Context, req *accountv1.FileComplaintRequest) (*accountv1.CommandReply, error) {
	return s.apply(ctx, "FileComplaint", Command{
		CustomerID:     req.GetCustomerId(),
		IdempotencyKey: req.GetIdempotencyKey(),
		Kind:           CmdComplaint,
		Text:           req.GetText(),
	})
}

// GetAccount returns one POV account with its positions valued at the
// current day.
func (s *GRPCServer) GetAccount(ctx context.Context, req *accountv1.GetAccountRequest) (*accountv1.Account, error) {
	customerID, err := identity.ParseV7(req.GetCustomerId())
	if err != nil {
		return nil, errInvalidCustomer
	}
	var account *accountv1.Account
	err = s.store.WithTx(ctx, func(tx Tx) error {
		found, ok, err := loadAccount(ctx, tx, customerID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrUnknownCustomer
		}
		account = found
		return nil
	})
	if err != nil {
		return nil, s.statusOf("GetAccount", err)
	}
	return account, nil
}

// ListAccounts returns the seeded POV accounts that exist in the store.
// POVSeed lists the customers in byte-wise id order, the order ResetPOV locks
// them in, so the per-customer locks GetAccount takes cannot deadlock.
func (s *GRPCServer) ListAccounts(ctx context.Context, _ *accountv1.ListAccountsRequest) (*accountv1.ListAccountsResponse, error) {
	seed := POVSeed()
	accounts := make([]*accountv1.Account, 0, len(seed))
	err := s.store.WithTx(ctx, func(tx Tx) error {
		for _, want := range seed {
			account, ok, err := loadAccount(ctx, tx, want.CustomerID)
			if err != nil {
				return err
			}
			if ok {
				accounts = append(accounts, account)
			}
		}
		return nil
	})
	if err != nil {
		return nil, s.statusOf("ListAccounts", err)
	}
	return &accountv1.ListAccountsResponse{Accounts: accounts}, nil
}

// ListProducts returns the fictional product catalog.
func (s *GRPCServer) ListProducts(ctx context.Context, _ *accountv1.ListProductsRequest) (*accountv1.ListProductsResponse, error) {
	var products []Product
	err := s.store.WithTx(ctx, func(tx Tx) error {
		found, err := tx.ListProducts(ctx)
		products = found
		return err
	})
	if err != nil {
		return nil, s.statusOf("ListProducts", err)
	}
	out := make([]*accountv1.Product, 0, len(products))
	for _, product := range products {
		out = append(out, productToProto(product))
	}
	return &accountv1.ListProductsResponse{Products: out}, nil
}

// GetRegistration returns the fictional registration data of one customer.
func (s *GRPCServer) GetRegistration(ctx context.Context, req *accountv1.GetRegistrationRequest) (*accountv1.Registration, error) {
	customerID, err := identity.ParseV7(req.GetCustomerId())
	if err != nil {
		return nil, errInvalidCustomer
	}
	var registration Registration
	err = s.store.WithTx(ctx, func(tx Tx) error {
		found, ok, err := tx.GetRegistration(ctx, customerID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrUnknownCustomer
		}
		registration = found
		return nil
	})
	if err != nil {
		return nil, s.statusOf("GetRegistration", err)
	}
	return &accountv1.Registration{
		CustomerId:    registration.CustomerID,
		Email:         registration.Email,
		Phone:         registration.Phone,
		City:          registration.City,
		AccountNumber: registration.AccountNumber,
	}, nil
}

// loadAccount reads one customer's cash, positions, and the class aggregates
// summed from those same positions.
func loadAccount(ctx context.Context, tx Tx, customerID string) (*accountv1.Account, bool, error) {
	account, ok, err := tx.GetAccount(ctx, customerID)
	if err != nil || !ok {
		return nil, false, err
	}
	return accountToProto(account), true, nil
}

var errInvalidCustomer = status.Error(codes.InvalidArgument, "customer_id must be a UUIDv7")

func (s *GRPCServer) apply(ctx context.Context, method string, cmd Command) (*accountv1.CommandReply, error) {
	customerID, err := identity.ParseV7(cmd.CustomerID)
	if err != nil {
		return nil, errInvalidCustomer
	}
	cmd.CustomerID = customerID
	result, err := Apply(ctx, s.store, cmd)
	if err != nil {
		return nil, s.statusOf(method, err)
	}
	return &accountv1.CommandReply{EventId: result.EventID, Replay: result.Replay}, nil
}

// statusOf maps sim refusals to gRPC codes and context errors to Canceled or
// DeadlineExceeded. Any other failure is Internal with a fixed message, so no
// SQL or DSN text reaches the caller or the log.
func (s *GRPCServer) statusOf(method string, err error) error {
	switch {
	case errors.Is(err, ErrInsufficient):
		return status.Error(codes.FailedPrecondition, "withdrawal exceeds available cash")
	case errors.Is(err, ErrUnknownCustomer):
		return status.Error(codes.NotFound, "customer not found")
	case errors.Is(err, identity.ErrInvalidID):
		return errInvalidCustomer
	case errors.Is(err, ErrAmount), errors.Is(err, ErrCommand), errors.Is(err, ErrKey):
		// These messages come from sim validation only, never from storage.
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, context.Canceled):
		// The caller gave up; that is not a server failure. Fixed messages keep
		// wrapped storage text out of the status.
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	}
	s.logger.Error("account command failed",
		"service", "account-sim",
		"method", method,
		"cause", failureCause(err),
	)
	return status.Error(codes.Internal, "internal error")
}

// failureCause names an internal failure without its text: a PostgreSQL
// SQLSTATE or "unknown".
func failureCause(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return "sqlstate " + pgErr.Code
	}
	return "unknown"
}

func accountToProto(a Account) *accountv1.Account {
	out := make([]*accountv1.Position, 0, len(a.Positions))
	for _, position := range a.Positions {
		out = append(out, &accountv1.Position{
			ProductId:    position.ProductID,
			AssetClass:   position.AssetClass,
			AppliedCents: position.AppliedCents,
			ValueCents:   position.ValueCents,
		})
	}
	return &accountv1.Account{
		CustomerId:     a.CustomerID,
		AcoesCents:     a.Acoes,
		EtfsCents:      a.ETFs,
		RendaFixaCents: a.RendaFixa,
		CaixaCents:     a.Caixa,
		Positions:      out,
		PatrimonyCents: a.Assets(),
	}
}

func productToProto(p Product) *accountv1.Product {
	return &accountv1.Product{
		Id:           p.ID,
		Name:         p.Name,
		AssetClass:   p.AssetClass,
		Risk:         int32(p.Risk),
		ReturnLabel:  p.ReturnLabel,
		MinimumCents: p.MinimumCents,
	}
}
