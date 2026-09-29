# Advisor Radar

Fictional advisory desk. A visitor can open the team queue or enter Orla Invest as one seeded client and cause the same events the team already sees.

The public app is rooted at `/advisor-radar`.

## Selection

`/advisor-radar` is the selection screen. It does not redirect to the queue.

- **Visão do time** opens `/advisor-radar/fila`. Review, the manager panel, and the customer 360 stay at `/advisor-radar/revisao`, `/advisor-radar/painel`, and `/advisor-radar/clientes/{id}`.
- **Visão do cliente** opens `/advisor-radar/client-pov`.

The screen states three pillars (proactive alerts, message triage, cases with SLA) and plays an eight-step complaint walkthrough. The walkthrough pauses and stays still when `prefers-reduced-motion` is set.

## Client point of view

`/advisor-radar/client-pov` lists three seeded clients:

| Client | Segment | Assets | Available cash |
|---|---|---:|---:|
| Mariana Costa | Singular | USD 248,300 | USD 60,000 |
| Fernanda Lima | Essencial | USD 8,200 | USD 1,148 |
| Thiago Azevedo | Advance | USD 68,000 | USD 60,520 |

The phone app can deposit, withdraw, send a free message, or file a preset complaint. Desktop actions use a 440px panel. The client app is dark by default and can switch to the light theme. The team list and the selection screen stay light.

An accepted action returns `202` with `event_id`. The confirmation protocol is the first two UUID groups of that id, uppercased. Bastidores follows `GET /v1/client-pov/customers/{id}/stream`. The queue link is `/advisor-radar/fila`.

Money on the POV wire is integer USD cents. Schema version 1 events stay whole dollars. Schema version 2 and 3 account events are cents; advisory converts them into the dollar book before the phase-1 rules run. Schema version 3 is the purchase (`kind: aplicacao`, from `POST /v1/client-pov/customers/{id}/purchases`): cash moves into a position at the catalog price, so `before` equals `after`, and the payload adds `product_id`, `asset_class`, and `risk`. A purchase whose risk is above the client's investor profile raises the alert-only `perfil` card "Compra acima do perfil de investidor" in the team queue; it opens no case. The phase-2 commands stay at version 2.

Names, balances, and Orla Invest are fictional. No message is sent to a real person.

## Demo script

1. Open `/advisor-radar` and walk the eight steps in about 30 seconds.
2. Enter as Mariana Costa and send "Estou pensando em sair".
3. Show Bastidores moving through the outbox, RabbitMQ, and triage.
4. Open `/advisor-radar/fila`. The churn alert is at the top of the queue with the Singular SLA.
5. Return as Fernanda Lima, deposit US$ 10.000, and show the large-deposit and segment-change alerts.
6. Submit the same withdrawal twice with the same idempotency key and show a single event.
7. Press "Avançar um dia" three times. On day 3 Cobalto Semicondutores falls 53,5%: Mariana's home shows "Mariana, sua carteira caiu 15,5% hoje" with the day-change pill, the team queue shows her `queda` alert, and Thiago, who holds little Cobalto, gets no alert.

The simulated day is global. The day-3 shock hits every account that holds Cobalto, not only the demo clients, so a non-demo account with a large Cobalto position may raise its own drop alert. Advancing is limited to 20 days per 10 minutes for everyone; a reseed (`go run ./cmd/db seed`) returns the day to 0 and the values to the seed.

## Phase 3 demo

Phase 3 makes the client app server-driven. The BFF composes each screen (Início, Investir, Carteira, Perfil) for the client's moment, and web only renders what it receives. The seven steps below walk the phase-3 success scenarios on the local stack. The [Demo script](#demo-script) above stays the phase-1 and phase-2 walk. Its deposit and advance-day steps return here, seen through the server-driven screens.

### Run the stack

You need Docker, Go 1.26, Node.js 24, and pnpm 11. The Dev Container pins all of them. Run every command from the repository root.

Start the infrastructure and wait until it is healthy:

