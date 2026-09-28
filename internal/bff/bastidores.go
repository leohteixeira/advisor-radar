package bff

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

const quietEval = 3 * time.Second

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
	quiet     bool
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
		Payload    struct {
			SourceEventID string `json:"source_event_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.CustomerID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	eventID := raw.EventID
	if raw.Payload.SourceEventID != "" {
		if _, ok := h.rows[raw.CustomerID][raw.Payload.SourceEventID]; ok {
			eventID = raw.Payload.SourceEventID
		}
	}
	row := h.rows[raw.CustomerID][eventID]
	if row == nil && (routingKey == event.NameAccountEventRecorded || routingKey == event.NameMessageReceived) && raw.EventID != "" {
		eventID = raw.EventID
		row = h.row(raw.CustomerID, eventID)
	}
	if row == nil && (routingKey == event.NameAlertRaised || routingKey == event.NameMessageTriaged) {
		eventID, row = h.openRowLocked(raw.CustomerID)
	}
	if row == nil {
		return
	}
	freshPublish := false
	switch routingKey {
	case event.NameAccountEventRecorded, event.NameMessageReceived:
		freshPublish = !row.published
		row.published = true
	case event.NameMessageTriaged, event.NameAlertRaised:
		row.evaluated = true
		row.queued = true
	default:
		return
	}
	h.publishLocked(raw.CustomerID, h.viewLocked(eventID, row))
	if freshPublish {
		customerID := raw.CustomerID
		time.AfterFunc(quietEval, func() {
			h.settleQuiet(customerID, eventID)
		})
	}
}

func (h *bastidoresHub) settleQuiet(customerID, eventID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	row := h.rows[customerID][eventID]
	if row == nil || row.evaluated || !row.published {
		return
	}
	row.evaluated = true
	row.quiet = true
	h.publishLocked(customerID, h.viewLocked(eventID, row))
}

func (h *bastidoresHub) openRowLocked(customerID string) (string, *bastidoresRow) {
	for id, row := range h.rows[customerID] {
		if row.published && !row.queued {
			return id, row
		}
	}
	return "", nil
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
	pending := make([]BastidoresView, 0, len(h.rows[customerID]))
	for id, row := range h.rows[customerID] {
		pending = append(pending, h.viewLocked(id, row))
	}
	h.mu.Unlock()
	for _, view := range pending {
		select {
		case ch <- view:
		default:
		}
	}
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
			{ID: "queue", Label: fourth, State: stepState(row.queued, row.evaluated && !row.queued && !row.quiet)},
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
