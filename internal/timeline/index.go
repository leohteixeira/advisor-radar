// Package timeline holds the customer 360 memory index and optional adapters.
package timeline

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/screen"
	"github.com/leohteixeira/advisor-radar/internal/triage"
)

// Chip filter values accepted by Search. Empty or "todos" means no chip filter.
const (
	KindTodos     = "todos"
	KindConta     = "conta"
	KindMensagens = "mensagens"
	KindCasos     = "casos"
	KindNotas     = "notas"
)

var contaKinds = map[string]struct{}{
	"saque": {}, "aporte": {}, "queda": {}, "segmento": {}, "contato": {},
	"aplicacao": {}, "reavaliacao": {}, "perfil": {},
}

// Entry is one timeline row shown in the customer 360.
type Entry struct {
	EventID    string `json:"event_id"`
	CustomerID string `json:"customer_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Text       string `json:"text"`
	Meta       string `json:"meta"`
	// Ago is the whole minutes from OccurredAt to the instant of the Search
	// that returned the row, never negative. It is not stored: the index keeps
	// OccurredAt, so an age never freezes at the moment the row was indexed.
	Ago int `json:"-"`
	// Source is the routing key of the event the row came from.
	Source string `json:"source,omitempty"`
	// OccurredAt is the event time.
	OccurredAt time.Time `json:"occurred_at,omitzero"`
	// ProductID is the catalog product an account event names, such as the
	// one an aplicacao bought.
	ProductID string `json:"product_id,omitempty"`
	// AmountCents is an account event's amount in integer USD cents, whatever
	// its schema version (version 1 whole dollars are scaled). Zero for rows
	// that are not account events, and for an account event whose amount is
	// not a number or is past 2^53 cents.
	AmountCents int64 `json:"amount_cents,omitempty"`
	// SimDay is the simulated day a reavaliacao revalued the account to, and
	// ProductChangeBP the signed day change in basis points of the product
	// that moved it the most. Both are 0 on every other row.
	SimDay          int `json:"sim_day,omitempty"`
	ProductChangeBP int `json:"product_change_bp,omitempty"`

	// messageID is set while indexing a message.triaged: the event_id of the
	// message.received it classifies, whose row it joins.
	messageID string
}

// Index is an in-memory customer timeline. It has no database.
type Index struct {
	mu       sync.Mutex
	now      func() time.Time
	products map[string]string
	// byEvent maps every applied event_id to the EventID of the row it went
	// into: its own, or the message's for a message.triaged.
	byEvent map[string]string
	byCust  map[string][]Entry
}

// Option configures an Index.
type Option func(*Index)

// WithClock sets the clock Search measures ages against. The default is
// time.Now.
func WithClock(now func() time.Time) Option {
	return func(idx *Index) { idx.now = now }
}

// WithProductNames sets the catalog names, by product id, that the row of a
// purchase or of a revaluation names its product with. Without a name the row
// leaves the product out.
func WithProductNames(names map[string]string) Option {
	return func(idx *Index) { idx.products = maps.Clone(names) }
}

// NewIndex returns an empty in-memory customer timeline.
func NewIndex(opts ...Option) *Index {
	idx := &Index{
		now:     time.Now,
		byEvent: make(map[string]string),
		byCust:  make(map[string][]Entry),
	}
	for _, opt := range opts {
		opt(idx)
	}
	return idx
}

// Search returns entries for customerID filtered by optional query and kind
// chip, most recent first. Each entry's Ago is measured against the clock at
// this call.
func (idx *Index) Search(ctx context.Context, customerID, query, kind string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()

	rows := idx.byCust[customerID]
	if len(rows) == 0 {
		return []Entry{}, nil
	}

	q := fold(strings.TrimSpace(query))
	chip := strings.ToLower(strings.TrimSpace(kind))
	now := idx.now()
	out := make([]Entry, 0, len(rows))
	for _, e := range rows {
		if !matchChip(e.Kind, chip) {
			continue
		}
		if q != "" && !matchQuery(e, q) {
			continue
		}
		e.Ago = minutesBetween(e.OccurredAt, now)
		out = append(out, e)
	}
	// Most recent first; tie-break on event_id for a stable order.
	slices.SortFunc(out, func(a, b Entry) int {
		if c := b.OccurredAt.Compare(a.OccurredAt); c != 0 {
			return c
		}
		return cmp.Compare(a.EventID, b.EventID)
	})
	return out, nil
}

// ApplyDelivery indexes one event. A second delivery of the same event_id is a no-op.
// When a row is stored, applied is true and entry holds that row.
//
// One fact is one row. A message is told by two events, message.received and
// the message.triaged that classifies it (source_event_id); they share the
// row of the message, whichever arrives first: the message gives it its
// id, time, and text, and the classification its meta when the message
// brought none. Listing both showed every message twice.
func (idx *Index) ApplyDelivery(ctx context.Context, routingKey string, body []byte) (applied bool, entry Entry, err error) {
	if err := ctx.Err(); err != nil {
		return false, Entry{}, err
	}
	if len(body) == 0 {
		return false, Entry{}, fmt.Errorf("timeline: empty delivery")
	}

	var raw struct {
		EventID       string          `json:"event_id"`
		OccurredAt    time.Time       `json:"occurred_at"`
		CustomerID    string          `json:"customer_id"`
		SchemaVersion int             `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return false, Entry{}, fmt.Errorf("timeline: decode delivery: %w", err)
	}
	if raw.EventID == "" || raw.CustomerID == "" || raw.OccurredAt.IsZero() {
		return false, Entry{}, fmt.Errorf("timeline: invalid envelope")
	}
	if !event.KnownSchemaVersion(raw.SchemaVersion) {
		return false, Entry{}, fmt.Errorf("timeline: unsupported schema_version %d", raw.SchemaVersion)
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()
	if _, ok := idx.byEvent[raw.EventID]; ok {
		return false, Entry{}, nil
	}

	entry, err = idx.entryFromEvent(routingKey, raw.SchemaVersion, raw.Payload)
	if err != nil {
		return false, Entry{}, err
	}
	rowID := raw.EventID
	if entry.messageID != "" {
		rowID = entry.messageID
	}
	entry.messageID = ""
	entry.EventID = rowID
	entry.CustomerID = raw.CustomerID
	entry.Source = routingKey
	entry.OccurredAt = raw.OccurredAt.UTC()

	idx.byEvent[raw.EventID] = rowID
	rows := idx.byCust[raw.CustomerID]
	if i := slices.IndexFunc(rows, func(e Entry) bool { return e.EventID == rowID }); i >= 0 {
		rows[i] = mergeMessage(rows[i], entry)
		return true, rows[i], nil
	}
	idx.byCust[raw.CustomerID] = append(rows, entry)
	return true, entry, nil
}

