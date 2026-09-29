# ADR 0010 — Individual per-product positions in account-sim

- **Status:** accepted
- **Date:** 2026-09-28

## Context

Phase-2 closed decision 9 (client POV brief, section 15, item 9) keeps four
class balances per account in account-sim: stocks, ETFs, fixed income, and cash,
with no individual asset, and cash as the available balance. Phase 3 adds a
purchase of a fictional product, a Carteira screen with per-product applied
value, current value, and return, and a simulated market day that revalues
portfolios. None of these fit four class totals.

## Decision

- **This revokes phase-2 closed decision 9** ("four classes, no individual
  asset").
- account-sim stores positions per fictional product. Each position belongs to
  one product of the fictional catalog and holds units and an applied value in
  integer USD cents. Units are expressed in day-0 cents: the fixed catalog price
  is normalized to one cent per unit, so there is no price column. Its current
  value is `units × factor(product, sim_day)`, where the factor is 1 except on
  the scripted shocks. A purchase buys units at the current day's price, and its
  applied value is the cash spent.
- The four phase-2 classes become an aggregate: stocks, ETFs, and fixed income
  are the sum of the positions of that class, and cash stays its own balance.
- Patrimony is positions at market value plus cash, everywhere: the client app,
  the advisory book, the alert rules, and the client moments.
- The seed keeps every phase-2 class total, so phase-1 rules fire at the same
  values. Customer ids stay as in ADR 0007.
- A position's value is a pure function of product and the global simulated day,
  with no randomness and no wall clock. Equal holdings move equally.
- account-sim owns and persists the global `sim_day`. It advances only by
  command and returns to 0 on reseed. Each advance publishes one `reavaliacao`
  event per account for that day, keyed `reavaliacao:{customer_id}:{sim_day}`.
  A scripted shock may raise drop alerts on several accounts at once; that is
  expected.
- There is no new event type. The purchase (`kind: aplicacao`) and the
  revaluation (`kind: reavaliacao`) ride on `account.event.recorded` at
  `schema_version` 3, following the schema-version pattern of ADR 0008. Amounts
  stay integer USD cents, as in version 2. Consumers accept versions 1, 2, and 3
  and scale by version before a rule runs. A `schema_version` outside 1–3 is
  rejected, not scaled. The phase-2 commands keep publishing version 2.
- account-sim stays the source of truth. It writes positions, cash, and the
  outbox row in one transaction; advisory follows it through events.

## Consequences

A purchase moves cash into a position and leaves patrimony unchanged. A
revaluation changes patrimony without any client action, and advisory's drop and
segment rules see it as an account event. Every consumer of
`account.event.recorded` must handle version 3 before account-sim publishes it.
Readers that still expect four class balances read the aggregate instead of a
stored column.

## Reassessment trigger

Revisit if positions need real quotes, lots, sells, or pending orders, or if the
per-class aggregate becomes too costly to compute on read.
