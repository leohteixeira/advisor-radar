// Package screen composes the client app's Server-Driven UI screens (ADR
// 0009). The engine fetches a Snapshot of the customer from its sources under
// one screen deadline, picks one variant per section from the embedded
// catalog, and fills Portuguese copy templates with values Go has already
// formatted. The JSON shape is the screen envelope in specs/http/bff.md.
package screen

// SchemaVersion is the version of the envelope and of the component table in
// specs/http/bff.md.
const SchemaVersion = 1

// Tones a component may carry. Web maps a tone to a color.
const (
	TonePos     = "pos"
	ToneNeg     = "neg"
	ToneInfo    = "info"
	ToneGold    = "gold"
	ToneNeutral = "neutral"
)

// Action types and the panel targets web knows how to open.
const (
	ActionNavigate = "navigate"
	ActionPanel    = "panel"
	ActionNote     = "note"
	ActionLink     = "link"

	PanelDeposit   = "deposit"
	PanelWithdraw  = "withdraw"
	PanelMessage   = "message"
	PanelComplaint = "complaint"
	// PanelPurchase opens the coded purchase form for Action.ProductID.
	PanelPurchase = "purchase"

	// ScreenInvestir is the navigate target of the Investir screen.
	ScreenInvestir = "investir"
)

// ReasonBuildError is the omitted reason for a section whose variant failed to
// build. A section omitted for a failed source carries that source's name.
const ReasonBuildError = "build_error"

// Page is the screen envelope. Omitted lists the sections left out of this
// response, so the Raio-X view can draw them; it is always present.
type Page struct {
	SchemaVersion int       `json:"schema_version"`
	Slug          string    `json:"slug"`
	Revision      string    `json:"revision"`
	Title         string    `json:"title"`
	Subtitle      string    `json:"subtitle,omitempty"`
	Sections      []Section `json:"sections"`
	Omitted       []Omitted `json:"omitted"`
}

// Section is one rendered section. The order of Page.Sections is the render
// order.
type Section struct {
	ID         string      `json:"id"`
	Components []Component `json:"components"`
}

// Component is one semantic domain component. Props is one of the props
// types below, matching Type.
type Component struct {
	Type    string `json:"type"`
	Variant string `json:"variant"`
	Props   any    `json:"props"`
}

// Omitted names a section of the screen's catalog revision that this response
// left out, and why: a failed source name or ReasonBuildError.
type Omitted struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// Action is one control. Target is the slug for navigate and the panel for
// panel; ProductID, Text, and Href belong to purchase panels, notes, and links.
type Action struct {
	Type      string `json:"type"`
	Label     string `json:"label"`
	Target    string `json:"target,omitempty"`
	ProductID string `json:"product_id,omitempty"`
	Text      string `json:"text,omitempty"`
	Href      string `json:"href,omitempty"`
}

// MomentCard is the moment_card props.
type MomentCard struct {
	Kicker string  `json:"kicker"`
	Title  string  `json:"title"`
	Body   string  `json:"body"`
	Meta   string  `json:"meta,omitempty"`
	Tone   string  `json:"tone"`
	Icon   string  `json:"icon,omitempty"`
	Action *Action `json:"action,omitempty"`
}

// WealthSummary is the wealth_summary props. CashCents is the cash a form may
// need as a number.
type WealthSummary struct {
	TotalLabel string          `json:"total_label"`
	Total      string          `json:"total"`
	CashLabel  string          `json:"cash_label"`
	Cash       string          `json:"cash"`
	CashCents  int64           `json:"cash_cents"`
	Allocation []AllocationRow `json:"allocation"`
}

// AllocationRow is one asset class of an allocation. BarWidth is 0–100.
type AllocationRow struct {
	Class    string `json:"class"`
	Label    string `json:"label"`
	Share    string `json:"share"`
	BarWidth int    `json:"bar_width"`
}

// ActionGrid is the action_grid props.
type ActionGrid struct {
	Items []GridItem `json:"items"`
}

// GridItem is one action_grid control.
type GridItem struct {
	Label  string `json:"label"`
	Icon   string `json:"icon,omitempty"`
	Action Action `json:"action"`
}

// AdvisorCard is the advisor_card props.
type AdvisorCard struct {
	Kicker   string  `json:"kicker"`
	Name     string  `json:"name"`
	Initials string  `json:"initials"`
	Meta     string  `json:"meta"`
	Action   *Action `json:"action,omitempty"`
}

// ActivityList is the activity_list props. Items is empty, never null, for
// the empty variant.
type ActivityList struct {
	Title     string         `json:"title"`
	Items     []ActivityItem `json:"items"`
	EmptyText string         `json:"empty_text,omitempty"`
}

// ActivityItem is one activity row.
type ActivityItem struct {
	Icon  string `json:"icon,omitempty"`
	Title string `json:"title"`
	Meta  string `json:"meta"`
	Value string `json:"value,omitempty"`
	Tone  string `json:"tone,omitempty"`
}

// InvestSummary is the invest_summary props. CashCents is the cash the
// purchase form caps its amount at. ProfileChip is absent when the investor
// profile is unavailable.
type InvestSummary struct {
	CashLabel   string `json:"cash_label"`
	Cash        string `json:"cash"`
	CashCents   int64  `json:"cash_cents"`
	ProfileChip string `json:"profile_chip,omitempty"`
}

// ProductRail is the product_rail props: the products picked for the
// customer's investor profile, in pick order.
type ProductRail struct {
	Title    string        `json:"title"`
	Subtitle string        `json:"subtitle"`
	Products []ProductItem `json:"products"`
}

// ProductList is the product_list props: every catalog product of one asset
// class, by risk and then id.
type ProductList struct {
	Title    string        `json:"title"`
	Products []ProductItem `json:"products"`
}

// ProductItem is one product as the Investir screen and the purchase form
// show it. Risk is 1–5 for the bars. Badge and Warning are set only when
// AboveProfile is true. Action is a purchase panel for ProductID.
type ProductItem struct {
	ProductID    string `json:"product_id"`
	Name         string `json:"name"`
	ClassLabel   string `json:"class_label"`
	Risk         int    `json:"risk"`
	RiskLabel    string `json:"risk_label"`
	ReturnLabel  string `json:"return_label"`
	Minimum      string `json:"minimum"`
	MinimumCents int64  `json:"minimum_cents"`
	AboveProfile bool   `json:"above_profile"`
	Badge        string `json:"badge,omitempty"`
	Warning      string `json:"warning,omitempty"`
	Action       Action `json:"action"`
}
