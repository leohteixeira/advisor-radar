-- Book, inbox, alerts, and outbox for advisory. Apply against the advisory database.
CREATE TABLE IF NOT EXISTS book (
    customer_id TEXT PRIMARY KEY,
    name        TEXT             NOT NULL,
    segment     TEXT             NOT NULL,
    aum         DOUBLE PRECISION NOT NULL,
    advisor     TEXT             NOT NULL,
    since       TEXT             NOT NULL
);

CREATE TABLE IF NOT EXISTS inbox (
    event_id    TEXT        PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alerts (
    id              TEXT        PRIMARY KEY,
    customer_id     TEXT        NOT NULL,
    kind            TEXT        NOT NULL,
    rule            TEXT        NOT NULL,
    source_event_id TEXT        NOT NULL,
    raised_at       TIMESTAMPTZ NOT NULL,
    payload         JSONB       NOT NULL
);

CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     TEXT        NOT NULL,
    routing_key  TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ NULL,
    CONSTRAINT advisory_outbox_event_id_key UNIQUE (event_id)
);

CREATE INDEX IF NOT EXISTS advisory_outbox_unpublished_idx
    ON outbox (id)
    WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS signal_action (
    signal_id     TEXT PRIMARY KEY,
    contacted_at  TIMESTAMPTZ NULL,
    snoozed_until TIMESTAMPTZ NULL
);
