# ADR 0012 — The SLA of an open case follows the client's segment

- **Status:** accepted
- **Date:** 2026-11-13

## Context

A case froze its SLA at opening: `sla_total_minutes` came from the client's segment at that
instant and the clock always counted from `opened_at`. The segment is live (advisory moves the
book on every account event), so a client who went from Essencial to Advance with an open case
kept a 24 h promise while every screen already called them Advance, and a downgrade kept a
promise the new segment does not make.

Escalation had two more limits. `sla_delay` held one row per case and the publisher marked it by
case id, so a case could be armed only once. And every delay went to one queue with a
per-message TTL: RabbitMQ only expires the message at the head of a queue, so a 1 h delay
published behind a 24 h one dead-lettered after 24 h (measured on RabbitMQ 4.3: a 1 s message
behind a 12 s message expired at 12 s).

## Decision

- **Rule.** When a client's segment changes while they have a case that is not `Resolvido`, the
  case's SLA becomes the new segment's SLA, counted from the instant of the segment change. It
  applies to upgrades and downgrades. A `Resolvido` case is never touched.
- **Trigger and truth.** cases consumes `alert.raised` on its own queue (`cases.alert.raised`,
  with a dead-letter queue) and reacts only to kind `segmento`. The alert is the trigger; the
  segment is read from advisory over gRPC (`GetCustomer`), the same lookup the intake uses. The
  book is the truth because a `segmento` alert is also raised for a schema-version-1 fact that
  does not move the book, and because alerts can be redelivered or arrive out of order.
- **Halving.** The halving factors (churn risk, frustration, human request, relevant withdrawal)
  belong to what opened the case, not to the segment, so a halved case stays halved: the new
  total is the new segment's base, halved when the current total is a halved one. The six totals
  (1440, 720, 240, 120, 60, 30) are distinct, so "halved" is read from the stored total and no
  column is added for it; a unit test fails if a new base breaks that property.
- **State.** `cases.sla_started_at` (nullable) holds the instant the clock counts from; `NULL`
  means `opened_at`, so rows and binaries from before this ADR keep working. The start is the
  alert's `occurred_at` (the account fact's time), never earlier than the previous start. When
  the alert is processed more than one minute after its `occurred_at` (a backlog, services
  stopped by a deploy or a reseed), the start is the processing instant instead: the delay
  message is armed with the full SLA as TTL when it is published, so counting a late alert from
  `occurred_at` would show the case "Vencido" for the length of the lag before its tick fires.
  Counting from the processing instant keeps the deadline on screen and the tick within that
  minute of each other, at the price of giving the client's new SLA from when cases learned of
  the change. The
  change writes one `case_history` row. Idempotency is the inbox claim on the alert's `event_id`
  under the per-customer lock; an alert that finds the total already right changes nothing.
- **Escalation.** A recalculation re-arms the case's `sla_delay` row (`armed_at` identifies the
  arm, so the publisher cannot mark a newer arm with the acknowledgement of an older one). The
  breach consumer compares the tick with the case's current deadline (`start + total`): a tick
  before it belongs to a superseded clock and is acknowledged without claiming the inbox; a tick
  at or after it escalates as before, once per case (the inbox key stays `breach:<case id>`), so
  a case already escalated stays escalated and is not escalated again.
- **Delay queues.** Delays are published to one queue per TTL (`cases.sla.delay.<ttl ms>`, queue
  TTL, same dead-letter route), so every queue is FIFO in expiry order and a short delay is never
  held behind a long one. This still is ADR 0003: a TTL queue and a dead-letter exchange, no cron.
  The legacy `cases.sla.delay` queue stays declared and is no longer published to.

## Consequences

- Queue card, manager panel and at-risk list read `remaining_minutes` computed by cases from the
  new start; the BFF no longer derives the remaining time from `opened_ago`.
- cases also decides the SLA state (`sla_state`: "No prazo", "Vencendo" while less than a third
  of the SLA and never less than 20 minutes is left, "Vencido", "Concluído"). The queue card, the
  queue SLA filter, the at-risk list and the backlog counts all use it, so the manager panel
  cannot call a case at risk while the queue calls it on time. The former 60-minute at-risk
  threshold is gone.
- The client's `case_open` moment states the SLA of the case itself (`sla_total`, written as
  "12 h" or "30 min"), next to the live segment: "Como cliente Advance, este atendimento tem
  resposta em até 4 h". It is the number the queue clock counts from, before and after a
  recalculation, including the halving. A halved case names both numbers ("Como cliente
  Singular, seu prazo padrão é de 1 h. Este atendimento é prioritário: resposta em até 30 min."),
  and cases reports `sla_halved` so the advisor's card can say the same; otherwise 30 min next to
  "Singular · 1 h" reads as a contradiction.
- Messages already in `cases.sla.delay` from the previous version still dead-letter with the case
  id as body. For a case never recalculated they fire at or after its deadline and escalate. For a
  recalculated case they are ignored unless the current deadline has passed. They keep the
  head-of-line order of the old queue, so such a case can escalate late, as it could before.
- No event is published for the recalculation: no service consumes it, and the readers get the
  clock over gRPC.
- Known limit: the breach consumer compares the tick's arrival with the wall clock. A backward
  step of the wall clock can make the tick of the current clock read as superseded, and the case
  is then not escalated by that tick.
- Known limit: a case opened within milliseconds of a segment change can be opened with the old
  segment read from the book after the alert for that change found no open case. It keeps the
  old segment's SLA until the next segment change.

## Reassessment trigger

Revisit if a case must be escalated more than once, if the SLA must pause (for example while
`Aguardando cliente`), or if a segment gains a base that collides with a halved total.
