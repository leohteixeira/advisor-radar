# BFF HTTP contract

The browser talks only to the BFF. Paths below are the BFF paths. The Vite app prefixes them with `/advisor-radar`.

The BFF stores nothing durable. POV progress lives in an in-memory hub. Requests are traced with OpenTelemetry (README "Observability"), but no response carries a `trace_id`: trace context does not cross RabbitMQ yet, so Bastidores shows `event_id` and the steps the hub has observed.

## Team routes

| Method | Path | Result |
|---|---|---|
| GET | `/v1/queue` | JSON `{"items":[...],"facets":...}` |
| GET | `/v1/queue/stream` | `text/event-stream`, event name `signal` |
| GET | `/v1/cases` | Open cases |
| POST | `/v1/cases/{id}/advance` | Next case state |
| GET | `/v1/actions` | Advisor actions |
| PUT | `/v1/actions/{id}` | Contatado or Adiar 1 h |
| DELETE | `/v1/actions/{id}` | Undo |
| GET | `/v1/customers/{id}` | Customer book row |
| GET | `/v1/customers/{id}/timeline` | Customer 360 rows. Each row may carry `source`, the routing key of its event, and `occurred_at`, the event time in RFC 3339; `ago` stays as indexed |
| GET | `/v1/review` | Triage review queue |
| PUT | `/v1/review/{id}` | Correct an intent |
| GET | `/v1/manager` | Manager snapshot |

A queue alert item carries `alert`, the advisory kind (`saque`, `queda`, `aporte`, `segmento`, `contato`, or `perfil`), with `rule` and `reason`. A `perfil` item ("Compra acima do perfil de investidor") also carries `amount` in dollars, `product_id`, `risk`, `profile`, and `max_risk`; its `reason` names the product from the account-sim catalog, for example "Compra de US$ 1.000,00 em Cobalto Semicondutores, risco 5. Perfil conservador vai até risco 2.", and falls back to the `product_id`, then "produto", when the catalog is unavailable; without a `risk` it leaves the risk out. The BFF reads the catalog with a 300 ms deadline and keeps the names after the first successful read; a failed read is retried on the next render. The motivo facet labels it "Compra acima do perfil". A `perfil` alert opens no case.

## POV routes

Money fields are integer USD cents. Every POST requires `Idempotency-Key`. A missing key is `400`. A new command spends a budget of 10 per minute and 40 per 24 hours for that customer. A replay of a known key does not spend the budget and does not append a second event. Over the limit the response is `429` and account-sim is not called. A withdrawal or purchase above caixa is `422` and publishes nothing.

| Method | Path | Result |
|---|---|---|
| GET | `/v1/client-pov/customers` | The three clients: name, segment, assets, SLA, advisor, since from the book, hint |
| GET | `/v1/client-pov/customers/{id}` | Home: assets, caixa, allocation, advisor, since from the book, activity, messages |
| POST | `/v1/client-pov/customers/{id}/deposits` | `202` `{"event_id"}`. Body: `amount`, `origin` |
| POST | `/v1/client-pov/customers/{id}/withdrawals` | `202`, or `422` when amount exceeds caixa. Body: `amount`, `destination` |
| POST | `/v1/client-pov/customers/{id}/messages` | `202`. Body: `channel` `chat` or `e-mail`, `text` |
| POST | `/v1/client-pov/customers/{id}/complaints` | `202`. Body: `text`. Channel is chat |
| POST | `/v1/client-pov/customers/{id}/purchases` | `202` `{"event_id"}`, or `422` `{"error":"insufficient"}` when `amount_cents` exceeds caixa, or `422` `{"error":"invalid"}` for a bad body, an unknown product, or an amount at or below 0, above the command maximum, or below the product minimum. Body: `product_id`, `amount_cents` |
| GET | `/v1/client-pov/customers/{id}/stream` | SSE event `bastidores` with `event_id` and steps `feito`, `agora`, or `aguardando` |
| GET | `/v1/client-pov/counters` | `actions` by type (`deposit`, `withdrawal`, `message`, `complaint`, `purchase`), `refusals` by rule, `duplicates` |

`202` has `event_id` and no protocol field. The UI formats the protocol as the first two UUID groups, uppercased.