// mergeMessage joins the two rows of one message: the message.received one
// stands, and takes the classification's meta when it has none.
func mergeMessage(stored, incoming Entry) Entry {
	message, other := stored, incoming
	if incoming.Source == event.NameMessageReceived {
		message, other = incoming, stored
	}
	if message.Meta == "" {
		message.Meta = other.Meta
	}
	return message
}

// Forget removes a row by its EventID, with every event that went into it,
// so a requeued delivery can apply again (e.g. after IndexDoc failure).
func (idx *Index) Forget(rowID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	maps.DeleteFunc(idx.byEvent, func(_, row string) bool { return row == rowID })
	for cust, rows := range idx.byCust {
		idx.byCust[cust] = slices.DeleteFunc(rows, func(e Entry) bool { return e.EventID == rowID })
	}
}

func matchChip(entryKind, chip string) bool {
	switch chip {
	case "", KindTodos:
		return true
	case KindConta:
		_, ok := contaKinds[entryKind]
		return ok
	case KindMensagens:
		return entryKind == "mensagem"
	case KindCasos:
		return entryKind == "caso"
	case KindNotas:
		return entryKind == "nota"
	default:
		return false
	}
}

func matchQuery(e Entry, q string) bool {
	return strings.Contains(fold(e.Title+" "+e.Text+" "+e.Meta), q)
}

// accents maps the accented lower-case letters of Portuguese text to their
// base letters.
var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// fold is the form the search compares in: lower case without accents, so
// "sergio" finds "Sérgio" and "orlândo" finds "Orlando".
func fold(s string) string {
	return accents.Replace(strings.ToLower(s))
}

