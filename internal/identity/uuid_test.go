package identity_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/leohteixeira/advisor-radar/internal/identity"
)

func TestParseV7_AcceptsLowerAndUpper(t *testing.T) {
	t.Parallel()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	raw := id.String()
	got, err := identity.ParseV7(raw)
	if err != nil {
		t.Fatalf("parse lower: %v", err)
	}
	if got != strings.ToLower(raw) {
		t.Fatalf("got %q want %q", got, strings.ToLower(raw))
	}
	got, err = identity.ParseV7(strings.ToUpper(raw))
	if err != nil {
		t.Fatalf("parse upper: %v", err)
	}
	if got != strings.ToLower(raw) {
		t.Fatalf("upper got %q", got)
	}
}

func TestParseV7_RejectsV4AndGarbage(t *testing.T) {
	t.Parallel()
	v4 := uuid.New()
	if _, err := identity.ParseV7(v4.String()); !errors.Is(err, identity.ErrInvalidID) {
		t.Fatalf("v4 err = %v", err)
	}
	if _, err := identity.ParseV7("c01"); !errors.Is(err, identity.ErrInvalidID) {
		t.Fatalf("c01 err = %v", err)
	}
	if identity.IsV7("not-a-uuid") {
		t.Fatal("expected false")
	}
}

func TestNewV7_IsVersion7(t *testing.T) {
	t.Parallel()
	s, err := identity.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	if !identity.IsV7(s) {
		t.Fatalf("%q is not v7", s)
	}
}
