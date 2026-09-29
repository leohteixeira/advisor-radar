-- POV cash balances in integer USD cents (ADR 0010). The class totals are
-- aggregates of the positions in 003_pov_positions.sql. Reseed restores these
-- three rows.
INSERT INTO pov_account (customer_id, caixa) VALUES
  ('01a0e3a4-9a44-7566-b5de-eb2e365799f8', 6000000),
  ('01a0e3a4-9a44-757a-ac8f-dab7db5eb068', 114800),
  ('01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 6052000)
ON CONFLICT (customer_id) DO UPDATE SET
  caixa = EXCLUDED.caixa;
