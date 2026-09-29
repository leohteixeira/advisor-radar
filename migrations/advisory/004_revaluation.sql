-- The latest reavaliacao (daily revaluation of the simulated market) per
-- customer, for the portfolio_drop moment. One row per customer; a
-- revaluation replaces it when it is from another account-sim epoch (a reseed
-- starts the days over) or from a later day, so a redelivered older day never
-- replaces a newer one. Money is integer USD cents: amount_cents is
-- after − before, signed. product_id is the position that moved the most, and
-- product_change_bp its day change in signed basis points.
CREATE TABLE IF NOT EXISTS revaluation (
    customer_id       UUID    PRIMARY KEY,
    epoch             UUID    NOT NULL,
    sim_day           INTEGER NOT NULL CHECK (sim_day >= 1),
    amount_cents      BIGINT  NOT NULL,
    before_cents      BIGINT  NOT NULL,
    product_id        TEXT    NOT NULL,
    product_change_bp INTEGER NOT NULL,
    source_event_id   UUID    NOT NULL
);
