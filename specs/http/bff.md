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
| GET | `/v1/customers/{id}/timeline` | Customer 360 rows. Each row may carry `source`, the routing key of its event, and `occurred_at`, the event time in RFC 3339; `ago` stays as indexed. An account row may also carry `product_id` (a purchase) and `amount_cents`, the event amount in USD cents (v1 dollars are scaled by 100; an amount that is not a number or is past 2^53 cents is left out) |
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
| PUT | `/v1/client-pov/customers/{id}/preferences` | `200` `{"channel","beta"}` with the stored values. Body: `channel` `chat` or `email`, `beta` boolean, both required. `400` for a bad id, `404` for an unknown customer, `422` `{"error":"invalid"}` for a bad body or channel, `502` when account-sim fails |
| GET | `/v1/client-pov/customers/{id}/stream` | SSE event `bastidores` with `event_id` and steps `feito`, `agora`, or `aguardando` |
| GET | `/v1/client-pov/counters` | `actions` by type (`deposit`, `withdrawal`, `message`, `complaint`, `purchase`), `refusals` by rule, `duplicates` |
| GET | `/v1/client-pov/simulation` | `200` `{"sim_day"}`: the global simulated day, for the simulation strip. `502` when account-sim fails |
| POST | `/v1/client-pov/simulation/advance-day` | `202` `{"sim_day", "event_id"}`: the day after the advance and the first `reavaliacao` event id (one per account; the others are not returned). No body. `429` `{"error":"ten_minutes"}` over the limit; `502` when account-sim fails |

`202` has `event_id` and no protocol field. The UI formats the protocol as the first two UUID groups, uppercased.

Advance-day is global, not per customer. It requires `Idempotency-Key` (`400` without) and has its own budget of 20 advances per 10 minutes, shared by every caller and separate from the per-customer budget. A replay of a known key answers the original reply, advances nothing, publishes nothing, and does not spend the budget. Over the limit the response is `429` and account-sim is not called. A failure that may have committed in account-sim spends the budget; only `Unavailable`, left after the client retries, refunds it. In one transaction account-sim adds 1 to the day and writes one `account.event.recorded` `kind: reavaliacao` outbox row per account, at schema version 3: `amount` is `after − before` in signed cents, `before` and `after` the patrimony at the old and the new day, `sim_day` the new day, `product_id` the position whose value moved the most (empty when nothing moved), `product_change_bp` that product's day change in signed basis points (Cobalto on day 3: `-5350`), and `epoch` the simulation epoch (a UUID every reseed replaces). advisory keeps a client's latest revaluation and replaces it only with one from another epoch or a later day, so a redelivered older day is ignored. The event id is a name-based UUID of `reavaliacao:{customer_id}:{sim_day}` within the current seed epoch, so a reseed starts new ids. A reseed also forgets the advance keys, so a key used before it advances the new epoch instead of replaying the old day. Positions are never rewritten: account-sim values them at read time from the stored day, and every command (deposit, withdrawal, purchase) values them at that day too. The only scripted price move is Cobalto, −53,5% from day 3 on; every other product is flat.

Deposit and withdrawal become `account.event.recorded` at schema version 2. A purchase becomes `account.event.recorded` `kind: aplicacao` at schema version 3: `amount` is the purchase, `before` and `after` both equal the patrimony (cash moves into a position at the catalog price), and the payload adds `product_id`, `asset_class`, and `risk` from the catalog row. Consumers accept versions 1, 2, and 3 and read v3 money as cents, like v2. An undecodable purchase body is `422 invalid` and spends no budget; account-sim refusals (`insufficient`, `invalid`) spend it, as for withdrawals. The Bastidores stream follows a purchase like the other account events. Message and complaint become `message.received` with `origin: "client_app"` in the payload (seeded and burst messages omit `origin`); triage copies it into `message.triaged`. The BFF does not publish. account-sim writes state and the outbox row in one transaction.

The preferences PUT is idempotent and takes no `Idempotency-Key`. The BFF reads the stored preferences first: a PUT that changes nothing answers `200` and spends no budget; a change spends the same per-customer budget as a command (`429` over it, and account-sim is not written). An `Unavailable` account-sim failure refunds the budget, as for commands. account-sim stores the choice in `pov_preferences` (migration `account_sim/005_pov_preferences.sql`); a reseed restores `chat` with beta off. No outbox row, no event, and no counter: Bastidores never shows a preference change. Its channel vocabulary is `chat` or `email`, distinct from the messages route, which takes `chat` or `e-mail`.

## Screens (phase 3)

