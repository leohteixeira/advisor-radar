-- POV client preferences: the contact channel and the beta program flag.
-- A table of its own rather than columns on pov_account keeps pov_account the
-- cash-only balance row that ADR 0010 left, so no command that moves money
-- ever touches preferences. The rows come from seeds/account_sim, and reseed
-- restores them to chat and beta off.
CREATE TABLE IF NOT EXISTS pov_preferences (
    customer_id TEXT PRIMARY KEY REFERENCES pov_account (customer_id) ON DELETE CASCADE,
    channel     TEXT NOT NULL CHECK (channel IN ('chat', 'email')),
    beta        BOOLEAN NOT NULL
);
