-- SLA follows the segment (ADR 0012) and signals joined to a case (ADR 0013).
-- Additive and safe on a populated database: every new column is nullable or
-- has a default, so existing rows, the seed, and a binary from before this
-- migration keep inserting and updating as they did.

-- The instant the SLA clock counts from. NULL means opened_at: no segment
-- change has moved the clock.
ALTER TABLE cases
    ADD COLUMN IF NOT EXISTS sla_started_at TIMESTAMPTZ NULL;

-- The signal a history row is about, set when an advisor's "open case"
-- command joins an existing case. A signal joins a case at most once.
ALTER TABLE case_history
    ADD COLUMN IF NOT EXISTS signal_id UUID NULL;

CREATE UNIQUE INDEX IF NOT EXISTS case_history_case_signal_idx
    ON case_history (case_id, signal_id)
    WHERE signal_id IS NOT NULL;

-- When the delay row was last armed. A recalculated SLA re-arms the row; the
-- publisher marks the arm it sent, never a newer one.
ALTER TABLE sla_delay
    ADD COLUMN IF NOT EXISTS armed_at TIMESTAMPTZ NOT NULL DEFAULT now();
