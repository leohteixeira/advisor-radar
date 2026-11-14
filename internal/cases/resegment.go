package cases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// alertKindSegment is the alert.raised kind advisory raises when a client's
// assets cross a segment bound.
const alertKindSegment = "segmento"

// Resegment applies one alert.raised delivery body (ADR 0012). Only a
// segmento alert does anything, and it never opens a case: when the client
// has a case that is not Resolvido, the case's SLA becomes the SLA of the
// client's current segment, counted from the alert's occurred_at (from the
// processing instant when the alert is late, see ResegmentAt), and the
// escalation delay is re-armed for it.
//
// The alert is only the trigger. The segment comes from lookup, because a
// segmento alert is also raised for a fact that does not move the book, and
// alerts can be redelivered or arrive out of order. The new total keeps the
// halving of the current one. An alert that finds the total already right
// (the case was opened after the change, or a later alert got here first)
// changes nothing.
//
// A PermanentDeliveryError means redelivery cannot succeed; any other error
// is transient. The lookup runs outside the transaction; all writes happen in
// one transaction, under the inbox claim on the alert's event_id.
func Resegment(ctx context.Context, store IntakeStore, lookup CustomerLookup, body []byte) (IntakeResult, error) {
	return ResegmentAt(ctx, store, lookup, body, time.Now().UTC())
}

// ResegmentLagTolerance is how late an alert may be processed and still
// count the new SLA from its occurred_at. The escalation delay is armed with
// the full SLA when it is published, so its tick fires that long after the
// processing, not after the alert: counting a late alert from occurred_at
// would show the case overdue for the length of the lag before it escalates.
const ResegmentLagTolerance = time.Minute

// ResegmentAt is Resegment with the processing instant given. An alert
// processed more than ResegmentLagTolerance after its occurred_at (a backlog,
// services stopped by a deploy or a reseed) counts the new SLA from now, so
// the deadline the screens show and the escalation tick agree.
func ResegmentAt(ctx context.Context, store IntakeStore, lookup CustomerLookup, body []byte, now time.Time) (IntakeResult, error) {
	if store == nil {
		return IntakeResult{}, errors.New("cases: store is required")
	}
	if lookup == nil {
		return IntakeResult{}, errors.New("cases: customer lookup is required")
	}

	env, payload, err := decodeEnvelope(event.NameAlertRaised, "alert", body)
	res := IntakeResult{EventID: env.EventID, CustomerID: env.CustomerID}
	if err != nil {
		return res, PermanentDeliveryError{Err: err}
	}
	var alert struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(payload, &alert); err != nil {
		return res, PermanentDeliveryError{Err: fmt.Errorf("cases: decode alert payload: %w", err)}
	}
	if alert.Kind != alertKindSegment {
		res.Decision = DecisionIgnored
		return res, nil
	}

	seen, err := store.InboxSeen(ctx, env.EventID)
	if err != nil {
		return res, fmt.Errorf("cases: resegment %s: %w", env.EventID, err)
	}
	if seen {
		res.Decision = DecisionDuplicate
		return res, nil
	}
	_, hasOpen, err := store.OpenCaseFor(ctx, env.CustomerID)
	if err != nil {
		return res, fmt.Errorf("cases: resegment %s: %w", env.EventID, err)
	}
	if !hasOpen {
		// A case opened from now on reads the segment the book already has.
		return claimOnly(ctx, store, res)
	}
	customer, err := lookup.Lookup(ctx, env.CustomerID)
	if err != nil {
		if errors.Is(err, ErrUnknownCustomer) || errors.Is(err, ErrUnusableCustomer) {
			return res, PermanentDeliveryError{Err: fmt.Errorf("cases: resegment %s: %w", env.EventID, err)}
		}
		return res, fmt.Errorf("cases: resegment %s: lookup customer: %w", env.EventID, err)
	}
	if BaseMinutes(customer.Segment) == 0 {
		return res, PermanentDeliveryError{
			Err: fmt.Errorf("cases: resegment %s: %w: segment %q", env.EventID, ErrUnusableCustomer, customer.Segment),
		}
	}

	var decided IntakeResult
	err = store.WithTx(ctx, func(tx Tx) error {
		decided = res
		claimed, err := tx.ClaimInbox(ctx, env.EventID)
		if err != nil {
			return fmt.Errorf("cases: claim inbox: %w", err)
		}
		if !claimed {
			decided.Decision = DecisionDuplicate
			return nil
		}
		if err := tx.LockCustomer(ctx, env.CustomerID); err != nil {
			return fmt.Errorf("cases: lock customer: %w", err)
		}
		open, found, err := tx.OpenCaseFor(ctx, env.CustomerID)
		if err != nil {
			return fmt.Errorf("cases: find open case: %w", err)
		}
		decided.Decision = DecisionIgnored
		if !found {
			return nil
		}
		decided.CaseID = open.ID
		total := RetotalMinutes(open.SLATotalMinutes, customer.Segment)
		if total == open.SLATotalMinutes {
			return nil
		}

		// The clock never restarts before its previous start.
		start := env.OccurredAt
		if now.Sub(start) > ResegmentLagTolerance {
			start = now
		}
		if start.Before(open.SLAStart()) {
			start = open.SLAStart()
		}
		open.SLATotalMinutes = total
		open.SLAStartedAt = start
		if err := tx.UpdateCase(ctx, open); err != nil {
			return fmt.Errorf("cases: update case %s: %w", open.ID, err)
		}
		if err := tx.RearmDelay(ctx, open.ID, SLADelayArgs(total).TTLMs); err != nil {
			return fmt.Errorf("cases: rearm delay %s: %w", open.ID, err)
		}
		decided.Decision = DecisionRecalculated
		return insertHistory(ctx, tx, HistoryRow{
			CaseID:     open.ID,
			Kind:       HistoryKindSLA,
			Text:       fmt.Sprintf("Cliente passou para o segmento %s: SLA recalculado para %s", customer.Segment, slaLabel(total)),
			OccurredAt: start,
		})
	})
	if err != nil {
		return res, fmt.Errorf("cases: resegment %s: %w", env.EventID, err)
	}
	return decided, nil
}

// slaLabel formats an SLA total for a history line: "4 h", or "30 min" when
// it is not a whole number of hours.
func slaLabel(minutes int) string {
	if minutes%60 == 0 {
		return fmt.Sprintf("%d h", minutes/60)
	}
	return fmt.Sprintf("%d min", minutes)
}
