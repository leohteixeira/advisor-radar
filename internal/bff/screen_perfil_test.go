package bff_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// maxRiskTable is the advisory max-risk table as GetInvestorProfile sends it.
func maxRiskTable() []bff.ProfileMaxRisk {
	return []bff.ProfileMaxRisk{
		{Profile: "conservador", MaxRisk: 2},
		{Profile: "moderado", MaxRisk: 3},
		{Profile: "arrojado", MaxRisk: 5},
	}
}

type headerProps struct {
	Initials string `json:"initials"`
	Name     string `json:"name"`
	Subtitle string `json:"subtitle"`
	Account  string `json:"account"`
}

type scaleProps struct {
	Title        string `json:"title"`
	Subtitle     string `json:"subtitle"`
	CurrentLabel string `json:"current_label"`
	Levels       []struct {
		Key         string `json:"key"`
		Label       string `json:"label"`
		Description string `json:"description"`
		Limit       string `json:"limit"`
		MaxRisk     int    `json:"max_risk"`
		Current     bool   `json:"current"`
	} `json:"levels"`
	Footer string `json:"footer"`
}

type fieldsProps struct {
	Title  string `json:"title"`
	Fields []struct {
		Label string `json:"label"`
		Value string `json:"value"`
	} `json:"fields"`
	Footnote string `json:"footnote"`
}

type preferencesProps struct {
	Title string `json:"title"`
	Theme struct {
		Label string `json:"label"`
		Hint  string `json:"hint"`
	} `json:"theme"`
	Channel struct {
		Label   string `json:"label"`
		Hint    string `json:"hint"`
		Value   string `json:"value"`
		Options []struct {
			Value string `json:"value"`
			Label string `json:"label"`
		} `json:"options"`
	} `json:"channel"`
	Beta struct {
		Label   string `json:"label"`
		Hint    string `json:"hint"`
		Enabled bool   `json:"enabled"`
	} `json:"beta"`
}

// startPerfil serves the real account-sim server over a fresh seeded memory
// store and returns the BFF wired to it the production way, plus the store so
// a test can read its outbox. wrap, when set, replaces the POV source.
func startPerfil(t *testing.T, queue bff.QueueSource, wrap func(bff.POVSource) bff.POVSource) (*bff.Server, *sim.Memory) {
	t.Helper()
	memory := sim.NewMemory()
	pov := dialPOV(t, sim.NewGRPCServer(memory, nil))
	if wrap != nil {
		pov = wrap(pov)
	}
	return bff.NewHandlerWithPOV(bff.NewBoard(), nil, bff.EmptyTimeline{}, queue, nil, nil, pov, nil), memory
}

