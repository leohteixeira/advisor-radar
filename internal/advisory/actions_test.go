package advisory_test

import (
	"context"
	"testing"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/advisory"
)

func TestActions_SnoozeContactUndo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fixed := time.Date(2026, 9, 27, 15, 0, 0, 0, time.UTC)
	store := advisory.NewMemoryActionStore()
	svc := advisory.NewActions(store, func() time.Time { return fixed })

	if err := svc.Snooze(ctx, "s01"); err != nil {
		t.Fatalf("snooze: %v", err)
	}
	row, ok, err := store.Get(ctx, "s01")
	if err != nil || !ok {
		t.Fatalf("get after snooze: ok=%v err=%v", ok, err)
	}
	wantUntil := fixed.Add(time.Hour)
	if row.SnoozedUntil == nil || !row.SnoozedUntil.Equal(wantUntil) {
		t.Fatalf("snoozed_until = %v, want %v", row.SnoozedUntil, wantUntil)
	}

	if err := svc.Contact(ctx, "s01"); err != nil {
		t.Fatalf("contact s01: %v", err)
	}
	row, ok, err = store.Get(ctx, "s01")
	if err != nil || !ok {
		t.Fatalf("get after contact s01: ok=%v err=%v", ok, err)
	}
	if row.ContactedAt == nil || !row.ContactedAt.Equal(fixed) {
		t.Fatalf("s01 contacted_at = %v, want %v", row.ContactedAt, fixed)
	}
	if row.SnoozedUntil != nil {
		t.Fatalf("s01 snoozed_until = %v, want nil after contact", row.SnoozedUntil)
	}

	if err := svc.Contact(ctx, "s02"); err != nil {
		t.Fatalf("contact: %v", err)
	}
	row, ok, err = store.Get(ctx, "s02")
	if err != nil || !ok {
		t.Fatalf("get after contact: ok=%v err=%v", ok, err)
	}
	if row.ContactedAt == nil || !row.ContactedAt.Equal(fixed) {
		t.Fatalf("contacted_at = %v, want %v", row.ContactedAt, fixed)
	}
	if row.SnoozedUntil != nil {
		t.Fatalf("s02 should not be snoozed")
	}

	if err := svc.Undo(ctx, "s02"); err != nil {
		t.Fatalf("undo: %v", err)
	}
	_, ok, err = store.Get(ctx, "s02")
	if err != nil {
		t.Fatalf("get after undo: %v", err)
	}
	if ok {
		t.Fatal("s02 row still present after undo")
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].SignalID != "s01" {
		t.Fatalf("list = %+v, want one s01 row", list)
	}
	if list[0].SnoozedUntil != nil || list[0].ContactedAt == nil {
		t.Fatalf("list s01 = %+v, want contacted and not snoozed", list[0])
	}
}
