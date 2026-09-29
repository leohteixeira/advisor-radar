# ADR 0011 — advisory reads account-sim synchronously for the home moments

- **Status:** accepted
- **Date:** 2026-09-29

## Context

The client home picks one moment from facts advisory evaluates
(`GetMomentFacts`). Three of them, `segment_upgrade_near`, `idle_cash`, and the
patrimony they report, depend on the client's current patrimony and cash.
Those balances live in account-sim. The advisory book only holds the AUM of
the last account event it applied, which lags a command still in the outbox
and does not split cash from positions. The BFF must not compare thresholds
itself (ADR 0009), so the evaluation has to happen in advisory with the
current balance.

## Decision

- advisory calls account-sim `account/v1` `GetAccount` synchronously, once per
  `GetMomentFacts` request, through a consumer port (`AccountReader`) dialed on
  `ACCOUNT_SIM_GRPC_TARGET`. This is a query between services over gRPC, as
  ADR 0001 allows; no state changes on this path.
- The call carries the caller's deadline. When the request has none, the
  adapter bounds it at 2 s. The BFF's own screen deadline (800 ms) is shorter,
  so in practice the screen deadline governs.
- Any failure to read the balance, including account-sim not knowing the
  customer, makes `GetMomentFacts` answer `Unavailable`. The BFF then skips
  the moments that need facts and falls back as its failure policy says, ending
  at `welcome`. When the caller has already gone, the answer carries the
  context's code instead.
- Without `ACCOUNT_SIM_GRPC_TARGET`, advisory starts and logs a warning, and
  `GetMomentFacts` is always `Unavailable`.

## Consequences

- advisory now depends at request time on account-sim for the moment facts.
  An account-sim outage degrades the home moment to `welcome` (and omits the
  wealth section, which the BFF reads from account-sim directly); the rest of
  advisory is unaffected.
- One home render reads the balance twice: the BFF reads it for the wealth
  section, and advisory reads it again for the facts. The two reads are not
  atomic, so a command landing between them can show a wealth total and a
  moment from slightly different balances for one render. This is accepted:
  the next render converges, the demo drives one client at a time, and making
  the reads atomic would mean passing the BFF's balance into advisory, which
  would let the BFF feed the facts it is not allowed to evaluate.
- The segment and the segment alerts that decide `segment_upgraded` still come
  from the advisory book, read in one snapshot, so an upgrade shows only once
  advisory has applied the event that caused it.

## Reassessment trigger

Revisit if the double read shows visibly inconsistent homes, if account-sim
latency pushes `GetMomentFacts` past the screen deadline, or if advisory starts
to keep a current balance from the account events it already consumes.
