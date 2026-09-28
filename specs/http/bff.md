# BFF HTTP contract

The browser talks only to the BFF. Paths below are the BFF paths. The Vite app prefixes them with `/advisor-radar`.

The BFF stores nothing durable. POV progress lives in an in-memory hub. OpenTelemetry is not instrumented; Bastidores shows `event_id` and the steps the hub has observed, not a live `trace_id`.

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
| GET | `/v1/customers/{id}/timeline` | Customer 360 rows |
| GET | `/v1/review` | Triage review queue |
| PUT | `/v1/review/{id}` | Correct an intent |
| GET | `/v1/manager` | Manager snapshot |

## POV routes

Money fields are integer USD cents. Every POST requires `Idempotency-Key`. A missing key is `400`. A new command spends a budget of 10 per minute and 40 per 24 hours for that customer. A replay of a known key does not spend the budget and does not append a second event. Over the limit the response is `429` and account-sim is not called. A withdrawal above caixa is `422` and publishes nothing.

| Method | Path | Result |
|---|---|---|
| GET | `/v1/client-pov/customers` | The three clients: name, segment, assets, SLA, advisor, since from the book, hint |
| GET | `/v1/client-pov/customers/{id}` | Home: assets, caixa, allocation, advisor, since from the book, activity, messages |
| POST | `/v1/client-pov/customers/{id}/deposits` | `202` `{"event_id"}`. Body: `amount`, `origin` |
| POST | `/v1/client-pov/customers/{id}/withdrawals` | `202`, or `422` when amount exceeds caixa. Body: `amount`, `destination` |
| POST | `/v1/client-pov/customers/{id}/messages` | `202`. Body: `channel` `chat` or `e-mail`, `text` |
| POST | `/v1/client-pov/customers/{id}/complaints` | `202`. Body: `text`. Channel is chat |
| GET | `/v1/client-pov/customers/{id}/stream` | SSE event `bastidores` with `event_id` and steps `feito`, `agora`, or `aguardando` |
| GET | `/v1/client-pov/counters` | `actions` by type, `refusals` by rule, `duplicates` |

`202` has `event_id` and no protocol field. The UI formats the protocol as the first two UUID groups, uppercased.

Deposit and withdrawal become `account.event.recorded` at schema version 2. Message and complaint become `message.received`. The BFF does not publish. account-sim writes state and the outbox row in one transaction.