Deposit and withdrawal become `account.event.recorded` at schema version 2. A purchase becomes `account.event.recorded` `kind: aplicacao` at schema version 3: `amount` is the purchase, `before` and `after` both equal the patrimony (cash moves into a position at the catalog price), and the payload adds `product_id`, `asset_class`, and `risk` from the catalog row. Consumers accept versions 1, 2, and 3 and read v3 money as cents, like v2. An undecodable purchase body is `422 invalid` and spends no budget; account-sim refusals (`insufficient`, `invalid`) spend it, as for withdrawals. The Bastidores stream follows a purchase like the other account events. Message and complaint become `message.received` with `origin: "client_app"` in the payload (seeded and burst messages omit `origin`); triage copies it into `message.triaged`. The BFF does not publish. account-sim writes state and the outbox row in one transaction.

## Screens (phase 3)

The client app gets each screen from the BFF as a page of sections and components, already filled for that client. Web renders by `type` and computes no percentage, currency, date, validation, or SLA. The decision is ADR 0009.

| Method | Path | Result |
|---|---|---|
| GET | `/v1/client-pov/customers/{id}/screens/{slug}` | `200` screen envelope. `slug` is `home`, `investir`, `carteira`, or `perfil` |

- The BFF serves `home` now. `investir`, `carteira`, and `perfil` answer `404` until their stories land and add them here.
- The web home renders from this screen route. The phase-2 home route `GET /v1/client-pov/customers/{id}` stays: web still reads the shell (name, segment) and the coded panels (cash, messages) from it, and falls back to it when the screen request fails or answers something that is not an envelope, until story 10 removes that fallback.
- Opening a screen is one HTTP request. There is no pagination and no single-section reload.
- A slug outside the four is `404`. An unknown customer is `404` only when account-sim answers `NotFound` for that customer. Any other source failure never changes the status.
- Screen responses carry `Cache-Control: no-store`.
- `X-SDUI-Schema` is not implemented yet: the BFF ignores the header until story 16 adds it. When it lands, it behaves as follows. `X-SDUI-Schema` is an optional request header with the range of `schema_version` values the client renders: one integer (`1`) or an inclusive range (`1-2`). A range needs `1 ≤ min ≤ max`. The BFF serves only its current `schema_version`, never a lower one. Without the header, it serves that version. When the range excludes it, the answer is `406 Not Acceptable` with the supported range in the body, `{"supported":{"min":1,"max":1}}`. A malformed header or an invalid range is `400`. On `406`, web shows "Atualize o app para ver esta tela." with a reload button; the simulation strip and the tabs stay.

### Envelope

Thiago's home with the timeline down. Only the `moment` section is shown; `wealth`, `actions`, and `advisor` follow it in the real response, and `activity` is the one omitted section. Without the timeline, the `idle_cash` body cannot date the idle cash and leaves the days out.

```json
{
  "schema_version": 1,
  "slug": "home",
  "revision": "v1",
  "title": "Olá, Thiago",
  "subtitle": "Cliente Advance desde 2024",
  "sections": [
    { "id": "moment", "components": [
      { "type": "moment_card", "variant": "idle_cash",
        "props": { "kicker": "Caixa parado",
                   "title": "Thiago, 89% do seu patrimônio está em caixa",
                   "body": "US$ 60.520,00 parados em caixa. Veja produtos para o seu perfil arrojado.",
                   "tone": "info",
                   "icon": "cash",
                   "action": { "type": "navigate", "label": "Ver produtos", "target": "investir" } } } ] }
  ],
  "omitted": [
    { "id": "activity", "type": "activity_list", "reason": "timeline" }
  ]
}
```

- `schema_version` is an integer, the version of this envelope and of the component table below.
- A section object has `id` and `components`, an array of `{type, variant, props}`.
- `omitted` is always present: one `{id, type, reason}` per catalog section this response left out, in catalog order, so Raio-X can draw a placeholder. `reason` is the failed source (`account-sim`, `advisory`, `timeline`, `moments`, `profile`, or `cases`) or `build_error`. `advisory` is the customer read; `moments` and `profile` are the advisory moment facts and investor profile. Web renders nothing for an omitted section outside Raio-X.
- `title` and `subtitle?` are the screen heading, filled from the catalog: home "Olá, {{first}}" / "Cliente {{segment}} desde {{since}}", investir "Investir" / "Produtos fictícios · preço fixo da simulação", carteira "Carteira" / "Valores de mercado no dia simulado {{day}}", perfil "Perfil" / "Seus dados, seu perfil de investidor e suas preferências".
- `revision` names the catalog revision of the screen. A client outside the beta gets `v1`. A beta client gets home `v2`, which inserts the `highlights` section right after `moment`. Investir, Carteira, and Perfil stay at `v1`.
- Section order in the array is the render order. Web owns style, the breakpoint grid, and the span of each section.
- `variant` is informational for web: it drives Raio-X and telemetry, never rendering logic. A new variant of an existing type needs no web change. A new `type` needs web code and a row in the table below.
- An unknown `type` renders nothing and is reported: web logs it with `console.error`. A component that throws is caught by its own boundary. The rest of the screen stays.

