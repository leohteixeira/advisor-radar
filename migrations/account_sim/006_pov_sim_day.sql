-- Global simulated market day (ADR 0010). One row: the day starts at 0,
-- advances only through AdvanceDay, and reseed returns it to 0. Positions keep
-- their units; values are computed at read time from units and this day, so an
-- advance rewrites no position. epoch namespaces the name-based reavaliacao
-- event ids and is replaced on every reseed, so a replayed day publishes new
-- ids instead of colliding with events consumers already claimed.
CREATE TABLE IF NOT EXISTS pov_sim (
    id      BOOLEAN PRIMARY KEY DEFAULT true CHECK (id),
    sim_day INT NOT NULL,
    epoch   UUID NOT NULL DEFAULT gen_random_uuid(),
    CONSTRAINT pov_sim_day_non_negative CHECK (sim_day >= 0)
);

INSERT INTO pov_sim (id, sim_day) VALUES (true, 0)
ON CONFLICT (id) DO NOTHING;

-- The reply of each advance-day idempotency key: a replay returns it and
-- advances nothing. event_ids are in byte-wise customer id order.
CREATE TABLE IF NOT EXISTS pov_advance (
    idem_key  TEXT PRIMARY KEY,
    sim_day   INT NOT NULL,
    event_ids TEXT[] NOT NULL
);
