package bff_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/bff"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

type fakePOV struct {
	mu       sync.Mutex
	accounts []bff.POVAccount
	applied  []bff.POVCommand
	eventID  string
	replay   bool
	err      error
}

func (f *fakePOV) List(context.Context) ([]bff.POVAccount, error) {
	return f.accounts, nil
}

func (f *fakePOV) Get(_ context.Context, id string) (bff.POVAccount, error) {
	for _, account := range f.accounts {
		if account.CustomerID == id {
			return account, nil
		}
	}
	return bff.POVAccount{}, sim.ErrUnknownCustomer
}

func (f *fakePOV) Apply(_ context.Context, cmd bff.POVCommand) (bff.POVResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return bff.POVResult{}, f.err
	}
	for _, prev := range f.applied {
		if prev.CustomerID == cmd.CustomerID && prev.IdempotencyKey == cmd.IdempotencyKey {
			return bff.POVResult{EventID: f.eventID, Replay: true}, nil
		}
	}
	f.applied = append(f.applied, cmd)
	return bff.POVResult{EventID: f.eventID, Replay: f.replay}, nil
}

func TestPOVListAndGet(t *testing.T) {
	t.Parallel()
	src := &fakePOV{accounts: []bff.POVAccount{{
		CustomerID: sim.CustomerFernanda,
		Acoes:      164_000, ETFs: 369_000, RendaFixa: 172_200, Caixa: 114_800,
	}}}
	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, nil, nil, nil, src, func() time.Time {
		return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/client-pov/customers", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Fernanda Lima") || !strings.Contains(rr.Body.String(), "820000") {
		t.Fatalf("list body = %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/client-pov/customers/"+sim.CustomerFernanda, nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"caixa":114800`) {
		t.Fatalf("get = %d %s", rr.Code, rr.Body.String())
	}
}

func TestPOVDepositIdempotencyAndRefusal(t *testing.T) {
	t.Parallel()
	src := &fakePOV{eventID: "01a0e3a4-9a44-7566-b5de-eb2e365799f8"}
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	h := bff.NewHandlerWithPOV(bff.NewBoard(), nil, nil, nil, nil, nil, src, func() time.Time { return now })

	post := func(key, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/client-pov/customers/"+sim.CustomerFernanda+"/deposits", strings.NewReader(body))
		req.Header.Set("Idempotency-Key", key)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}
	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/v1/client-pov/customers/"+sim.CustomerFernanda+"/deposits", strings.NewReader(`{"amount":1000000,"origin":"pix"}`)))
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing key = %d", missing.Code)
	}

	first := post("k1", `{"amount":1000000,"origin":"pix"}`)
	if first.Code != http.StatusAccepted || !strings.Contains(first.Body.String(), src.eventID) {
		t.Fatalf("first = %d %s", first.Code, first.Body.String())
	}
	second := post("k1", `{"amount":1000000,"origin":"pix"}`)
	if second.Code != http.StatusAccepted || !strings.Contains(second.Body.String(), src.eventID) {
		t.Fatalf("replay = %d %s", second.Code, second.Body.String())
	}

	src.err = sim.ErrInsufficient
	refused := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/client-pov/customers/"+sim.CustomerMariana+"/withdrawals", strings.NewReader(`{"amount":99999999,"destination":"conta-eua"}`))
	req.Header.Set("Idempotency-Key", "over")
	h.ServeHTTP(refused, req)
	if refused.Code != http.StatusUnprocessableEntity {
		t.Fatalf("refusal = %d %s", refused.Code, refused.Body.String())
	}

	for i := 0; i < 9; i++ {
		src.err = nil
		rr := post("extra-"+string(rune('a'+i)), `{"amount":1,"origin":"pix"}`)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("extra %d = %d", i, rr.Code)
		}
	}
	blocked := post("over-limit", `{"amount":1,"origin":"pix"}`)
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("limit = %d %s", blocked.Code, blocked.Body.String())
	}
	replay := post("k1", `{"amount":1000000,"origin":"pix"}`)
	if replay.Code != http.StatusAccepted {
		t.Fatalf("replay over limit = %d", replay.Code)
	}

	counters := httptest.NewRecorder()
	h.ServeHTTP(counters, httptest.NewRequest(http.MethodGet, "/v1/client-pov/counters", nil))
	var got struct {
		Actions    map[string]int `json:"actions"`
		Refusals   map[string]int `json:"refusals"`
		Duplicates int            `json:"duplicates"`
	}
	if err := json.Unmarshal(counters.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Refusals["insufficient"] != 1 || got.Duplicates < 1 || got.Actions["deposit"] < 1 {
		t.Fatalf("counters = %+v", got)
	}
}
