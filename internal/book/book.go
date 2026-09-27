// Package book holds the fictional advisor book and segment bounds.
package book

// Segment bounds in USD. Essencial is inclusive at the upper bound;
// Advance is exclusive below and inclusive at the Singular bound.
const (
	BoundEssencialMax = 10_000
	BoundAdvanceMax   = 200_000
)

// Segment names match the design seed.
const (
	SegmentEssencial = "Essencial"
	SegmentAdvance   = "Advance"
	SegmentSingular  = "Singular"
)

// Client is one row of the seed book.
type Client struct {
	ID      string
	Name    string
	Segment string
	AUM     float64
	Advisor string
	Since   string
}

// Clients is the 22-client seed book keyed by customer id.
var Clients = map[string]Client{
	"c01": {ID: "c01", Name: "Mariana Costa", Segment: SegmentSingular, AUM: 248300, Advisor: "Ana Paula Ribeiro", Since: "2021"},
	"c02": {ID: "c02", Name: "Paulo Henrique Souza", Segment: SegmentAdvance, AUM: 96400, Advisor: "Ana Paula Ribeiro", Since: "2022"},
	"c03": {ID: "c03", Name: "Fernanda Lima", Segment: SegmentEssencial, AUM: 8200, Advisor: "Ana Paula Ribeiro", Since: "2024"},
	"c04": {ID: "c04", Name: "Carlos Eduardo Ramos", Segment: SegmentAdvance, AUM: 142000, Advisor: "Ana Paula Ribeiro", Since: "2020"},
	"c05": {ID: "c05", Name: "Juliana Martins", Segment: SegmentSingular, AUM: 512900, Advisor: "Ana Paula Ribeiro", Since: "2019"},
	"c06": {ID: "c06", Name: "Roberto Nascimento", Segment: SegmentEssencial, AUM: 7800, Advisor: "Ana Paula Ribeiro", Since: "2025"},
	"c07": {ID: "c07", Name: "Ana Beatriz Oliveira", Segment: SegmentAdvance, AUM: 190500, Advisor: "Ana Paula Ribeiro", Since: "2021"},
	"c08": {ID: "c08", Name: "Lucas Pereira", Segment: SegmentEssencial, AUM: 6100, Advisor: "Ana Paula Ribeiro", Since: "2023"},
	"c09": {ID: "c09", Name: "Patrícia Gomes", Segment: SegmentSingular, AUM: 780000, Advisor: "Ana Paula Ribeiro", Since: "2018"},
	"c10": {ID: "c10", Name: "Marcelo Ferreira", Segment: SegmentAdvance, AUM: 88000, Advisor: "Ana Paula Ribeiro", Since: "2022"},
	"c11": {ID: "c11", Name: "Sérgio Cardoso", Segment: SegmentSingular, AUM: 450000, Advisor: "Ana Paula Ribeiro", Since: "2019"},
	"c12": {ID: "c12", Name: "Helena Barbosa", Segment: SegmentAdvance, AUM: 108000, Advisor: "Ana Paula Ribeiro", Since: "2023"},
	"c13": {ID: "c13", Name: "Thiago Azevedo", Segment: SegmentAdvance, AUM: 68000, Advisor: "Ana Paula Ribeiro", Since: "2024"},
	"c14": {ID: "c14", Name: "Camila Rodrigues", Segment: SegmentAdvance, AUM: 175000, Advisor: "Ana Paula Ribeiro", Since: "2020"},
	"c15": {ID: "c15", Name: "Rafael Monteiro", Segment: SegmentSingular, AUM: 950000, Advisor: "Ana Paula Ribeiro", Since: "2017"},
	"c16": {ID: "c16", Name: "Beatriz Santana", Segment: SegmentEssencial, AUM: 7500, Advisor: "Ana Paula Ribeiro", Since: "2025"},
	"c17": {ID: "c17", Name: "Diego Carvalho", Segment: SegmentEssencial, AUM: 8900, Advisor: "Ana Paula Ribeiro", Since: "2024"},
	"c18": {ID: "c18", Name: "Vanessa Moreira", Segment: SegmentAdvance, AUM: 119000, Advisor: "Ana Paula Ribeiro", Since: "2021"},
	"c19": {ID: "c19", Name: "Gustavo Teixeira", Segment: SegmentAdvance, AUM: 141000, Advisor: "Ana Paula Ribeiro", Since: "2022"},
	"c20": {ID: "c20", Name: "Isabela Nunes", Segment: SegmentEssencial, AUM: 9400, Advisor: "Ana Paula Ribeiro", Since: "2025"},
	"c21": {ID: "c21", Name: "Otávio Freitas", Segment: SegmentEssencial, AUM: 5800, Advisor: "Bruno Dias", Since: "2024"},
	"c22": {ID: "c22", Name: "Renata Albuquerque", Segment: SegmentAdvance, AUM: 67000, Advisor: "Carla Menezes", Since: "2023"},
}

// SegmentFromAssets maps assets in USD to Essencial, Advance, or Singular.
// Essencial is up to BoundEssencialMax inclusive; Advance is above that through
// BoundAdvanceMax inclusive; Singular is above BoundAdvanceMax.
func SegmentFromAssets(assets float64) string {
	switch {
	case assets <= BoundEssencialMax:
		return SegmentEssencial
	case assets <= BoundAdvanceMax:
		return SegmentAdvance
	default:
		return SegmentSingular
	}
}