The client app gets each screen from the BFF as a page of sections and components, already filled for that client. Web renders by `type` and computes no percentage, currency, date, validation, or SLA. The decision is ADR 0009.

| Method | Path | Result |
|---|---|---|
| GET | `/v1/client-pov/customers/{id}/screens/{slug}` | `200` screen envelope. `slug` is `home`, `investir`, `carteira`, or `perfil`. `400` or `406` from `X-SDUI-Schema` (below) |

- The BFF serves all four screens: `home`, `investir`, `carteira`, and `perfil`.
- The web home, Investir, Carteira, and Perfil render from this screen route. The phase-2 home route `GET /v1/client-pov/customers/{id}` stays: web still reads the shell (name, segment) and the coded panels (cash, messages) from it. There is no fallback screen: when the screen request fails, times out, or answers something that is not an envelope, the screen area shows an error state with a retry, and the shell, tabs, and panels stay.
- Opening a screen is one HTTP request. There is no pagination and no single-section reload.
- A slug outside the four is `404`. An unknown customer is `404` only when account-sim answers `NotFound` for that customer. Any other source failure never changes the status.
- Screen responses carry `Cache-Control: no-store`.
- `X-SDUI-Schema` is an optional request header with the range of `schema_version` values the client renders: one integer (`1`) or an inclusive range (`1-2`), digits only, with the whitespace around the value trimmed. A range needs `1 ≤ min ≤ max`. The BFF serves only its current `schema_version`, which is `1`, never a lower one. Without the header, it serves that version. When the range excludes it, the answer is `406 Not Acceptable` with the supported range in the body, `{"supported":{"min":1,"max":1}}`. A malformed header, a repeated header, or an invalid range (`2-1`, `abc`, `0`) is `400` `{"error":"invalid_schema_range"}`. The checks run in a fixed order: a bad customer id is `400`, then the header (`400` or `406`), then the slug (`404`), so an unknown slug with `X-SDUI-Schema: 2` is `406`. No source is read before the header passes; both header answers carry `Cache-Control: no-store`.
- Web sends `X-SDUI-Schema: 1` on every screen request. On `406`, and on a `200` whose `schema_version` is outside the range it renders, web shows "Atualize o app para ver esta tela." with a button that reloads the page; the simulation strip and the tabs stay.

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
- `omitted` is always present: one `{id, type, reason}` per catalog section this response left out because a source failed or a build broke, in catalog order, so Raio-X can draw a placeholder. `reason` is the failed source (`account-sim`, `catalog`, `registration`, `preferences`, `advisory`, `timeline`, `moments`, `profile`, or `cases`) or `build_error`. `account-sim` is the account read, `catalog` the account-sim product catalog (`ListProducts`), and `registration` and `preferences` the account-sim `GetRegistration` and `GetPreferences` reads; `advisory` is the customer read; `moments` and `profile` are the advisory moment facts and investor profile. Web renders nothing for an omitted section outside Raio-X. A section that is absent by design, such as a Carteira class with no position, is neither in `sections` nor in `omitted`.
- `title` and `subtitle?` are the screen heading, filled from the catalog: home "Olá, {{first}}" / "Cliente {{segment}} desde {{since}}", investir "Investir" / "Produtos fictícios · preço fixo da simulação", carteira "Carteira" / "Valores de mercado no dia simulado {{day}}", perfil "Perfil" / "Seus dados, seu perfil de investidor e suas preferências".
- `revision` names the catalog revision of the screen. It is chosen per request from the customer's stored beta flag (the account-sim preferences, read in the same Snapshot): a beta client gets home `v2`, which inserts the `highlights` section right after `moment`, and every other client gets `v1`. Investir, Carteira, and Perfil have only `v1`. Web never chooses the revision; it renders what arrives.
- Section order in the array is the render order. Web owns style, the breakpoint grid, and the span of each section.
- `variant` is informational for web: it drives Raio-X and telemetry, never rendering logic. A new variant of an existing type needs no web change. A new `type` needs web code and a row in the table below.
- An unknown `type` renders nothing and is reported: web logs it with `console.error`. A component that throws is caught by its own boundary. The rest of the screen stays.

### Values

- Money, percentages, dates, and counts arrive as display strings formatted in Go, for example `"US$ 48.210,00"`, `"62%"`, `"−US$ 38.520,00 (−15,5%) no dia 3"`, and `"12/03/2026"`.
- A value a form needs as a number is also sent as an integer USD cents field named `*_cents`.
- Every visible label is a prop, so copy changes need no web change. Copy is Portuguese.
- Negative signs use U+2212 "−", not the hyphen-minus.
- A signed amount puts its sign right before the currency, with no space: "+US$ 19.400,00", "−US$ 250,00", and "US$ 0,00" at zero. A signed percentage has one decimal, rounded half away from zero: "+11,5%", "−0,4%", and "0,0%" at zero or when there is nothing to measure against.
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
- A `purchase` panel without `product_id` is hidden and reported with `console.error`.
- `link.href` must use `https:` and opens with `rel="noopener noreferrer"`. Any other scheme is refused and the control is hidden.

