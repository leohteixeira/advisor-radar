-- Rebuild triage schema with UUIDv7 columns and corrected_intent.

DROP TABLE IF EXISTS outbox CASCADE;
DROP TABLE IF EXISTS results CASCADE;
DROP TABLE IF EXISTS inbox CASCADE;

CREATE TABLE inbox (
    event_id    UUID        PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE results (
    id               UUID             PRIMARY KEY,
    source_event_id  UUID             NOT NULL,
    customer_id      UUID             NOT NULL,
    intent           TEXT             NOT NULL,
    intent_prob      DOUBLE PRECISION NOT NULL,
    frustration      DOUBLE PRECISION NOT NULL,
    churn_risk       DOUBLE PRECISION NOT NULL,
    wants_human      DOUBLE PRECISION NOT NULL,
    classifier       TEXT             NOT NULL,
    model_version    TEXT             NOT NULL,
    degraded         BOOLEAN          NOT NULL,
    needs_review     BOOLEAN          NOT NULL,
    corrected_intent TEXT             NULL,
    created_at       TIMESTAMPTZ      NOT NULL,
    payload          JSONB            NOT NULL
);

CREATE TABLE outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     UUID        NOT NULL,
    routing_key  TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ NULL,
    CONSTRAINT triage_outbox_event_id_key UNIQUE (event_id)
);

CREATE INDEX triage_outbox_unpublished_idx
    ON outbox (id)
    WHERE published_at IS NULL;
