// Package bff serves the in-memory advisor queue over HTTP and SSE.
package bff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

const ringCapacity = 64

// Signal is one queue card. Field names match SIGNALS / STREAM in mock-data.js.
type Signal struct {
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	Client      string             `json:"client"`
	Ago         int                `json:"ago,omitempty"`
	Text        string             `json:"text,omitempty"`
	Intent      string             `json:"intent,omitempty"`
	Dist        map[string]float64 `json:"dist,omitempty"`
	Frustration *int               `json:"frustration,omitempty"`
	Churn       *bool              `json:"churn,omitempty"`
	ChurnConf   string             `json:"churnConf,omitempty"`
	Human       *bool              `json:"human,omitempty"`
	Channel     string             `json:"channel,omitempty"`
	Fallback    bool               `json:"fallback,omitempty"`
	Alert       string             `json:"alert,omitempty"`
	Amount      float64            `json:"amount,omitempty"`
	Before      float64            `json:"before,omitempty"`
	After       float64            `json:"after,omitempty"`
	From        string             `json:"from,omitempty"`
	To          string             `json:"to,omitempty"`
	Days        int                `json:"days,omitempty"`
	Rule        string             `json:"rule,omitempty"`
	Reason      string             `json:"reason,omitempty"`
	ContactedAt *time.Time         `json:"contacted_at,omitempty"`
}

// StreamEvent is one retained SSE frame (event name is always "signal").
type StreamEvent struct {
	ID   string
	Data []byte
}

type subscriber struct {
	ch       chan StreamEvent
	isClosed bool
	mu       sync.Mutex
}

// Board is the in-memory queue and SSE ring. It has no database.
type Board struct {
	mu    sync.Mutex
	items []Signal
	seen  map[string]struct{}
	ring  []StreamEvent
	subs  map[*subscriber]struct{}
}

// NewBoard returns a board seeded with signals s01–s17.
func NewBoard() *Board {
	return &Board{
		items: seedSignals(),
		seen:  make(map[string]struct{}),
		subs:  make(map[*subscriber]struct{}),
	}
}

// Items returns a copy of the current queue.
func (b *Board) Items() []Signal {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Signal, len(b.items))
	copy(out, b.items)
	return out
}

// permanentError marks a decode/validate failure that must not be requeued.
type permanentError struct {
	err error
}

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// IsPermanent reports whether err is a non-requeueable delivery failure.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

// ApplyDelivery appends one signal from a broker body. routingKey is the
// event name. A second delivery of the same event_id is a no-op.
func (b *Board) ApplyDelivery(ctx context.Context, routingKey string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(body) == 0 {
		return permanentError{err: fmt.Errorf("bff: empty delivery")}
	}

	var raw struct {
		EventID       string          `json:"event_id"`
		OccurredAt    time.Time       `json:"occurred_at"`
		CustomerID    string          `json:"customer_id"`
		SchemaVersion int             `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return permanentError{err: fmt.Errorf("bff: decode delivery: %w", err)}
	}
	if raw.EventID == "" || raw.CustomerID == "" || raw.OccurredAt.IsZero() || raw.SchemaVersion < 1 {
		return permanentError{err: fmt.Errorf("bff: invalid envelope")}
	}

	switch routingKey {
	case "alert.raised":
		return b.applyAlert(ctx, raw.EventID, raw.CustomerID, raw.Payload)
	case "message.triaged":
		return b.applyTriaged(ctx, raw.EventID, raw.CustomerID, raw.Payload)
	default:
		return permanentError{err: fmt.Errorf("bff: unknown routing key %q", routingKey)}
	}
}

func (b *Board) applyAlert(ctx context.Context, eventID, customerID string, payload json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var p struct {
		Kind   string  `json:"kind"`
		Rule   string  `json:"rule"`
		Amount float64 `json:"amount"`
		Before float64 `json:"before"`
		After  float64 `json:"after"`
		From   string  `json:"from"`
		To     string  `json:"to"`
		Days   int     `json:"days"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return permanentError{err: fmt.Errorf("bff: decode alert payload: %w", err)}
	}
	switch p.Kind {
	case "saque", "queda", "aporte", "segmento", "contato":
	default:
		return permanentError{err: fmt.Errorf("bff: unknown alert kind %q", p.Kind)}
	}

	sig := Signal{
		ID:     eventID,
		Kind:   "alert",
		Client: customerID,
		Alert:  p.Kind,
		Amount: p.Amount,
		Before: p.Before,
		After:  p.After,
		From:   p.From,
		To:     p.To,
		Days:   p.Days,
		Rule:   p.Rule,
	}
	return b.appendLive(sig)
}

