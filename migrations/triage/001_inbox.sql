-- Inbox, results, and outbox for triage. Apply against the triage database.
CREATE TABLE IF NOT EXISTS inbox (
    event_id    TEXT        PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS results (
    id              TEXT             PRIMARY KEY,
    source_event_id TEXT             NOT NULL,
    customer_id     TEXT             NOT NULL,
    intent          TEXT             NOT NULL,
    intent_prob     DOUBLE PRECISION NOT NULL,
    frustration     DOUBLE PRECISION NOT NULL,
    churn_risk      DOUBLE PRECISION NOT NULL,
    wants_human     DOUBLE PRECISION NOT NULL,
    classifier      TEXT             NOT NULL,
    model_version   TEXT             NOT NULL,
    degraded        BOOLEAN          NOT NULL,
    needs_review    BOOLEAN          NOT NULL,
    created_at      TIMESTAMPTZ      NOT NULL,
    payload         JSONB            NOT NULL
);

CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     TEXT        NOT NULL,
    routing_key  TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ NULL,
    CONSTRAINT triage_outbox_event_id_key UNIQUE (event_id)
);

CREATE INDEX IF NOT EXISTS triage_outbox_unpublished_idx
    ON outbox (id)
    WHERE published_at IS NULL;