func TestGetScreen_PerfilSeedClients(t *testing.T) {
	t.Parallel()
	h, _ := startPerfil(t, screenBook(), nil)
	tests := []struct {
		name        string
		id          string
		header      headerProps
		suitability string
		footer      string
		email       string
		advisor     string
	}{
		{
			name:        "fernanda",
			id:          sim.CustomerFernanda,
			header:      headerProps{Initials: "FL", Name: "Fernanda Lima", Subtitle: "Cliente Essencial desde 2024", Account: "Conta 3301-7 · Orla Invest"},
			suitability: "conservador",
			footer:      "Última avaliação em 12/03/2026. Para refazer o questionário, fale com a sua assessora.",
			email:       "fernanda.lima@example.com",
			advisor:     "default",
		},
		{
			name:        "thiago",
			id:          sim.CustomerThiago,
			header:      headerProps{Initials: "TA", Name: "Thiago Azevedo", Subtitle: "Cliente Advance desde 2024", Account: "Conta 2847-1 · Orla Invest"},
			suitability: "arrojado",
			footer:      "Última avaliação em 04/08/2026. Para refazer o questionário, fale com a sua assessora.",
			email:       "thiago.azevedo@example.com",
			advisor:     "default",
		},
		{
			name:        "mariana",
			id:          sim.CustomerMariana,
			header:      headerProps{Initials: "MC", Name: "Mariana Costa", Subtitle: "Cliente Singular desde 2021", Account: "Conta 1190-4 · Orla Invest"},
			suitability: "moderado",
			footer:      "Última avaliação em 20/01/2026. Para refazer o questionário, fale com a sua assessora.",
			email:       "mariana.costa@example.com",
			advisor:     "dedicated",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rr, body := getScreen(t, h, tt.id, "perfil")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d %s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control = %q", rr.Header().Get("Cache-Control"))
			}
			if body.Slug != "perfil" || body.Revision != "v1" || body.Title != "Perfil" ||
				body.Subtitle != "Seus dados, seu perfil de investidor e suas preferências" || len(body.Omitted) != 0 {
				t.Errorf("envelope = %s %s %q %q omitted %+v", body.Slug, body.Revision, body.Title, body.Subtitle, body.Omitted)
			}
			if want := []string{"header", "suitability", "registration", "preferences", "advisor"}; !slices.Equal(body.ids(), want) {
				t.Fatalf("sections = %v, want %v", body.ids(), want)
			}

			kind, raw := body.component(t, "header")
			var header headerProps
			decodeProps(t, raw, &header)
			if kind != "profile_header/default" || header != tt.header {
				t.Errorf("header = %s %+v, want %+v", kind, header, tt.header)
			}

			kind, raw = body.component(t, "suitability")
			var scale scaleProps
			decodeProps(t, raw, &scale)
			if kind != "profile_scale/"+tt.suitability || scale.Footer != tt.footer || scale.Title != "Seu perfil de investidor" {
				t.Errorf("suitability = %s %+v", kind, scale)
			}
			var risks []int
			var current []string
			for _, level := range scale.Levels {
				risks = append(risks, level.MaxRisk)
				if level.Current {
					current = append(current, level.Key)
				}
			}
			if !slices.Equal(risks, []int{2, 3, 5}) || !slices.Equal(current, []string{tt.suitability}) {
				t.Errorf("levels risks %v, current %v", risks, current)
			}

			kind, raw = body.component(t, "registration")
			var fields fieldsProps
			decodeProps(t, raw, &fields)
			if kind != "profile_field_list/default" || len(fields.Fields) != 6 || fields.Fields[1].Label != "E-mail" || fields.Fields[1].Value != tt.email {
				t.Errorf("registration = %s %+v", kind, fields)
			}

			kind, raw = body.component(t, "preferences")
			var prefs preferencesProps
			decodeProps(t, raw, &prefs)
			if kind != "preference_list/default" || prefs.Channel.Value != "chat" || prefs.Beta.Enabled ||
				prefs.Beta.Hint != "Veja antes as novas versões das telas." || len(prefs.Channel.Options) != 2 {
				t.Errorf("preferences = %s %+v", kind, prefs)
			}

			kind, raw = body.component(t, "advisor")
			var advisor advisorProps
			decodeProps(t, raw, &advisor)
			if kind != "advisor_card/"+tt.advisor || advisor.Name != "Ana Paula Ribeiro" {
				t.Errorf("advisor = %s %+v", kind, advisor)
			}
		})
	}
}

// downPOV fails the chosen account-sim Perfil reads and forwards the rest.
type downPOV struct {
	bff.POVSource
	registration bool
	preferences  bool
}

func (p downPOV) Registration(ctx context.Context, id string) (bff.POVRegistration, error) {
	if p.registration {
		return bff.POVRegistration{}, errAccountSimDown
	}
	return p.POVSource.Registration(ctx, id)
}

func (p downPOV) Preferences(ctx context.Context, id string) (bff.POVPreferences, error) {
	if p.preferences {
		return bff.POVPreferences{}, errAccountSimDown
	}
	return p.POVSource.Preferences(ctx, id)
}

// noCustomerQueue is the seed book with the advisory customer read down.
type noCustomerQueue struct {
	stubQueue
}

func (noCustomerQueue) GetCustomer(context.Context, string) (bff.Customer, error) {
	return bff.Customer{}, errors.New("advisory unavailable")
}

