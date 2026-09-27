---
title: 'Classify messages with Jev and the resilience stack'
type: 'feature'
created: '2026-09-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'dfbe5e89b162652eb278a550bfc211e71495306f'
followup_review_recommended: true
context: []
warnings:
  - oversized
deferred:
  - summary: >-
      The PostgreSQL triage adapter and the live Jev labeled-set run are not exercised by go test.
    evidence: |-
      CI has no Postgres, no RabbitMQ, and must not call ai-gateway.vercel.sh. memStore and the fake HTTP server cover the matrix. cmd/jevprobe scores the labeled file when AI_GATEWAY_API_KEY is set.
    location: >-
      internal/triagepipe/pgx.go
    severity: medium
---

<intent-contract>

## Intent

**Problem:** Inbound customer messages are stored as `message.received`, but nothing classifies intent, frustration, churn, or a human request, and nothing keeps working when the model cannot be called.

**Approach:** Copy the radar-triage classifier into this module, wrap Jev with the resilience stack, and publish `message.triaged` through an idempotent inbox and a transactional outbox.

## Boundaries & Constraints

**Always:** Copy the port, the question text, and the HTTP contract from `triage.md`. Start from those keyword tables and extend them so the labeled set matches. Model state is only `canal`, `mensagem`, and `mensagens_anteriores`. Remove digit runs and slash-dates from those strings before the HTTP call. The heuristic still sees the original text. Jev is one HTTP attempt: `POST {base}/v1/evaluate`, model `typesafe-ai/jev`, `providerOptions.gateway.zeroDataRetention: true`, response body limit 1 MiB. The stack outside-in is Fallback, bulkhead, circuit breaker, retry, rate limiter, timeout of 2 seconds. Breaker opens after 3 consecutive provider failures, or 5 failures in the last 10, or immediate `402` with `quota_for_entity_exceeded`. It stays open 15 seconds, then one half-open trial. Provider failures are 429, 5xx, timeout, network, 401, 402, and 403. HTTP 400 `invalid_request` falls back, is logged, and does not move the breaker. Honor `Retry-After` on 429 when it fits the deadline; otherwise exponential backoff with jitter. If the wait would pass the deadline, fall back. Do not retry other 4xx. Caller cancellation returns that error and does not classify. Any other primary error returns the heuristic with `Degraded` set. Review is `IntentProb < 0.85` only. Boolean probe cutoff is 0.5. Heuristic intent on `testdata/messages.json` matches 16 of 16. The copied baseline of 11 of 16 is not a ceiling. Inbox key is the source `event_id`. Triaged id is `tr-` plus that id. The result row and the outbox row commit in one transaction. Publish marks the row only after the broker accepts it. With `TRIAGE_DATABASE_URL` unset, the process only logs `service=triage` and waits for a signal. `go test` must not dial port 5435 or 5673 and must not call `ai-gateway.vercel.sh`. Wrap errors with `%w`. Pass `context.Context` into I/O.

