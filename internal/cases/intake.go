package cases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// ChurnRiskOpen is the lowest churn_risk that opens a case on its own and
// sets the ChurnRisk clock factor.
const ChurnRiskOpen = 0.5

// WantsHumanFactor is the lowest wants_human that sets the HumanRequested
// clock factor.
const WantsHumanFactor = 0.5

// OriginClientApp marks a message the customer sent from the client app.
// Only such messages open or join cases; seeded and burst messages carry no
// origin.
const OriginClientApp = "client_app"

// Triaged intents that open a case regardless of churn risk.
const (
	intentComplaint = "reclamacao"
	intentClosing   = "encerramento"
)

// History kinds written by the intake.
const (
	HistoryKindCase    = "caso"
	HistoryKindMessage = "mensagem"
)

// Intake retry policy for transient errors.
const (
	IntakeAttempts    = 5
	IntakeBaseBackoff = 200 * time.Millisecond
)

var (
	// ErrUnknownCustomer means advisory does not know the customer.
	ErrUnknownCustomer = errors.New("cases: unknown customer")
	// ErrUnusableCustomer means advisory returned a customer without a known
	// segment or an advisor, so no SLA clock or owner can be set.
	ErrUnusableCustomer = errors.New("cases: unusable customer")
)

// Reason says why a triaged message qualifies for a case.
type Reason int

// Reasons in precedence order: an intent wins over the churn signal.
const (
	ReasonNone Reason = iota
	ReasonComplaint
	ReasonClosing
	ReasonChurnRisk
)

// String is the Portuguese label used in case history.
func (r Reason) String() string {
	switch r {
	case ReasonComplaint:
		return "reclamação"
	case ReasonClosing:
		return "pedido de encerramento"
	case ReasonChurnRisk:
		return "risco de saída"
	default:
		return ""
	}
}

// openedText matches the seed wording of the first case history row.
func (r Reason) openedText() string {
	switch r {
	case ReasonComplaint:
		return "Caso aberto a partir de mensagem com reclamação"
	case ReasonClosing:
		return "Caso aberto a partir de pedido de encerramento"
	case ReasonChurnRisk:
		return "Caso aberto a partir de risco de saída"
	default:
		return "Caso aberto"
	}
}

// Decision is the intake outcome for one triaged message.
type Decision string

// Intake decisions.
const (
	DecisionOpened    Decision = "opened"
	DecisionJoined    Decision = "joined"
	DecisionIgnored   Decision = "ignored"
	DecisionDuplicate Decision = "duplicate"
)

// TriagedMessage is the part of the message.triaged payload cases reads.
type TriagedMessage struct {
	SourceEventID string  `json:"source_event_id"`
	Origin        string  `json:"origin,omitempty"`
	Intent        string  `json:"intent"`
	Frustration   float64 `json:"frustration"`
	ChurnRisk     float64 `json:"churn_risk"`
	WantsHuman    float64 `json:"wants_human"`
	Degraded      bool    `json:"degraded"`
	NeedsReview   bool    `json:"needs_review"`
}

// Qualifies reports whether a triaged message opens (or joins) a case. Only a
// client-app message qualifies, by intent or by churn risk.
func Qualifies(m TriagedMessage) (Reason, bool) {
	if m.Origin != OriginClientApp {
		return ReasonNone, false
	}
	switch m.Intent {
	case intentComplaint:
		return ReasonComplaint, true
	case intentClosing:
		return ReasonClosing, true
	}
	if m.ChurnRisk >= ChurnRiskOpen {
		return ReasonChurnRisk, true
	}
	return ReasonNone, false
}

// clockFactors maps the triage scores onto the SLA halving factors.
func (m TriagedMessage) clockFactors() ClockFactors {
	return ClockFactors{
		ChurnRisk:          m.ChurnRisk >= ChurnRiskOpen,
		Frustration:        int(math.Round(m.Frustration)),
		HumanRequested:     m.WantsHuman >= WantsHumanFactor,
		RelevantWithdrawal: false,
	}
}

// Customer is the advisory data a new case needs.
type Customer struct {
	Segment   string
	AdvisorID string
}

// CustomerLookup resolves a customer's segment and advisor. An error wrapping
// ErrUnknownCustomer or ErrUnusableCustomer is permanent; any other is
// transient.
type CustomerLookup interface {
	Lookup(ctx context.Context, customerID string) (Customer, error)
}

// HistoryRow is one case_history row.
type HistoryRow struct {
	ID         string
	CaseID     string
	Kind       string
	Text       string
	OccurredAt time.Time
}

