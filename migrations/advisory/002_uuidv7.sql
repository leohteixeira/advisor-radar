-- Rebuild advisory schema with UUIDv7 columns, operators, and queue signals.
-- Fresh demo databases: drop and recreate. No FK across services.

DROP TABLE IF EXISTS signal_action CASCADE;
DROP TABLE IF EXISTS outbox CASCADE;
DROP TABLE IF EXISTS alerts CASCADE;
DROP TABLE IF EXISTS queue_signals CASCADE;
DROP TABLE IF EXISTS notes CASCADE;
DROP TABLE IF EXISTS inbox CASCADE;
DROP TABLE IF EXISTS book CASCADE;
DROP TABLE IF EXISTS operators CASCADE;

CREATE TABLE operators (
    id   UUID PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE book (
    customer_id UUID PRIMARY KEY,
    name        TEXT             NOT NULL,
    segment     TEXT             NOT NULL,
    aum         DOUBLE PRECISION NOT NULL,
    advisor_id  UUID             NOT NULL,
    since       TEXT             NOT NULL
);

CREATE TABLE inbox (
    event_id    UUID        PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alerts (
    id              UUID        PRIMARY KEY,
    customer_id     UUID        NOT NULL,
    kind            TEXT        NOT NULL,
    rule            TEXT        NOT NULL,
    source_event_id UUID        NOT NULL,
    raised_at       TIMESTAMPTZ NOT NULL,
    payload         JSONB       NOT NULL
);

-- Demo and live queue cards (messages and alerts) for ListQueue.
CREATE TABLE queue_signals (
    id          UUID PRIMARY KEY,
    customer_id UUID        NOT NULL,
    kind        TEXT        NOT NULL,
    raised_at   TIMESTAMPTZ NOT NULL,
    payload     JSONB       NOT NULL
);

CREATE TABLE notes (
    id          UUID PRIMARY KEY,
    customer_id UUID        NOT NULL,
    kind        TEXT        NOT NULL,
    title       TEXT        NOT NULL,
    text        TEXT        NOT NULL,
    meta        TEXT        NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     UUID        NOT NULL,
    routing_key  TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ NULL,
    CONSTRAINT advisory_outbox_event_id_key UNIQUE (event_id)
);

CREATE INDEX advisory_outbox_unpublished_idx
    ON outbox (id)
    WHERE published_at IS NULL;

CREATE TABLE signal_action (
    signal_id     UUID PRIMARY KEY,
    contacted_at  TIMESTAMPTZ NULL,
    snoozed_until TIMESTAMPTZ NULL
);
