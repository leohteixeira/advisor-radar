// Package timeline holds the customer 360 memory index and optional adapters.
package timeline

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
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
}

// Entry is one timeline row shown in the customer 360.
type Entry struct {
	EventID    string `json:"event_id"`
	CustomerID string `json:"customer_id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Text       string `json:"text"`
	Meta       string `json:"meta"`
	Ago        int    `json:"ago"`
	// Source is the routing key of the event the row came from.
	Source string `json:"source,omitempty"`
	// OccurredAt is the event time; Ago is frozen at index time.
	OccurredAt time.Time `json:"occurred_at,omitzero"`
}

// Index is an in-memory customer timeline. It has no database.
type Index struct {
	mu      sync.Mutex
	byEvent map[string]struct{}
	byCust  map[string][]Entry
}

// NewIndex returns an empty in-memory customer timeline.
func NewIndex() *Index {
	return &Index{
		byEvent: make(map[string]struct{}),
		byCust:  make(map[string][]Entry),
	}
}

// Search returns entries for customerID filtered by optional query and kind chip.
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

	q := strings.ToLower(strings.TrimSpace(query))
	chip := strings.ToLower(strings.TrimSpace(kind))
	out := make([]Entry, 0, len(rows))
	for _, e := range rows {
		if !matchChip(e.Kind, chip) {
			continue
		}
		if q != "" && !matchQuery(e, q) {
			continue
		}
		out = append(out, e)
	}
	// Smaller ago is more recent; tie-break on event_id for a stable order.
	slices.SortFunc(out, func(a, b Entry) int {
		if c := cmp.Compare(a.Ago, b.Ago); c != 0 {
			return c
		}
		return cmp.Compare(a.EventID, b.EventID)
	})
	return out, nil
}

// ApplyDelivery indexes one event. A second delivery of the same event_id is a no-op.
// When a new row is stored, applied is true and entry holds that row.
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
	if raw.EventID == "" || raw.CustomerID == "" || raw.OccurredAt.IsZero() || raw.SchemaVersion < 1 {
		return false, Entry{}, fmt.Errorf("timeline: invalid envelope")
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()
	if _, ok := idx.byEvent[raw.EventID]; ok {
		return false, Entry{}, nil
	}

	entry, err = entryFromEvent(routingKey, raw.EventID, raw.CustomerID, raw.OccurredAt, raw.Payload)
	if err != nil {
		return false, Entry{}, err
	}
	entry.Source = routingKey
	entry.OccurredAt = raw.OccurredAt.UTC()

	idx.byEvent[raw.EventID] = struct{}{}
	idx.byCust[raw.CustomerID] = append(idx.byCust[raw.CustomerID], entry)
	return true, entry, nil
}

// Forget removes an event_id so a requeued delivery can apply again (e.g. after IndexDoc failure).
func (idx *Index) Forget(eventID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.byEvent, eventID)
	for cust, rows := range idx.byCust {
		out := make([]Entry, 0, len(rows))
		for _, e := range rows {
			if e.EventID != eventID {
				out = append(out, e)
			}
		}
		idx.byCust[cust] = out
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
	hay := strings.ToLower(e.Title + " " + e.Text + " " + e.Meta)
	return strings.Contains(hay, q)
}

func entryFromEvent(name, eventID, customerID string, at time.Time, payload json.RawMessage) (Entry, error) {
	ago := minutesAgo(at)
	switch name {
	case event.NameAccountEventRecorded:
		var p struct {
			Kind   string `json:"kind"`
			Title  string `json:"title"`
			Text   string `json:"text"`
			Meta   string `json:"meta"`
			Amount any    `json:"amount"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return Entry{}, fmt.Errorf("timeline: decode account payload: %w", err)
		}
		kind := p.Kind
		if kind == "" {
			kind = "saque"
		}
		title := p.Title
		if title == "" {
			title = kindTitle(kind)
		}
		return Entry{EventID: eventID, CustomerID: customerID, Kind: kind, Title: title, Text: p.Text, Meta: p.Meta, Ago: ago}, nil
	case event.NameMessageReceived, event.NameMessageTriaged:
		var p struct {
			Text    string `json:"text"`
			Intent  string `json:"intent"`
			Channel string `json:"channel"`
			Title   string `json:"title"`
			Meta    string `json:"meta"`
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
			meta = p.Intent
		}
		return Entry{EventID: eventID, CustomerID: customerID, Kind: "mensagem", Title: title, Text: p.Text, Meta: meta, Ago: ago}, nil
	case event.NameAlertRaised:
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
			kind = "saque"
		}
		return Entry{EventID: eventID, CustomerID: customerID, Kind: kind, Title: kindTitle(kind), Text: p.Reason, Meta: p.Rule, Ago: ago}, nil
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
		title := "Caso"
		if p.CaseID != "" {
			title = "Caso " + p.CaseID
			if p.State != "" {
				title += " " + p.State
			}
		}
		return Entry{EventID: eventID, CustomerID: customerID, Kind: "caso", Title: title, Text: p.Text, Meta: p.Meta, Ago: ago}, nil
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
		return Entry{EventID: eventID, CustomerID: customerID, Kind: kind, Title: title, Text: p.Text, Meta: p.Meta, Ago: ago}, nil
	default:
		return Entry{}, fmt.Errorf("timeline: unsupported event %q", name)
	}
}

func kindTitle(kind string) string {
	switch kind {
	case "saque":
		return "Saque"
	case "aporte":
		return "Aporte"
	case "queda":
		return "Queda"
	case "segmento":
		return "Segmento"
	case "contato":
		return "Contato"
	default:
		return kind
	}
}

func minutesAgo(at time.Time) int {
	d := time.Since(at)
	if d < 0 {
		d = -d
	}
	return int(d.Minutes())
}