The `deposit`, `withdraw`, `message`, and `complaint` panels submit through the POV routes above. The `purchase` panel opens the coded purchase form, which reads the product and `invest_summary.cash_cents` from the Investir envelope on view and submits `POST /v1/client-pov/customers/{id}/purchases` `{product_id, amount_cents}`; that route is documented under the POV commands above. The form keeps one `Idempotency-Key` per open form and amount: a retry of the same amount reuses it, and a changed amount, a reopened form, a `202`, or a definite `4xx` makes a new one. It shows `minimum` and keeps Confirm disabled below `minimum_cents`. The refusals it maps are `422` `{"error":"insufficient"}` ("Valor acima do caixa disponível.") and `422` `{"error":"invalid"}`, which includes an amount below the product minimum ("Confira o valor e tente de novo."); `429` and any other failure show the notices of the other panels. The over-cash check runs inside the account-sim transaction and answers `422`. Capping a form's input at a `*_cents` value the BFF sent, and the minimum check, are only form conveniences.

### Failure policy

- The BFF reads account-sim (the account, the product catalog, the registration, and the preferences, each its own source), advisory (customer, moment facts, and investor profile, each its own source), cases, and timeline in parallel under one screen deadline. One source failing never cancels the others.
- A section whose source failed or ran past the deadline falls back to its default variant when the default does not need that source. Example: `moment` becomes `welcome` when moment facts fail.
- A section that depends only on the failed source is omitted, and so is a section whose default also needs the failed source. Example: `activity` without timeline.
- When cases fails, `case_open` does not match and moment evaluation continues down the priority list.
- When the advisory customer read fails, the moments whose copy names the advisor (`case_open` and `portfolio_review`) do not match and evaluation continues.
- When the moment facts fail, only `case_open` and `welcome` can match.
- When the beta flag is unavailable (the preferences read failed), the home is revision `v1`. The preferences read is fetched but never required, so the fallback is silent in the response: no section is omitted, the screen does not fail, and no failed part is logged. It is visible only as an error on the `sdui.snapshot.preferences` source span.
- When the investor profile fact fails, `idle_cash` does not match, and the sections whose variant depends on it (`highlights` and `suitability`) are omitted. On Investir, `cash` then has no `profile_chip` and every product carries `above_profile: false` with no `badge` or `warning`.
- When the catalog fails, the Investir `highlights`, `fixed_income`, `etfs`, and `stocks` sections are omitted with reason `catalog`.
- `title` and `subtitle` fall back to "Olá" and no subtitle when their source fails. The subtitle's source is the one its fields read: advisory for the home fields, account-sim for the Carteira `{{day}}`. A subtitle whose source failed is left out and the title stays; a heading with no field (Investir) keeps its title and subtitle.
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

- **home:** `moment`, `wealth`, `actions`, `advisor`, `activity`. Revision `v2` (beta) is `moment`, `highlights`, `wealth`, `actions`, `advisor`, `activity`.
- **investir:** `cash`, `highlights`, `fixed_income`, `etfs`, `stocks`.
- **carteira:** `summary`, `allocation`, `positions_stocks`, `positions_etf`, `positions_fixed_income`, `history`. A class with no position has no section.
- **perfil:** `header`, `suitability`, `registration`, `preferences`, `advisor`.

### Home `v1` served now

The BFF serves home `v1` with every moment variant of the priority list and the `with_day_change` wealth. Beta clients get home `v2`, below. The screen deadline is 800 ms.

| Section | Type | Variants in evaluation order | Source | When the source fails |
|---|---|---|---|---|
| `moment` | `moment_card` | `portfolio_drop`, `case_open`, `segment_upgraded`, `segment_upgrade_near`, `idle_cash`, `portfolio_review`, `welcome` | moments, advisory, catalog, cases, profile, timeline (per variant, below) | the variant is skipped; `welcome` needs nothing, so the section is always rendered |
| `wealth` | `wealth_summary` | `with_day_change` (the day moved), `default` | account-sim | omitted, reason `account-sim` |
| `actions` | `action_grid` | `default` | none | always rendered |
| `advisor` | `advisor_card` | `dedicated` (Singular), `default` | advisory | omitted, reason `advisory` |
| `activity` | `activity_list` | `recent` (at least one row), `empty` | timeline | omitted, reason `timeline` |

