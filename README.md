# Advisor Radar

Fictional advisory desk. A visitor can open the team queue or enter Orla Invest as one seeded client and cause the same events the team already sees.

The public app is rooted at `/advisor-radar`.

## Selection

`/advisor-radar` is the selection screen. It does not redirect to the queue.

- **Visão do time** opens `/advisor-radar/fila`. Review, the manager panel, and the customer 360 stay at `/advisor-radar/revisao`, `/advisor-radar/painel`, and `/advisor-radar/clientes/{id}`.
- **Visão do cliente** opens `/advisor-radar/client-pov`.

The screen states three pillars (proactive alerts, message triage, cases with SLA) and plays an eight-step complaint walkthrough. The walkthrough pauses and stays still when `prefers-reduced-motion` is set.

## Client point of view

`/advisor-radar/client-pov` lists three seeded clients:

| Client | Segment | Assets | Available cash |
|---|---|---:|---:|
| Mariana Costa | Singular | USD 248,300 | USD 60,000 |
| Fernanda Lima | Essencial | USD 8,200 | USD 1,148 |
| Thiago Azevedo | Advance | USD 68,000 | USD 60,520 |

The phone app can deposit, withdraw, send a free message, or file a preset complaint. Desktop actions use a 440px panel. The client app is dark by default and can switch to the light theme. The team list and the selection screen stay light.

An accepted action returns `202` with `event_id`. The confirmation protocol is the first two UUID groups of that id, uppercased. Bastidores follows `GET /v1/client-pov/customers/{id}/stream`. The queue link is `/advisor-radar/fila`.

Money on the POV wire is integer USD cents. Schema version 1 events stay whole dollars. Schema version 2 account events are cents; advisory converts them into the dollar book before the phase-1 rules run.

Names, balances, and Orla Invest are fictional. No message is sent to a real person.

## Demo script

1. Open `/advisor-radar` and walk the eight steps in about 30 seconds.
2. Enter as Mariana Costa and send "Estou pensando em sair".
3. Show Bastidores moving through the outbox, RabbitMQ, and triage.
4. Open `/advisor-radar/fila`. The churn alert is at the top of the queue with the Singular SLA.
5. Return as Fernanda Lima, deposit US$ 10.000, and show the large-deposit and segment-change alerts.
6. Submit the same withdrawal twice with the same idempotency key and show a single event.

## Contract

The BFF HTTP contract, including the POV routes, is in [specs/http/bff.md](specs/http/bff.md). Client commands are recorded in [specs/adr/0008-client-command-grpc-outbox.md](specs/adr/0008-client-command-grpc-outbox.md). OpenTelemetry is not instrumented.
