-- Triage review results (7 under threshold + 1 at 0.85 excluded by query).

INSERT INTO results (
  id, source_event_id, customer_id, intent, intent_prob, frustration, churn_risk,
  wants_human, classifier, model_version, degraded, needs_review, corrected_intent, created_at, payload
) VALUES (
  '01a0e3a4-9a44-7703-a783-9b51d7e16ba7', '01a0e3a4-9a44-7857-b585-39ae3bc52365', '01a0e3a4-9a44-7590-b715-07e7c4798c60', 'Resgate', 0.54, 0, 0.2, 0, 'jev', 'seed', false, true, NULL,
  now() - interval '33 minutes', '{"channel":"chat","dist":{"Câmbio":0.31,"Encerramento":0.05,"Operacional":0.1,"Resgate":0.54},"text":"Preciso sacar 5 mil dólares e trazer de volta para o Brasil."}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  source_event_id = EXCLUDED.source_event_id,
  customer_id = EXCLUDED.customer_id,
  intent = CASE WHEN results.corrected_intent IS NULL THEN EXCLUDED.intent ELSE results.intent END,
  intent_prob = EXCLUDED.intent_prob,
  frustration = EXCLUDED.frustration,
  churn_risk = EXCLUDED.churn_risk,
  wants_human = EXCLUDED.wants_human,
  classifier = EXCLUDED.classifier,
  model_version = EXCLUDED.model_version,
  degraded = EXCLUDED.degraded,
  needs_review = EXCLUDED.needs_review,
  created_at = EXCLUDED.created_at,
  payload = EXCLUDED.payload;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7857-b585-39ae3bc52365') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO results (
  id, source_event_id, customer_id, intent, intent_prob, frustration, churn_risk,
  wants_human, classifier, model_version, degraded, needs_review, corrected_intent, created_at, payload
) VALUES (
  '01a0e3a4-9a44-770c-b25c-c3d9a7534819', '01a0e3a4-9a44-7888-b69f-7ddfec06a76e', '01a0e3a4-9a44-75be-a38e-f2f87b8089f1', 'Operacional', 0.58, 2, 0.3, 0.6, 'jev', 'seed', false, true, NULL,
  now() - interval '18 minutes', '{"channel":"chat","dist":{"Contato":0.07,"Operacional":0.58,"Reclamação":0.35},"text":"Estou frustrado com a demora pra liberar a transferência. Alguém pode me explicar?"}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  source_event_id = EXCLUDED.source_event_id,
  customer_id = EXCLUDED.customer_id,
  intent = CASE WHEN results.corrected_intent IS NULL THEN EXCLUDED.intent ELSE results.intent END,
  intent_prob = EXCLUDED.intent_prob,
  frustration = EXCLUDED.frustration,
  churn_risk = EXCLUDED.churn_risk,
  wants_human = EXCLUDED.wants_human,
  classifier = EXCLUDED.classifier,
  model_version = EXCLUDED.model_version,
  degraded = EXCLUDED.degraded,
  needs_review = EXCLUDED.needs_review,
  created_at = EXCLUDED.created_at,
  payload = EXCLUDED.payload;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7888-b69f-7ddfec06a76e') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO results (
  id, source_event_id, customer_id, intent, intent_prob, frustration, churn_risk,
  wants_human, classifier, model_version, degraded, needs_review, corrected_intent, created_at, payload
) VALUES (
  '01a0e3a4-9a44-7715-93fc-4ce729753e76', '01a0e3a4-9a44-7894-a788-33bad1a7f8ed', '01a0e3a4-9a44-762d-8db2-85b1e2c338af', 'Câmbio', 0.41, 0, 0.1, 0, 'jev', 'seed', false, true, NULL,
  now() - interval '44 minutes', '{"channel":"chat","dist":{"Câmbio":0.41,"Investimento":0.12,"Operacional":0.38,"Resgate":0.09},"text":"Quero mandar dinheiro pra minha filha que estuda fora."}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  source_event_id = EXCLUDED.source_event_id,
  customer_id = EXCLUDED.customer_id,
  intent = CASE WHEN results.corrected_intent IS NULL THEN EXCLUDED.intent ELSE results.intent END,
  intent_prob = EXCLUDED.intent_prob,
  frustration = EXCLUDED.frustration,
  churn_risk = EXCLUDED.churn_risk,
  wants_human = EXCLUDED.wants_human,
  classifier = EXCLUDED.classifier,
  model_version = EXCLUDED.model_version,
  degraded = EXCLUDED.degraded,
  needs_review = EXCLUDED.needs_review,
  created_at = EXCLUDED.created_at,
  payload = EXCLUDED.payload;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7894-a788-33bad1a7f8ed') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO results (
  id, source_event_id, customer_id, intent, intent_prob, frustration, churn_risk,
  wants_human, classifier, model_version, degraded, needs_review, corrected_intent, created_at, payload
) VALUES (
  '01a0e3a4-9a44-771f-bbb5-cb84467879d1', '01a0e3a4-9a44-789d-acec-2c36f903b28a', '01a0e3a4-9a44-7636-8319-a6d8d039bfbe', 'Operacional', 0.45, 1, 0.2, 0, 'jev', 'seed', false, true, NULL,
  now() - interval '71 minutes', '{"channel":"chat","dist":{"Operacional":0.45,"Reclamação":0.4,"Tributação":0.15},"text":"Isso aqui não está batendo com o extrato."}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  source_event_id = EXCLUDED.source_event_id,
  customer_id = EXCLUDED.customer_id,
  intent = CASE WHEN results.corrected_intent IS NULL THEN EXCLUDED.intent ELSE results.intent END,
  intent_prob = EXCLUDED.intent_prob,
  frustration = EXCLUDED.frustration,
  churn_risk = EXCLUDED.churn_risk,
  wants_human = EXCLUDED.wants_human,
  classifier = EXCLUDED.classifier,
  model_version = EXCLUDED.model_version,
  degraded = EXCLUDED.degraded,
  needs_review = EXCLUDED.needs_review,
  created_at = EXCLUDED.created_at,
  payload = EXCLUDED.payload;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-789d-acec-2c36f903b28a') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO results (
  id, source_event_id, customer_id, intent, intent_prob, frustration, churn_risk,
  wants_human, classifier, model_version, degraded, needs_review, corrected_intent, created_at, payload
) VALUES (
  '01a0e3a4-9a44-7728-9611-2f0e0f111603', '01a0e3a4-9a44-7831-8b59-09e3634d198d', '01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'Reclamação', 0.82, 2, 0.8, 0, 'jev', 'seed', false, true, NULL,
  now() - interval '12 minutes', '{"channel":"chat","dist":{"Encerramento":0.11,"Operacional":0.04,"Reclamação":0.82,"Resgate":0.03},"text":"Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora."}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  source_event_id = EXCLUDED.source_event_id,
  customer_id = EXCLUDED.customer_id,
  intent = CASE WHEN results.corrected_intent IS NULL THEN EXCLUDED.intent ELSE results.intent END,
  intent_prob = EXCLUDED.intent_prob,
  frustration = EXCLUDED.frustration,
  churn_risk = EXCLUDED.churn_risk,
  wants_human = EXCLUDED.wants_human,
  classifier = EXCLUDED.classifier,
  model_version = EXCLUDED.model_version,
  degraded = EXCLUDED.degraded,
  needs_review = EXCLUDED.needs_review,
  created_at = EXCLUDED.created_at,
  payload = EXCLUDED.payload;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7831-8b59-09e3634d198d') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO results (
  id, source_event_id, customer_id, intent, intent_prob, frustration, churn_risk,
  wants_human, classifier, model_version, degraded, needs_review, corrected_intent, created_at, payload
) VALUES (
  '01a0e3a4-9a44-7730-a8ce-5d41d866374e', '01a0e3a4-9a44-7860-83bd-04ce7e7b8777', '01a0e3a4-9a44-7599-93ae-de61ea009668', 'Operacional', 0.71, 1, 0.1, 0, 'jev', 'seed', true, true, NULL,
  now() - interval '50 minutes', '{"channel":"chat","dist":{"Contato":0.1,"Operacional":0.71,"Reclamação":0.19},"text":"Meu cartão foi recusado na viagem, o que eu faço?"}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  source_event_id = EXCLUDED.source_event_id,
  customer_id = EXCLUDED.customer_id,
  intent = CASE WHEN results.corrected_intent IS NULL THEN EXCLUDED.intent ELSE results.intent END,
  intent_prob = EXCLUDED.intent_prob,
  frustration = EXCLUDED.frustration,
  churn_risk = EXCLUDED.churn_risk,
  wants_human = EXCLUDED.wants_human,
  classifier = EXCLUDED.classifier,
  model_version = EXCLUDED.model_version,
  degraded = EXCLUDED.degraded,
  needs_review = EXCLUDED.needs_review,
  created_at = EXCLUDED.created_at,
  payload = EXCLUDED.payload;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7860-83bd-04ce7e7b8777') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO results (
  id, source_event_id, customer_id, intent, intent_prob, frustration, churn_risk,
  wants_human, classifier, model_version, degraded, needs_review, corrected_intent, created_at, payload
) VALUES (
  '01a0e3a4-9a44-773b-b323-a394dbc33377', '01a0e3a4-9a44-7875-933d-50bb6c416da5', '01a0e3a4-9a44-75ab-9a7d-ce6feb41b137', 'Câmbio', 0.79, 1, 0.1, 0, 'jev', 'seed', false, true, NULL,
  now() - interval '120 minutes', '{"channel":"chat","dist":{"Câmbio":0.79,"Operacional":0.06,"Reclamação":0.15},"text":"Qual a cotação que vocês usam no câmbio? Está muito diferente do Google."}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  source_event_id = EXCLUDED.source_event_id,
  customer_id = EXCLUDED.customer_id,
  intent = CASE WHEN results.corrected_intent IS NULL THEN EXCLUDED.intent ELSE results.intent END,
  intent_prob = EXCLUDED.intent_prob,
  frustration = EXCLUDED.frustration,
  churn_risk = EXCLUDED.churn_risk,
  wants_human = EXCLUDED.wants_human,
  classifier = EXCLUDED.classifier,
  model_version = EXCLUDED.model_version,
  degraded = EXCLUDED.degraded,
  needs_review = EXCLUDED.needs_review,
  created_at = EXCLUDED.created_at,
  payload = EXCLUDED.payload;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7875-933d-50bb6c416da5') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO results (
  id, source_event_id, customer_id, intent, intent_prob, frustration, churn_risk,
  wants_human, classifier, model_version, degraded, needs_review, corrected_intent, created_at, payload
) VALUES (
  '01a0e3a4-9a44-7744-b40c-26645b5052eb', '01a0e3a4-9a44-7759-805d-04d97179d63d', '01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'Tributação', 0.85, 0, 0, 0, 'jev', 'seed', false, false, NULL,
  now() - interval '1 minutes', '{"channel":"chat","dist":{"Operacional":0.15,"Tributação":0.85},"text":"fixture at the review threshold"}'::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  source_event_id = EXCLUDED.source_event_id,
  customer_id = EXCLUDED.customer_id,
  intent = CASE WHEN results.corrected_intent IS NULL THEN EXCLUDED.intent ELSE results.intent END,
  intent_prob = EXCLUDED.intent_prob,
  frustration = EXCLUDED.frustration,
  churn_risk = EXCLUDED.churn_risk,
  wants_human = EXCLUDED.wants_human,
  classifier = EXCLUDED.classifier,
  model_version = EXCLUDED.model_version,
  degraded = EXCLUDED.degraded,
  needs_review = EXCLUDED.needs_review,
  created_at = EXCLUDED.created_at,
  payload = EXCLUDED.payload;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7759-805d-04d97179d63d') ON CONFLICT (event_id) DO NOTHING;

