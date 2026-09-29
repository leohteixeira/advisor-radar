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
		return 0, fmt.Errorf("advisory: unknown investor profile %q", profile)
	}
	return risk, nil
}

// InvestorProfile is the suitability profile of one book customer and the
// date it was assessed.
type InvestorProfile struct {
	Profile    string
	AssessedOn time.Time
}
