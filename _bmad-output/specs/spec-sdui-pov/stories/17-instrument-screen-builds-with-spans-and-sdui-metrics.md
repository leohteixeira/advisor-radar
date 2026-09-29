---
title: 'Instrument screen builds with spans and SDUI metrics'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '88a121f1a8dc4dedbc597df74a16eda3c491e281'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/5-build-the-bff-screen-engine-and-serve-the-home.md'
warnings: []
deferred:
  - summary: >-
      Trace context does not cross RabbitMQ or outbox rows.
    evidence: |-
      Excluded by the story's Never list; needs a traceparent column in every outbox table and header injection/extraction in every publisher and consumer.
    location: >-
      internal/outbox, cmd/*/main.go consumers
    severity: medium
  - summary: >-
      The BFF to advisory actions HTTP client is untraced, and so are the Jev model client and the Elasticsearch client.
    evidence: |-
      They use a plain http.Client without otelhttp.NewTransport (internal/jev/client.go, internal/timeline/elastic.go); model usage should be instrumented per workspace rules.
    severity: medium
  - summary: >-
      slog records carry no trace_id/span_id, so logs are not correlated with traces.
    evidence: |-
      The intent keeps logs as slog JSON; a handler wrapper reading trace.SpanContextFromContext is the missing piece.
    severity: medium
  - summary: >-
      A canceled screen build's span Error status is not asserted by a test.
    evidence: |-
      TestEngine_Build_RequestEnded builds without observed providers; the error return itself is covered.
    location: >-
      internal/screen/engine.go Build
    severity: low
---

<intent-contract>

## Intent

**Problem:** No service has OpenTelemetry. Nobody can see how long a screen takes to build, which variants are served, or which components are dropped. The engine's drop reporter is a no-op (story 5 deferral), and there is no local backend to look at traces or metrics.

**Approach:** Bootstrap the OTel SDK with OTLP exporters in every service command, set up in one small shared package. Add `grafana/otel-lgtm` to Compose on free ports. Instrument the BFF screen engine:
- one span per screen build and one per section, with `sdui.*` attributes;
- Snapshot child spans;
- metrics: variants served, components dropped, build latency, purchases by class, and suitability-mismatch alerts.

## Boundaries & Constraints

**Always:**
- Load `golang-how-to` first and apply the Go skills in `/workspace/repos/advisor-radar/CLAUDE.md`.
- **Shared package.** `internal/telemetry` (not `utils`) exposes `Setup(ctx, serviceName) (shutdown func(context.Context) error, err error)`. It:
  - configures a tracer provider and a meter provider with OTLP exporters, gRPC or HTTP, whichever the pinned modules make simpler;
  - uses the W3C trace-context and baggage propagators;
  - sets a resource with `service.name` set in code per command. `OTEL_SERVICE_NAME` is only a process-level override, never set in `.env`.
- **Disabled mode.** When `OTEL_EXPORTER_OTLP_ENDPOINT` is unset, `Setup` installs no-op or unexported providers, so tests and local runs without a collector have no errors and no dials.
- **Commands.** Every command under `cmd/` calls `Setup` and calls shutdown on graceful exit, bounded by a timeout.
- **Dependencies.** Add the OTel modules `go.opentelemetry.io/otel`, `sdk`, `sdk/metric`, the OTLP trace and metric exporters, and the `otelhttp`/`otelgrpc` contrib. Pin current releases compatible with Go 1.26, and make sure `go mod tidy` is clean.
- **Transport instrumentation.**
  - BFF HTTP handlers are wrapped with `otelhttp`, and route patterns are used as span names.
  - gRPC clients and servers get the `otelgrpc` stats handlers. This is the tracing half of the "gRPC interceptors cover tracing" rule.
- **Screen spans.**
  - One span per build, `sdui.screen`, with the attributes `sdui.slug` and `sdui.revision`.
  - One child span per section, `sdui.section`, with `sdui.section` and `sdui.variant`. An omitted section records `sdui.omitted_reason`.
  - Snapshot source calls are child spans of the screen span, named per source.
- **Metrics** (names as in architecture.md):
  - `sdui_variant_served_total{slug,section,variant}` (counter);
  - `sdui_component_dropped_total{slug,type,reason}` (counter), fed by wiring the engine's drop reporter;
  - `sdui_screen_build_seconds{slug}` (histogram);
  - `pov_purchases_total{asset_class}` (counter, in the BFF on `202` purchase or in account-sim on commit; pick one and document it);
  - `advisory_suitability_alerts_total` (counter, in advisory when the `perfil` rule fires).
  - Label values are bounded enums only: never customer ids or copy.
- **Compose.** Add a `grafana/otel-lgtm` service to `compose.yaml` with a pinned image tag. Grafana is published on **3410**, and OTLP gRPC/HTTP on **4417**/**4418** of the host (container 4317/4318). Do not use 3000, which belongs to Cybersecurity.
- **Docs.** Add the new ports to the repository port table in `AGENTS.md`/`CLAUDE.md`. That table is the repository's and the architecture requires it. Document `OTEL_EXPORTER_OTLP_ENDPOINT` in the README configuration section (example `http://127.0.0.1:4418` or the gRPC equivalent).
- **Tests.**
  - Unit tests use the SDK's in-memory span recorder (`tracetest`) and a manual metric reader. They assert:
    - span names, attributes, and parent/child relationships for one home build;
    - an omitted section's attribute;
    - variant-served and dropped counters, including a `Build` error;
    - that the build histogram records.
  - A test that `Setup` with no endpoint returns a working no-op shutdown.