```bash
docker compose up -d --wait postgres rabbitmq elasticsearch otel-lgtm   # otel-lgtm only for the optional Grafana step
```

Then write the variables below into your local `.env` (never committed). Each service reads its variables from the process environment first and then from `.env` in the working directory. The database URLs point at the `account_sim`, `advisory`, `triage`, and `cases` databases on PostgreSQL 5435, and the broker URLs point at RabbitMQ on 5673. Both use the Compose default credentials in `compose.yaml` (`POSTGRES_USER`/`POSTGRES_PASSWORD` and `RABBITMQ_DEFAULT_USER`/`RABBITMQ_DEFAULT_PASS`), which `.env` can override. [Configuration](#configuration) shows the URL shapes.

The addresses in the table are the ones this script was checked with. Other ports work under these rules:

- Each `*_GRPC_TARGET` matches its `*_GRPC_ADDR` (for example `ACCOUNT_SIM_GRPC_TARGET=127.0.0.1:8460`).
- `ADVISORY_HTTP_URL` matches the port of `ADVISORY_HTTP_ADDR` (`http://127.0.0.1:8410`).
- `BFF_HTTP_ADDR` stays on 8400, because the Vite proxy and the step-7 `curl` call the BFF there.

| Service | Variables |
|---|---|
| account-sim | `ACCOUNT_SIM_DATABASE_URL`, `ACCOUNT_SIM_BROKER_URL`, `ACCOUNT_SIM_GRPC_ADDR` (`0.0.0.0:8460`) |
| advisory | `ADVISORY_DATABASE_URL`, `ADVISORY_BROKER_URL`, `ADVISORY_GRPC_ADDR` (`0.0.0.0:8430`), `ADVISORY_HTTP_ADDR` (`0.0.0.0:8410`), `ACCOUNT_SIM_GRPC_TARGET` |
| triage | `TRIAGE_DATABASE_URL`, `TRIAGE_BROKER_URL`, `TRIAGE_GRPC_ADDR` (`0.0.0.0:8440`), `AI_GATEWAY_API_KEY` (optional: without it, or when the model does not answer, classification falls back to the heuristic and is marked degraded) |
| cases | `CASES_DATABASE_URL`, `CASES_BROKER_URL`, `CASES_GRPC_ADDR` (`0.0.0.0:8450`), `ADVISORY_GRPC_TARGET` |
| timeline-indexer | `ELASTICSEARCH_URL` (`http://127.0.0.1:9201`), `TIMELINE_BROKER_URL`, `TIMELINE_GRPC_ADDR` (`0.0.0.0:8420`). At startup it also replays the outboxes behind `ACCOUNT_SIM_DATABASE_URL`, `ADVISORY_DATABASE_URL`, and `CASES_DATABASE_URL`. |
| bff | `BFF_HTTP_ADDR` (`0.0.0.0:8400`, where Vite proxies `/advisor-radar/v1`), `BFF_BROKER_URL`, `ACCOUNT_SIM_GRPC_TARGET`, `ADVISORY_GRPC_TARGET`, `ADVISORY_HTTP_URL` (`http://127.0.0.1:8410`), `TRIAGE_GRPC_TARGET`, `CASES_GRPC_TARGET`, `TIMELINE_GRPC_TARGET` |
| every service | `OTEL_EXPORTER_OTLP_ENDPOINT` (`http://127.0.0.1:4418`), optional, for the Grafana step |

Apply the migrations, load the seed, and install the web dependencies:

```bash
go run ./cmd/db migrate
go run ./cmd/db seed
pnpm --dir web install --frozen-lockfile
```

Start the six services, one terminal each, then the web app:

```bash
go run ./cmd/account-sim
go run ./cmd/advisory
go run ./cmd/triage
go run ./cmd/cases
go run ./cmd/timeline-indexer
go run ./cmd/bff
pnpm --dir web dev   # http://127.0.0.1:3400/advisor-radar/
```

**Reseed before each walk.** `go run ./cmd/db seed` returns the simulated day to 0 and resets balances, positions, preferences (beta off), and investor profiles. It only upserts seed rows, so what an earlier walk added stays:

- A case opened from the app stays open, and Mariana's home then opens on `case_open` instead of `portfolio_review`. Close it in `/advisor-radar/fila` first: press "Iniciar atendimento →", "Aguardar cliente →", and "Marcar resolvido →" on her case.
- The team queue, "Atividade recente", and "Movimentações" keep the events of earlier walks.
- Known issue: once a case opened from the app passes its SLA, timeline-indexer no longer starts. cases writes a `case.sla.breached` row to its outbox whatever the case state, so closing the case does not prevent it. For the step-4 case, that happens 30 minutes after step 4. The timeline-indexer outbox replay then stops on that row (`timeline: unsupported event "case.sla.breached"`), and reseeding does not clear it. On a cases database with no earlier breach, walk step 7 within 30 minutes of step 4. After a breach, every later start of timeline-indexer needs this workaround until the bug is fixed: `CASES_DATABASE_URL= go run ./cmd/timeline-indexer`. With the empty value, it skips the cases replay, and cases reach the customer 360 only live.

### Script

1. **Selection screen.** Open <http://127.0.0.1:3400/advisor-radar/>. The header has the "Arquitetura" and "Server-Driven UI" ("SDUI" on a phone) anchors (the second marked "Novo"), and the band "Novo nesta versão · Server-Driven UI" ends in "Ver como funciona", which jumps to `#sdui` ("O backend monta a tela de cada cliente"). Switch between "Fernanda · Essencial", "Thiago · Advance", and "Mariana · Singular". Each tab shows the live BFF response ("200 · 1 requisição"), the moment chosen by advisory with the rule that picked it ("4 de 7 · segment_upgrade_near · advisory", "5 de 7 · idle_cash · advisory", "6 de 7 · portfolio_review · advisory"), and the home the app draws from it. The three homes differ.
2. **Three homes and Raio-X.** Open the "Visão do cliente" card ("Escolher cliente", `/advisor-radar/client-pov`) and enter as each client. In the simulation strip, press "Raio-X SDUI" ("Raio-X" on a phone). The banner shows the request and "slug home · revision v1 · schema 1 · 5 seções", and each section is outlined with `id · type · variant`:
   - Fernanda Lima: `moment · moment_card · segment_upgrade_near`, "Fernanda, faltam US$ 1.800,00 para o Advance".
   - Thiago Azevedo: `moment · moment_card · idle_cash`, "Thiago, 89% do seu patrimônio está em caixa".
   - Mariana Costa: `moment · moment_card · portfolio_review`, "Mariana, sua revisão de carteira está disponível", with `advisor · advisor_card · dedicated`.

   Raio-X stays on in this browser until you turn it off.
3. **Segment upgrade.** As Fernanda, press "Depositar", choose "US$ 10.000", and press "Confirmar depósito". "Depósito solicitado" shows the protocol and Bastidores for `account.event.recorded` (`kind: aporte · amount: 10000`). Back on Início, the moment is `segment_upgraded`, "Fernanda, você agora é cliente Advance", with "Patrimônio total" at US$ 18.200,00. In `/advisor-radar/fila`, Fernanda has a "Mudança de segmento" card next to "Aporte grande".
4. **Complaint and case.** As Mariana, press "Reclamar", pick "Estou pensando em sair", and press "Enviar reclamação". "Reclamação registrada" shows Bastidores for `message.received` up to "Classificado pela triagem". Her Início now opens on `case_open`: "Sua reclamação está com a Ana Paula Ribeiro" and "Como cliente Singular, você recebe resposta em até 1 h.", with the case protocol. In the queue, her case is in the "Aberto" column on the Singular clock. It shows 00:30:00 because triage scored a churn risk, which halves the 1 h base. Use the "Estou pensando em sair" preset. Triage classifies the message: the model when `AI_GATEWAY_API_KEY` is set, otherwise the heuristic. Only a triaged complaint, closing request, or churn risk of 0.5 or more opens a case. Other presets can be classified as another intent (for example "Minha transferência está atrasada" as `resgate`). They then reach the queue as a card and open no case.
5. **Purchases.** As Thiago, open Investir. Under ETFs, pick "Maré Ações Globais ETF", press "Tudo", and confirm with "Confirmar compra de US$ 60.520,00". "Compra enviada" shows Bastidores `account.event.recorded · kind aplicacao · schema 3`. No rule fires, so "Na fila da assessoria, se uma regra disparar" stays "aguardando". Início now opens on `welcome` ("Olá, Thiago. Sua conta está em dia."): `idle_cash` is gone and "Disponível para saque" is US$ 0,00. Carteira lists the ETF position at "Aplicado US$ 65.720,00", which is the purchase plus his seeded US$ 5.200,00 in the same ETF, and shows the purchase under "Movimentações".
   Then, as Fernanda, open Investir. Farol Saúde and Cobalto Semicondutores carry "Acima do seu perfil". Pick Cobalto Semicondutores. The form shows "Este produto tem risco 5. Seu perfil é conservador, que vai até risco 2. Você pode investir mesmo assim, e a sua assessora será avisada.", and the confirm button stays enabled. Confirm the default US$ 1.000,00. Bastidores reaches "Regra: compra acima do perfil" and "Na fila da assessoria", and the queue shows her "Compra acima do perfil" card (kind `perfil`). No case opens.
6. **Market day.** As Mariana, press "Avançar um dia" in the simulation strip ("+1 dia" on a phone) three times, until "Dia simulado 3". The day is global, so every client moves with it. Her Início shows `portfolio_drop`, "Mariana, sua carteira caiu 15,5% hoje", and `wealth · wealth_summary · with_day_change` with the pill "−US$ 38.520,00 (−15,5%) no dia 3". The drop outranks her open case. The queue shows her "Queda de patrimônio" card (`queda`). Carteira reads "Valores de mercado no dia simulado 3": "Patrimônio total" US$ 209.780,00, Cobalto Semicondutores at US$ 33.480,00 (−44,2% on US$ 60.000,00 applied), and "Reavaliação diária" with "dia simulado 3 · Cobalto Semicondutores −53,5%" under "Movimentações".
7. **Isolated failure and versioning.**
   - Stop timeline-indexer (Ctrl+C) and reload Mariana's Início. The home still answers `200` and renders. With Raio-X on, the banner ends in "4 seções · timeline fora do ar", and a dashed `activity · activity_list · omitido` placeholder reads "O timeline-indexer não respondeu a tempo e o bff tirou a seção." Start timeline-indexer again, and the section returns.
   - Ask for a schema the BFF does not serve. This `curl` calls the BFF directly on 8400, not through the Vite proxy path `/advisor-radar/v1`, for Mariana's home (`01a0e3a4-9a44-7566-b5de-eb2e365799f8`). The other customer ids are in the app URL and in `GET /v1/client-pov/customers`:

     ```bash
     curl -i -H 'X-SDUI-Schema: 2' \
       http://127.0.0.1:8400/v1/client-pov/customers/01a0e3a4-9a44-7566-b5de-eb2e365799f8/screens/home
     ```

     It answers `406 Not Acceptable` with `{"supported":{"min":1,"max":1}}`. `X-SDUI-Schema: 1` or `1-2` answers `200`, and a malformed value answers `400`.
   - As Mariana, open Perfil and turn on "Programa beta". It reads "Ligado. Você recebe a revision v2 do início antes dos outros clientes." Início then reports "revision v2 · schema 1 · 6 seções", with `highlights · product_rail · profile_moderado` right after the moment. Turning it off brings back `v1`.
   - Optionally, with `OTEL_EXPORTER_OTLP_ENDPOINT` set, open Grafana on <http://127.0.0.1:3410> (the local login and `OTEL_METRIC_EXPORT_INTERVAL` are under [Observability](#observability); metrics export every 60 s by default, so counters can lag), go to Explore, choose Tempo, and run `{ name = "sdui.screen" }`. Each trace is the BFF route span with one `sdui.screen` span (`sdui.slug`, `sdui.revision`), one `sdui.snapshot.<source>` child per source, and one `sdui.section` span per section with `sdui.variant`, continuing into account-sim, advisory, cases, and timeline-indexer. In Prometheus, `sdui_variant_served_total` counts each variant served, and `sdui_component_dropped_total{reason="timeline"}` counts the failure above. `pov_purchases_total` and `advisory_suitability_alerts_total` count the step-5 purchases and the `perfil` alert.

To finish, close Mariana's case in the queue as described under "Reseed before each walk", then run `go run ./cmd/db seed` to put the day, balances, positions, and preferences back.

### Decisions and contract

- [ADR 0008: client commands over gRPC with outbox publish](specs/adr/0008-client-command-grpc-outbox.md)
- [ADR 0009: Server-Driven UI composed in the BFF](specs/adr/0009-server-driven-ui-in-the-bff.md)
- [ADR 0010: individual per-product positions in account-sim](specs/adr/0010-individual-positions.md)
- [BFF HTTP contract, screens, and components](specs/http/bff.md)
- [Design reference](docs/design/sdui-full-pov/)
- [`add-sdui-variant` skill](.claude/skills/add-sdui-variant/SKILL.md) for adding a section variant

## Configuration

Services read their settings from environment variables, loaded from the local root `.env` (never committed). Client commands reach account-sim over gRPC ([ADR 0008](specs/adr/0008-client-command-grpc-outbox.md)); add these two lines to your local `.env`:

| Variable | Example | Purpose |
|---|---|---|
| `ACCOUNT_SIM_DATABASE_URL` | `postgres://…@127.0.0.1:5435/account_sim` | account-sim PostgreSQL database (state, idempotency keys, outbox). Unset, account-sim only waits for a signal. |
| `ACCOUNT_SIM_BROKER_URL` | `amqp://…@127.0.0.1:5673/` | RabbitMQ the outbox relay publishes to. Unset, the relay does not run. |
| `ACCOUNT_SIM_GRPC_ADDR` | `0.0.0.0:8460` | account-sim gRPC listen address. The server starts only when `ACCOUNT_SIM_DATABASE_URL` is also set. |
| `ACCOUNT_SIM_GRPC_TARGET` | `127.0.0.1:8460` | account-sim target the BFF and advisory dial. The BFF requires it for the client POV; unset, the POV home is `404` and POV commands are `502`. advisory reads each client's balance through it for `GetMomentFacts`, with the caller's deadline or 2 s; unset, advisory logs a warning and `GetMomentFacts` answers `Unavailable`, so the home moment falls back to `welcome`. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://127.0.0.1:4418` | OTLP/HTTP collector every service exports traces and metrics to; `4418` is the `otel-lgtm` Compose service. Unset, OpenTelemetry stays off: no exporter, no dial, no export errors. Only OTLP over HTTP is supported, so point it at an HTTP port (`4418`), not the gRPC one (`4417`). |

`ACCOUNT_SIM_TEST_DATABASE_URL` points the gated `internal/sim` PostgreSQL tests at a database; each test migrates and drops its own schema. Unset, those tests skip. `CASES_TEST_DATABASE_URL` and `ADVISORY_TEST_DATABASE_URL` do the same for the gated `internal/cases` and `internal/advisory` tests.

cases consumes `message.triaged` on `cases.message.triaged` (dead letters go to `cases.message.triaged.dlq`) when both `CASES_BROKER_URL` and `ADVISORY_GRPC_TARGET` are set; with the broker set and no advisory target it logs a warning and runs without intake. Only messages sent from the client app open or join cases: account-sim marks POV messages and complaints with `origin: "client_app"` on `message.received`, triage copies it into `message.triaged`, and seeded or burst messages (no `origin`) are claimed and ignored. A client-app complaint, closing request, or churn risk of 0.5 or more opens one case per customer, with the segment and advisor read from advisory `GetCustomer`; a later qualifying message while that case is open is added to its history. Run `go run ./cmd/db migrate` to apply `cases/003_one_open_case.sql`.

After `go run ./cmd/db migrate` applies `account_sim/004_pov_positions.sql`, run `go run ./cmd/db seed` (reseed): 004 drops the per-class balance columns and leaves the product catalog, positions, and registration tables empty until the seed loads them.

After `go run ./cmd/db migrate` applies `account_sim/005_pov_preferences.sql`, run `go run ./cmd/db seed` (reseed) to write each client's preferences row; until then Perfil shows `chat` with beta off for every client.

After `go run ./cmd/db migrate` applies `advisory/003_investor_profile.sql`, run `go run ./cmd/db seed` (reseed): 003 gives every existing book row the migration default profile (conservador, assessed 2026-01-01) until the seed writes each customer's profile.

`account_sim/006_pov_sim_day.sql` adds the global simulated day, starting at 0, and `advisory/004_revaluation.sql` the latest revaluation per client; neither needs a reseed.

Deploy advisory before cases. Cases intake reads the advisor from advisory `GetCustomer.advisor_id`; against an advisory that does not send it yet, cases drops every qualifying message as `ErrUnusableCustomer` (dead-lettered, no case opened).

### Observability

`docker compose up -d otel-lgtm` starts Grafana with Tempo, Mimir, and Loki behind one OpenTelemetry collector. Grafana is on <http://127.0.0.1:3410> (user `admin`, password `admin`, local only); OTLP is on `4417` (gRPC) and `4418` (HTTP) of the host. With `OTEL_EXPORTER_OTLP_ENDPOINT` set, each service (`account-sim`, `advisory`, `triage`, `cases`, `timeline-indexer`, `bff`) exports under its own `service.name`, set in code; `OTEL_SERVICE_NAME` exported in the shell overrides it for one process and never goes in `.env`. The standard `OTEL_*` exporter variables apply, for example `OTEL_METRIC_EXPORT_INTERVAL=5000` to export metrics every 5 s instead of every 60 s. Shutdown flushes the last batch within 5 s.

- **Traces.** BFF HTTP requests are server spans named after the route pattern (`GET /v1/client-pov/customers/{id}/screens/{slug}`). Every gRPC client and server carries the `otelgrpc` stats handler, so a query from the BFF continues in advisory, cases, account-sim, or timeline. A screen build is one `sdui.screen` span (`sdui.slug`, `sdui.revision`) with one `sdui.snapshot.<source>` child per Snapshot source and one `sdui.section` child per section (`sdui.section`, `sdui.variant`, and `sdui.omitted_reason` when omitted). Trace context does not cross RabbitMQ yet.
- **Metrics.** `sdui_variant_served_total{slug,section,variant}`, `sdui_component_dropped_total{slug,type,reason}`, and `sdui_screen_build_seconds{slug}` from the BFF screen engine; `pov_purchases_total{asset_class}` from account-sim when a purchase (`aplicacao`) commits, so replays and refusals do not count; `advisory_suitability_alerts_total` from advisory when the suitability-mismatch (`perfil`) rule raises an alert. Labels are bounded: catalog keys, source names, and catalog asset classes, never customer ids or copy.

`scripts/gen-proto.sh` regenerates every `proto/*/v1/*.proto` into `gen/`. It requires protoc 29.3, protoc-gen-go v1.36.5, and protoc-gen-go-grpc 1.5.1, and adds the Go install directory (`GOBIN`, else `$(go env GOPATH)/bin`) to `PATH` for the plugins.

## Contract

The BFF HTTP contract, including the POV routes, is in [specs/http/bff.md](specs/http/bff.md). To add a variant to an existing screen section, follow the [`add-sdui-variant` skill](.claude/skills/add-sdui-variant/SKILL.md). Client commands are recorded in [specs/adr/0008-client-command-grpc-outbox.md](specs/adr/0008-client-command-grpc-outbox.md). OpenTelemetry is described under [Observability](#observability).