### Values

- Money, percentages, dates, and counts arrive as display strings formatted in Go, for example `"US$ 48.210,00"`, `"62%"`, `"− US$ 38.502,00 (−15,5%) no dia 3"`, and `"12/03/2026"`.
- A value a form needs as a number is also sent as an integer USD cents field named `*_cents`.
- Every visible label is a prop, so copy changes need no web change. Copy is Portuguese.
- Negative signs use U+2212 "−", not the hyphen-minus.
- Relative times read "agora" (under a minute), "há 5 min", "há 3 h", "ontem" (24 to 48 hours), and "há 4 dias".
- Shares use largest-remainder rounding in Go, so the allocation shares of one component sum to 100%.
- `tone` is `pos`, `neg`, `info`, `gold`, or `neutral`. Each type lists the tones it accepts. Web maps tone to color, and renders an unknown `tone` as `neutral`.
- `icon` is an optional semantic name from the web icon set (`in`, `out`, `msg`, `alert`, `segment`, `drop`, `inbox`, `cash`, `calendar`, `spark`, `deposit`, `withdraw`). An unknown name renders no icon.
- `bar_width` is a number from 0 to 100, computed in Go. Web uses it as a width and never derives it.
- A field marked `?` may be absent.

### Actions

An action is an object with `type` and `label`. There are four types:

- **navigate** opens another SDUI screen of the same customer: `{"type":"navigate","label":"Ver carteira","target":"carteira"}`. `target` is one of the four slugs.
- **panel** opens a coded panel: `{"type":"panel","label":"Depositar","target":"deposit"}`. `target` is `deposit`, `withdraw`, `message`, `complaint`, or `purchase`. A `purchase` panel also carries `product_id`.
- **note** shows a notice in place: `{"type":"note","label":"Saiba mais","text":"…"}`.
- **link** opens an external URL: `{"type":"link","label":"…","href":"https://…"}`.

- An unknown action `type` or `target` hides the control and is reported with `console.error`.
- A `purchase` panel without `product_id` is hidden.
- `link.href` must use `https:` and opens with `rel="noopener noreferrer"`. Any other scheme is refused and the control is hidden.

The `deposit`, `withdraw`, `message`, and `complaint` panels submit through the POV routes above. The purchase route `/v1/client-pov/customers/{id}/purchases` exists now, but the web purchase form ships in story 10; until then web keeps the `purchase` panel hidden. The over-cash check runs inside the account-sim transaction and answers `422`. Capping a form's input at a `*_cents` value the BFF sent is only a form convenience.

### Failure policy

- The BFF reads account-sim, advisory (customer, moment facts, and investor profile, each its own source), cases, and timeline in parallel under one screen deadline. One source failing never cancels the others.
- A section whose source failed or ran past the deadline falls back to its default variant when the default does not need that source. Example: `moment` becomes `welcome` when moment facts fail.
- A section that depends only on the failed source is omitted, and so is a section whose default also needs the failed source. Example: `activity` without timeline.
- When cases fails, `case_open` does not match and moment evaluation continues down the priority list.
- When the advisory customer read fails, the moments whose copy names the advisor (`case_open` and `portfolio_review`) do not match and evaluation continues.
- When the moment facts fail, only `case_open` and `welcome` can match.
- When the beta flag is unavailable, the BFF serves revision `v1`.
- When the investor profile fact fails, `idle_cash` does not match, and the sections whose variant depends on it (`highlights` and `suitability`) are omitted.
- `title` and `subtitle` fall back to "Olá" and no subtitle when their source fails.
- A `Build` error drops only that component. A section left with zero components is omitted with reason `build_error`.
- Every omitted section is listed in `omitted` with its reason.
- The response is still `200` with the remaining sections. When every section is omitted, it is `200` with `"sections": []`.

### Home moment priority

Advisory evaluates each moment condition and returns the result as a moment fact. Cases supplies the open-case fact. The BFF compares no threshold and computes no segmentation; it only applies this fixed priority order to the facts, top to bottom, for the home `moment` section.