- **Live check** (recommended). Start lgtm, run the BFF with the endpoint, request a home, and confirm the trace appears in Grafana (Tempo) and the counter in Prometheus/Mimir via the lgtm UI or API. Record the result.
- **Logging.** Never record secrets or copy in attributes. Logs stay `slog` JSON.

**Never:**
- No trace-context propagation through RabbitMQ headers or outbox rows in this story. It needs a traceparent column in every outbox; record it as deferred.
- No change to response bodies.
- Unit tests do not dial 5435/5673/8400/8420/9201/4417/4418.
- Do not read or edit `.env` / `.env.*`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Home build | Thiago, all OK | span `sdui.screen` (slug home, revision v1) with 5 child `sdui.section` spans + snapshot child spans; `sdui_variant_served_total` +1 per section | — |
| Omitted section | timeline down | section span with `sdui.omitted_reason=timeline` | — |
| Build error | a variant's Build fails | `sdui_component_dropped_total{reason="build_error"}` +1 | — |
| No endpoint | env unset | services start, no export errors, no dials | — |
| Purchase | `202` | `pov_purchases_total{asset_class}` +1 | — |

</intent-contract>

## Code Map

- `cmd/*/main.go` -- six services; add `Setup`/shutdown.
- `internal/screen/{engine,snapshot}.go` -- spans, drop reporter, metrics.
- `internal/bff/{http,screen,clients,account}.go` -- otelhttp, otelgrpc on clients.
- `internal/advisory/rules.go`/`apply.go` -- suitability counter.
- `compose.yaml`, `AGENTS.md`, `CLAUDE.md`, `README.md`.

## Tasks & Acceptance

**Execution:**
- `internal/telemetry` and its wiring into every command.
- Screen spans and metrics, with tests.
- Transport instrumentation.
- Compose, port tables, README.

**Acceptance Criteria:**
- Given lgtm running and the BFF exporting, when a home is requested, then one trace shows the screen span with per-section and per-source children, and `sdui_variant_served_total` increases.
- Given the change, when the Go suite runs without an endpoint, then all tests pass without dialing any collector.

## Verification

