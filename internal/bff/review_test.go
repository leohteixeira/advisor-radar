package bff_test

import (
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/bff"
)

func TestValidIntents_EightLabels(t *testing.T) {
	t.Parallel()
	if len(bff.ValidIntents) != 8 {
		t.Fatalf("want 8 intents, got %d", len(bff.ValidIntents))
	}
}