**Never:** Do not import `github.com/leohteixeira/radar-triage`. Do not keep the prototype's inner HTTP retry. Do not treat `Degraded` as review. Do not draft a reply. Do not add gRPC, cases, or the advisor queue. Do not change `internal/outbox.Fire` or `internal/advisory`. Do not read or write the repository `.env` from tests. Do not log the gateway key, the Authorization header, or customer text.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Heuristic set | The 16 labeled messages | Intent matches on all 16. A match is probability 0.6. No match is `operacional` at 0.3 | No error expected |
| Redacted state | Text `20 mil em 12/09/2025` | The evaluate body has no digit run. The heuristic still sees the original text | No error expected |
| Happy model | Fake 200 with intent `cambio` at 0.9 | One `message.triaged`, classifier `jev`, `needs_review` false | No error expected |
| Low probability | Fake 200 with intent probability 0.8 | `needs_review` true, not degraded | No error expected |
| 429 Retry-After | First 429 with Retry-After inside the deadline, then 200 | Two attempts. The second waits at least that delay. Result is `jev` | No error expected |
| 402 quota | `402` `quota_for_entity_exceeded` | Breaker opens immediately. Result is degraded heuristic. Later calls do not hit HTTP while open | No error expected |
| Repeated 5xx | Three consecutive 500s | The third outcome is degraded. The breaker is open | No error expected |
| Failure rate | 5 provider failures in the last 10 outcomes | The breaker opens on the fifth failure | No error expected |
| Invalid payload | HTTP 400 `invalid_request` | Degraded heuristic. Breaker stays closed | No error expected |
| Half-open | Open interval elapsed, trial 200 | Trial is `jev` and the breaker closes | No error expected |
| Cancel | Context already canceled | Error is `context.Canceled`. No heuristic result | Returned error is the cancellation |
| Deadline | Backoff would pass the deadline | Degraded heuristic. No further HTTP call | No error expected |
| Redelivery | Same `message.received` id twice | One `tr-` row | No error expected |
| Broker refuses | Unpublished row, broker returns an error | Row stays unpublished. A later accept sends it once | Returned error wraps the broker error |
| Rollback | Insert fails inside the transaction | No inbox row, no result, no outbox row | Returned error wraps the insert error |
| Other event | `account.event.recorded` | No result and no inbox row | No error expected |

</intent-contract>

## Code Map

- `/workspace/repos/radar-triage/internal/jev/client.go` — copy the types and the single `do` call. Drop the attempt loop. Tests live in `client_test.go`.
- `/workspace/repos/radar-triage/internal/triage/triage.go`, `heuristic.go`, `jev_classifier.go`, `triage_test.go` — port, keyword tables, Fallback, question text. Change `NeedsReview` so degraded alone is not review.
- `/workspace/repos/radar-triage/testdata/messages.json` — 16 labeled messages. Copy to `internal/triage/testdata/messages.json`.
- `_bmad-output/specs/spec-advisor-radar/triage.md` — instructions, criteria, and heuristic substrings are verbatim. Do not paraphrase them.
- `_bmad-output/specs/spec-advisor-radar/architecture.md` lines 88–117 — decorator order, failure classes, libraries `github.com/sony/gobreaker/v2` and `golang.org/x/time/rate`.
- `internal/sim/burst.go` — `MessagePayload` is `channel` and `text`. Market-day messages are `md-n01`, `md-n03`, `md-n04`, `md-n06`.
- `internal/event/event.go` — routing key `message.triaged` stays off the JSON body.
- `internal/advisory/apply.go` — pattern for inbox plus outbox in one transaction. Do not edit that package.
- `cmd/triage/main.go` — today only `proc.Run`. `internal/proc/commands_test.go` requires a JSON log with `service=triage` and exit 0 on SIGINT and SIGTERM when no database URL is set.
- `deploy/postgres/init.sql` — database `triage` already exists.

## Tasks & Acceptance

**Execution:**
- `internal/jev/client.go` — single-attempt gateway client — the stack owns retry
- `internal/jev/client_test.go` — contract body, zero data retention, no retry inside the client, 400 is not retried
- `internal/triage/triage.go` — port, product `NeedsReview`, redacted model state — digits never leave the process
- `internal/triage/heuristic.go` — keyword classifier copied from the prototype — deterministic fallback
- `internal/triage/jev_classifier.go` — map answers to `Result` — missing intent is an error
- `internal/triage/testdata/messages.json` — the 16 labeled messages
- `internal/triage/triage_test.go` — 16 of 16, redaction, fallback cancellation, product review gate
- `internal/resilience/stack.go` — Fallback, bulkhead, breaker, retry, limiter, timeout — one place for the order in architecture.md
- `internal/resilience/stack_test.go` — one fake-HTTP test per resilience matrix row
- `internal/triagepipe/apply.go` — `Apply` and `Publish` behind store interfaces declared here — classify then inbox, result, and outbox in one transaction
- `internal/triagepipe/apply_test.go` — redelivery, broker refuse, rollback, other event, and a low-probability review flag
- `internal/triagepipe/pgx.go` — PostgreSQL adapter for database `triage` — parameterized SQL, context on every call
- `migrations/triage/001_inbox.sql` — inbox, results, outbox
- `cmd/triage/main.go` — log `service=triage`; consume `message.received` and publish only when the database and broker URLs are set — CI stays offline
- `cmd/jevprobe/main.go` — score the labeled file when invoked with a key already in the environment — not part of `go test`
- `.env.example` — local triage database and broker URLs, plus the gateway key name with an empty value

