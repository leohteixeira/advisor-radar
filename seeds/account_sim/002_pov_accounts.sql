-- POV balances in integer USD cents. Reseed restores these three rows.
INSERT INTO pov_account (customer_id, acoes, etfs, renda_fixa, caixa) VALUES
  ('01a0e3a4-9a44-7566-b5de-eb2e365799f8', 9090000, 6060000, 3680000, 6000000),
  ('01a0e3a4-9a44-757a-ac8f-dab7db5eb068', 164000, 369000, 172200, 114800),
  ('01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 204000, 544000, 0, 6052000)
ON CONFLICT (customer_id) DO UPDATE SET
  acoes = EXCLUDED.acoes,
  etfs = EXCLUDED.etfs,
  renda_fixa = EXCLUDED.renda_fixa,
  caixa = EXCLUDED.caixa;