- `moment`: one card, the first variant of the list whose sources answered and whose fact holds. Every `{{first}}` title has a form without the name for when the advisory customer read fails ("Faltam …", "Você agora é …", "Sua revisão …", "89% do seu …").
  - `portfolio_drop` (moments and advisory; catalog for the product name): advisory reports `portfolio_drop` when the customer's latest `reavaliacao` is for the current simulated day (the day account-sim reports) and lost more than 15% of the patrimony before it, with `drop_bp` (the loss in positive basis points), `drop_product_id`, `drop_product_bp` (signed), and `drop_day`. Kicker "Sua carteira hoje", title "{{first}}, sua carteira caiu {{drop_pct}} hoje" ("Sua carteira caiu …" without the name), body "{{product}} recuou {{product_pct}} no dia simulado {{day}}. A {{advisor}} já foi avisada e vai falar com você.", `tone` `neg`, `icon` `drop`, action "Ver carteira" → `navigate` `carteira`. `drop_pct` and `product_pct` are unsigned with one decimal ("15,5%", "53,5%"); `product` is the catalog name. When the catalog fails or does not name the product, the product did not fall (`drop_product_bp` ≥ 0), or advisory has no advisor, the variant is skipped. On the next flat day the fact no longer holds, so Mariana is back to `portfolio_review` (or `case_open` with an open case).
  - `case_open` (cases and advisory): kicker "Atendimento em andamento", title "Sua reclamação está com a {{advisor}}", body "Como cliente {{segment}}, você recebe resposta em até {{sla}}." with the book segment's catalog SLA (a segment without one skips the variant), meta "Protocolo {{protocol}} · aberto {{age}}", `tone` `info`, `icon` `inbox`, action "Ver conversa" → `panel` `message`. It shows the most recently opened case that is not "Resolvido". `protocol` is the first 13 characters of the case id, uppercased (`01A0E3A5-2F4C`); `age` is the relative time since the case opened ("há 3 min").
  - `segment_upgraded` (moments): kicker "Novo segmento", title "{{first}}, você agora é cliente {{segment}}", body "Sua assessoria passa a responder em até {{sla}}." with the upgraded segment and its SLA (a segment without one skips the variant), `tone` `gold`, `icon` `segment`, action "Ver produtos" → `navigate` `investir`.
  - `segment_upgrade_near` (moments): kicker "Perto do Advance", title "{{first}}, faltam {{gap}} para o Advance" with the advisory gap in cents formatted as money, body "A partir de {{threshold}} você vira cliente Advance, com resposta da assessoria em até {{sla}}." with the Essencial bound formatted as money ("US$ 10.000,00") and the Advance catalog SLA, `tone` `gold`, `icon` `segment`, action "Depositar" → `panel` `deposit`.
  - `idle_cash` (moments and profile): kicker "Caixa parado", title "{{first}}, {{cash_share}} do seu patrimônio está em caixa", body "{{cash}} parados há {{idle_days}}. Veja produtos para o seu perfil {{profile}}.", `tone` `info`, `icon` `cash`, action "Ver produtos" → `navigate` `investir`. `cash_share` is the largest-remainder share of cash in the advisory patrimony, `profile` is lowercase, and `idle_days` ("1 dia", "4 dias") is the whole days since the newest `account.event.recorded` row of the timeline that is not a `reavaliacao`, at least 1. When the timeline failed or has no such row, the body is "{{cash}} parados em caixa. Veja produtos para o seu perfil {{profile}}."
  - `portfolio_review` (moments and advisory): kicker "Sua assessora", title "{{first}}, sua revisão de carteira está disponível", body "A {{advisor}} separou 30 minutos nesta semana para revisar a carteira com você.", `tone` `neutral`, `icon` `calendar`, action "Conversar" → `panel` `message`.
  - `welcome` (none): kicker "Tudo em dia", title "Olá, {{first}}. Sua conta está em dia." ("Olá. Sua conta está em dia." without advisory), body "Quando algo mudar na sua carteira, você vê aqui primeiro.", `tone` `neutral`, `icon` `spark`, no action.
  - On day 0 of the seed, Fernanda gets `segment_upgrade_near`, Thiago `idle_cash`, and Mariana `portfolio_review`. After Fernanda deposits US$ 10.000 she gets `segment_upgraded`; after Mariana files a complaint she gets `case_open`.
