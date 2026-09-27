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

// ManagerSnapshot is the demo-day manager panel payload from MANAGER in mock-data.js.
type ManagerSnapshot struct {
	Backlog                  []ManagerBacklog `json:"backlog"`
	AvgFirstContactMin       int              `json:"avgFirstContactMin"`
	AvgFirstContactYesterday int              `json:"avgFirstContactYesterday"`
	ReviewPct                int              `json:"reviewPct"`
	FallbackPct              int              `json:"fallbackPct"`
	Intents                  map[string]int   `json:"intents"`
	AtRisk                   []ManagerAtRisk  `json:"atRisk"`
}

// ManagerSnapshotSeed returns the fixed demo-day manager numbers.
func ManagerSnapshotSeed() ManagerSnapshot {
	return ManagerSnapshot{
		Backlog: []ManagerBacklog{
			{Advisor: "Ana Paula Ribeiro", Open: 17, Risk: 4, Overdue: 1},
			{Advisor: "Bruno Dias", Open: 9, Risk: 1, Overdue: 0},
			{Advisor: "Carla Menezes", Open: 13, Risk: 2, Overdue: 2},
			{Advisor: "Diego Rocha", Open: 6, Risk: 0, Overdue: 0},
		},
		AvgFirstContactMin:       14,
		AvgFirstContactYesterday: 19,
		ReviewPct:                18,
		FallbackPct:              6,
		Intents: map[string]int{
			"Operacional":  31,
			"Tributação":   22,
			"Reclamação":   14,
			"Câmbio":       11,
			"Investimento": 9,
			"Contato":      7,
			"Resgate":      4,
			"Encerramento": 2,
		},
		AtRisk: []ManagerAtRisk{
			{ID: "k1042", Client: "Paulo Henrique Souza", Advisor: "Ana Paula Ribeiro", Segment: "Advance", Remaining: 46},
			{ID: "k1051", Client: "Renata Albuquerque", Advisor: "Carla Menezes", Segment: "Advance", Remaining: 12},
			{ID: "k1049", Client: "Otávio Freitas", Advisor: "Bruno Dias", Segment: "Essencial", Remaining: 95},
			{ID: "k1044", Client: "Sérgio Cardoso", Advisor: "Ana Paula Ribeiro", Segment: "Singular", Remaining: -8},
			{ID: "k1040", Client: "Marina Duarte", Advisor: "Carla Menezes", Segment: "Singular", Remaining: -35},
		},
	}
}
