package bff

import (
	"encoding/json"
	"sync"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// BastidoresStep is one progress line on the confirmation panel.
type BastidoresStep struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	State string `json:"state"`
}

// BastidoresView is the snapshot pushed on the customer stream.
type BastidoresView struct {
	EventID string           `json:"event_id"`
	Steps   []BastidoresStep `json:"steps"`
}

type bastidoresHub struct {
	mu   sync.Mutex
	rows map[string]map[string]*bastidoresRow
	subs map[string]map[chan BastidoresView]struct{}
}

type bastidoresRow struct {
	kind      string
	outbox    bool
	published bool
	evaluated bool
	queued    bool
}

func newBastidoresHub() *bastidoresHub {
	return &bastidoresHub{
		rows: map[string]map[string]*bastidoresRow{},
		subs: map[string]map[chan BastidoresView]struct{}{},
	}
}

func (h *bastidoresHub) markCommand(customerID, eventID, kind string) BastidoresView {
	h.mu.Lock()
	defer h.mu.Unlock()
	row := h.row(customerID, eventID)
	row.kind = kind
	row.outbox = true
	view := h.viewLocked(eventID, row)
	h.publishLocked(customerID, view)
	return view
}

func (h *bastidoresHub) observe(routingKey string, body []byte) {
	var raw struct {
		EventID    string `json:"event_id"`
		CustomerID string `json:"customer_id"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.EventID == "" || raw.CustomerID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	row := h.row(raw.CustomerID, raw.EventID)
	switch routingKey {
	case event.NameAccountEventRecorded, event.NameMessageReceived:
		row.published = true
	case event.NameMessageTriaged:
		row.evaluated = true
		row.queued = true
	case event.NameAlertRaised:
		row.evaluated = true
		row.queued = true
	default:
		return
	}
	h.publishLocked(raw.CustomerID, h.viewLocked(raw.EventID, row))
}

func (h *bastidoresHub) snapshot(customerID, eventID string) (BastidoresView, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	row, ok := h.rows[customerID][eventID]
	if !ok {
		return BastidoresView{}, false
	}
	return h.viewLocked(eventID, row), true
}

func (h *bastidoresHub) subscribe(customerID string) (<-chan BastidoresView, func()) {
	ch := make(chan BastidoresView, 8)
	h.mu.Lock()
	if h.subs[customerID] == nil {
		h.subs[customerID] = map[chan BastidoresView]struct{}{}
	}
	h.subs[customerID][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs[customerID], ch)
		h.mu.Unlock()
	}
}

func (h *bastidoresHub) row(customerID, eventID string) *bastidoresRow {
	if h.rows[customerID] == nil {
		h.rows[customerID] = map[string]*bastidoresRow{}
	}
	row := h.rows[customerID][eventID]
	if row == nil {
		row = &bastidoresRow{}
		h.rows[customerID][eventID] = row
	}
	return row
}

func (h *bastidoresHub) publishLocked(customerID string, view BastidoresView) {
	for ch := range h.subs[customerID] {
		select {
		case ch <- view:
		default:
		}
	}
}

func (h *bastidoresHub) viewLocked(eventID string, row *bastidoresRow) BastidoresView {
	third := "Avaliado pelas regras do advisory"
	fourth := "Na fila da assessoria, se uma regra disparar"
	if row.kind == sim.CmdComplaint || row.kind == sim.CmdMessage {
		third = "Classificado pela triagem"
		fourth = "Na fila da assessoria"
	}
	return BastidoresView{
		EventID: eventID,
		Steps: []BastidoresStep{
			{ID: "outbox", Label: "Gravado na outbox do account-sim", State: stepState(row.outbox, !row.published)},
			{ID: "broker", Label: "Publicado no RabbitMQ", State: stepState(row.published, row.outbox && !row.published)},
			{ID: "evaluated", Label: third, State: stepState(row.evaluated, row.published && !row.evaluated)},
			{ID: "queue", Label: fourth, State: stepState(row.queued, row.evaluated && !row.queued)},
		},
	}
}

func stepState(done, current bool) string {
	if done {
		return "feito"
	}
	if current {
		return "agora"
	}
	return "aguardando"
}