- `wealth`: `total_label` "Patrimônio total" with the patrimony account-sim reports, `cash_label` "Disponível para saque", and one allocation row per non-zero class in the order Ações (`stocks`), ETFs (`etfs`), Renda fixa (`fixed_income`), Caixa (`cash`). `bar_width` equals the rounded share. `with_day_change` matches when account-sim reports `day_change_cents` ≠ 0 (patrimony at the day minus patrimony the day before) and adds `day_change` "{{signed money}} ({{signed pct}}) no dia {{day}}", for example "−US$ 38.520,00 (−15,5%) no dia 3", and `day_change_tone` `neg` or `pos`. The percentage is of the patrimony the day before, with one decimal and U+2212 for a loss.
- `actions`: Depositar (`deposit` icon), Sacar (`withdraw`), Mensagem (`msg`), and Reclamar (`alert`), each a `panel` action.
- `advisor`: `name` is the advisory book advisor, `initials` its first two name initials, `meta` "Resposta em até {{sla}} · cliente {{segment}}" with the catalog SLA per segment (Essencial 24 h, Advance 4 h, Singular 1 h), and action "Conversar" → `panel` `message`. An unknown segment is a `build_error`.
- `activity`: title "Atividade recente". `recent` holds the five most recent client-facing timeline rows as `{icon, title, meta, value?, tone?}`. A row is client-facing when its timeline `source` is `account.event.recorded` or `message.received`; rows from `alert.raised`, `message.triaged`, `case.*`, advisor notes, and calls never reach the client, and neither does a row without `source`. `meta` is the relative time from the row's `occurred_at` to the request time, or from the indexed `ago` when `occurred_at` is absent. Icons are `aporte` `in`, `saque` and `aplicacao` `out`, and `mensagem` `msg`; any other kind has no icon. Every row except `aplicacao` and `reavaliacao` keeps its indexed title. With `amount_cents`, an `aporte` carries `value` signed positive ("+US$ 60.000,00") with `tone` `pos`, and a `saque` carries it signed negative ("−US$ 20.000,00") with `tone` `neg`. An `aplicacao` reads "Compra · {{product}}" with the catalog product name ("Compra" when the catalog failed or does not name the product; never the id) and carries `value`, the purchase amount signed negative ("−US$ 250,00"), when the row has `amount_cents`. A `reavaliacao` reads "Reavaliação diária" with `meta` "dia simulado {{day}} · {{product}} {{pct}}" (for example "dia simulado 3 · Cobalto Semicondutores −53,5%"; "dia simulado {{day}}" when the product has no catalog name), `value` the signed patrimony change ("−US$ 38.520,00"), `tone` `neg` or `pos`, and `icon` `drop` for a loss. A `reavaliacao` that changed nothing is not shown and does not take a place in the limit. With no client-facing row, `empty` carries `items: []` and the empty text.

### Home `v2` (beta) served now

A client whose stored beta flag is on gets home `v2`: the `v1` sections, variants, and heading, with the `highlights` section inserted right after `moment`.

| Section | Type | Variants | Source | When the source fails |
|---|---|---|---|---|
| `highlights` | `product_rail` | `profile_conservador`, `profile_moderado`, `profile_arrojado` | catalog, profile | omitted, reason `catalog` or `profile`; an unknown profile matches no variant and is `build_error` |

- `highlights` is the Investir `highlights` component, built by the same variant with the same props and copy: "Para o seu perfil {{profile}}", "Escolhidos pelo backend a partir do seu perfil de investidor.", and the fixed pick per profile.
- Every other section behaves as in `v1`.
- On day 0 of the seed, with beta on, Fernanda's rail is `profile_conservador` (`tbill`, `corp`), Thiago's `profile_arrojado` (`cobalto`, `acoesg`), and Mariana's `profile_moderado` (`acoesg`, `corp`).

### Investir `v1` served now

The BFF serves investir `v1` under the same screen deadline. The heading is "Investir" / "Produtos fictícios · preço fixo da simulação".

| Section | Type | Variants | Source | When the source fails |
|---|---|---|---|---|
| `cash` | `invest_summary` | `default` | account-sim; profile for the chip | account-sim: omitted, reason `account-sim`; profile: no chip |
| `highlights` | `product_rail` | `profile_conservador`, `profile_moderado`, `profile_arrojado` | catalog, profile | omitted, reason `catalog` or `profile`; an unknown profile matches no variant and is `build_error` |
| `fixed_income` | `product_list` | `fixed_income` | catalog; profile for the badges | catalog: omitted, reason `catalog`; profile: no badge |
| `etfs` | `product_list` | `etf` | catalog; profile for the badges | as `fixed_income` |
| `stocks` | `product_list` | `stocks` | catalog; profile for the badges | as `fixed_income` |

