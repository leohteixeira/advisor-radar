-- Reseed returns the simulated market day to 0 in a new epoch (ADR 0010), so
-- every position is worth its seed value again and the next advances publish
-- fresh reavaliacao event ids.
INSERT INTO pov_sim (id, sim_day, epoch) VALUES
  (true, 0, gen_random_uuid())
ON CONFLICT (id) DO UPDATE SET
  sim_day = EXCLUDED.sim_day,
  epoch = EXCLUDED.epoch;

-- Advance keys belong to the old epoch: a replay would answer its day and
-- event ids while the day is back at 0.
DELETE FROM pov_advance;
