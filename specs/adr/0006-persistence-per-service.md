# ADR 0006 — Persistence per service

- **Status:** accepted
- **Date:** 2026-09-27

## Context

Sharing one database across services couples schemas and migration ownership. Each service owns
different data and failure domains.

## Decision

Databases are not shared across services:

- PostgreSQL for `account-sim`, `advisory`, `triage`, and `cases` (one database each).
- Elasticsearch for `timeline-indexer`.
- The BFF stores nothing.

Local Compose runs one PostgreSQL server that hosts those four named databases. Schemas and
ownership stay separate.

## Consequences

Each service migrates and scales its store independently. Cross-service reads go through gRPC or
events, not shared tables. Cutting Elasticsearch later follows the documented cut order and
reads the timeline from PostgreSQL, without rewriting shared schemas.

## Reassessment trigger

Revisit only if a later ADR explicitly adopts a shared database — which the product contract
forbids for the MVP.
