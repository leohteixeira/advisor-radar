package bff

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/screen"
)

// customerBook loads display fields for the given customer ids from advisory.
func (h *Handler) customerBook(ctx context.Context, ids []string) map[string]Customer {
	out := make(map[string]Customer)
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		c, err := h.queue.GetCustomer(ctx, id)
		if err != nil {
			continue
		}
		out[id] = c
	}
	return out
}

// enrichSignals fills name and segment from the advisory book when missing
// (for example live SSE cards that never joined book).
func (h *Handler) enrichSignals(ctx context.Context, items []Signal) {
	need := make([]string, 0, len(items))
	for _, it := range items {
		if it.Name == "" || it.Segment == "" {
			need = append(need, it.Client)
		}
	}
	book := h.customerBook(ctx, need)
	for i := range items {
		c, ok := book[items[i].Client]
		if !ok {
			continue
		}
		if items[i].Name == "" {
			items[i].Name = c.Name
		}
		if items[i].Segment == "" {
			items[i].Segment = c.Segment
		}
	}
	h.describePurchases(ctx, items)
}

// ProductCatalog is the optional account-sim catalog read the queue uses to
// name the product of a perfil card. A POVSource that implements it lends
// its names; otherwise the card names the product id.
type ProductCatalog interface {
	Products(ctx context.Context) ([]POVProduct, error)
}

// describePurchases writes the reason of every perfil card that has none:
// what was bought, its risk, and the profile's limit. The catalog is read at
// most once per call, and only when such a card is present.
func (h *Handler) describePurchases(ctx context.Context, items []Signal) {
	var names map[string]string
	for i := range items {
		sig := &items[i]
		if sig.Kind != "alert" || sig.Alert != alertPerfil || sig.Reason != "" {
			continue
		}
		if names == nil {
			names = h.productNames(ctx)
		}
		sig.Reason = perfilReason(*sig, names[sig.ProductID])
	}
}

// catalogTimeout bounds the catalog read of one render, so a slow account-sim
// only costs the product names.
const catalogTimeout = 300 * time.Millisecond

// productNameCache keeps the product names after the first successful catalog
// read; the catalog is fixed for the life of the simulation. A failed read is
// not cached, so the next render tries again.
type productNameCache struct {
	mu    sync.Mutex
	names map[string]string
}

func (c *productNameCache) get() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.names
}

func (c *productNameCache) set(names map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.names = names
}

// productNames maps product id to name, or is empty when the catalog is not
// available; a failed read only costs the names. The returned map is shared
// and must not be modified.
func (h *Handler) productNames(ctx context.Context) map[string]string {
	if names := h.products.get(); names != nil {
		return names
	}
	catalog, ok := h.pov.(ProductCatalog)
	if !ok {
		return map[string]string{}
	}
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	products, err := catalog.Products(ctx)
	if err != nil {
		return map[string]string{}
	}
	names := make(map[string]string, len(products))
	for _, p := range products {
		names[p.ID] = p.Name
	}
	h.products.set(names)
	return names
}

// perfilReason is the card body of a purchase above the investor profile,
// for example "Compra de US$ 30.000,00 em Cobalto Semicondutores, risco 5.
// Perfil conservador vai até risco 2.". Amount is whole dollars, as on every
// alert card. Without a name it uses the product id, then "produto"; without
// a risk it leaves the risk out.
func perfilReason(sig Signal, name string) string {
	if name == "" {
		name = sig.ProductID
	}
	if name == "" {
		name = "produto"
	}
	cents := int64(math.Round(sig.Amount * 100))
	reason := fmt.Sprintf("Compra de %s em %s", screen.Money(cents), name)
	if sig.Risk > 0 {
		reason += fmt.Sprintf(", risco %d", sig.Risk)
	}
	reason += "."
	if sig.Profile != "" && sig.MaxRisk > 0 {
		reason += fmt.Sprintf(" Perfil %s vai até risco %d.", sig.Profile, sig.MaxRisk)
	}
	return reason
}

// enrichCases fills name and segment from the advisory book.
func (h *Handler) enrichCases(ctx context.Context, items []Case) {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.Client)
	}
	book := h.customerBook(ctx, ids)
	for i := range items {
		c, ok := book[items[i].Client]
		if !ok {
			continue
		}
		items[i].Name = c.Name
		items[i].Segment = c.Segment
	}
}

// enrichReviews fills name and segment from the advisory book.
func (h *Handler) enrichReviews(ctx context.Context, items []ReviewRow) {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.Client)
	}
	book := h.customerBook(ctx, ids)
	for i := range items {
		c, ok := book[items[i].Client]
		if !ok {
			continue
		}
		items[i].Name = c.Name
		items[i].Segment = c.Segment
	}
}
