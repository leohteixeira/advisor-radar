-- Preferences of the three POV accounts: every client starts on chat with the
-- beta program off. Reseed restores these rows.
INSERT INTO pov_preferences (customer_id, channel, beta) VALUES
  ('01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'chat', false),
  ('01a0e3a4-9a44-757a-ac8f-dab7db5eb068', 'chat', false),
  ('01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 'chat', false)
ON CONFLICT (customer_id) DO UPDATE SET
  channel = EXCLUDED.channel,
  beta = EXCLUDED.beta;