- `cash`: `cash_label` "Disponível para investir", `cash` and `cash_cents` the account-sim cash, and `profile_chip` "Perfil {{profile}}" with the lowercase advisory investor profile.
- `highlights`: `title` "Para o seu perfil {{profile}}", `subtitle` "Escolhidos pelo backend a partir do seu perfil de investidor.", and a fixed pick per profile, in order: conservador `tbill`, `corp`; moderado `acoesg`, `corp`; arrojado `cobalto`, `acoesg`. A pick missing from the catalog is a `build_error`.
- The lists hold every catalog product of their `asset_class` (`renda_fixa`, `etfs`, `acoes`), ordered by risk and then id, with titles "Renda fixa", "ETFs", and "Ações". A class with no product is a `build_error`.
- Products: `class_label` "Renda fixa", "ETF", or "Ação"; `risk_label` "Risco {{risk}} de 5"; `minimum` "Mínimo {{minimum}}" with whole dollars written without cents ("Mínimo US$ 1.000"); `above_profile` is true when `risk` is above the profile's `max_risk`, which is the same test the advisory suitability rule applies to a purchase. Above the profile, `badge` is "Acima do seu perfil" and `warning` is "Este produto tem risco {{risk}}. Seu perfil é {{profile}}, que vai até risco {{max_risk}}. Você pode investir mesmo assim, e a sua assessora será avisada." The action is "Investir" → `panel` `purchase` with the `product_id`.
- On day 0 of the seed, Thiago (arrojado) has nothing above his profile; Fernanda (conservador, max 2) sees `acoesg`, `farol`, and `cobalto` above it; Mariana (moderado, max 3) sees `farol` and `cobalto` above it.

### Carteira `v1` served now

The BFF serves carteira `v1` under the same screen deadline. The heading is "Carteira" / "Valores de mercado no dia simulado {{day}}"; without account-sim the subtitle is left out.

| Section | Type | Variants | Source | When the source fails |
|---|---|---|---|---|
| `summary` | `portfolio_summary` | `with_day_change` (the day moved), `default` | account-sim | omitted, reason `account-sim` |
| `allocation` | `allocation_breakdown` | `default` | account-sim | omitted, reason `account-sim` |
| `positions_stocks` | `position_list` | `stocks` | account-sim, catalog | omitted, reason `account-sim` or `catalog` |
| `positions_etf` | `position_list` | `etf` | account-sim, catalog | as `positions_stocks` |
| `positions_fixed_income` | `position_list` | `fixed_income` | account-sim, catalog | as `positions_stocks` |
| `history` | `activity_list` | `history` | timeline; catalog for product names | timeline: omitted, reason `timeline`; catalog: purchase rows read "Compra" and revaluation rows leave the product out |

