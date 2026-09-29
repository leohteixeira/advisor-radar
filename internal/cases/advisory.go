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

// Lookup reads the customer's segment and advisor id from advisory
// GetCustomer. NotFound, InvalidArgument, or Unimplemented maps to
// ErrUnknownCustomer; any other failure is transient.
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
	return Customer{Segment: res.GetSegment(), AdvisorID: res.GetAdvisorId()}, nil
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
