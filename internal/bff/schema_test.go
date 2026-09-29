package bff_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// countingPOV counts the account-sim reads a screen Snapshot makes.
type countingPOV struct {
	bff.POVSource
	reads *atomic.Int32
}

func (p countingPOV) Get(ctx context.Context, id string) (bff.POVAccount, error) {
	p.reads.Add(1)
	return p.POVSource.Get(ctx, id)
}

func (p countingPOV) Products(ctx context.Context) ([]bff.POVProduct, error) {
	p.reads.Add(1)
	return p.POVSource.Products(ctx)
}

func (p countingPOV) Preferences(ctx context.Context, id string) (bff.POVPreferences, error) {
	p.reads.Add(1)
	return p.POVSource.Preferences(ctx, id)
}

func (p countingPOV) Registration(ctx context.Context, id string) (bff.POVRegistration, error) {
	p.reads.Add(1)
	return p.POVSource.Registration(ctx, id)
}

// getScreenWith requests a screen with the given X-SDUI-Schema values; none
// sends no header.
func getScreenWith(t *testing.T, h http.Handler, customerID, slug string, schema ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/client-pov/customers/"+customerID+"/screens/"+slug, nil)
	for _, v := range schema {
		req.Header.Add("X-SDUI-Schema", v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestGetScreen_SchemaHeader(t *testing.T) {
	t.Parallel()
	const (
		invalid    = `{"error":"invalid_schema_range"}`
		notServed  = `{"supported":{"min":1,"max":1}}`
		jsonHeader = "application/json"
	)
	tests := []struct {
		name     string
		schema   []string
		slug     string
		expected int
		body     string
	}{
		{name: "no header", expected: http.StatusOK},
		{name: "current version", schema: []string{"1"}, expected: http.StatusOK},
		{name: "range including the current version", schema: []string{"1-2"}, expected: http.StatusOK},
		{name: "whitespace around the value", schema: []string{" 1-2 "}, expected: http.StatusOK},
		{name: "only a newer version", schema: []string{"2"}, expected: http.StatusNotAcceptable, body: notServed},
		{name: "newer range", schema: []string{"2-3"}, expected: http.StatusNotAcceptable, body: notServed},
		{name: "newer version on another screen", schema: []string{"2"}, slug: "perfil", expected: http.StatusNotAcceptable, body: notServed},
		{name: "newer version on an unknown slug", schema: []string{"2"}, slug: "x", expected: http.StatusNotAcceptable, body: notServed},
		{name: "reversed range", schema: []string{"2-1"}, expected: http.StatusBadRequest, body: invalid},
		{name: "letters", schema: []string{"abc"}, expected: http.StatusBadRequest, body: invalid},
		{name: "zero", schema: []string{"0"}, expected: http.StatusBadRequest, body: invalid},
		{name: "empty value", schema: []string{""}, expected: http.StatusBadRequest, body: invalid},
		{name: "repeated header", schema: []string{"1", "1"}, expected: http.StatusBadRequest, body: invalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reads := &atomic.Int32{}
			h, _ := startPerfil(t, screenBook(), func(p bff.POVSource) bff.POVSource {
				return countingPOV{POVSource: p, reads: reads}
			})
			slug := tt.slug
			if slug == "" {
				slug = "home"
			}
			rr := getScreenWith(t, h, sim.CustomerThiago, slug, tt.schema...)
			if rr.Code != tt.expected {
				t.Fatalf("status = %d %s, want %d", rr.Code, rr.Body.String(), tt.expected)
			}
			if got := rr.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if got := rr.Header().Get("Content-Type"); got != jsonHeader {
				t.Errorf("Content-Type = %q, want %s", got, jsonHeader)
			}
			if tt.expected == http.StatusOK {
				if !strings.Contains(rr.Body.String(), `"schema_version":1`) {
					t.Errorf("body = %s, want schema_version 1", rr.Body.String())
				}
				return
			}
			if got := strings.TrimSpace(rr.Body.String()); got != tt.body {
				t.Errorf("body = %s, want %s", got, tt.body)
			}
			// The header is checked before the Snapshot: account-sim is never read.
			if n := reads.Load(); n != 0 {
				t.Errorf("account-sim reads = %d, want 0 before a %d", n, rr.Code)
			}
		})
	}
}

// TestGetScreen_SchemaHeaderAfterID keeps a bad id a plain 400, whatever the
// header says.
func TestGetScreen_SchemaHeaderAfterID(t *testing.T) {
	t.Parallel()
	h, _ := startPerfil(t, screenBook(), nil)
	rr := getScreenWith(t, h, "not-a-uuid", "home", "2")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
