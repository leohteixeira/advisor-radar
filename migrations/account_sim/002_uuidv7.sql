-- Rebuild account_sim outbox with UUIDv7 event_id.

DROP TABLE IF EXISTS outbox CASCADE;

CREATE TABLE outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     UUID        NOT NULL,
    routing_key  TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ NULL,
    CONSTRAINT outbox_event_id_key UNIQUE (event_id)
);

CREATE INDEX outbox_unpublished_idx
    ON outbox (id)
    WHERE published_at IS NULL;
