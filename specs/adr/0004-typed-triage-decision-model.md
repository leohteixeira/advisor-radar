# ADR 0004 — Typed triage decision model with heuristic fallback

- **Status:** accepted
- **Date:** 2026-09-27

## Context

Inbound messages must be classified by intent, frustration, churn risk, and human request. The
external model can fail or time out; the consumer must still produce a result without drafting
reply text.

## Decision

Triage keeps a typed `Classifier` port with a Jev adapter as the primary path and a keyword
heuristic as fallback. Numbers and dates stay in code and never enter model state. The model
decides labels; it does not draft text. `Fallback` marks degraded heuristic results. Human review
uses the chosen intent's probability against the 0.85 gate.

## Consequences

Classification stays typed and testable. Model outages degrade to heuristic results instead of
blocking the queue. Review membership is probability-driven, not driven by the degraded flag alone.

## Reassessment trigger

Revisit if the product requires generated reply text or drops the external model permanently.