| # | Variant | Fact | Evaluated by |
|---:|---|---|---|
| 1 | `portfolio_drop` | The latest `reavaliacao` loss is above 15% of patrimony before the revaluation | advisory |
| 2 | `case_open` | The client has an open case (cases, filtered by customer) | cases |
| 3 | `segment_upgraded` | A live POV account event (`schema_version` 2 or later) raised a segment upgrade for the client in the last 24 h, to the segment the book holds now | advisory |
| 4 | `segment_upgrade_near` | The book segment is Essencial and `750000 ≤ patrimony_cents ≤ 1000000` (at or below US$ 10.000,00 a client is still Essencial); the gap is `1000000 − patrimony_cents`, at least 1 | advisory |
| 5 | `idle_cash` | Patrimony is above 0 and cash is at or above 50% of patrimony | advisory |
| 6 | `portfolio_review` | The client is Singular | advisory |
| 7 | `welcome` | Default, including when moment facts fail | none |

A segment downgrade has no variant and falls through the list. The most recent live segment alert in the window decides, so a later live downgrade cancels an earlier upgrade, and so does a reseed that moved the book back.

Moment tones: `portfolio_drop` is `neg`; `case_open` and `idle_cash` are `info`; `segment_upgraded` and `segment_upgrade_near` are `gold`; `portfolio_review` and `welcome` are `neutral`.

### Sections per screen

The catalog's starting order. The server may change it without a web change.

- **home:** `moment`, `wealth`, `actions`, `advisor`, `activity`. Revision `v2` adds `highlights` after `moment`.
- **investir:** `cash`, `highlights`, `fixed_income`, `etfs`, `stocks`.
- **carteira:** `summary`, `allocation`, `positions_stocks`, `positions_etf`, `positions_fixed_income`, `history`. A class with no position has no section.
- **perfil:** `header`, `suitability`, `registration`, `preferences`, `advisor`.

### Home `v1` served now

The BFF serves home `v1` with every moment variant of the priority list except `portfolio_drop`, which comes with the market-day story, as do `with_day_change` and revision `v2`. The screen deadline is 800 ms.

| Section | Type | Variants in evaluation order | Source | When the source fails |
|---|---|---|---|---|
| `moment` | `moment_card` | `case_open`, `segment_upgraded`, `segment_upgrade_near`, `idle_cash`, `portfolio_review`, `welcome` | cases, advisory, moments, profile, timeline (per variant, below) | the variant is skipped; `welcome` needs nothing, so the section is always rendered |
| `wealth` | `wealth_summary` | `default` | account-sim | omitted, reason `account-sim` |
| `actions` | `action_grid` | `default` | none | always rendered |
| `advisor` | `advisor_card` | `dedicated` (Singular), `default` | advisory | omitted, reason `advisory` |
| `activity` | `activity_list` | `recent` (at least one row), `empty` | timeline | omitted, reason `timeline` |

