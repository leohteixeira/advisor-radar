-- Cases, inbox, and outbox for the cases service. Apply against the cases database.
CREATE TABLE IF NOT EXISTS cases (
    id                TEXT        PRIMARY KEY,
    customer_id       TEXT        NOT NULL,
    state             TEXT        NOT NULL,
    sla_total_minutes INTEGER     NOT NULL,
    escalated         BOOLEAN     NOT NULL DEFAULT false,
    opened_at         TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS inbox (
    event_id    TEXT        PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     TEXT        NOT NULL,
    routing_key  TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ NULL,
    CONSTRAINT cases_outbox_event_id_key UNIQUE (event_id)
);

CREATE INDEX IF NOT EXISTS cases_outbox_unpublished_idx
    ON outbox (id)
    WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS sla_delay (
    case_id      TEXT        PRIMARY KEY,
    ttl_ms       INTEGER     NOT NULL,
    published_at TIMESTAMPTZ NULL
);
