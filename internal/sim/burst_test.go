package sim_test

import (
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/sim"
)

func TestMessagePayloadFields(t *testing.T) {
	t.Parallel()
	p := sim.MessagePayload{Channel: "chat", Text: "hello"}
	if p.Channel != "chat" || p.Text != "hello" {
		t.Fatalf("unexpected %+v", p)
	}
}