- `moment`: one card, the first variant of the list whose sources answered and whose fact holds. Every `{{first}}` title has a form without the name for when the advisory customer read fails ("Faltam …", "Você agora é …", "Sua revisão …", "89% do seu …").
  - `case_open` (cases and advisory): kicker "Atendimento em andamento", title "Sua reclamação está com a {{advisor}}", body "Como cliente {{segment}}, você recebe resposta em até {{sla}}." with the book segment's catalog SLA (a segment without one skips the variant), meta "Protocolo {{protocol}} · aberto {{age}}", `tone` `info`, `icon` `inbox`, action "Ver conversa" → `panel` `message`. It shows the most recently opened case that is not "Resolvido". `protocol` is the first 13 characters of the case id, uppercased (`01A0E3A5-2F4C`); `age` is the relative time since the case opened ("há 3 min").
  - `segment_upgraded` (moments): kicker "Novo segmento", title "{{first}}, você agora é cliente {{segment}}", body "Sua assessoria passa a responder em até {{sla}}." with the upgraded segment and its SLA (a segment without one skips the variant), `tone` `gold`, `icon` `segment`, action "Ver produtos" → `navigate` `investir`.
  - `segment_upgrade_near` (moments): kicker "Perto do Advance", title "{{first}}, faltam {{gap}} para o Advance" with the advisory gap in cents formatted as money, body "A partir de {{threshold}} você vira cliente Advance, com resposta da assessoria em até {{sla}}." with the Essencial bound formatted as money ("US$ 10.000,00") and the Advance catalog SLA, `tone` `gold`, `icon` `segment`, action "Depositar" → `panel` `deposit`.
  - `idle_cash` (moments and profile): kicker "Caixa parado", title "{{first}}, {{cash_share}} do seu patrimônio está em caixa", body "{{cash}} parados há {{idle_days}}. Veja produtos para o seu perfil {{profile}}.", `tone` `info`, `icon` `cash`, action "Ver produtos" → `navigate` `investir`. `cash_share` is the largest-remainder share of cash in the advisory patrimony, `profile` is lowercase, and `idle_days` ("1 dia", "4 dias") is the whole days since the newest `account.event.recorded` row of the timeline, at least 1. When the timeline failed or has no such row, the body is "{{cash}} parados em caixa. Veja produtos para o seu perfil {{profile}}."
  - `portfolio_review` (moments and advisory): kicker "Sua assessora", title "{{first}}, sua revisão de carteira está disponível", body "A {{advisor}} separou 30 minutos nesta semana para revisar a carteira com você.", `tone` `neutral`, `icon` `calendar`, action "Conversar" → `panel` `message`.
  - `welcome` (none): kicker "Tudo em dia", title "Olá, {{first}}. Sua conta está em dia." ("Olá. Sua conta está em dia." without advisory), body "Quando algo mudar na sua carteira, você vê aqui primeiro.", `tone` `neutral`, `icon` `spark`, no action.
  - On day 0 of the seed, Fernanda gets `segment_upgrade_near`, Thiago `idle_cash`, and Mariana `portfolio_review`. After Fernanda deposits US$ 10.000 she gets `segment_upgraded`; after Mariana files a complaint she gets `case_open`.
- `wealth`: `total_label` "Patrimônio total" with the patrimony account-sim reports, `cash_label` "Disponível para saque", and one allocation row per non-zero class in the order Ações (`stocks`), ETFs (`etfs`), Renda fixa (`fixed_income`), Caixa (`cash`). `bar_width` equals the rounded share.
- `actions`: Depositar (`deposit` icon), Sacar (`withdraw`), Mensagem (`msg`), and Reclamar (`alert`), each a `panel` action.
- `advisor`: `name` is the advisory book advisor, `initials` its first two name initials, `meta` "Resposta em até {{sla}} · cliente {{segment}}" with the catalog SLA per segment (Essencial 24 h, Advance 4 h, Singular 1 h), and action "Conversar" → `panel` `message`. An unknown segment is a `build_error`.
- `activity`: title "Atividade recente". `recent` holds the five most recent client-facing timeline rows as `{icon, title, meta}`. A row is client-facing when its timeline `source` is `account.event.recorded` or `message.received`; rows from `alert.raised`, `message.triaged`, `case.*`, advisor notes, and calls never reach the client, and neither does a row without `source`. `meta` is the relative time from the row's `occurred_at` to the request time, or from the indexed `ago` when `occurred_at` is absent. Icons are `aporte` `in`, `saque` and `aplicacao` `out`, and `mensagem` `msg`; any other kind has no icon. With no client-facing row, `empty` carries `items: []` and the empty text.

### Components (15 types)

Shared objects used in the table:

- **Product:** `product_id`, `name`, `class_label` (`"ETF"`), `risk` (integer 1–5, for the bars), `risk_label` (`"Risco 3 de 5"`), `return_label` (`"+11,2% em 12 meses"`), `minimum` (`"Mínimo US$ 50"`), `minimum_cents`, `above_profile` (boolean), `badge?` (`"Acima do seu perfil"`, set only when above profile), `warning?` (the above-profile text for the purchase form, set only when above profile), `action` (`panel` `purchase` with `product_id`).
- **Allocation row:** `class` (`stocks`, `etfs`, `fixed_income`, or `cash`; web maps it to a color), `label`, `share` (`"62%"`), `bar_width`.
- **Activity item:** `icon?`, `title`, `meta`, `value?` (signed money), `tone?` (`pos`, `neg`, or `neutral`).
- **Stat:** `label`, `value`, `tone?` (`pos`, `neg`, or `neutral`).

The Variants column is not evaluation order; the catalog holds each section's ordered list. Each section's default variant: `moment` is `welcome`; `wealth` and `summary` are `default`; `advisor` is `default`; `activity` is `recent` or `empty`, chosen by item count; `history` is `history`; every other section has a single variant, or the profile variant for `highlights` and `suitability`, and that is its default.