func (b *Board) applyTriaged(ctx context.Context, eventID, customerID string, payload json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var p struct {
		Text        string  `json:"text"`
		Intent      string  `json:"intent"`
		IntentProb  float64 `json:"intent_prob"`
		Frustration float64 `json:"frustration"`
		ChurnRisk   float64 `json:"churn_risk"`
		WantsHuman  float64 `json:"wants_human"`
		Degraded    bool    `json:"degraded"`
		Channel     string  `json:"channel"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return permanentError{err: fmt.Errorf("bff: decode triaged payload: %w", err)}
	}
	label, ok := intentLabel(p.Intent)
	if !ok {
		return permanentError{err: fmt.Errorf("bff: unknown intent %q", p.Intent)}
	}

	frustration := int(p.Frustration)
	if frustration < 0 {
		frustration = 0
	}
	if frustration > 3 {
		frustration = 3
	}
	churn := p.ChurnRisk >= 0.5
	human := p.WantsHuman >= 0.5
	sig := Signal{
		ID:          eventID,
		Kind:        "message",
		Client:      customerID,
		Text:        p.Text,
		Intent:      label,
		Dist:        map[string]float64{label: p.IntentProb},
		Frustration: &frustration,
		Churn:       &churn,
		ChurnConf:   confidenceBand(p.ChurnRisk),
		Human:       &human,
		Channel:     p.Channel,
		Fallback:    p.Degraded,
	}
	return b.appendLive(sig)
}

func (b *Board) appendLive(sig Signal) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.seen[sig.ID]; ok {
		return nil
	}
	b.seen[sig.ID] = struct{}{}
	b.items = append(b.items, sig)

	data, err := json.Marshal(sig)
	if err != nil {
		return fmt.Errorf("bff: marshal signal: %w", err)
	}
	ev := StreamEvent{ID: sig.ID, Data: data}
	if len(b.ring) >= ringCapacity {
		b.ring = b.ring[1:]
	}
	b.ring = append(b.ring, ev)

	for s := range b.subs {
		s.enqueue(ev)
	}
	return nil
}

// Subscribe registers for live SSE frames.
// An empty lastEventID starts from live only.
// A known lastEventID replays later retained events.
// An unknown lastEventID replays the retained window.
func (b *Board) Subscribe(lastEventID string) (<-chan StreamEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := &subscriber{ch: make(chan StreamEvent, ringCapacity+8)}
	for _, ev := range b.catchUp(lastEventID) {
		s.ch <- ev
	}
	b.subs[s] = struct{}{}

	unsub := func() {
		b.mu.Lock()
		delete(b.subs, s)
		b.mu.Unlock()
		s.close()
	}
	return s.ch, unsub
}

func (b *Board) catchUp(lastEventID string) []StreamEvent {
	if lastEventID == "" {
		return nil
	}
	for i, ev := range b.ring {
		if ev.ID == lastEventID {
			if i+1 >= len(b.ring) {
				return nil
			}
			out := make([]StreamEvent, len(b.ring)-i-1)
			copy(out, b.ring[i+1:])
			return out
		}
	}
	out := make([]StreamEvent, len(b.ring))
	copy(out, b.ring)
	return out
}

// Stream writes catch-up and live events until ctx is canceled.
func (b *Board) Stream(ctx context.Context, lastEventID string, emit func(StreamEvent) error) error {
	ch, unsub := b.Subscribe(lastEventID)
	defer unsub()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := emit(ev); err != nil {
				return err
			}
		}
	}
}

// Drain closes every subscriber so SSE handlers can return on shutdown.
func (b *Board) Drain() {
	b.mu.Lock()
	subs := make([]*subscriber, 0, len(b.subs))
	for s := range b.subs {
		subs = append(subs, s)
		delete(b.subs, s)
	}
	b.mu.Unlock()
	for _, s := range subs {
		s.close()
	}
}

func (s *subscriber) enqueue(ev StreamEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isClosed {
		return
	}
	if len(s.ch) == cap(s.ch) {
		select {
		case <-s.ch:
		default:
		}
	}
	select {
	case s.ch <- ev:
	default:
	}
}

func (s *subscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isClosed {
		return
	}
	s.isClosed = true
	close(s.ch)
}

func intentLabel(wire string) (string, bool) {
	labels := map[string]string{
		"operacional":  "Operacional",
		"cambio":       "Câmbio",
		"tributacao":   "Tributação",
		"investimento": "Investimento",
		"resgate":      "Resgate",
		"reclamacao":   "Reclamação",
		"encerramento": "Encerramento",
		"contato":      "Contato",
	}
	label, ok := labels[wire]
	return label, ok
}

func confidenceBand(prob float64) string {
	switch {
	case prob >= 0.75:
		return "alta"
	case prob >= 0.5:
		return "média"
	default:
		return "baixa"
	}
}

func seedSignals() []Signal {
	i := func(v int) *int { return &v }
	b := func(v bool) *bool { return &v }
	return []Signal{
		{ID: "s01", Kind: "message", Client: "c01", Ago: 12, Text: "Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.", Intent: "Reclamação", Dist: map[string]float64{"Reclamação": 0.82, "Encerramento": 0.11, "Operacional": 0.04, "Resgate": 0.03}, Frustration: i(2), Churn: b(true), ChurnConf: "alta", Human: b(false), Channel: "chat"},
		{ID: "s02", Kind: "message", Client: "c02", Ago: 25, Text: "Já é a terceira vez que eu explico o mesmo problema e ninguém resolve. Um absurdo.", Intent: "Reclamação", Dist: map[string]float64{"Reclamação": 0.91, "Operacional": 0.06, "Encerramento": 0.03}, Frustration: i(3), Churn: b(false), ChurnConf: "média", Human: b(false), Channel: "e-mail"},
		{ID: "s03", Kind: "message", Client: "c03", Ago: 8, Text: "Consegue pedir para o meu assessor me ligar?", Intent: "Contato", Dist: map[string]float64{"Contato": 0.94, "Operacional": 0.04, "Investimento": 0.02}, Frustration: i(0), Churn: b(false), ChurnConf: "alta", Human: b(true), Channel: "chat"},
		{ID: "s04", Kind: "message", Client: "c04", Ago: 100, Text: "Vendi ações com lucro em julho. Tenho que pagar DARF?", Intent: "Tributação", Dist: map[string]float64{"Tributação": 0.88, "Investimento": 0.08, "Operacional": 0.04}, Frustration: i(0), Churn: b(false), ChurnConf: "alta", Human: b(false), Channel: "e-mail"},
		{ID: "s05", Kind: "message", Client: "c05", Ago: 33, Text: "Preciso sacar 5 mil dólares e trazer de volta para o Brasil.", Intent: "Resgate", Dist: map[string]float64{"Resgate": 0.54, "Câmbio": 0.31, "Operacional": 0.10, "Encerramento": 0.05}, Frustration: i(0), Churn: b(false), ChurnConf: "média", Human: b(false), Channel: "chat"},
		{ID: "s06", Kind: "message", Client: "c06", Ago: 50, Text: "Meu cartão foi recusado na viagem, o que eu faço?", Intent: "Operacional", Dist: map[string]float64{"Operacional": 0.71, "Reclamação": 0.19, "Contato": 0.10}, Frustration: i(1), Churn: b(false), ChurnConf: "alta", Human: b(false), Channel: "chat", Fallback: true},
		{ID: "s07", Kind: "message", Client: "c07", Ago: 65, Text: "Quero encerrar minha conta. Como faço para transferir os ativos?", Intent: "Encerramento", Dist: map[string]float64{"Encerramento": 0.86, "Resgate": 0.09, "Operacional": 0.05}, Frustration: i(0), Churn: b(true), ChurnConf: "alta", Human: b(false), Channel: "e-mail"},
		{ID: "s08", Kind: "message", Client: "c08", Ago: 120, Text: "Qual a cotação que vocês usam no câmbio? Está muito diferente do Google.", Intent: "Câmbio", Dist: map[string]float64{"Câmbio": 0.79, "Reclamação": 0.15, "Operacional": 0.06}, Frustration: i(1), Churn: b(false), ChurnConf: "alta", Human: b(false), Channel: "chat"},
		{ID: "s09", Kind: "message", Client: "c09", Ago: 185, Text: "Vocês têm alguma recomendação de renda fixa com vencimento em 2028?", Intent: "Investimento", Dist: map[string]float64{"Investimento": 0.93, "Operacional": 0.05, "Tributação": 0.02}, Frustration: i(0), Churn: b(false), ChurnConf: "alta", Human: b(false), Channel: "e-mail"},
		{ID: "s10", Kind: "message", Client: "c10", Ago: 18, Text: "Estou frustrado com a demora pra liberar a transferência. Alguém pode me explicar?", Intent: "Operacional", Dist: map[string]float64{"Operacional": 0.58, "Reclamação": 0.35, "Contato": 0.07}, Frustration: i(2), Churn: b(false), ChurnConf: "média", Human: b(true), Channel: "chat"},
		{ID: "s11", Kind: "alert", Client: "c11", Ago: 22, Alert: "saque", Amount: 190000, Before: 640000, After: 450000, Rule: "Saque acima de 20% do patrimônio em 24 horas", Reason: "Saque de US$ 190.000,00, 30% do patrimônio"},
		{ID: "s12", Kind: "alert", Client: "c12", Ago: 180, Alert: "queda", Amount: -22000, Before: 130000, After: 108000, Rule: "Queda acima de 15% em 5 dias úteis", Reason: "Patrimônio caiu 17% em 5 dias"},
		{ID: "s13", Kind: "alert", Client: "c13", Ago: 240, Alert: "aporte", Amount: 60000, Before: 8000, After: 68000, Rule: "Aporte maior que o patrimônio anterior", Reason: "Aporte de US$ 60.000,00, 7,5× o patrimônio"},
		{ID: "s14", Kind: "alert", Client: "c13", Ago: 238, Alert: "segmento", Before: 8000, After: 68000, From: "Essencial", To: "Advance", Rule: "Patrimônio cruzou a faixa de US$ 10 mil", Reason: "Subiu de Essencial para Advance"},
		{ID: "s15", Kind: "alert", Client: "c14", Ago: 1440, Alert: "contato", Days: 94, Before: 175000, After: 175000, Rule: "Sem contato há mais de 90 dias", Reason: "Último contato há 94 dias"},
		{ID: "s16", Kind: "alert", Client: "c15", Ago: 6, Alert: "saque", Amount: 300000, Before: 1250000, After: 950000, Rule: "Saque acima de 20% do patrimônio em 24 horas", Reason: "Saque de US$ 300.000,00, 24% do patrimônio"},
		{ID: "s17", Kind: "alert", Client: "c16", Ago: 300, Alert: "queda", Amount: -2100, Before: 9600, After: 7500, Rule: "Queda acima de 15% em 5 dias úteis", Reason: "Patrimônio caiu 22% em 5 dias"},
	}
}
