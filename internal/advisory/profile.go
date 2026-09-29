package advisory

import (
	"errors"
	"fmt"
	"time"
)

// Investor profiles stored in book.investor_profile.
const (
	ProfileConservador = "conservador"
	ProfileModerado    = "moderado"
	ProfileArrojado    = "arrojado"
)

// ErrUnknownCustomer is returned by the book reads when the customer is not
// in the book.
var ErrUnknownCustomer = errors.New("advisory: unknown customer")

// ErrUnknownProfile is returned by MaxRisk for a profile outside the table.
var ErrUnknownProfile = errors.New("advisory: unknown investor profile")

// maxRisk is the highest product risk (1–5) each investor profile accepts. It
// is the only place this mapping lives: the BFF badge, the purchase warning,
// and the suitability rule all read it through advisory.
var maxRisk = map[string]int{
	ProfileConservador: 2,
	ProfileModerado:    3,
	ProfileArrojado:    5,
}

// MaxRisk returns the highest product risk the profile accepts. An unknown
// profile is an error, never a default.
func MaxRisk(profile string) (int, error) {
	risk, ok := maxRisk[profile]
	if !ok {
		return 0, fmt.Errorf("%w %q", ErrUnknownProfile, profile)
	}
	return risk, nil
}

// ProfileMaxRisk is one row of the max-risk table.
type ProfileMaxRisk struct {
	Profile string
	MaxRisk int
}

// MaxRiskTable returns the whole max-risk table, from the most conservative
// profile to the boldest, so a client can show every level next to its own.
func MaxRiskTable() []ProfileMaxRisk {
	profiles := []string{ProfileConservador, ProfileModerado, ProfileArrojado}
	table := make([]ProfileMaxRisk, 0, len(profiles))
	for _, profile := range profiles {
		table = append(table, ProfileMaxRisk{Profile: profile, MaxRisk: maxRisk[profile]})
	}
	return table
}

// InvestorProfile is the suitability profile of one book customer and the
// date it was assessed.
type InvestorProfile struct {
	Profile    string
	AssessedOn time.Time
}
