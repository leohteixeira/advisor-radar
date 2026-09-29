package bff_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// putPreferences sends PUT …/preferences for customerID with body.
func putPreferences(t *testing.T, h http.Handler, customerID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		"/v1/client-pov/customers/"+customerID+"/preferences", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// preferencesHandler serves a fakePOV holding the three seed accounts.
func preferencesHandler(src *fakePOV) *bff.Server {
	src.accounts = []bff.POVAccount{
		{CustomerID: sim.CustomerFernanda},
		{CustomerID: sim.CustomerThiago},
		{CustomerID: sim.CustomerMariana},
	}
	return bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, nil, nil, nil, src, func() time.Time {
		return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	})
}

func TestPutPOVPreferences(t *testing.T) {
	t.Parallel()
	const unknown = "01a0e3a4-9a44-757a-ac8f-000000000000"
	tests := []struct {
		name        string
		id          string
		body        string
		src         *fakePOV
		wantCode    int
		wantBody    string
		wantUpdates int
	}{
		{name: "change channel", id: sim.CustomerFernanda, body: `{"channel":"email","beta":false}`,
			src: &fakePOV{}, wantCode: http.StatusOK, wantBody: `{"channel":"email","beta":false}`, wantUpdates: 1},
		{name: "beta on", id: sim.CustomerThiago, body: `{"channel":"chat","beta":true}`,
			src: &fakePOV{}, wantCode: http.StatusOK, wantBody: `{"channel":"chat","beta":true}`, wantUpdates: 1},
		{name: "unchanged is a no-op", id: sim.CustomerMariana, body: `{"channel":"chat","beta":false}`,
			src: &fakePOV{}, wantCode: http.StatusOK, wantBody: `{"channel":"chat","beta":false}`},
		{name: "unknown channel", id: sim.CustomerFernanda, body: `{"channel":"sms","beta":false}`,
			src: &fakePOV{}, wantCode: http.StatusUnprocessableEntity, wantBody: `{"error":"invalid"}`},
		{name: "missing beta", id: sim.CustomerFernanda, body: `{"channel":"email"}`,
			src: &fakePOV{}, wantCode: http.StatusUnprocessableEntity, wantBody: `{"error":"invalid"}`},
		{name: "missing channel", id: sim.CustomerFernanda, body: `{"beta":true}`,
			src: &fakePOV{}, wantCode: http.StatusUnprocessableEntity, wantBody: `{"error":"invalid"}`},
		{name: "wrong type", id: sim.CustomerFernanda, body: `{"channel":"email","beta":"yes"}`,
			src: &fakePOV{}, wantCode: http.StatusUnprocessableEntity, wantBody: `{"error":"invalid"}`},
		{name: "not json", id: sim.CustomerFernanda, body: `channel=email`,
			src: &fakePOV{}, wantCode: http.StatusUnprocessableEntity, wantBody: `{"error":"invalid"}`},
		{name: "empty body", id: sim.CustomerFernanda, body: ``,
			src: &fakePOV{}, wantCode: http.StatusUnprocessableEntity, wantBody: `{"error":"invalid"}`},
		{name: "bad id", id: "not-a-uuid", body: `{"channel":"email","beta":false}`,
			src: &fakePOV{}, wantCode: http.StatusBadRequest},
		{name: "unknown customer", id: unknown, body: `{"channel":"email","beta":false}`,
			src: &fakePOV{}, wantCode: http.StatusNotFound},
		{name: "read fails", id: sim.CustomerFernanda, body: `{"channel":"email","beta":false}`,
			src: &fakePOV{prefsErr: errAccountSimDown}, wantCode: http.StatusBadGateway},
		{name: "update refused as invalid", id: sim.CustomerFernanda, body: `{"channel":"email","beta":false}`,
			src:      &fakePOV{updateErr: fmt.Errorf("bff: account-sim update preferences: %w", sim.ErrCommand)},
			wantCode: http.StatusUnprocessableEntity, wantBody: `{"error":"invalid"}`, wantUpdates: 1},
		{name: "update finds no customer", id: sim.CustomerFernanda, body: `{"channel":"email","beta":false}`,
			src:      &fakePOV{updateErr: fmt.Errorf("bff: account-sim update preferences: %w", sim.ErrUnknownCustomer)},
			wantCode: http.StatusNotFound, wantUpdates: 1},
		{name: "update fails", id: sim.CustomerFernanda, body: `{"channel":"email","beta":false}`,
			src: &fakePOV{updateErr: errors.New("account-sim exploded")}, wantCode: http.StatusBadGateway, wantUpdates: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := preferencesHandler(tt.src)
			rr := putPreferences(t, h, tt.id, tt.body)
			if rr.Code != tt.wantCode {
				t.Fatalf("status = %d %s, want %d", rr.Code, rr.Body.String(), tt.wantCode)
			}
			if tt.wantBody != "" && strings.TrimSpace(rr.Body.String()) != tt.wantBody {
				t.Errorf("body = %s, want %s", rr.Body.String(), tt.wantBody)
			}
			if tt.src.updates != tt.wantUpdates {
				t.Errorf("updates = %d, want %d", tt.src.updates, tt.wantUpdates)
			}
			// Preferences are no POV action: the counters never move.
			if got := countersOf(t, h); len(got.Actions) != 0 || len(got.Refusals) != 0 || got.Duplicates != 0 {
				t.Errorf("counters = %+v, want untouched", got)
			}
		})
	}
}

