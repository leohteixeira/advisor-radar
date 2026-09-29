-- Investor profile per book customer, and the schema version of the account
-- event behind each alert. Safe on a populated database: the new book columns
-- are added with a default that fills existing rows, then the default is
-- dropped so every later insert must state the profile. The seed sets the
-- real values.

ALTER TABLE book
    ADD COLUMN IF NOT EXISTS investor_profile TEXT NOT NULL DEFAULT 'conservador',
    ADD COLUMN IF NOT EXISTS profile_assessed_on DATE NOT NULL DEFAULT DATE '2026-01-01';

ALTER TABLE book
    ALTER COLUMN investor_profile DROP DEFAULT,
    ALTER COLUMN profile_assessed_on DROP DEFAULT;

ALTER TABLE book
    DROP CONSTRAINT IF EXISTS book_investor_profile_check;

ALTER TABLE book
    ADD CONSTRAINT book_investor_profile_check
    CHECK (investor_profile IN ('conservador', 'moderado', 'arrojado'));

-- NULL for alerts raised before this migration, seeded alerts, and alerts
-- that do not come from an account event.
ALTER TABLE alerts
    ADD COLUMN IF NOT EXISTS source_schema_version INTEGER NULL;

CREATE INDEX IF NOT EXISTS alerts_customer_kind_raised_idx
    ON alerts (customer_id, kind, raised_at DESC);