// IntakeResult describes what Intake did. EventID and CustomerID are set as
// soon as the envelope decodes, even when Intake fails.
type IntakeResult struct {
	EventID    string
	CustomerID string
	CaseID     string
	Decision   Decision
}

// IntakeStore is the Store plus the reads Intake makes before its transaction.
type IntakeStore interface {
	Store
	// InboxSeen reports whether eventID is already claimed.
	InboxSeen(ctx context.Context, eventID string) (bool, error)
	// OpenCaseFor returns the customer's case that is not Resolvido, if any.
	OpenCaseFor(ctx context.Context, customerID string) (CaseRow, bool, error)
}

// errLookupNeeded is transient: the open case seen before the transaction was
// resolved before it, so the retry must look the customer up to open a case.
var errLookupNeeded = errors.New("cases: open case resolved before the transaction; customer lookup needed")

// Intake applies one message.triaged delivery body. It opens a case for the
// customer, adds the message to the customer's open case, or only claims the
// inbox. A PermanentDeliveryError means redelivery cannot succeed; any other
// error is transient.
//
// A duplicate, or a message that joins an open case, never calls advisory, so
// neither depends on advisory being up. The lookup runs outside the
// transaction; all writes happen in one transaction.
func Intake(ctx context.Context, store IntakeStore, lookup CustomerLookup, body []byte) (IntakeResult, error) {
	if store == nil {
		return IntakeResult{}, errors.New("cases: store is required")
	}
	if lookup == nil {
		return IntakeResult{}, errors.New("cases: customer lookup is required")
	}

	env, msg, err := decodeTriaged(body)
	res := IntakeResult{EventID: env.EventID, CustomerID: env.CustomerID}
	if err != nil {
		return res, PermanentDeliveryError{Err: err}
	}

	reason, qualifies := Qualifies(msg)
	if !qualifies {
		return claimOnly(ctx, store, res)
	}

	seen, err := store.InboxSeen(ctx, env.EventID)
	if err != nil {
		return res, fmt.Errorf("cases: intake %s: %w", env.EventID, err)
	}
	if seen {
		res.Decision = DecisionDuplicate
		return res, nil
	}
	_, hasOpen, err := store.OpenCaseFor(ctx, env.CustomerID)
	if err != nil {
		return res, fmt.Errorf("cases: intake %s: %w", env.EventID, err)
	}
	var customer Customer
	looked := false
	if !hasOpen {
		customer, err = lookup.Lookup(ctx, env.CustomerID)
		if err != nil {
			if errors.Is(err, ErrUnknownCustomer) || errors.Is(err, ErrUnusableCustomer) {
				return res, PermanentDeliveryError{Err: fmt.Errorf("cases: intake %s: %w", env.EventID, err)}
			}
			return res, fmt.Errorf("cases: intake %s: lookup customer: %w", env.EventID, err)
		}
		if err := customer.validate(); err != nil {
			return res, PermanentDeliveryError{Err: fmt.Errorf("cases: intake %s: %w", env.EventID, err)}
		}
		looked = true
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
		if found {
			decided.CaseID = open.ID
			decided.Decision = DecisionJoined
			return insertHistory(ctx, tx, open.ID, HistoryKindMessage,
				"Nova mensagem do cliente: "+reason.String(), env.OccurredAt)
		}
		if !looked {
			return errLookupNeeded
		}

		caseID, err := identity.NewV7()
		if err != nil {
			return fmt.Errorf("cases: case id: %w", err)
		}
		row, err := openInTx(ctx, tx, OpenInput{
			ID:         caseID,
			CustomerID: env.CustomerID,
			SignalID:   signalID(msg.SourceEventID, env.EventID),
			AdvisorID:  customer.AdvisorID,
			Segment:    customer.Segment,
			Factors:    msg.clockFactors(),
			OccurredAt: env.OccurredAt,
		})
		if err != nil {
			return err
		}
		decided.CaseID = row.ID
		decided.Decision = DecisionOpened
		return insertHistory(ctx, tx, row.ID, HistoryKindCase, reason.openedText(), row.OpenedAt)
	})
	if err != nil {
		return res, fmt.Errorf("cases: intake %s: %w", env.EventID, err)
	}
	return decided, nil
}