func TestGetScreen_PerfilFailurePolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		queue    bff.QueueSource
		wrap     func(bff.POVSource) bff.POVSource
		sections []string
		omitted  []string
		account  bool
	}{
		{
			name:     "advisory down",
			queue:    noCustomerQueue{stubQueue: screenBook()},
			sections: []string{"preferences"},
			omitted:  []string{"header:advisory", "suitability:advisory", "registration:advisory", "advisor:advisory"},
		},
		{
			name:     "registration down",
			queue:    screenBook(),
			wrap:     func(p bff.POVSource) bff.POVSource { return downPOV{POVSource: p, registration: true} },
			sections: []string{"header", "suitability", "preferences", "advisor"},
			omitted:  []string{"registration:registration"},
		},
		{
			name:     "preferences down",
			queue:    screenBook(),
			wrap:     func(p bff.POVSource) bff.POVSource { return downPOV{POVSource: p, preferences: true} },
			sections: []string{"header", "suitability", "registration", "advisor"},
			omitted:  []string{"preferences:preferences"},
			account:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, _ := startPerfil(t, tt.queue, tt.wrap)
			rr, body := getScreen(t, h, sim.CustomerFernanda, "perfil")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d %s", rr.Code, rr.Body.String())
			}
			if !slices.Equal(body.ids(), tt.sections) {
				t.Errorf("sections = %v, want %v", body.ids(), tt.sections)
			}
			var omitted []string
			for _, o := range body.Omitted {
				omitted = append(omitted, o.ID+":"+o.Reason)
			}
			if !slices.Equal(omitted, tt.omitted) {
				t.Errorf("omitted = %v, want %v", omitted, tt.omitted)
			}
			if slices.Contains(tt.sections, "header") {
				_, raw := body.component(t, "header")
				var header headerProps
				decodeProps(t, raw, &header)
				if (header.Account != "") != tt.account {
					t.Errorf("header account = %q, want present %t", header.Account, tt.account)
				}
			}
		})
	}
}

// perfilPreferences reads the preference_list props of one customer's Perfil.
func perfilPreferences(t *testing.T, h http.Handler, customerID string) preferencesProps {
	t.Helper()
	rr, body := getScreen(t, h, customerID, "perfil")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET perfil = %d %s", rr.Code, rr.Body.String())
	}
	_, raw := body.component(t, "preferences")
	var prefs preferencesProps
	decodeProps(t, raw, &prefs)
	return prefs
}

// TestPutPOVPreferences_RoundTrip writes through the gRPC adapter into the real
// account-sim store: Perfil shows the stored value, no event is written, and
// a reseed restores chat with beta off.
func TestPutPOVPreferences_RoundTrip(t *testing.T) {
	t.Parallel()
	h, memory := startPerfil(t, screenBook(), nil)
	outbox := len(memory.PendingOutbox())

	rr := putPreferences(t, h, sim.CustomerFernanda, `{"channel":"email","beta":true}`)
	if rr.Code != http.StatusOK || strings.TrimSpace(rr.Body.String()) != `{"channel":"email","beta":true}` {
		t.Fatalf("PUT = %d %s", rr.Code, rr.Body.String())
	}
	got := perfilPreferences(t, h, sim.CustomerFernanda)
	if got.Channel.Value != "email" || !got.Beta.Enabled ||
		got.Beta.Hint != "Ligado. Você recebe a revision v2 do início antes dos outros clientes." {
		t.Errorf("after PUT = %+v", got)
	}
	if other := perfilPreferences(t, h, sim.CustomerThiago); other.Channel.Value != "chat" || other.Beta.Enabled {
		t.Errorf("Thiago changed: %+v", other)
	}
	if n := len(memory.PendingOutbox()); n != outbox {
		t.Errorf("outbox rows = %d, want %d: preferences publish no event", n, outbox)
	}

	// account-sim answers NotFound for a well-formed id it does not hold.
	if rr := putPreferences(t, h, "01a0e3a4-9a44-757a-ac8f-000000000000", `{"channel":"email","beta":true}`); rr.Code != http.StatusNotFound {
		t.Errorf("unknown customer PUT = %d %s, want 404", rr.Code, rr.Body.String())
	}

	if err := sim.Reseed(t.Context(), memory); err != nil {
		t.Fatalf("Reseed: %v", err)
	}
	if got := perfilPreferences(t, h, sim.CustomerFernanda); got.Channel.Value != "chat" || got.Beta.Enabled {
		t.Errorf("after reseed = %+v", got)
	}
}
