# Advisor Radar — view selection and client point of view

Design canvas for the entry screen and the client point-of-view (POV) flow. Source canvas:
<https://claude.ai/artifact/KKDUpYVYbAE26tJKHFkEmM>. Each `*.dc.html` file is one artboard;
`canvas.json` holds the layout. `support.js` is the same runtime copied from `../base/`.
UI copy is Portuguese; data, names, and branding are fictional.

## Routes and artboards

| Route | Mobile (390 px) | Desktop (1440 px) |
|---|---|---|
| `/advisor-radar` | `Selecao-mobile.dc.html` | `Main.dc.html` |
| `/advisor-radar/client-pov` | `ClientPov.dc.html` | `ClientPov-Desktop.dc.html` |
| `/advisor-radar/client-pov/{id}` | `ClientApp.dc.html` (+ state artboards) | `ClientApp-Desktop*.dc.html` |

`ClientApp.dc.html` is the single interactive component. The other `ClientApp-*` artboards
import it with fixed props (`client`, `screen`, `theme`, `layout`, `amount`, `preset`, `trace`)
to show each state.

## Selection screen

- Two choices at the top: team view (links to `/fila`) and client view (links to `/client-pov`),
  each with a short description of what it contains.
- A brief product summary: proactive alerts, message triage, and cases with SLA.
- An animated architecture walkthrough of one complaint: web → BFF (HTTP) → account-sim (gRPC,
  outbox) → RabbitMQ → triage (Jev with heuristic fallback) → advisory and cases →
  timeline-indexer (Elasticsearch) → BFF SSE → advisor queue. It auto-advances every 2.6 s, can
  be paused, and respects `prefers-reduced-motion`.

## Client POV

- One seeded customer per segment, reused from `seeds/advisory/001_cast.sql`: Fernanda Lima
  (Essencial), Thiago Azevedo (Advance), Mariana Costa (Singular).
- The client app is the fictional brokerage "Orla Invest". It does not copy any real brokerage's
  brand. It has a dark and a light theme with a toggle, and the Advisor Radar simulation strip
  stays on top.
- Actions and the events they publish:
  - Deposit (`Depositar`): `account.event.recorded`, kind `aporte`.
  - Withdrawal / wire-out (`Sacar`): `account.event.recorded`, kind `saque`.
  - Free message (chat or e-mail): `message.received`.
  - Complaint, from four presets or free text: `message.received`, channel `chat`.
- Deposit uses a plus-in-circle icon and withdrawal uses a bank icon. Up/down arrows were rejected
  as ambiguous.
- On desktop, actions open in a 440 px side panel over the home screen; on mobile they are full
  screens.
- After an action, the confirmation shows a "Bastidores" panel with the routing key, `event_id`,
  `trace_id`, and the progress outbox → RabbitMQ → triage/rules → queue.

## Open decisions for implementation

- BFF → account-sim is a gRPC command, and account-sim writes the message and the event to its
  outbox in one transaction. The BFF has no database and cannot publish through an outbox.
  Record this as an ADR before implementing.
- The architecture animation labels the call `POST /api/v1/client-pov/{id}/complaints`. That
  name is a placeholder; the BFF already versions routes as `/v1/...`, so the real contract
  should follow that prefix.
- The root `/advisor-radar` stops redirecting to `/fila` and renders the selection screen.
- Available cash and allocation percentages are not in the seed yet. The values in the design
  are illustrative and must come from the backend. The seed must let every phase-1 alert rule
  fire during the demo: for example, Mariana's available cash must exceed 20% of her AUM for a
  withdrawal to count as relevant.
- Balance validation in the withdrawal form is presentational; the backend stays authoritative.
