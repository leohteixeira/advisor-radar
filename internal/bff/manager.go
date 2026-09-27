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

// ManagerSnapshot is the manager panel payload.
type ManagerSnapshot struct {
	Backlog                  []ManagerBacklog `json:"backlog"`
	AvgFirstContactMin       int              `json:"avgFirstContactMin"`
	AvgFirstContactYesterday int              `json:"avgFirstContactYesterday"`
	ReviewPct                int              `json:"reviewPct"`
	FallbackPct              int              `json:"fallbackPct"`
	Intents                  map[string]int   `json:"intents"`
	AtRisk                   []ManagerAtRisk  `json:"atRisk"`
}