- `summary`: `total_label` "Patrimônio total" and `total` the account-sim patrimony at market value. `stats`, in order: "Valor aplicado" (the sum of the positions' applied amounts), "Rentabilidade" ("{{amount}} ({{percent}})": the market value of the positions minus what was applied, signed, and its percentage of the applied amount, for example "+US$ 19.400,00 (+11,5%)", `tone` `pos`, `neg`, or `neutral` at zero), "Caixa", and "Dia simulado", the account-sim simulated day. Every money stat carries `money: true`. `with_day_change` follows the home `wealth` rule and adds the same `day_change` and `day_change_tone`.
- `allocation`: `title` "Alocação" and four rows, always in the order Ações (`stocks`), ETFs (`etfs`), Renda fixa (`fixed_income`), Caixa (`cash`), zero classes included, each with `value`, the largest-remainder `share` of the patrimony, and `bar_width` equal to it.
- Position lists: one section per class with at least one position; a class with none has no section and is not listed in `omitted`. Titles "Ações", "ETFs", and "Renda fixa"; `subtotal` is the market value of the class; `applied_label` "Aplicado". Items are ordered by `value` descending, then `product_id`. `name` comes from the catalog; a position the catalog does not name is a `build_error`. `return` is the signed percentage of value over applied with one decimal, "0,0%" with nothing applied. When the catalog fails, all three lists are omitted with reason `catalog`, including a class with no position.
- `history`: `title` "Movimentações" and up to 20 client-facing timeline rows through the home activity mapping (the home keeps five), revaluation rows included. With no row, `items: []` and `empty_text` "Nenhuma movimentação ainda.".
- The response is `200` with whatever remains: without account-sim only `history`, without the catalog no position list, without the timeline no `history`.
- On day 0 of the seed, Mariana and Fernanda have all three position lists and Thiago has no `positions_fixed_income`.

### Perfil `v1` served now

The BFF serves perfil `v1` under the same screen deadline. The heading is "Perfil" / "Seus dados, seu perfil de investidor e suas preferências".

| Section | Type | Variants | Source | When the source fails |
|---|---|---|---|---|
| `header` | `profile_header` | `default` | advisory; registration for `account` | advisory: omitted, reason `advisory`; registration: no `account` |
| `suitability` | `profile_scale` | `conservador`, `moderado`, `arrojado` (the profile) | advisory, profile | omitted, reason `advisory` or `profile`; an unknown profile matches no variant and is `build_error` |
| `registration` | `profile_field_list` | `default` | registration, advisory | omitted, reason `registration` or `advisory` |
| `preferences` | `preference_list` | `default` | preferences | omitted, reason `preferences` |
| `advisor` | `advisor_card` | `dedicated` (Singular), `default` | advisory | omitted, reason `advisory` |

- `header`: `initials` and `name` from the advisory customer, `subtitle` "Cliente {{segment}} desde {{since}}" ("Cliente {{segment}}" without a since), and `account` the registration `account_number` ("Conta 3301-7 · Orla Invest"), left out when the registration read fails.
- `suitability`: `title` "Seu perfil de investidor", `subtitle` "Define os destaques de Investir e quando uma compra recebe aviso.", `current_label` "Seu perfil", and `levels` conservador, moderado, and arrojado with the `ux.md` descriptions, `limit` "Produtos até risco {{max_risk}}", and `current` true for the client's level. The client's `max_risk` is the investor profile's; the other levels read the `max_risk_table` that advisory `GetInvestorProfile` returns, so the BFF and web hold no table. A level missing from the table, or a `max_risk` outside 1–5, is a `build_error`. `footer` is "Última avaliação em {{dd/mm/yyyy}}. Para refazer o questionário, fale com a sua assessora." The section needs the advisory customer too: with advisory down it is omitted with the header and advisor.
- `registration`: `title` "Dados cadastrais", `fields` Nome (advisory), E-mail, Telefone (already masked), Cidade (registration), Segmento, and Cliente desde (advisory), and `footnote` "Dados fictícios. Alterar cadastro fica fora da simulação." Nome, Segmento, and Cliente desde come from advisory, so with advisory down the section is omitted rather than trimmed to a list without a name.
- `preferences`: `title` "Preferências"; `theme` `{label "Tema", hint "Fica salvo só neste navegador"}`, which web applies locally and never sends; `channel` `{label "Canal preferido", hint "Por onde a assessoria fala com você", value, options [{chat, "Chat"}, {email, "E-mail"}]}` with the stored channel; `beta` `{label "Programa beta", hint, enabled}` with hint "Veja antes as novas versões das telas." when off and "Ligado. Você recebe a revision v2 do início antes dos outros clientes." when on. Beta on serves home `v2` from the next home request; beta off serves `v1` again.
- `advisor`: the home `advisor_card` variants and props.
- On day 0 of the seed, Fernanda is conservador (assessed 12/03/2026), Thiago arrojado (04/08/2026), and Mariana moderado (20/01/2026); all three have `chat` with beta off.

### Components (15 types)

Shared objects used in the table:

- **Product:** `product_id`, `name`, `class_label` (`"ETF"`), `risk` (integer 1–5, for the bars), `risk_label` (`"Risco 3 de 5"`), `return_label` (`"+11,2% em 12 meses"`), `minimum` (`"Mínimo US$ 50"`), `minimum_cents`, `above_profile` (boolean), `badge?` (`"Acima do seu perfil"`, set only when above profile), `warning?` (the above-profile text for the purchase form, set only when above profile), `action` (`panel` `purchase` with `product_id`).
- **Allocation row:** `class` (`stocks`, `etfs`, `fixed_income`, or `cash`; web maps it to a color), `label`, `share` (`"62%"`), `bar_width`.
- **Activity item:** `icon?`, `title`, `meta`, `value?` (signed money, `"−US$ 250,00"` on a purchase), `tone?` (`pos`, `neg`, or `neutral`).
- **Stat:** `label`, `value`, `tone?` (`pos`, `neg`, or `neutral`), `money?` (`true` when `value` is money; web masks it with the eye toggle).

The Variants column is not evaluation order; the catalog holds each section's ordered list. Each section's default variant: `moment` is `welcome`; `wealth` and `summary` are `default`; `advisor` is `default`; `activity` is `recent` or `empty`, chosen by item count; `history` is `history`; every other section has a single variant, or the profile variant for `highlights` and `suitability`, and that is its default.

| Type | Sections | Variants | Props |
|---|---|---|---|
| `moment_card` | home `moment` | `portfolio_drop`, `case_open`, `segment_upgraded`, `segment_upgrade_near`, `idle_cash`, `portfolio_review`, `welcome` (default) | `kicker`, `title`, `body`, `meta?` (`case_open`: "Protocolo … · aberto há …"), `tone` (`neg`, `info`, `gold`, or `neutral`), `icon?`, `action?` (`welcome` has none) |
| `wealth_summary` | home `wealth` | `default`, `with_day_change` | `total_label`, `total`, `cash_label`, `cash`, `cash_cents`, `allocation` (allocation rows); `with_day_change` adds `day_change` (`"−US$ 38.520,00 (−15,5%) no dia 3"`) and `day_change_tone` (`pos` or `neg`; the variant never matches a zero change). Masking values is web presentation only |
| `action_grid` | home `actions` | `default` | `items`: `[{label, icon?, action}]`, four `panel` actions: Depositar `deposit`, Sacar `withdraw`, Mensagem `message`, Reclamar `complaint` |
| `advisor_card` | home `advisor`, perfil `advisor` | `default`, `dedicated` (Singular) | `kicker` ("Sua assessora" or "Sua assessora dedicada"), `name`, `initials`, `meta` ("Resposta em até … · cliente …"), `action?` (`panel` `message`) |
| `activity_list` | home `activity`, carteira `history` | `recent`, `empty`, `history` | `title` ("Atividade recente" or "Movimentações"), `items` (activity items: at most 5 on home, 20 in `history`; empty for `empty`), `empty_text?` (`empty`: "Suas movimentações aparecem aqui assim que acontecerem."; `history` with no items: "Nenhuma movimentação ainda.") |
| `invest_summary` | investir `cash` | `default` | `cash_label`, `cash`, `cash_cents`, `profile_chip?` ("Perfil moderado", left out without a profile) |
| `product_rail` | investir `highlights`; home `highlights` in beta revision `v2` | `profile_conservador`, `profile_moderado`, `profile_arrojado` | `title` ("Para o seu perfil …"), `subtitle`, `products` (products) |
| `product_list` | investir `fixed_income`, `etfs`, `stocks` | `fixed_income`, `etf`, `stocks` | `title` ("Renda fixa", "ETFs", or "Ações"), `products` (products) |
| `portfolio_summary` | carteira `summary` | `default`, `with_day_change` (not served yet; the market-day story, 13, serves it) | `total_label`, `total`, `stats` (stats: "Valor aplicado", "Rentabilidade" signed with percentage, "Caixa", "Dia simulado"); `with_day_change` adds `day_change` and `day_change_tone` |
| `allocation_breakdown` | carteira `allocation` | `default` | `title`, `rows`: allocation rows plus `value` (money) for Ações, ETFs, Renda fixa, and Caixa, zero classes included. Money values are masked by the eye toggle |
| `position_list` | carteira `positions_stocks`, `positions_etf`, `positions_fixed_income` | `stocks`, `etf`, `fixed_income` | `title`, `subtotal`, `applied_label` ("Aplicado"), `items`: `[{product_id, name, applied, value, return, return_tone}]`, `return` signed percentage, `return_tone` `pos`, `neg`, or `neutral` at zero. `subtotal`, `applied`, and `value` are masked by the eye toggle; `return` is not |
| `profile_header` | perfil `header` | `default` | `initials`, `name`, `subtitle` ("Cliente … desde …"), `account` ("Conta 3301-7 · Orla Invest") |
| `profile_scale` | perfil `suitability` | `conservador`, `moderado`, `arrojado` | `title`, `subtitle`, `current_label` ("Seu perfil"), `levels`: `[{key, label, description, limit, max_risk, current}]` for conservador, moderado, and arrojado, `limit` "Produtos até risco …", `max_risk` integer for the bars, `current` boolean; `footer` ("Última avaliação em …") |
| `profile_field_list` | perfil `registration` | `default` | `title`, `fields`: `[{label, value}]` for Nome, E-mail, Telefone (masked), Cidade, Segmento, Cliente desde; `footnote` |
| `preference_list` | perfil `preferences` | `default` | `title`; `theme`: `{label, hint}`, local to the browser; `channel`: `{label, hint, value, options: [{value, label}]}`, `value` `chat` or `email`; `beta`: `{label, hint, enabled}`, `hint` for the current state |

`preference_list` shows the stored channel and beta flag. Its channel vocabulary is `chat` or `email`, distinct from the messages route, which takes `chat` or `e-mail`. Web writes them with `PUT /v1/client-pov/customers/{id}/preferences` and reloads the screen after a change; on a failed write it keeps the previous value. Later stories that refine a prop list update this table in the same change.