| Type | Sections | Variants | Props |
|---|---|---|---|
| `moment_card` | home `moment` | `portfolio_drop`, `case_open`, `segment_upgraded`, `segment_upgrade_near`, `idle_cash`, `portfolio_review`, `welcome` (default) | `kicker`, `title`, `body`, `meta?` (`case_open`: "Protocolo … · aberto há …"), `tone` (`neg`, `info`, `gold`, or `neutral`), `icon?`, `action?` (`welcome` has none) |
| `wealth_summary` | home `wealth` | `default`, `with_day_change` | `total_label`, `total`, `cash_label`, `cash`, `cash_cents`, `allocation` (allocation rows); `with_day_change` adds `day_change` (`"− US$ 38.502,00 (−15,5%) no dia 3"`) and `day_change_tone` (`pos`, `neg`, or `neutral` at zero). Masking values is web presentation only |
| `action_grid` | home `actions` | `default` | `items`: `[{label, icon?, action}]`, four `panel` actions: Depositar `deposit`, Sacar `withdraw`, Mensagem `message`, Reclamar `complaint` |
| `advisor_card` | home `advisor`, perfil `advisor` | `default`, `dedicated` (Singular) | `kicker` ("Sua assessora" or "Sua assessora dedicada"), `name`, `initials`, `meta` ("Resposta em até … · cliente …"), `action?` (`panel` `message`) |
| `activity_list` | home `activity`, carteira `history` | `recent`, `empty`, `history` | `title`, `items` (activity items; empty for `empty`), `empty_text?` (`empty`: "Suas movimentações aparecem aqui assim que acontecerem."; `history` with no items: "Nenhuma movimentação ainda.") |
| `invest_summary` | investir `cash` | `default` | `cash_label`, `cash`, `cash_cents`, `profile_chip` ("Perfil moderado") |
| `product_rail` | investir `highlights`; home `highlights` in beta revision `v2` | `profile_conservador`, `profile_moderado`, `profile_arrojado` | `title` ("Para o seu perfil …"), `subtitle`, `products` (products) |
| `product_list` | investir `fixed_income`, `etfs`, `stocks` | `fixed_income`, `etf`, `stocks` | `title` ("Renda fixa", "ETFs", or "Ações"), `products` (products) |
| `portfolio_summary` | carteira `summary` | `default`, `with_day_change` | `total_label`, `total`, `stats` (stats: "Valor aplicado", "Rentabilidade" signed with percentage, "Caixa", "Dia simulado"); `with_day_change` adds `day_change` and `day_change_tone` |
| `allocation_breakdown` | carteira `allocation` | `default` | `title`, `rows`: allocation rows plus `value` (money) for Ações, ETFs, Renda fixa, and Caixa |
| `position_list` | carteira `positions_stocks`, `positions_etf`, `positions_fixed_income` | `stocks`, `etf`, `fixed_income` | `title`, `subtotal`, `applied_label` ("Aplicado"), `items`: `[{product_id, name, applied, value, return, return_tone}]`, `return` signed percentage, `return_tone` `pos`, `neg`, or `neutral` at zero |
| `profile_header` | perfil `header` | `default` | `initials`, `name`, `subtitle` ("Cliente … desde …"), `account` ("Conta 3301-7 · Orla Invest") |
| `profile_scale` | perfil `suitability` | `conservador`, `moderado`, `arrojado` | `title`, `subtitle`, `current_label` ("Seu perfil"), `levels`: `[{key, label, description, limit, max_risk, current}]` for conservador, moderado, and arrojado, `limit` "Produtos até risco …", `max_risk` integer for the bars, `current` boolean; `footer` ("Última avaliação em …") |
| `profile_field_list` | perfil `registration` | `default` | `title`, `fields`: `[{label, value}]` for Nome, E-mail, Telefone (masked), Cidade, Segmento, Cliente desde; `footnote` |
| `preference_list` | perfil `preferences` | `default` | `title`; `theme`: `{label, hint}`, local to the browser; `channel`: `{label, hint, value, options: [{value, label}]}`, `value` `chat` or `email`; `beta`: `{label, hint, enabled}`, `hint` for the current state |

`preference_list` shows the stored channel and beta flag. Its channel vocabulary is `chat` or `email`, distinct from the messages route, which takes `chat` or `e-mail`. The route that writes them arrives with the Perfil story and is documented here then. Web reloads the screen after a change. Later stories that refine a prop list update this table in the same change.
