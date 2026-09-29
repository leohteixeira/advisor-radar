package cases

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	advisoryv1 "github.com/leohteixeira/advisor-radar/gen/advisory/v1"
)

// DefaultLookupTimeout bounds one advisory lookup when the caller's context
// has no deadline.
const DefaultLookupTimeout = 2 * time.Second

// AdvisoryLookup is a CustomerLookup over the advisory gRPC service.
type AdvisoryLookup struct {
	client  advisoryv1.AdvisoryServiceClient
	timeout time.Duration
}

// NewAdvisoryLookup wraps an AdvisoryService client with DefaultLookupTimeout.
func NewAdvisoryLookup(client advisoryv1.AdvisoryServiceClient) *AdvisoryLookup {
	return &AdvisoryLookup{client: client, timeout: DefaultLookupTimeout}
}

// Lookup reads the customer's segment and advisor from advisory. A
// GetCustomer NotFound, InvalidArgument, or Unimplemented maps to
// ErrUnknownCustomer, and the same ListOperators codes map to
// ErrUnusableCustomer; any other failure is transient.
//
// GetCustomer returns the advisor's display name, not the operator id, so the
// id is resolved through ListOperators under the same deadline. A name that
// matches no operator, or more than one, leaves AdvisorID empty.
func (l *AdvisoryLookup) Lookup(ctx context.Context, customerID string) (Customer, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.timeout)
		defer cancel()
	}

	res, err := l.client.GetCustomer(ctx, &advisoryv1.GetCustomerRequest{CustomerId: customerID})
	if err != nil {
		return Customer{}, classifyAdvisoryError("get customer", err, ErrUnknownCustomer)
	}
	customer := Customer{Segment: res.GetSegment()}
	if res.GetAdvisor() == "" {
		return customer, nil
	}

	ops, err := l.client.ListOperators(ctx, &advisoryv1.ListOperatorsRequest{})
	if err != nil {
		return Customer{}, classifyAdvisoryError("list operators", err, ErrUnusableCustomer)
	}
	matches := 0
	for _, op := range ops.GetItems() {
		if op.GetName() == res.GetAdvisor() {
			customer.AdvisorID = op.GetId()
			matches++
		}
	}
	if matches != 1 {
		customer.AdvisorID = ""
	}
	return customer, nil
}

// classifyAdvisoryError wraps codes that a retry cannot change with the
// permanent sentinel; every other code stays a plain, transient error.
func classifyAdvisoryError(op string, err, permanent error) error {
	switch status.Code(err) {
	case codes.NotFound, codes.InvalidArgument, codes.Unimplemented:
		return fmt.Errorf("%w: advisory %s: %w", permanent, op, err)
	default:
		return fmt.Errorf("cases: advisory %s: %w", op, err)
	}
}
