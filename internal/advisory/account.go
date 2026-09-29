package advisory

import (
	"context"
	"errors"
	"fmt"
	"time"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
)

// DefaultAccountTimeout bounds one account-sim read when the caller's context
// has no deadline.
const DefaultAccountTimeout = 2 * time.Second

// errNoAccountSim is the balance failure when advisory runs without an
// account-sim target.
var errNoAccountSim = errors.New("advisory: account-sim is not configured")

// AccountReader reads a client's balance from account-sim. Implementations
// must return when ctx is done.
type AccountReader interface {
	Balance(ctx context.Context, customerID string) (Balance, error)
}

// AccountSim is an AccountReader over the account-sim account/v1 service.
type AccountSim struct {
	client  accountv1.AccountServiceClient
	timeout time.Duration
}

// NewAccountSim wraps an AccountService client with DefaultAccountTimeout.
func NewAccountSim(client accountv1.AccountServiceClient) *AccountSim {
	return &AccountSim{client: client, timeout: DefaultAccountTimeout}
}

// Balance reads patrimony and cash through GetAccount. The caller's deadline
// travels with the call; without one, the call gets DefaultAccountTimeout.
// Every failure, NotFound included, is returned wrapped with its status.
func (a *AccountSim) Balance(ctx context.Context, customerID string) (Balance, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}
	res, err := a.client.GetAccount(ctx, &accountv1.GetAccountRequest{CustomerId: customerID})
	if err != nil {
		return Balance{}, fmt.Errorf("advisory: account-sim get account: %w", err)
	}
	return Balance{PatrimonyCents: res.GetPatrimonyCents(), CashCents: res.GetCaixaCents()}, nil
}

// noAccountSim is the AccountReader of an advisory without account-sim: every
// balance read fails, so GetMomentFacts answers Unavailable.
type noAccountSim struct{}

func (noAccountSim) Balance(context.Context, string) (Balance, error) {
	return Balance{}, errNoAccountSim
}
