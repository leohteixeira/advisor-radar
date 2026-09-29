-- Individual per-product positions (ADR 0010). pov_account keeps only the cash
-- balance; the stocks, ETFs, and fixed-income classes become aggregates of
-- pov_position valued in Go. Positions, catalog, and registration rows come
-- from seeds/account_sim, so run `cmd/db seed` after this migration.
ALTER TABLE pov_account
    DROP CONSTRAINT IF EXISTS pov_account_non_negative,
    DROP COLUMN IF EXISTS acoes,
    DROP COLUMN IF EXISTS etfs,
    DROP COLUMN IF EXISTS renda_fixa,
    ADD CONSTRAINT pov_account_caixa_non_negative CHECK (caixa >= 0);

-- Fictional product catalog. Amounts are integer USD cents.
CREATE TABLE IF NOT EXISTS pov_product (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    asset_class   TEXT NOT NULL,
    risk          SMALLINT NOT NULL,
    return_label  TEXT NOT NULL,
    minimum_cents BIGINT NOT NULL,
    CONSTRAINT pov_product_asset_class CHECK (asset_class IN ('renda_fixa', 'etfs', 'acoes')),
    CONSTRAINT pov_product_risk CHECK (risk BETWEEN 1 AND 5),
    CONSTRAINT pov_product_minimum CHECK (minimum_cents > 0)
);

-- units_cents is the holding expressed in day-0 cents; its market value is
-- units_cents times the product factor of the simulated day.
CREATE TABLE IF NOT EXISTS pov_position (
    customer_id   TEXT NOT NULL REFERENCES pov_account (customer_id) ON DELETE CASCADE,
    product_id    TEXT NOT NULL REFERENCES pov_product (id),
    units_cents   BIGINT NOT NULL,
    applied_cents BIGINT NOT NULL,
    PRIMARY KEY (customer_id, product_id),
    CONSTRAINT pov_position_non_negative CHECK (units_cents >= 0 AND applied_cents >= 0)
);

CREATE INDEX IF NOT EXISTS pov_position_product_idx ON pov_position (product_id);

-- Fictional registration data. phone is already masked display text.
CREATE TABLE IF NOT EXISTS pov_registration (
    customer_id    TEXT PRIMARY KEY REFERENCES pov_account (customer_id) ON DELETE CASCADE,
    email          TEXT NOT NULL,
    phone          TEXT NOT NULL,
    city           TEXT NOT NULL,
    account_number TEXT NOT NULL
);