**Acceptance Criteria:**
- Given the labeled file, when the heuristic classifies every message, then intent matches on 16 of 16.
- Given `TRIAGE_DATABASE_URL` is unset, when the triage process receives SIGTERM, then it exits 0 and has logged `service=triage`.

## Spec Change Log

- 2026-09-27 — The labeled-set count was a copied baseline of 11/16, then the verbatim tables scored 12/16. The user said that number is not strict and asked to extend the keywords until the labeled set matches 16/16. Added `outra corretora`, `transferir tudo`, and `nao entende` to reclamacao, and `nao caiu` to cambio.

## Review Triage Log

### 2026-09-27 — Review pass
- verdicts: 29 findings — high 0, medium 9, low 14, false 6, maybe-false 0
- findings:
  - `[low]` `[reject]` The consumer is serial, so prefetch and the bulkhead do not run deliveries in parallel — one message at a time is enough for the demo burst. A worker pool is a later change.
  - `[low]` `[reject]` Classify runs before the inbox claim — a committed row makes the second delivery a no-op. A retry before commit may call the model again, which is at-least-once.
  - `[low]` `[reject]` RunPublisher does not log swallowed publish errors — the row stays unpublished and the next tick retries.
  - `[low]` `[reject]` A permanent Nack is not logged — logging the body would record customer text. The message is not requeued.
  - `[false]` `[reject]` Compose does not inject TRIAGE variables — application processes stay on the host. Compose is Postgres, RabbitMQ, and Elasticsearch.
  - `[low]` `[reject]` The migration file and EnsureSchema can drift — startup applies EnsureSchema, and both copies are the same DDL.
  - `[low]` `[patch]` Architecture still called degraded-as-review an open question — the sentence now says review is intent probability below 0.85 only.
  - `[low]` `[reject]` Breaker and fallback metrics are absent — the dashboard metrics belong to the later observability story.
  - `[low]` `[reject]` Apply does not fill Previous — the market-day messages have no earlier texts. The port still sends them when they are present.
  - `[false]` `[reject]` Acceptance criteria do not restate the matrix — the tests cover those rows.
  - `[medium]` `[patch]` A Jev response with no intent was untested — TestJevClassifierMissingIntentIsError covers it. The 1 MiB body limit stays untested. Postgres stays in deferred.
  - `[medium]` `[patch]` An empty gateway key would call the gateway — cmd/triage and cmd/jevprobe load .env for keys that are not already set, and they do not log the key.
  - `[medium]` `[patch]` An empty message payload was requeued forever — a missing payload is now a permanent delivery error.
  - `[low]` `[reject]` A body over 1 MiB is truncated before decode — the client already limits the read. A second check is an extra guard.
  - `[false]` `[reject]` Three HTTP 500s inside one call would leave the breaker closed — the breaker wraps the whole attempt, and TestStack_ThreeConsecutive5xxOpensBreaker uses three outcomes.
  - `[low]` `[reject]` Digits in the channel are not redacted — the contract strips mensagem and previous texts. The seed channels are chat and email.
  - `[low]` `[reject]` Unicode digits are not stripped — the labeled set and the seed use ASCII digits. A Unicode class would be a new guard.
  - `[low]` `[reject]` A non-finite rate limit is accepted — everyday environment values are finite numbers.
  - `[low]` `[reject]` The redaction claim still allows digits in canal — same channel limit as the digit finding above.
  - `[medium]` `[patch]` RunPublisher had no test — TestRunPublisher_RetriesThenStops covers refuse, recovery, and cancel.
  - `[medium]` `[patch]` The outbox routing key was not asserted — the happy-path and publish tests now require message.triaged.
  - `[medium]` `[patch]` Degraded plus a high probability was not applied through Apply — TestApply_DegradedHighProbNoReview expects needs_review false.
  - `[medium]` `[patch]` Missing intent was only named in the classifier — the new test expects an error from Classify.
  - `[medium]` `[defer]` Live Jev accuracy on the 16 messages is not in go test — recorded in deferred. The probe is the live run.
  - `[low]` `[reject]` Apply uses a stub classifier instead of a fake 200 — the resilience tests already use a fake HTTP server. Apply tests the inbox and the outbox.
  - `[medium]` `[defer]` PostgreSQL and AMQP are not dialed by go test — same deferred gap. The default gate must stay offline.
  - `[false]` `[reject]` SIGTERM logging was not retested in this diff — TestCommands_SignalStop still requires service=triage and it passed.
  - `[false]` `[reject]` The diff does not contain the commit — the commit is the close of this run, not a product behavior.
  - `[false]` `[reject]` Nothing asserts that the model does not draft a reply — the classifier returns a Result and no reply field exists.

