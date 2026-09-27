package bff

import "context"

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
