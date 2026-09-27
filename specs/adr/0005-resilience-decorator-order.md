# ADR 0005 — Resilience decorator order and error classes

- **Status:** accepted
- **Date:** 2026-09-27

## Context

The Jev call is the only synchronous external dependency. Without ordered limits, one slow or
failing provider stalls consumers and spends gateway budget without a bound.

## Decision

Outside-in, per message, wrap the call in this order:

1. Fallback — any failure becomes a degraded heuristic result, with the reason recorded.
2. Bulkhead — semaphore on in-flight model calls, equal to the consumer prefetch.
3. Circuit breaker — opens after 3 consecutive message failures, or 5 failures in the last 10, or
   an immediate quota error; then answers without calling the API.
4. Retry — limited attempts, only inside the remaining deadline.
5. Rate limiter — local token bucket; each real attempt takes one token.
6. Timeout — 2 seconds on that HTTP attempt.

Breaker failure classes: 429, 5xx, timeout, and network errors; also 401, 402, and 403 because they
repeat. HTTP 400 `invalid_request` falls back and does not count as a provider failure.
`402` with `quota_for_entity_exceeded` opens the breaker immediately. Libraries:
`github.com/sony/gobreaker/v2` and `golang.org/x/time/rate`. Bulkhead and retry use the standard
library.

## Consequences

One message is one breaker outcome. Each HTTP attempt spends a limiter token. Caller cancellation
skips fallback. The stack is testable against a fake HTTP server for 429, 402, 5xx, half-open, and
deadline exhaustion.

## Reassessment trigger

Revisit if the gateway publishes a fixed rate limit or if another synchronous provider is added.