func TestPutPOVPreferences_RateLimit(t *testing.T) {
	t.Parallel()
	src := &fakePOV{}
	h := preferencesHandler(src)
	bodies := [2]string{`{"channel":"email","beta":true}`, `{"channel":"chat","beta":false}`}
	for i := range 10 {
		if rr := putPreferences(t, h, sim.CustomerFernanda, bodies[i%2]); rr.Code != http.StatusOK {
			t.Fatalf("change %d = %d %s", i, rr.Code, rr.Body.String())
		}
	}
	// Ten changes left chat/off stored: repeating it is free even over the limit.
	if rr := putPreferences(t, h, sim.CustomerFernanda, bodies[1]); rr.Code != http.StatusOK {
		t.Fatalf("no-op over the limit = %d %s", rr.Code, rr.Body.String())
	}
	blocked := putPreferences(t, h, sim.CustomerFernanda, bodies[0])
	if blocked.Code != http.StatusTooManyRequests || strings.TrimSpace(blocked.Body.String()) != `{"error":"minute"}` {
		t.Fatalf("eleventh change = %d %s, want 429 minute", blocked.Code, blocked.Body.String())
	}
	if src.updates != 10 {
		t.Errorf("updates = %d, want 10 (the blocked change never reaches account-sim)", src.updates)
	}
	// The limit is per customer.
	if rr := putPreferences(t, h, sim.CustomerThiago, bodies[0]); rr.Code != http.StatusOK {
		t.Errorf("other customer = %d %s", rr.Code, rr.Body.String())
	}
}

func TestPutPOVPreferences_UnavailableIsRefunded(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      error
		wantLast int
	}{
		{name: "unavailable spends nothing", err: status.Error(codes.Unavailable, "down"), wantLast: http.StatusOK},
		{name: "other failures spend the budget", err: status.Error(codes.Internal, "boom"), wantLast: http.StatusTooManyRequests},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := &fakePOV{updateErr: tt.err}
			h := preferencesHandler(src)
			for i := range 10 {
				if rr := putPreferences(t, h, sim.CustomerFernanda, `{"channel":"email","beta":false}`); rr.Code != http.StatusBadGateway {
					t.Fatalf("failing change %d = %d %s", i, rr.Code, rr.Body.String())
				}
			}
			src.mu.Lock()
			src.updateErr = nil
			src.mu.Unlock()
			if rr := putPreferences(t, h, sim.CustomerFernanda, `{"channel":"email","beta":false}`); rr.Code != tt.wantLast {
				t.Fatalf("change after failures = %d %s, want %d", rr.Code, rr.Body.String(), tt.wantLast)
			}
		})
	}
}
