package bff

import (
	"encoding/json"
	"testing"

	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

func TestBastidoresDepositThenAlert(t *testing.T) {
	t.Parallel()
	hub := newBastidoresHub()
	view := hub.markCommand("c-fernanda", "evt-1", sim.CmdDeposit)
	if view.Steps[0].State != "feito" || view.Steps[1].State != "agora" {
		t.Fatalf("after command = %+v", view.Steps)
	}
	body, _ := json.Marshal(map[string]string{"event_id": "evt-1", "customer_id": "c-fernanda"})
	hub.observe(event.NameAccountEventRecorded, body)
	got, ok := hub.snapshot("c-fernanda", "evt-1")
	if !ok || got.Steps[1].State != "feito" || got.Steps[2].State != "agora" {
		t.Fatalf("after publish = %+v", got.Steps)
	}
	hub.observe(event.NameAlertRaised, body)
	got, _ = hub.snapshot("c-fernanda", "evt-1")
	if got.Steps[2].State != "feito" || got.Steps[3].State != "feito" {
		t.Fatalf("after alert = %+v", got.Steps)
	}
	if got.Steps[2].Label != "Avaliado pelas regras do advisory" {
		t.Fatalf("label = %s", got.Steps[2].Label)
	}
}

func TestBastidoresComplaintUsesTriageLabels(t *testing.T) {
	t.Parallel()
	hub := newBastidoresHub()
	hub.markCommand("c-mariana", "evt-2", sim.CmdComplaint)
	body, _ := json.Marshal(map[string]string{"event_id": "evt-2", "customer_id": "c-mariana"})
	hub.observe(event.NameMessageReceived, body)
	hub.observe(event.NameMessageTriaged, body)
	got, _ := hub.snapshot("c-mariana", "evt-2")
	if got.Steps[2].Label != "Classificado pela triagem" || got.Steps[3].State != "feito" {
		t.Fatalf("steps = %+v", got.Steps)
	}
}
