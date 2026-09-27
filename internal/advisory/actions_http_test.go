package advisory_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/advisory"
)

func TestActionsHTTP_PutSnoozeContactDelete(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 9, 27, 15, 0, 0, 0, time.UTC)
	store := advisory.NewMemoryActionStore()
	svc := advisory.NewActions(store, func() time.Time { return fixed })
	h := advisory.NewActionsHandler(svc)

	put := func(id, action string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/v1/actions/"+id, strings.NewReader(`{"action":"`+action+`"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	rr := put("s01", "contact")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("contact status = %d", rr.Code)
	}
	row, ok, err := store.Get(t.Context(), "s01")
	if err != nil || !ok {
		t.Fatalf("contact row: ok=%v err=%v", ok, err)
	}
	if row.ContactedAt == nil || !row.ContactedAt.Equal(fixed) {
		t.Fatalf("contacted_at = %v", row.ContactedAt)
	}

	rr = put("s02", "snooze")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("snooze status = %d", rr.Code)
	}
	row, ok, err = store.Get(t.Context(), "s02")
	if err != nil || !ok {
		t.Fatalf("snooze row: ok=%v err=%v", ok, err)
	}
	wantUntil := fixed.Add(time.Hour)
	if row.SnoozedUntil == nil || !row.SnoozedUntil.Equal(wantUntil) {
		t.Fatalf("snoozed_until = %v, want %v", row.SnoozedUntil, wantUntil)
	}

	del := httptest.NewRequest(http.MethodDelete, "/v1/actions/s01", nil)
	delRR := httptest.NewRecorder()
	h.ServeHTTP(delRR, del)
	if delRR.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", delRR.Code)
	}
	_, ok, err = store.Get(t.Context(), "s01")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if ok {
		t.Fatal("s01 still present after delete")
	}

	bad := put("s03", "wave")
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown action status = %d, want 400", bad.Code)
	}
}
