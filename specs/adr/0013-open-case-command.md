# ADR 0013 — Opening a case from a signal is an asynchronous command

- **Status:** accepted
- **Date:** 2026-11-13

## Context

ADR 0001 says that creating a case from an alert is an asynchronous command, but nothing
implemented it: "Abrir caso" in the advisor queue only drew a card in the browser. The command
needs a durable path from the BFF to cases. The BFF has no database, so it cannot own an outbox
(ADR 0002), and a gRPC write from the BFF into cases would be the synchronous cross-service write
ADR 0001 rules out.

## Decision

- **Split.** The HTTP request is synchronous and only accepts the command; the case is created
  asynchronously.
  1. The browser sends `POST /v1/signals/{id}/case` with an `Idempotency-Key`.
  2. The BFF resolves the signal in the queue it already serves (advisory's seeded signals plus
     the live board) and sends the command to advisory over the existing actions HTTP port
     (`ADVISORY_HTTP_URL`), next to Contatado and Adiar: advisory owns the signals and has an
     outbox. The call carries the client, the signal id, its label and the facts that halve an
     SLA (churn risk, frustration, human request, relevant withdrawal).
  3. advisory writes one outbox row, event `case.open.requested`, and answers `202` with the
     event id as `request_id`. The id is derived from the signal id and the idempotency key, so a
     retry of the same request writes nothing new.
  4. cases consumes `case.open.requested` on its own queue (`cases.case.open.requested`, with a
     dead-letter queue), with the inbox, the per-customer lock, the advisory lookup outside the
     transaction and the retry policy of the triaged intake.
- **Invariant.** One open case per customer decides the outcome. Without an open case, cases
  opens one for the signal with the SLA of the client's segment (halved by the same factors as
  the intake), counted from the command's `occurred_at`. With an open case, the signal joins that
  case's history (`case_history.signal_id`, at most once per case and signal) and no case is
  created.
- **Outcome.** There is no reply event. The browser shows the card as pending and re-reads
  `GET /v1/cases` for a bounded time: a case whose `signal` is the signal means "opened"; a
  history entry whose `signal` is the signal means "joined the client's open case", and the UI
  says so. A timeout or a refused request is reported as a failure the advisor can retry.

## Consequences

- New event name `case.open.requested` (schema version 1), produced by advisory and consumed by
  cases. `cases.v1.CaseHistoryEntry` gains `signal`.
- cases stays the only writer of cases; advisory stores nothing but the outbox row.
- A signal that opened or joined a case is represented by that case: the BFF leaves it out of
  the "Novo sinal" column. The same holds for a later message of a client with an open case,
  which the triaged intake adds to that case's history with its signal id.
- A command for a client advisory does not know is dead-lettered by cases; the browser sees a
  timeout.
- The BFF does not consume `case.opened`; a push would replace the bounded re-read, at the cost
  of a second stream event type.

## Reassessment trigger

Revisit if the BFF starts consuming case events (the pending state can then end on the event), or
if an advisor must be able to open a second case for a client who already has one.
