// Package timeline holds the customer 360 memory index and optional adapters.
package timeline

import (
	"context"
	"encoding/json"
	"fmt"
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
}

// Index is an in-memory customer timeline. It has no database.
type Index struct {
	mu      sync.Mutex
	byEvent map[string]struct{}
	byCust  map[string][]Entry
}

// NewIndex returns an index seeded with the design TIMELINES.c01 rows.
func NewIndex() *Index {
	idx := &Index{
		byEvent: make(map[string]struct{}),
		byCust:  make(map[string][]Entry),
	}
	for _, e := range seedC01() {
		idx.byEvent[e.EventID] = struct{}{}
		idx.byCust[e.CustomerID] = append(idx.byCust[e.CustomerID], e)
	}
	return idx
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
			title = strings.ToUpper(kind[:1]) + kind[1:]
		}
		return Entry{EventID: eventID, CustomerID: customerID, Kind: kind, Title: title, Text: p.Text, Meta: p.Meta, Ago: ago}, nil
	case event.NameMessageTriaged:
		var p struct {
			Text    string `json:"text"`
			Intent  string `json:"intent"`
			Channel string `json:"channel"`
			Meta    string `json:"meta"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return Entry{}, fmt.Errorf("timeline: decode message payload: %w", err)
		}
		title := "Mensagem"
		if p.Channel != "" {
			title = "Mensagem · " + p.Channel
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

func seedC01() []Entry {
	const cust = "c01"
	return []Entry{
		{EventID: "tl-c01-msg-1", CustomerID: cust, Kind: "mensagem", Title: "Mensagem · chat", Text: "Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.", Meta: "Reclamação · Frustrado · risco de saída", Ago: 12},
		{EventID: "tl-c01-msg-2", CustomerID: cust, Kind: "mensagem", Title: "Mensagem · e-mail", Text: "A transferência que pedi na segunda ainda não caiu. Podem verificar?", Meta: "Operacional · Incomodado", Ago: 1500},
		{EventID: "tl-c01-nota-1", CustomerID: cust, Kind: "nota", Title: "Nota do assessor", Text: "Cliente pretende comprar imóvel em Orlando no 1º semestre. Precisa de liquidez em março.", Meta: "Ana Paula Ribeiro", Ago: 2900},
		{EventID: "tl-c01-saque-1", CustomerID: cust, Kind: "saque", Title: "Saque", Text: "US$ 20.000,00 para conta nos EUA", Meta: "Sem alerta · 7% do patrimônio", Ago: 4400},
		{EventID: "tl-c01-caso-1", CustomerID: cust, Kind: "caso", Title: "Caso k0977 resolvido", Text: "Dúvida sobre DARF de venda de ETF", Meta: "Tributação · 2 dias", Ago: 10100},
		{EventID: "tl-c01-aporte-1", CustomerID: cust, Kind: "aporte", Title: "Aporte", Text: "US$ 45.000,00", Meta: "Câmbio a 5,42", Ago: 21000},
		{EventID: "tl-c01-tel-1", CustomerID: cust, Kind: "telefone", Title: "Ligação", Text: "Revisão semestral da carteira · 32 min", Meta: "Ana Paula Ribeiro", Ago: 43000},
	}
}
