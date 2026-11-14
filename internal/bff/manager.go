package bff

// ManagerBacklog is one advisor row on the manager panel.
type ManagerBacklog struct {
	Advisor string `json:"advisor"`
	Open    int    `json:"open"`
	Risk    int    `json:"risk"`
	Overdue int    `json:"overdue"`
}

// ManagerAtRisk is one case with SLA at risk or overdue.
type ManagerAtRisk struct {
	ID        string `json:"id"`
	Client    string `json:"client"`
	Advisor   string `json:"advisor"`
	Segment   string `json:"segment"`
	Remaining int    `json:"remaining"`
}

// ManagerSnapshot is the manager panel payload. ReviewPct is the share of
// triaged messages still awaiting human review and Intents maps each intent
// label to its share of the triaged messages, both in whole percent.
type ManagerSnapshot struct {
	Backlog                  []ManagerBacklog `json:"backlog"`
	AvgFirstContactMin       int              `json:"avgFirstContactMin"`
	AvgFirstContactYesterday int              `json:"avgFirstContactYesterday"`
	ReviewPct                int              `json:"reviewPct"`
	FallbackPct              int              `json:"fallbackPct"`
	Intents                  map[string]int   `json:"intents"`
	AtRisk                   []ManagerAtRisk  `json:"atRisk"`
}

// intentShares turns per-intent message counts into whole percentages of the
// total, rounded half up, so the panel shows shares instead of raw counts.
// Rounding may leave the sum a point off 100. Intents without messages are
// dropped.
func intentShares(counts map[string]int) map[string]int {
	total := 0
	for _, n := range counts {
		total += max(n, 0)
	}
	shares := make(map[string]int, len(counts))
	if total == 0 {
		return shares
	}
	for label, n := range counts {
		if n > 0 {
			shares[label] = (n*200 + total) / (total * 2)
		}
	}
	return shares
}
