package screen

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Component types of the Perfil screen. Perfil also reuses advisor_card.
const (
	typeProfileHeader    = "profile_header"
	typeProfileScale     = "profile_scale"
	typeProfileFieldList = "profile_field_list"
	typePreferenceList   = "preference_list"
)

// copyProfileLevel keys the label and description of each investor profile
// level. It is not a component type: profile_scale reads it for every level.
const copyProfileLevel = "profile_level"

// profileLevels are the investor profiles from the most conservative to the
// boldest. Each is a profile_scale variant and one level of the scale.
var profileLevels = []string{"conservador", "moderado", "arrojado"}

// Contact channels account-sim stores for a POV client.
const (
	channelChat  = "chat"
	channelEmail = "email"
)

// assessedLayout formats the investor profile assessment date (12/03/2026).
const assessedLayout = "02/01/2006"

// Registration is the account-sim fictional registration data. Phone is
// already masked display text.
type Registration struct {
	Email         string
	Phone         string
	City          string
	AccountNumber string
}

// Preferences are the client's contact channel ("chat" or "email") and beta
// program flag, as account-sim stores them.
type Preferences struct {
	Channel string
	Beta    bool
}

// ProfileMaxRisk is one row of the advisory max-risk table.
type ProfileMaxRisk struct {
	Profile string
	MaxRisk int
}

// RegistrationSource reads one customer's registration from account-sim.
// Implementations must return when ctx is done.
type RegistrationSource interface {
	Registration(ctx context.Context, customerID string) (Registration, error)
}

// PreferenceSource reads one customer's preferences from account-sim.
// Implementations must return when ctx is done.
type PreferenceSource interface {
	Preferences(ctx context.Context, customerID string) (Preferences, error)
}

// ProfileHeader is the profile_header props. Account is absent when the
// registration is unavailable.
type ProfileHeader struct {
	Initials string `json:"initials"`
	Name     string `json:"name"`
	Subtitle string `json:"subtitle"`
	Account  string `json:"account,omitempty"`
}

// ProfileScale is the profile_scale props: every investor profile level,
// with the client's own marked current.
type ProfileScale struct {
	Title        string         `json:"title"`
	Subtitle     string         `json:"subtitle"`
	CurrentLabel string         `json:"current_label"`
	Levels       []ProfileLevel `json:"levels"`
	Footer       string         `json:"footer"`
}

// ProfileLevel is one investor profile of the scale. MaxRisk (1–5) drives
// the bars; Limit is its display text.
type ProfileLevel struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Limit       string `json:"limit"`
	MaxRisk     int    `json:"max_risk"`
	Current     bool   `json:"current"`
}

// ProfileFieldList is the profile_field_list props.
type ProfileFieldList struct {
	Title    string         `json:"title"`
	Fields   []ProfileField `json:"fields"`
	Footnote string         `json:"footnote"`
}

// ProfileField is one registration row.
type ProfileField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// PreferenceList is the preference_list props. The theme is local to the
// browser, so it carries only its label and hint.
type PreferenceList struct {
	Title   string            `json:"title"`
	Theme   PreferenceTheme   `json:"theme"`
	Channel PreferenceChannel `json:"channel"`
	Beta    PreferenceBeta    `json:"beta"`
}

// PreferenceTheme is the local theme control's copy.
type PreferenceTheme struct {
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// PreferenceChannel is the stored contact channel and its options.
type PreferenceChannel struct {
	Label   string             `json:"label"`
	Hint    string             `json:"hint"`
	Value   string             `json:"value"`
	Options []PreferenceOption `json:"options"`
}

// PreferenceOption is one contact channel choice.
type PreferenceOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// PreferenceBeta is the stored beta flag, with the hint for its state.
type PreferenceBeta struct {
	Label   string `json:"label"`
	Hint    string `json:"hint"`
	Enabled bool   `json:"enabled"`
}

// perfilVariants registers the Perfil variants and the sources each reads.
// The header reads the registration only for the account number, so it needs
// advisory alone. The scale needs the customer read as well as the profile:
// with advisory down, the Perfil policy omits it. The registration list needs
// advisory for the name, segment, and client-since rows, so it is omitted
// rather than shown half empty.
func perfilVariants() map[variantKey]registered {
	variants := map[variantKey]registered{
		{typeProfileHeader, "default"}:    {variant: profileHeader{}, needs: []Source{SourceAdvisory}},
		{typeProfileFieldList, "default"}: {variant: profileFields{}, needs: []Source{SourceRegistration, SourceAdvisory}},
		{typePreferenceList, "default"}:   {variant: preferenceList{}, needs: []Source{SourcePreferences}},
	}
	for _, level := range profileLevels {
		variants[variantKey{typ: typeProfileScale, name: level}] = registered{
			variant: profileScale{profile: level},
			needs:   []Source{SourceAdvisory, SourceProfile},
		}
	}
	return variants
}

// profileHeader is the client's initials, name, and client-since line, plus
// the account number when the registration answered.
type profileHeader struct{}

func (profileHeader) Matches(Snapshot) bool { return true }

func (profileHeader) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Customer.OK() {
		return Component{}, fmt.Errorf("screen: profile header needs the customer: %w", s.Customer.Err)
	}
	name := strings.TrimSpace(s.Customer.Value.Name)
	if name == "" {
		return Component{}, errors.New("screen: customer has no name")
	}
	cp := copier{cat: c, typ: typeProfileHeader, variant: "default", fields: customerFields(s)}
	props := ProfileHeader{Initials: Initials(name), Name: name, Subtitle: cp.text("subtitle")}
	if s.Registration.OK() {
		props.Account = s.Registration.Value.AccountNumber
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeProfileHeader, Variant: "default", Props: props}, nil
}