// entryFromEvent maps one event to the text of its row: kind, title, text,
// meta, and the account facts. schemaVersion scales an account event's
// amount: version 1 carries whole dollars, later versions cents.
func (idx *Index) entryFromEvent(name string, schemaVersion int, payload json.RawMessage) (Entry, error) {
	switch name {
	case event.NameAccountEventRecorded:
		return idx.accountEntry(schemaVersion, payload)
	case event.NameMessageReceived, event.NameMessageTriaged:
		var p struct {
			Text    string `json:"text"`
			Intent  string `json:"intent"`
			Channel string `json:"channel"`
			Title   string `json:"title"`
			Meta    string `json:"meta"`
			// SourceEventID is on message.triaged: the message it classifies.
			SourceEventID string `json:"source_event_id"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return Entry{}, fmt.Errorf("timeline: decode message payload: %w", err)
		}
		title := p.Title
		if title == "" {
			title = "Mensagem"
			if p.Channel != "" {
				title = "Mensagem · " + p.Channel
			}
		}
		meta := p.Meta
		if meta == "" {
			// The triaged intent is a key ("reclamacao"); the row shows its label.
			meta = triage.IntentLabel(p.Intent)
		}
		return Entry{Kind: "mensagem", Title: title, Text: p.Text, Meta: meta, messageID: p.SourceEventID}, nil
	case event.NameAlertRaised:
		// Reason is advisory's own description of the fact that fired the
		// rule ("Desceu de Advance para Essencial"). An alert published before
		// advisory wrote one has none, and its row shows the rule alone.
		var p struct {
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
			Rule   string `json:"rule"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return Entry{}, fmt.Errorf("timeline: decode alert payload: %w", err)
		}
		kind := p.Kind
		if kind == "" {
			kind = kindSaque
		}
		return Entry{Kind: kind, Title: alertTitle(kind), Text: p.Reason, Meta: p.Rule}, nil
	case event.NameCaseOpened, event.NameCaseStatusChanged:
		var p struct {
			CaseID string `json:"case_id"`
			State  string `json:"state"`
			Text   string `json:"text"`
			Meta   string `json:"meta"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return Entry{}, fmt.Errorf("timeline: decode case payload: %w", err)
		}
		// The title carries the protocol the client app shows, not the raw
		// id, and the text says what happened to the case.
		title := "Caso"
		if p.CaseID != "" {
			title = "Caso " + screen.Protocol(p.CaseID)
		}
		text := p.Text
		if text == "" && p.State != "" {
			if name == event.NameCaseOpened {
				text = "Caso aberto"
			} else {
				text = "Caso passou para " + p.State
			}
		}
		return Entry{Kind: "caso", Title: title, Text: text, Meta: p.Meta}, nil
	case "advisory.note.recorded":
		var p struct {
			Kind  string `json:"kind"`
			Title string `json:"title"`
			Text  string `json:"text"`
			Meta  string `json:"meta"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return Entry{}, fmt.Errorf("timeline: decode note payload: %w", err)
		}
		kind := p.Kind
		if kind == "" {
			kind = "nota"
		}
		title := p.Title
		if title == "" {
			title = "Nota do assessor"
		}
		return Entry{Kind: kind, Title: title, Text: p.Text, Meta: p.Meta}, nil
	default:
		return Entry{}, fmt.Errorf("timeline: unsupported event %q", name)
	}
}

// maxExactCents bounds the amounts amountCents keeps: every integer up to
// 2^53 is exact in a float64.
const maxExactCents = 1 << 53

// amountCents reads an account event amount as integer USD cents. Schema
// version 1 carries whole dollars; versions 2 and 3 carry cents. An amount
// that is not a number or is too large to be exact reads 0: the row is still
// worth indexing without it.
func amountCents(amount any, schemaVersion int) int64 {
	n, ok := amount.(float64)
	if !ok {
		return 0
	}
	if schemaVersion == 1 {
		n *= 100
	}
	n = math.Round(n)
	if math.Abs(n) > maxExactCents {
		return 0
	}
	return int64(n)
}

// alertTitle is the title of an alert row: the queue card's name for the
// rule, marked as an alert. The account event that raised it has its own row
// ("Saque"), and the two must not read as two movements.
func alertTitle(kind string) string {
	switch kind {
	case kindSaque:
		return "Alerta: saque relevante"
	case kindQueda:
		return "Alerta: queda de patrimônio"
	case kindAporte:
		return "Alerta: aporte grande"
	case "segmento":
		return "Alerta: mudança de segmento"
	case "contato":
		return "Alerta: sem contato há muito tempo"
	case "perfil":
		return "Alerta: compra acima do perfil"
	default:
		return "Alerta: " + kind
	}
}

func kindTitle(kind string) string {
	switch kind {
	case kindSaque:
		return "Saque"
	case kindAporte:
		return "Aporte"
	case kindQueda:
		return "Queda"
	case "segmento":
		return "Segmento"
	case "contato":
		return "Contato"
	case kindAplicacao:
		return "Aplicação"
	case kindReavaliacao:
		return "Reavaliação"
	case "perfil":
		return "Compra acima do perfil"
	default:
		return kind
	}
}

// minutesBetween is the whole minutes from at to now, 0 when at is not in
// the past.
func minutesBetween(at, now time.Time) int {
	return int(max(now.Sub(at), 0) / time.Minute)
}
