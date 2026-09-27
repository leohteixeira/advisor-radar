-- Rebuild cases schema with UUIDv7 columns, advisor_id, and history.

DROP TABLE IF EXISTS sla_delay CASCADE;
DROP TABLE IF EXISTS outbox CASCADE;
DROP TABLE IF EXISTS case_history CASCADE;
DROP TABLE IF EXISTS inbox CASCADE;
DROP TABLE IF EXISTS cases CASCADE;

CREATE TABLE cases (
    id                UUID        PRIMARY KEY,
    customer_id       UUID        NOT NULL,
    signal_id         UUID        NULL,
    advisor_id        UUID        NOT NULL,
    state             TEXT        NOT NULL,
    sla_total_minutes INTEGER     NOT NULL,
    escalated         BOOLEAN     NOT NULL DEFAULT false,
    opened_at         TIMESTAMPTZ NOT NULL
);

CREATE TABLE case_history (
    id         UUID PRIMARY KEY,
    case_id    UUID        NOT NULL,
    kind       TEXT        NOT NULL,
    text       TEXT        NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE inbox (
    event_id    UUID        PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     UUID        NOT NULL,
    routing_key  TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ NULL,
    CONSTRAINT cases_outbox_event_id_key UNIQUE (event_id)
);

CREATE INDEX cases_outbox_unpublished_idx
    ON outbox (id)
    WHERE published_at IS NULL;

CREATE TABLE sla_delay (
    case_id      UUID        PRIMARY KEY,
    ttl_ms       INTEGER     NOT NULL,
    published_at TIMESTAMPTZ NULL
);
