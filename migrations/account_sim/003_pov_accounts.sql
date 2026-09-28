-- POV account balances in integer USD cents, plus command idempotency keys.
CREATE TABLE IF NOT EXISTS pov_account (
    customer_id TEXT PRIMARY KEY,
    acoes       BIGINT NOT NULL,
    etfs        BIGINT NOT NULL,
    renda_fixa  BIGINT NOT NULL,
    caixa       BIGINT NOT NULL,
    CONSTRAINT pov_account_non_negative CHECK (
        acoes >= 0 AND etfs >= 0 AND renda_fixa >= 0 AND caixa >= 0
    )
);

CREATE TABLE IF NOT EXISTS pov_idempotency (
    customer_id TEXT NOT NULL,
    idem_key    TEXT NOT NULL,
    event_id    TEXT NOT NULL,
    PRIMARY KEY (customer_id, idem_key)
);
