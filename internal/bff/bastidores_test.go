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

func TestBastidoresAlertFollowsSourceEvent(t *testing.T) {
	t.Parallel()
	hub := newBastidoresHub()
	hub.markCommand("c-fernanda", "evt-1", sim.CmdWithdrawal)
	hub.observe(event.NameAccountEventRecorded, []byte(`{"event_id":"evt-1","customer_id":"c-fernanda"}`))
	alert := []byte(`{"event_id":"alert-9","customer_id":"c-fernanda","payload":{"source_event_id":"evt-1"}}`)
	hub.observe(event.NameAlertRaised, alert)
	got, ok := hub.snapshot("c-fernanda", "evt-1")
	if !ok || got.Steps[2].State != "feito" || got.Steps[3].State != "feito" {
		t.Fatalf("steps = %+v", got.Steps)
	}
}

func TestBastidoresReplaysCurrentView(t *testing.T) {
	t.Parallel()
	hub := newBastidoresHub()
	hub.markCommand("c-fernanda", "evt-1", sim.CmdWithdrawal)
	updates, cancel := hub.subscribe("c-fernanda")
	defer cancel()
	select {
	case view := <-updates:
		if view.EventID != "evt-1" || view.Steps[0].State != "feito" {
			t.Fatalf("replay = %+v", view)
		}
	default:
		t.Fatal("subscribe did not replay the command")
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

// A purchase moves through Bastidores like any account command: the v3
// aplicacao event publishes it and the perfil alert that names it as source
// evaluates and queues it.
func TestBastidoresPurchaseFollowsAplicacaoAndPerfilAlert(t *testing.T) {
	t.Parallel()
	hub := newBastidoresHub()
	hub.markCommand("c-fernanda", "evt-buy", sim.CmdPurchase)
	hub.observe(event.NameAccountEventRecorded, []byte(
		`{"event_id":"evt-buy","customer_id":"c-fernanda","schema_version":3,`+
			`"payload":{"kind":"aplicacao","amount":100000,"before":820000,"after":820000,"product_id":"cobalto","asset_class":"acoes","risk":5}}`))
	got, ok := hub.snapshot("c-fernanda", "evt-buy")
	if !ok || got.Steps[1].State != "feito" || got.Steps[2].State != "agora" {
		t.Fatalf("after publish = %+v", got.Steps)
	}
	if got.Steps[2].Label != "Avaliado pelas regras do advisory" {
		t.Fatalf("label = %s", got.Steps[2].Label)
	}
	hub.observe(event.NameAlertRaised, []byte(
		`{"event_id":"alert-perfil","customer_id":"c-fernanda","payload":{"kind":"perfil","source_event_id":"evt-buy"}}`))
	got, _ = hub.snapshot("c-fernanda", "evt-buy")
	if got.Steps[2].State != "feito" || got.Steps[3].State != "feito" {
		t.Fatalf("after perfil alert = %+v", got.Steps)
	}
}
