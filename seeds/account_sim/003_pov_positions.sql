-- Fictional product catalog, day-0 positions, and registration data for the
-- three POV accounts (ADR 0010; architecture.md "Seed"). Amounts are integer
-- USD cents. units_cents is the holding in day-0 cents, so on day 0 a
-- position's value equals its units. Reseed restores all of it.

INSERT INTO pov_product (id, name, asset_class, risk, return_label, minimum_cents) VALUES
  ('tbill', 'Orla T-Bill 6 meses', 'renda_fixa', 1, '4,9% a.a.', 10000),
  ('corp', 'Orla Corporate IG 2029', 'renda_fixa', 2, '5,6% a.a.', 100000),
  ('renda', 'Maré Renda Global ETF', 'etfs', 2, '+3,8% em 12 meses', 5000),
  ('acoesg', 'Maré Ações Globais ETF', 'etfs', 3, '+11,2% em 12 meses', 5000),
  ('farol', 'Farol Saúde', 'acoes', 4, '+9,4% em 12 meses', 1000),
  ('cobalto', 'Cobalto Semicondutores', 'acoes', 5, '+27,1% em 12 meses', 1000)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  asset_class = EXCLUDED.asset_class,
  risk = EXCLUDED.risk,
  return_label = EXCLUDED.return_label,
  minimum_cents = EXCLUDED.minimum_cents;

-- Positions bought after the seed are dropped, so reseed returns each POV
-- account to exactly these rows.
DELETE FROM pov_position WHERE customer_id IN (
  '01a0e3a4-9a44-7566-b5de-eb2e365799f8',
  '01a0e3a4-9a44-757a-ac8f-dab7db5eb068',
  '01a0e3a4-9a44-75dd-b3a0-403a7a87836e'
);

INSERT INTO pov_position (customer_id, product_id, units_cents, applied_cents) VALUES
  ('01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'cobalto', 7200000, 6000000),
  ('01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'farol', 1890000, 1750000),
  ('01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'acoesg', 4060000, 3600000),
  ('01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'renda', 2000000, 1940000),
  ('01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'corp', 3680000, 3600000),
  ('01a0e3a4-9a44-757a-ac8f-dab7db5eb068', 'farol', 164000, 159000),
  ('01a0e3a4-9a44-757a-ac8f-dab7db5eb068', 'renda', 169000, 166000),
  ('01a0e3a4-9a44-757a-ac8f-dab7db5eb068', 'acoesg', 200000, 190000),
  ('01a0e3a4-9a44-757a-ac8f-dab7db5eb068', 'tbill', 172200, 169000),
  ('01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 'cobalto', 204000, 190000),
  ('01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 'acoesg', 544000, 520000)
ON CONFLICT (customer_id, product_id) DO UPDATE SET
  units_cents = EXCLUDED.units_cents,
  applied_cents = EXCLUDED.applied_cents;

INSERT INTO pov_registration (customer_id, email, phone, city, account_number) VALUES
  ('01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'mariana.costa@example.com', '+55 (11) •••••-7810', 'São Paulo, SP · Brasil', 'Conta 1190-4 · Orla Invest'),
  ('01a0e3a4-9a44-757a-ac8f-dab7db5eb068', 'fernanda.lima@example.com', '+55 (19) •••••-4471', 'Campinas, SP · Brasil', 'Conta 3301-7 · Orla Invest'),
  ('01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 'thiago.azevedo@example.com', '+55 (48) •••••-2093', 'Florianópolis, SC · Brasil', 'Conta 2847-1 · Orla Invest')
ON CONFLICT (customer_id) DO UPDATE SET
  email = EXCLUDED.email,
  phone = EXCLUDED.phone,
  city = EXCLUDED.city,
  account_number = EXCLUDED.account_number;
