-- At most one case per customer that is not Resolvido. The intake serializes
-- per customer with an advisory lock; this index is the database backstop.
CREATE UNIQUE INDEX IF NOT EXISTS cases_one_open_per_customer_idx
    ON cases (customer_id)
    WHERE state <> 'Resolvido';
