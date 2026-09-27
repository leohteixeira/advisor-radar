-- Outbox for account-sim. Apply against the account_sim database.
CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     TEXT        NOT NULL,
    routing_key  TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ NULL,
    CONSTRAINT outbox_event_id_key UNIQUE (event_id)
);

CREATE INDEX IF NOT EXISTS outbox_unpublished_idx
    ON outbox (id)
    WHERE published_at IS NULL;