**Commands:**
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean after the intended additions are committed in the same change (diff limited to the OTel modules).
- `docker compose config --quiet` -- expected: valid.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 30 findings — high 0, medium 6, low 21, false 1, maybe-false 2
- findings:
  - `[medium]` `patch` BFF accepts untrusted browser traceparent as parent — otelhttp.WithPublicEndpoint on the BFF handler.
  - `[low]` `patch` gRPC DeadlineExceeded/Canceled classified as "error" — failureClass also checks status codes; table rows added.
  - `[low]` `patch` ErrUnknownCustomer (404) marks the account-sim snapshot span Error — status skipped for unknown customer.
  - `[low]` `reject` Canceled/404 builds recorded in the build histogram — needs an outcome label or branches; low everyday impact.
  - `[medium]` `patch` SSE streams become connection-long server spans — otelhttp filter skips /stream routes; SSE-through-wrapper test added.
  - `[low]` `patch` ErrPartialResource from malformed OTEL_RESOURCE_ATTRIBUTES stops services — tolerated, partial resource kept.
  - `[low]` `reject` No startup log line for export on/off — adds surface; README documents the variable.
  - `[medium]` `defer` Logs not correlated with traces (no trace_id in slog) — beyond this story's intent; recorded in deferred.
  - `[medium]` `defer` Jev, Elasticsearch and actions HTTP clients untraced; consumers start no spans — recorded in deferred.
  - `[medium]` `defer` RabbitMQ/outbox trace propagation not recorded as a tracked deferral — excluded by intent; recorded in deferred.
  - `[medium]` `patch` Purchase counter never tested through Apply (replay/refusal) — TestApply_CountsPurchaseByClass added.
  - `[low]` `reject` Asset-class allow-list duplicates the catalog — the catalog's three classes are fixed constants; bounded label intended.
  - `[medium]` `patch` Tests leave global OTel state changed, order-dependent under -shuffle — globals restored with t.Cleanup.
  - `[low]` `reject` Grafana/OTLP ports bound on all interfaces — repository rule binds dev services to 0.0.0.0 for port forwarding; local demo only.
  - `[low]` `reject` Dependency bumps beyond OTel modules — forced by minimal version selection of the pinned OTel releases; gen-proto output unchanged.
  - `[low]` `reject` Duplicated Setup/Stop block in six mains — a few lines per command; a helper adds indirection for no behavior change.
  - `[low]` `patch` (dup of 2) gRPC deadline span status — same fix.
  - `[low]` `patch` (dup of 3) 404 account-sim span status — same fix.
  - `[low]` `patch` Heading build failure sets no span status — screen span gets the error status.
  - `[low]` `reject` tp and mp shutdown share one 5 s budget — only matters with an unreachable collector at exit.
  - `[low]` `reject` Per-signal endpoint variables do not enable export — only OTEL_EXPORTER_OTLP_ENDPOINT is documented.
  - `[maybe-false]` `reject` Scheme-less endpoint falls back to localhost:4318 — exporter behavior not verified; would only be low; README shows the http:// form.
  - `[low]` `patch` Setup tests read per-signal endpoint env from the shell — t.Setenv clears them.
  - `[maybe-false]` `reject` Go suite could dial a collector if per-signal env is set — covered by the previous patch; would only be low.
  - `[medium]` `patch` Suitability counter not tested on the withSuitability path — raise case with purchase and empty decisions added.
  - `[low]` `patch` (dup of 11) Purchase counter not verified through Apply — same test.
  - `[low]` `patch` BFF otelhttp wrapper and SSE through it untested — bff tests for the route-pattern span name and a text/event-stream response.
  - `[low]` `defer` Canceled build span status unasserted — recorded in deferred.
  - `[low]` `reject` Intent audit: drop counter counted inside the engine instead of through the DropReporter seam — the counter the intent names is fed and tested; the seam is an implementation detail.
  - `[false]` `reject` Intent audit: purchase counter in account-sim differs from the matrix "202" row — the intent explicitly allows "in account-sim on commit; pick one and document it", and the README documents it.

## Auto Run Result

**Summary:** OpenTelemetry now runs in every service through `internal/telemetry.Setup`:
- OTLP/HTTP export, W3C trace-context and baggage propagators;
- no-op when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset;
- `otelhttp` on the BFF, as a public endpoint with the SSE routes filtered out;
- `otelgrpc` on every gRPC client and server.

The screen engine emits:
- spans `sdui.screen`, `sdui.section` and `sdui.snapshot.<source>`;
- metrics `sdui_variant_served_total`, `sdui_component_dropped_total` and `sdui_screen_build_seconds`.

account-sim counts `pov_purchases_total{asset_class}` on commit, and advisory counts `advisory_suitability_alerts_total`. Compose adds `grafana/otel-lgtm:0.34.0` with Grafana on 3410 and OTLP on 4417/4418.

**Files:**
- `internal/telemetry/*`
- `internal/screen/{telemetry,engine,snapshot}.go` + tests
- `internal/sim/metrics.go`, `internal/sim/account.go`
- `internal/advisory/metrics.go`, `internal/advisory/apply.go`
- `internal/bff/{http,clients,account,timeline}.go` + `http_telemetry_test.go`
- all `cmd/*/main.go`
- `compose.yaml`, `compose_test.go`
- `go.mod`, `go.sum`
- `AGENTS.md`, `CLAUDE.md`, README, `specs/http/bff.md`

**Review:** 30 findings. 12 patched (4 medium, 8 low). 4 deferred, all in frontmatter: RabbitMQ propagation, untraced HTTP clients, log/trace correlation, and the canceled-span assertion. 14 rejected, with the reasons in the triage log.

**Followup review recommended:** yes. Four medium entries were patched: the public endpoint, the SSE filter, global test state, and the counters on real paths.

**Verification:**
- gofmt, vet and build are clean.
- `go test -race -shuffle=on ./...` passes.
- The gated pgx tests for `internal/sim` and `internal/advisory` passed before the review patches.
- `go mod tidy` is clean.
- web 192/192 passes with 100% coverage of `src/sdui/**`.
- The live Grafana check was not recorded. The lgtm container was started, but the full stack was not run in this session.

**Residual risks:**
- Nothing checks the live export to Tempo and Mimir.
- A heading-failure span status is untested.
- Trace context stops at RabbitMQ.