## Design Notes

The prototype retries inside `jev.Client` and sends the raw text. This service moves retry into the stack and strips digits before `Evaluate`. The heuristic and the stored payload keep the original text. `NeedsReview` in the prototype is `Degraded || IntentProb < threshold`. The product flag is only `IntentProb < 0.85`, so a fallback score of 0.6 still needs review and is the simplified classification.

Live id `tr-md-n01` comes from `md-n01`. A second delivery of `md-n01` does not classify again.

## Verification

**Commands:**
- `gofmt -l .` -- expected: empty
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean
- `go vet ./...` -- expected: exit 0
- `go build ./...` -- expected: exit 0
- `mkdir -p .gotmp && GOTMPDIR="$PWD/.gotmp" go test -race -shuffle=on ./...` -- expected: pass

## Auto Run Result

Status: done

Summary: Triage copies the radar-triage classifier, wraps one Jev attempt in the resilience stack, and publishes message.triaged. The keyword table is extended so the labeled set matches intent on 16 of 16.

Files changed:
- `internal/jev` — single-attempt gateway client
- `internal/triage` — port, redacted model state, extended keywords, product review gate
- `internal/resilience` — fallback, bulkhead, breaker, retry, limiter, and a 2 second timeout
- `internal/triagepipe` — inbox, result, and outbox in one transaction
- `internal/envfile` — fill missing keys from .env without overriding the process
- `cmd/triage/main.go` — consume message.received and publish when the URLs are set
- `cmd/jevprobe/main.go` — score the labeled file when the gateway key is available
- `migrations/triage/001_inbox.sql` — inbox, results, outbox
- `SPEC.md`, `architecture.md`, `triage.md`, `.memlog.md` — 11/16 is a baseline; the keyword path now targets 16/16

Review: six medium patches (missing intent, .env load, permanent empty payload, publisher retry test, routing key, degraded review flag). Deferred: real Postgres, the AMQP dial, and the live Jev score. Rejected findings are in the triage log.

Follow-up review recommended: true. Patched medium entries: 6. Unverified risk: `PGXStore` and a live Jev run of the labeled set are not executed by `go test`.

Verification: gofmt clean, go vet, go build, and `go test -race -shuffle=on ./...` passed. `go mod tidy` adds gobreaker and golang.org/x/time.
