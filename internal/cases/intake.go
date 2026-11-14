package cases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
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

// KindComplaint marks a message the customer sent through the client app's
// complaint panel. The panel promises a case with a protocol, so such a
// message always qualifies.
const KindComplaint = "complaint"

// Triaged intents that open a case regardless of churn risk.
const (
	intentComplaint = "reclamacao"
	intentClosing   = "encerramento"
)

// History kinds written by the intake, the open-case command, and the SLA
// recalculation.
const (
	HistoryKindCase    = "caso"
	HistoryKindMessage = "mensagem"
	HistoryKindSignal  = "sinal"
	HistoryKindSLA     = "sla"
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

// Intake decisions. DecisionRecalculated is the SLA recalculation of an open
// case after a segment change.
const (
	DecisionOpened       Decision = "opened"
	DecisionJoined       Decision = "joined"
	DecisionIgnored      Decision = "ignored"
	DecisionDuplicate    Decision = "duplicate"
	DecisionRecalculated Decision = "recalculated"
)

// TriagedMessage is the part of the message.triaged payload cases reads.
type TriagedMessage struct {
	SourceEventID string  `json:"source_event_id"`
	Origin        string  `json:"origin,omitempty"`
	Kind          string  `json:"kind,omitempty"`
	Intent        string  `json:"intent"`
	Text          string  `json:"text,omitempty"`
	Frustration   float64 `json:"frustration"`
	ChurnRisk     float64 `json:"churn_risk"`
	WantsHuman    float64 `json:"wants_human"`
	Degraded      bool    `json:"degraded"`
	NeedsReview   bool    `json:"needs_review"`
}

// Qualifies reports whether a triaged message opens (or joins) a case. Only a
// client-app message qualifies: one filed as a complaint, or by intent or
// churn risk.
func Qualifies(m TriagedMessage) (Reason, bool) {
	if m.Origin != OriginClientApp {
		return ReasonNone, false
	}
	// The client filed it as a complaint: it is one whatever triage read.
	if m.Kind == KindComplaint {
		return ReasonComplaint, true
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

// HistoryRow is one case_history row. SignalID is set when the row records a
// signal an advisor joined to the case; a case keeps one row per signal.
type HistoryRow struct {
	ID         string
	CaseID     string
	Kind       string
	Text       string
	OccurredAt time.Time
	SignalID   string
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

	signal := signalID(msg.SourceEventID, env.EventID)
	reason, qualifies := Qualifies(msg)
	if !qualifies {
		if msg.Origin != OriginClientApp {
			return claimOnly(ctx, store, res)
		}
		// Any other message the client sends while a case is open belongs
		// to that case's history; without an open case it opens nothing.
		return follow(ctx, store, res, HistoryRow{
			Kind: HistoryKindMessage, Text: joinedText(ReasonNone, msg.Text), OccurredAt: env.OccurredAt, SignalID: signal,
		})
	}

	return admit(ctx, store, lookup, admission{
		eventID:    env.EventID,
		customerID: env.CustomerID,
		occurredAt: env.OccurredAt,
		signalID:   signal,
		factors:    msg.clockFactors(),
		openedText: reason.openedText(),
		joinKind:   HistoryKindMessage,
		joinText:   joinedText(reason, msg.Text),
		joinSignal: true,
	})
}

// maxHistoryQuoteRunes bounds the message text quoted in a history line.
const maxHistoryQuoteRunes = 160

// joinedText is the history line of a message that joins an open case: what
// kind of message it is, when it qualifies for a case on its own, and what
// the client wrote.
func joinedText(reason Reason, text string) string {
	line := "Nova mensagem do cliente"
	if reason != ReasonNone {
		line += ": " + reason.String()
	}
	quote := []rune(strings.TrimSpace(text))
	if len(quote) == 0 {
		return line
	}
	if len(quote) > maxHistoryQuoteRunes {
		quote = append(quote[:maxHistoryQuoteRunes], '…')
	}
	return line + " — “" + string(quote) + "”"
}

// follow adds line to the customer's open case, once per signal, or only
// claims the inbox when the customer has no open case. It never opens a case
// and never calls advisory.
func follow(ctx context.Context, store Store, res IntakeResult, line HistoryRow) (IntakeResult, error) {
	decided := res
	err := store.WithTx(ctx, func(tx Tx) error {
		decided = res
		claimed, err := tx.ClaimInbox(ctx, res.EventID)
		if err != nil {
			return fmt.Errorf("cases: claim inbox: %w", err)
		}
		if !claimed {
			decided.Decision = DecisionDuplicate
			return nil
		}
		if err := tx.LockCustomer(ctx, res.CustomerID); err != nil {
			return fmt.Errorf("cases: lock customer: %w", err)
		}
		open, found, err := tx.OpenCaseFor(ctx, res.CustomerID)
		if err != nil {
			return fmt.Errorf("cases: find open case: %w", err)
		}
		if !found {
			decided.Decision = DecisionIgnored
			return nil
		}
		decided.CaseID = open.ID
		decided.Decision = DecisionJoined
		line.CaseID = open.ID
		return insertHistory(ctx, tx, line)
	})
	if err != nil {
		return res, fmt.Errorf("cases: intake %s: %w", res.EventID, err)
	}
	return decided, nil
}

// admission is one request to open a case for a customer or, when the
// customer already has an open case, to add a line to that case's history.
// With joinSignal the line names signalID, and a case that already has the
// signal (as its own or in its history) gets no second line. Every caller
// sets it today: a signal that joined a case is represented by that case.
type admission struct {
	eventID    string
	customerID string
	occurredAt time.Time
	signalID   string
	factors    ClockFactors
	openedText string
	joinKind   string
	joinText   string
	joinSignal bool
}

// admit applies one admission under the one-open-case-per-customer invariant.
// A duplicate, or an admission that joins an open case, never calls advisory.
// The lookup runs outside the transaction; all writes happen in one
// transaction. A PermanentDeliveryError means redelivery cannot succeed; any
// other error is transient.
func admit(ctx context.Context, store IntakeStore, lookup CustomerLookup, a admission) (IntakeResult, error) {
	res := IntakeResult{EventID: a.eventID, CustomerID: a.customerID}
	seen, err := store.InboxSeen(ctx, a.eventID)
	if err != nil {
		return res, fmt.Errorf("cases: intake %s: %w", a.eventID, err)
	}
	if seen {
		res.Decision = DecisionDuplicate
		return res, nil
	}
	_, hasOpen, err := store.OpenCaseFor(ctx, a.customerID)
	if err != nil {
		return res, fmt.Errorf("cases: intake %s: %w", a.eventID, err)
	}
	var customer Customer
	looked := false
	if !hasOpen {
		customer, err = lookup.Lookup(ctx, a.customerID)
		if err != nil {
			if errors.Is(err, ErrUnknownCustomer) || errors.Is(err, ErrUnusableCustomer) {
				return res, PermanentDeliveryError{Err: fmt.Errorf("cases: intake %s: %w", a.eventID, err)}
			}
			return res, fmt.Errorf("cases: intake %s: lookup customer: %w", a.eventID, err)
		}
		if err := customer.validate(); err != nil {
			return res, PermanentDeliveryError{Err: fmt.Errorf("cases: intake %s: %w", a.eventID, err)}
		}
		looked = true
	}

	var decided IntakeResult
	err = store.WithTx(ctx, func(tx Tx) error {
		decided = res
		claimed, err := tx.ClaimInbox(ctx, a.eventID)
		if err != nil {
			return fmt.Errorf("cases: claim inbox: %w", err)
		}
		if !claimed {
			decided.Decision = DecisionDuplicate
			return nil
		}
		if err := tx.LockCustomer(ctx, a.customerID); err != nil {
			return fmt.Errorf("cases: lock customer: %w", err)
		}
		open, found, err := tx.OpenCaseFor(ctx, a.customerID)
		if err != nil {
			return fmt.Errorf("cases: find open case: %w", err)
		}
		if found {
			decided.CaseID = open.ID
			decided.Decision = DecisionJoined
			line := HistoryRow{CaseID: open.ID, Kind: a.joinKind, Text: a.joinText, OccurredAt: a.occurredAt}
			if a.joinSignal {
				if open.SignalID == a.signalID {
					return nil
				}
				line.SignalID = a.signalID
			}
			return insertHistory(ctx, tx, line)
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
			CustomerID: a.customerID,
			SignalID:   a.signalID,
			AdvisorID:  customer.AdvisorID,
			Segment:    customer.Segment,
			Factors:    a.factors,
			OccurredAt: a.occurredAt,
		})
		if err != nil {
			return err
		}
		decided.CaseID = row.ID
		decided.Decision = DecisionOpened
		return insertHistory(ctx, tx, HistoryRow{
			CaseID: row.ID, Kind: HistoryKindCase, Text: a.openedText, OccurredAt: row.OpenedAt,
		})
	})
	if err != nil {
		return res, fmt.Errorf("cases: intake %s: %w", a.eventID, err)
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

// insertHistory writes row under a new id.
func insertHistory(ctx context.Context, tx Tx, row HistoryRow) error {
	id, err := identity.NewV7()
	if err != nil {
		return fmt.Errorf("cases: history id: %w", err)
	}
	row.ID = id
	if err := tx.InsertHistory(ctx, row); err != nil {
		return fmt.Errorf("cases: insert history %s: %w", row.CaseID, err)
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
	env, payload, err := decodeEnvelope(event.NameMessageTriaged, "triaged", body)
	if err != nil {
		return env, TriagedMessage{}, err
	}
	var msg TriagedMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return env, TriagedMessage{}, fmt.Errorf("cases: decode triaged payload: %w", err)
	}
	env.Payload = msg
	return env, msg, nil
}

// decodeEnvelope parses and validates the envelope of a delivery of the event
// name and returns its raw payload; what labels the errors. Every error is
// permanent: the same body can never decode differently.
func decodeEnvelope(name, what string, body []byte) (event.Envelope, json.RawMessage, error) {
	var raw struct {
		EventID       string          `json:"event_id"`
		OccurredAt    time.Time       `json:"occurred_at"`
		CustomerID    string          `json:"customer_id"`
		SchemaVersion int             `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return event.Envelope{}, nil, fmt.Errorf("cases: decode %s: %w", what, err)
	}
	env := event.Envelope{
		Name:          name,
		EventID:       raw.EventID,
		OccurredAt:    raw.OccurredAt,
		CustomerID:    raw.CustomerID,
		SchemaVersion: raw.SchemaVersion,
	}
	if err := env.Validate(); err != nil {
		return env, nil, fmt.Errorf("cases: validate %s: %w", what, err)
	}
	// inbox.event_id and cases.customer_id are UUID columns.
	eventID, err := uuid.Parse(env.EventID)
	if err != nil {
		return env, nil, fmt.Errorf("cases: validate %s: event_id is not a uuid", what)
	}
	env.EventID = eventID.String()
	customerID, err := identity.ParseV7(env.CustomerID)
	if err != nil {
		return env, nil, fmt.Errorf("cases: validate %s: customer_id: %w", what, err)
	}
	env.CustomerID = customerID

	if len(raw.Payload) == 0 || string(raw.Payload) == "null" {
		return env, nil, fmt.Errorf("cases: validate %s: payload is required", what)
	}
	return env, raw.Payload, nil
}