// profileScale shows the three investor profile levels with the client's own
// marked. It matches only the client's profile; an unknown profile matches no
// variant, and the section is dropped as a build error.
type profileScale struct {
	profile string
}

func (v profileScale) Matches(s Snapshot) bool {
	profile, _, ok := investorProfile(s)
	return ok && profile == v.profile
}

// Build takes the client's own max risk from its profile and every other
// level's from the advisory max-risk table, so the BFF holds no table.
func (v profileScale) Build(s Snapshot, c Catalog) (Component, error) {
	profile, limit, ok := investorProfile(s)
	if !ok {
		return Component{}, errors.New("screen: suitability needs the investor profile")
	}
	assessed := s.Profile.Value.AssessedOn
	if assessed.IsZero() {
		return Component{}, errors.New("screen: investor profile has no assessment date")
	}
	table := make(map[string]int, len(s.Profile.Value.MaxRiskTable))
	for _, row := range s.Profile.Value.MaxRiskTable {
		table[strings.ToLower(row.Profile)] = row.MaxRisk
	}
	table[profile] = limit

	cp := copier{cat: c, typ: typeProfileScale, variant: v.profile, fields: Fields{AssessedOn: assessed.Format(assessedLayout)}}
	levels := make([]ProfileLevel, 0, len(profileLevels))
	for _, key := range profileLevels {
		risk, ok := table[key]
		if !ok || risk < minRisk || risk > maxRisk {
			return Component{}, fmt.Errorf("screen: max-risk table has no valid row for %q", key)
		}
		level := copier{cat: c, typ: copyProfileLevel, variant: key}
		cp.fields.MaxRisk = strconv.Itoa(risk)
		levels = append(levels, ProfileLevel{
			Key:         key,
			Label:       level.text("label"),
			Description: level.text("description"),
			Limit:       cp.text("limit"),
			MaxRisk:     risk,
			Current:     key == profile,
		})
		if level.err != nil {
			return Component{}, level.err
		}
	}
	props := ProfileScale{
		Title:        cp.text("title"),
		Subtitle:     cp.text("subtitle"),
		CurrentLabel: cp.text("current_label"),
		Levels:       levels,
		Footer:       cp.text("footer"),
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeProfileScale, Variant: v.profile, Props: props}, nil
}

// profileFields is the registration data: the name, segment, and
// client-since from the advisory book, the rest from account-sim.
type profileFields struct{}

func (profileFields) Matches(Snapshot) bool { return true }

func (profileFields) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Registration.OK() {
		return Component{}, fmt.Errorf("screen: registration needs account-sim: %w", s.Registration.Err)
	}
	if !s.Customer.OK() {
		return Component{}, fmt.Errorf("screen: registration needs the customer: %w", s.Customer.Err)
	}
	reg, customer := s.Registration.Value, s.Customer.Value
	cp := copier{cat: c, typ: typeProfileFieldList, variant: "default"}
	rows := []struct{ key, value string }{
		{"name", customer.Name},
		{"email", reg.Email},
		{"phone", reg.Phone},
		{"city", reg.City},
		{"segment", customer.Segment},
		{"since", customer.Since},
	}
	fields := make([]ProfileField, 0, len(rows))
	for _, row := range rows {
		// The book may not know when the client joined; the row is left out
		// rather than shown empty.
		if row.key == "since" && row.value == "" {
			continue
		}
		fields = append(fields, ProfileField{Label: cp.text(row.key), Value: row.value})
	}
	props := ProfileFieldList{Title: cp.text("title"), Fields: fields, Footnote: cp.text("footnote")}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typeProfileFieldList, Variant: "default", Props: props}, nil
}

// preferenceList is the local theme control and the stored contact channel
// and beta flag. A channel account-sim should never store is a build error.
type preferenceList struct{}

func (preferenceList) Matches(Snapshot) bool { return true }

func (preferenceList) Build(s Snapshot, c Catalog) (Component, error) {
	if !s.Preferences.OK() {
		return Component{}, fmt.Errorf("screen: preferences need account-sim: %w", s.Preferences.Err)
	}
	prefs := s.Preferences.Value
	if prefs.Channel != channelChat && prefs.Channel != channelEmail {
		return Component{}, fmt.Errorf("screen: unknown contact channel %q", prefs.Channel)
	}
	cp := copier{cat: c, typ: typePreferenceList, variant: "default"}
	betaHint := "beta_hint_off"
	if prefs.Beta {
		betaHint = "beta_hint_on"
	}
	props := PreferenceList{
		Title: cp.text("title"),
		Theme: PreferenceTheme{Label: cp.text("theme_label"), Hint: cp.text("theme_hint")},
		Channel: PreferenceChannel{
			Label: cp.text("channel_label"),
			Hint:  cp.text("channel_hint"),
			Value: prefs.Channel,
			Options: []PreferenceOption{
				{Value: channelChat, Label: cp.text("channel_chat")},
				{Value: channelEmail, Label: cp.text("channel_email")},
			},
		},
		Beta: PreferenceBeta{Label: cp.text("beta_label"), Hint: cp.text(betaHint), Enabled: prefs.Beta},
	}
	if cp.err != nil {
		return Component{}, cp.err
	}
	return Component{Type: typePreferenceList, Variant: "default", Props: props}, nil
}