// claimOnly claims the inbox for a message that opens nothing.
func claimOnly(ctx context.Context, store Store, res IntakeResult) (IntakeResult, error) {
	decided := res
	err := store.WithTx(ctx, func(tx Tx) error {
		claimed, err := tx.ClaimInbox(ctx, res.EventID)
		if err != nil {
			return fmt.Errorf("cases: claim inbox: %w", err)
		}
		decided.Decision = DecisionIgnored
		if !claimed {
			decided.Decision = DecisionDuplicate
		}
		return nil
	})
	if err != nil {
		return res, fmt.Errorf("cases: intake %s: %w", res.EventID, err)
	}
	return decided, nil
}

// RetryTransient calls fn until it succeeds, returns a PermanentDeliveryError,
// runs attempts times, or ctx ends. The wait doubles from base with equal
// jitter. It returns the last error.
func RetryTransient(ctx context.Context, attempts int, base time.Duration, fn func(context.Context) error) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for attempt := range attempts {
		err = fn(ctx)
		if err == nil {
			return nil
		}
		var perm PermanentDeliveryError
		if errors.As(err, &perm) {
			return err
		}
		if attempt == attempts-1 {
			break
		}
		if waitErr := sleepCtx(ctx, backoff(base, attempt)); waitErr != nil {
			return errors.Join(err, waitErr)
		}
	}
	return err
}

// backoff returns base·2^attempt with equal jitter: half fixed, half random.
func backoff(base time.Duration, attempt int) time.Duration {
	d := base << attempt
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + rand.N(half+1)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("cases: retry wait: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func (c Customer) validate() error {
	if c.Segment == "" || BaseMinutes(c.Segment) == 0 {
		return fmt.Errorf("%w: segment %q", ErrUnusableCustomer, c.Segment)
	}
	if c.AdvisorID == "" {
		return fmt.Errorf("%w: no advisor", ErrUnusableCustomer)
	}
	if _, err := uuid.Parse(c.AdvisorID); err != nil {
		return fmt.Errorf("%w: advisor id is not a uuid", ErrUnusableCustomer)
	}
	return nil
}

func insertHistory(ctx context.Context, tx Tx, caseID, kind, text string, at time.Time) error {
	id, err := identity.NewV7()
	if err != nil {
		return fmt.Errorf("cases: history id: %w", err)
	}
	if err := tx.InsertHistory(ctx, HistoryRow{
		ID:         id,
		CaseID:     caseID,
		Kind:       kind,
		Text:       text,
		OccurredAt: at,
	}); err != nil {
		return fmt.Errorf("cases: insert history %s: %w", caseID, err)
	}
	return nil
}

// signalID prefers the source message event id and falls back to the
// triaged event id when the source is not a UUID.
func signalID(sourceEventID, triagedEventID string) string {
	if id, err := uuid.Parse(sourceEventID); err == nil {
		return id.String()
	}
	return triagedEventID
}

// decodeTriaged parses and validates a message.triaged body. Every error is
// permanent: the same body can never decode differently.
func decodeTriaged(body []byte) (event.Envelope, TriagedMessage, error) {
	var raw struct {
		EventID       string          `json:"event_id"`
		OccurredAt    time.Time       `json:"occurred_at"`
		CustomerID    string          `json:"customer_id"`
		SchemaVersion int             `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return event.Envelope{}, TriagedMessage{}, fmt.Errorf("cases: decode triaged: %w", err)
	}
	env := event.Envelope{
		Name:          event.NameMessageTriaged,
		EventID:       raw.EventID,
		OccurredAt:    raw.OccurredAt,
		CustomerID:    raw.CustomerID,
		SchemaVersion: raw.SchemaVersion,
	}
	if err := env.Validate(); err != nil {
		return env, TriagedMessage{}, fmt.Errorf("cases: validate triaged: %w", err)
	}
	// inbox.event_id and cases.customer_id are UUID columns.
	eventID, err := uuid.Parse(env.EventID)
	if err != nil {
		return env, TriagedMessage{}, fmt.Errorf("cases: validate triaged: event_id is not a uuid")
	}
	env.EventID = eventID.String()
	customerID, err := identity.ParseV7(env.CustomerID)
	if err != nil {
		return env, TriagedMessage{}, fmt.Errorf("cases: validate triaged: customer_id: %w", err)
	}
	env.CustomerID = customerID

	if len(raw.Payload) == 0 || string(raw.Payload) == "null" {
		return env, TriagedMessage{}, errors.New("cases: validate triaged: payload is required")
	}
	var msg TriagedMessage
	if err := json.Unmarshal(raw.Payload, &msg); err != nil {
		return env, TriagedMessage{}, fmt.Errorf("cases: decode triaged payload: %w", err)
	}
	env.Payload = msg
	return env, msg, nil
}
