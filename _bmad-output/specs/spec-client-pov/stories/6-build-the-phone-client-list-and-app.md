---
title: 'Build the phone client list and app'
type: 'feature'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: '4b5071dc63332e2980263de6306bf3cfc792bce3'
followup_review_recommended: false
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:** The selection card points at `/advisor-radar/client-pov`, but that route has no list and no phone app, so a visitor cannot deposit, withdraw, or file the preset complaint.

**Approach:** The list opens a seeded client. The phone app posts the action and shows the protocol, the event_id, and the link to `/advisor-radar/fila`.

## Boundaries & Constraints

**Always:** Copy is Portuguese. The list stays on the light theme. The app is dark, with the light simulation strip. Protocol is the first two UUID groups, uppercased. A withdrawal above caixa keeps confirm disabled. The preset complaint is "Estou pensando em sair".

**Never:** Desktop layout, the light theme toggle, free message, live Bastidores, or an import from docs.

</intent-contract>

## Auto Run Result

Status: done

Summary: `/client-pov` lists the clients. Opening Fernanda shows the home, including the empty activity line. Deposit, the disabled over-cash withdrawal, and the preset complaint are covered. Confirmation shows `01A0E3A5-2F4C`, the event id, and the queue link.

Verification: `pnpm test` passed (40 tests) and `tsc --noEmit` passed. In the browser, with the BFF down, the list and the client page showed their error alerts, and "Voltar à lista" returned to the list. The success screens were exercised in the test, not against a live BFF.
