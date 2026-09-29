package screen

import (
	"cmp"
	"slices"
	"strconv"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/event"
)

// copyActivityRow keys the catalog copy of timeline rows whose text the BFF
// writes. It is not a component type: the home activity and the Carteira
// history both read it.
const copyActivityRow = "activity_row"

// Timeline kinds whose rows the client may see.
const (
	kindAporte      = "aporte"
	kindSaque       = "saque"
	kindAplicacao   = "aplicacao"
	kindMensagem    = "mensagem"
	kindReavaliacao = "reavaliacao"
)

// activityItems is the one timeline mapping of the home activity and the
// Carteira history: the client-facing rows, most recent first and at most
// limit, as activity items. Product names come from the catalog when it
// answered; the catalog is optional, so a row whose product is unknown still
// shows.
func activityItems(s Snapshot, c Catalog, limit int) ([]ActivityItem, error) {
	rows := visibleActivity(s.Activity.Value, s.Now, limit)
	names := productNames(s)
	items := make([]ActivityItem, 0, len(rows))
	for _, row := range rows {
		item, err := activityItem(row, names, c)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// activityItem maps one client-facing row. Every row has its kind's icon, the
// relative time as meta, and by default its indexed title. An aporte carries
// its amount signed positive with tone pos, a saque signed negative with tone
// neg, when the row has one. Kinds whose text the BFF writes are added as
// cases here: an aplicacao reads "Compra · {{product}}" ("Compra" without a
// known name) with the amount that left caixa, signed; a reavaliacao reads
// "Reavaliação diária" with the simulated day and the product that moved the
// most as meta, and the signed change of the patrimony as value.
func activityItem(row Activity, names map[string]string, c Catalog) (ActivityItem, error) {
	item := ActivityItem{Icon: activityIcon(row.Kind), Title: row.Title, Meta: RelativeTime(row.Age)}
	switch row.Kind {
	case kindAporte:
		if row.AmountCents > 0 {
			item.Value, item.Tone = SignedMoney(row.AmountCents), TonePos
		}
	case kindSaque:
		if row.AmountCents > 0 {
			item.Value, item.Tone = SignedMoney(-row.AmountCents), ToneNeg
		}
	case kindReavaliacao:
		return revaluationItem(row, names, c)
	case kindAplicacao:
		name := names[row.ProductID]
		cp := copier{cat: c, typ: copyActivityRow, variant: "default", fields: Fields{Product: name}}
		key := "aplicacao"
		if name == "" {
			key = "aplicacao_unnamed"
		}
		item.Title = cp.text(key)
		if row.AmountCents > 0 {
			item.Value = SignedMoney(-row.AmountCents)
		}
		if cp.err != nil {
			return ActivityItem{}, cp.err
		}
	}
	return item, nil
}

// revaluationItem maps a reavaliacao row: "dia simulado 3 · Cobalto
// Semicondutores −53,5%" as meta, or "dia simulado 3" when the product has no
// known name, and the signed patrimony change with its tone. A loss gets the
// drop icon.
func revaluationItem(row Activity, names map[string]string, c Catalog) (ActivityItem, error) {
	name := names[row.ProductID]
	cp := copier{cat: c, typ: copyActivityRow, variant: "default", fields: Fields{
		Day:     strconv.Itoa(row.SimDay),
		Product: name,
		Percent: ChangePercentBP(row.ProductChangeBP),
	}}
	metaKey := "reavaliacao_meta"
	if name == "" {
		metaKey = "reavaliacao_meta_unnamed"
	}
	item := ActivityItem{
		Title: cp.text("reavaliacao"),
		Meta:  cp.text(metaKey),
		Value: SignedMoney(row.AmountCents),
		Tone:  SignTone(row.AmountCents),
	}
	if row.AmountCents < 0 {
		item.Icon = "drop"
	}
	if cp.err != nil {
		return ActivityItem{}, cp.err
	}
	return item, nil
}

// productNames maps catalog product id to name, empty when the catalog
// failed.
func productNames(s Snapshot) map[string]string {
	if !s.Products.OK() {
		return map[string]string{}
	}
	names := make(map[string]string, len(s.Products.Value))
	for _, p := range s.Products.Value {
		names[p.ID] = p.Name
	}
	return names
}

// activityIcon maps a client-facing timeline kind to its web icon. Any other
// kind gets no icon.
func activityIcon(kind string) string {
	switch kind {
	case kindAporte:
		return "in"
	case kindSaque, kindAplicacao:
		// A purchase moves cash out of caixa into a position.
		return "out"
	case kindMensagem:
		return "msg"
	default:
		return ""
	}
}

// isClientSource reports whether a timeline row came from an event the client
// caused and may see: their own account movements and the messages they
// sent. Alerts, cases, triage results, and advisor notes are team-side.
func isClientSource(source string) bool {
	return source == event.NameAccountEventRecorded || source == event.NameMessageReceived
}

// visibleActivity is the client-facing rows, most recent first, at most
// limit. A reavaliacao that changed nothing is not shown. Each row's Age is
// measured from OccurredAt against now when both are known, else it keeps the
// indexed Age. rows is never changed.
func visibleActivity(rows []Activity, now time.Time, limit int) []Activity {
	out := make([]Activity, 0, min(len(rows), limit))
	for _, row := range rows {
		if !isClientSource(row.Source) || row.Kind == kindReavaliacao && row.AmountCents == 0 {
			continue
		}
		if !row.OccurredAt.IsZero() && !now.IsZero() {
			row.Age = now.Sub(row.OccurredAt)
		}
		out = append(out, row)
	}
	slices.SortStableFunc(out, func(a, b Activity) int { return cmp.Compare(a.Age, b.Age) })
	return out[:min(len(out), limit)]
}
