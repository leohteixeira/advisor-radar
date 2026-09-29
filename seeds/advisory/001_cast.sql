-- Advisory cast: operators, book, queue signals, alerts, notes, inbox, outbox.
-- Reseed refreshes clocks of these ids only; unknown ids are left alone.

INSERT INTO operators (id, name) VALUES
  ('01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', 'Ana Paula Ribeiro'),
  ('01a0e3a4-9a44-7552-8de2-e88e47b9affc', 'Bruno Dias'),
  ('01a0e3a4-9a44-755c-910e-1043a3fc1e60', 'Carla Menezes')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;

-- Investor profiles (migrations/advisory/003): Fernanda conservador
-- 2026-03-12, Mariana moderado 2026-01-20, Thiago arrojado 2026-08-04. Every
-- other book customer comes from one fixed-seed draw, kept here as literals:
-- Go math/rand/v2 rand.New(rand.NewPCG(20260929, 8)), in book order, the
-- profile from IntN(3) over (conservador, moderado, arrojado), then the date
-- 2025-09-01 plus IntN(365) days.
INSERT INTO book (customer_id, name, segment, aum, advisor_id, since, investor_profile, profile_assessed_on) VALUES
  ('01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'Mariana Costa', 'Singular', 248300, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2021', 'moderado', '2026-01-20'),
  ('01a0e3a4-9a44-7571-9cd6-29d603ed75d1', 'Paulo Henrique Souza', 'Advance', 96400, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2022', 'conservador', '2026-01-26'),
  ('01a0e3a4-9a44-757a-ac8f-dab7db5eb068', 'Fernanda Lima', 'Essencial', 8200, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2024', 'conservador', '2026-03-12'),
  ('01a0e3a4-9a44-7585-bc68-ce1a803f76a7', 'Carlos Eduardo Ramos', 'Advance', 142000, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2020', 'arrojado', '2025-10-23'),
  ('01a0e3a4-9a44-7590-b715-07e7c4798c60', 'Juliana Martins', 'Singular', 512900, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2019', 'moderado', '2025-09-20'),
  ('01a0e3a4-9a44-7599-93ae-de61ea009668', 'Roberto Nascimento', 'Essencial', 7800, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2025', 'moderado', '2025-11-05'),
  ('01a0e3a4-9a44-75a2-b139-40376b3a76ea', 'Ana Beatriz Oliveira', 'Advance', 190500, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2021', 'moderado', '2026-07-27'),
  ('01a0e3a4-9a44-75ab-9a7d-ce6feb41b137', 'Lucas Pereira', 'Essencial', 6100, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2023', 'moderado', '2025-09-07'),
  ('01a0e3a4-9a44-75b5-b359-8d4428470a99', 'Patrícia Gomes', 'Singular', 780000, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2018', 'arrojado', '2026-08-09'),
  ('01a0e3a4-9a44-75be-a38e-f2f87b8089f1', 'Marcelo Ferreira', 'Advance', 88000, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2022', 'conservador', '2026-05-02'),
  ('01a0e3a4-9a44-75c7-91c3-f5c3250f406c', 'Sérgio Cardoso', 'Singular', 450000, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2019', 'moderado', '2026-06-08'),
  ('01a0e3a4-9a44-75d0-bb2f-41cc8876e922', 'Helena Barbosa', 'Advance', 108000, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2023', 'arrojado', '2026-03-02'),
  ('01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 'Thiago Azevedo', 'Advance', 68000, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2024', 'arrojado', '2026-08-04'),
  ('01a0e3a4-9a44-75e6-a8e6-8bb9dcdda086', 'Camila Rodrigues', 'Advance', 175000, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2020', 'moderado', '2025-12-26'),
  ('01a0e3a4-9a44-75ee-ad63-ef224b47aaeb', 'Rafael Monteiro', 'Singular', 950000, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2017', 'moderado', '2026-05-09'),
  ('01a0e3a4-9a44-75fa-ac0a-25beade29a12', 'Beatriz Santana', 'Essencial', 7500, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2025', 'arrojado', '2026-04-05'),
  ('01a0e3a4-9a44-7603-bb01-1a44dd2d450c', 'Diego Carvalho', 'Essencial', 8900, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2024', 'arrojado', '2025-09-22'),
  ('01a0e3a4-9a44-760b-8bad-caa0bc7cab10', 'Vanessa Moreira', 'Advance', 119000, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2021', 'moderado', '2025-10-17'),
  ('01a0e3a4-9a44-7615-a44d-8902d124530a', 'Gustavo Teixeira', 'Advance', 141000, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2022', 'arrojado', '2026-07-29'),
  ('01a0e3a4-9a44-7622-b16b-5c5dcfe5a501', 'Isabela Nunes', 'Essencial', 9400, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2025', 'arrojado', '2026-03-10'),
  ('01a0e3a4-9a44-762d-8db2-85b1e2c338af', 'Otávio Freitas', 'Essencial', 5800, '01a0e3a4-9a44-7552-8de2-e88e47b9affc', '2024', 'moderado', '2026-05-07'),
  ('01a0e3a4-9a44-7636-8319-a6d8d039bfbe', 'Renata Albuquerque', 'Advance', 67000, '01a0e3a4-9a44-755c-910e-1043a3fc1e60', '2023', 'arrojado', '2026-08-19')
ON CONFLICT (customer_id) DO UPDATE SET
  name = EXCLUDED.name,
  segment = EXCLUDED.segment,
  aum = EXCLUDED.aum,
  advisor_id = EXCLUDED.advisor_id,
  since = EXCLUDED.since,
  investor_profile = EXCLUDED.investor_profile,
  profile_assessed_on = EXCLUDED.profile_assessed_on;

-- Queue message signals (reseed updates raised_at).
INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-7640-9d53-0fb421112eed', '01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'message', now() - interval '12 minutes', '{"channel":"chat","churn":true,"churnConf":"alta","dist":{"Encerramento":0.11,"Operacional":0.04,"Reclamação":0.82,"Resgate":0.03},"fallback":false,"frustration":2,"human":false,"intent":"Reclamação","text":"Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora."}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-764a-90c1-20f5fd0964b8', '01a0e3a4-9a44-7571-9cd6-29d603ed75d1', 'message', now() - interval '25 minutes', '{"channel":"e-mail","churn":false,"churnConf":"média","dist":{"Encerramento":0.03,"Operacional":0.06,"Reclamação":0.91},"fallback":false,"frustration":3,"human":false,"intent":"Reclamação","text":"Já é a terceira vez que eu explico o mesmo problema e ninguém resolve. Um absurdo."}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-7653-b766-1a6074a0db6b', '01a0e3a4-9a44-757a-ac8f-dab7db5eb068', 'message', now() - interval '8 minutes', '{"channel":"chat","churn":false,"churnConf":"alta","dist":{"Contato":0.94,"Investimento":0.02,"Operacional":0.04},"fallback":false,"frustration":0,"human":true,"intent":"Contato","text":"Consegue pedir para o meu assessor me ligar?"}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-765d-a2d1-2865bbc6c249', '01a0e3a4-9a44-7585-bc68-ce1a803f76a7', 'message', now() - interval '100 minutes', '{"channel":"e-mail","churn":false,"churnConf":"alta","dist":{"Investimento":0.08,"Operacional":0.04,"Tributação":0.88},"fallback":false,"frustration":0,"human":false,"intent":"Tributação","text":"Vendi ações com lucro em julho. Tenho que pagar DARF?"}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-7668-b012-f094b514fc33', '01a0e3a4-9a44-7590-b715-07e7c4798c60', 'message', now() - interval '33 minutes', '{"channel":"chat","churn":false,"churnConf":"média","dist":{"Câmbio":0.31,"Encerramento":0.05,"Operacional":0.1,"Resgate":0.54},"fallback":false,"frustration":0,"human":false,"intent":"Resgate","text":"Preciso sacar 5 mil dólares e trazer de volta para o Brasil."}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-7671-9883-fa3f35e6bb18', '01a0e3a4-9a44-7599-93ae-de61ea009668', 'message', now() - interval '50 minutes', '{"channel":"chat","churn":false,"churnConf":"alta","dist":{"Contato":0.1,"Operacional":0.71,"Reclamação":0.19},"fallback":true,"frustration":1,"human":false,"intent":"Operacional","text":"Meu cartão foi recusado na viagem, o que eu faço?"}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-767c-a2ae-1520073b09dd', '01a0e3a4-9a44-75a2-b139-40376b3a76ea', 'message', now() - interval '65 minutes', '{"channel":"e-mail","churn":true,"churnConf":"alta","dist":{"Encerramento":0.86,"Operacional":0.05,"Resgate":0.09},"fallback":false,"frustration":0,"human":false,"intent":"Encerramento","text":"Quero encerrar minha conta. Como faço para transferir os ativos?"}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-7685-8212-624e2d01d6e0', '01a0e3a4-9a44-75ab-9a7d-ce6feb41b137', 'message', now() - interval '120 minutes', '{"channel":"chat","churn":false,"churnConf":"alta","dist":{"Câmbio":0.79,"Operacional":0.06,"Reclamação":0.15},"fallback":false,"frustration":1,"human":false,"intent":"Câmbio","text":"Qual a cotação que vocês usam no câmbio? Está muito diferente do Google."}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-768f-b9d1-3d7f36df3063', '01a0e3a4-9a44-75b5-b359-8d4428470a99', 'message', now() - interval '185 minutes', '{"channel":"e-mail","churn":false,"churnConf":"alta","dist":{"Investimento":0.93,"Operacional":0.05,"Tributação":0.02},"fallback":false,"frustration":0,"human":false,"intent":"Investimento","text":"Vocês têm alguma recomendação de renda fixa com vencimento em 2028?"}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-7699-8dcd-fc4944b3f50a', '01a0e3a4-9a44-75be-a38e-f2f87b8089f1', 'message', now() - interval '18 minutes', '{"channel":"chat","churn":false,"churnConf":"média","dist":{"Contato":0.07,"Operacional":0.58,"Reclamação":0.35},"fallback":false,"frustration":2,"human":true,"intent":"Operacional","text":"Estou frustrado com a demora pra liberar a transferência. Alguém pode me explicar?"}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-76a2-b210-d5a1e4624b33', '01a0e3a4-9a44-75c7-91c3-f5c3250f406c', 'alert', now() - interval '22 minutes', '{"after":450000,"alert":"saque","amount":190000,"before":640000,"days":0,"from":"","reason":"Saque de US$ 190.000,00, 30% do patrimônio","rule":"Saque acima de 20% do patrimônio em 24 horas","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO alerts (id, customer_id, kind, rule, source_event_id, raised_at, payload)
VALUES ('01a0e3a4-9a44-76a2-b210-d5a1e4624b33', '01a0e3a4-9a44-75c7-91c3-f5c3250f406c', 'saque', 'Saque acima de 20% do patrimônio em 24 horas', '01a0e3a4-9a44-77fc-a69d-3b5115c6333e', now() - interval '22 minutes', '{"after":450000,"amount":190000,"before":640000,"days":0,"from":"","kind":"saque","reason":"Saque de US$ 190.000,00, 30% do patrimônio","rule":"Saque acima de 20% do patrimônio em 24 horas","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  rule = EXCLUDED.rule,
  source_event_id = EXCLUDED.source_event_id,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-76a2-b210-d5a1e4624b33',
  'alert.raised',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-76a2-b210-d5a1e4624b33'::text,
    'occurred_at', (now() - interval '22 minutes'),
    'customer_id', '01a0e3a4-9a44-75c7-91c3-f5c3250f406c'::text,
    'schema_version', 1,
    'payload', '{"after":450000,"amount":190000,"before":640000,"days":0,"from":"","kind":"saque","reason":"Saque de US$ 190.000,00, 30% do patrimônio","rule":"Saque acima de 20% do patrimônio em 24 horas","to":""}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-77fc-a69d-3b5115c6333e') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-76ae-8448-401d18db48b1', '01a0e3a4-9a44-75d0-bb2f-41cc8876e922', 'alert', now() - interval '180 minutes', '{"after":108000,"alert":"queda","amount":-22000,"before":130000,"days":0,"from":"","reason":"Patrimônio caiu 17% em 5 dias","rule":"Queda acima de 15% em 5 dias úteis","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO alerts (id, customer_id, kind, rule, source_event_id, raised_at, payload)
VALUES ('01a0e3a4-9a44-76ae-8448-401d18db48b1', '01a0e3a4-9a44-75d0-bb2f-41cc8876e922', 'queda', 'Queda acima de 15% em 5 dias úteis', '01a0e3a4-9a44-7805-a11e-75352ed7001e', now() - interval '180 minutes', '{"after":108000,"amount":-22000,"before":130000,"days":0,"from":"","kind":"queda","reason":"Patrimônio caiu 17% em 5 dias","rule":"Queda acima de 15% em 5 dias úteis","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  rule = EXCLUDED.rule,
  source_event_id = EXCLUDED.source_event_id,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-76ae-8448-401d18db48b1',
  'alert.raised',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-76ae-8448-401d18db48b1'::text,
    'occurred_at', (now() - interval '180 minutes'),
    'customer_id', '01a0e3a4-9a44-75d0-bb2f-41cc8876e922'::text,
    'schema_version', 1,
    'payload', '{"after":108000,"amount":-22000,"before":130000,"days":0,"from":"","kind":"queda","reason":"Patrimônio caiu 17% em 5 dias","rule":"Queda acima de 15% em 5 dias úteis","to":""}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7805-a11e-75352ed7001e') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-76b8-8ac9-c7954c163203', '01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 'alert', now() - interval '240 minutes', '{"after":68000,"alert":"aporte","amount":60000,"before":8000,"days":0,"from":"","reason":"Aporte de US$ 60.000,00, 7,5× o patrimônio","rule":"Aporte maior que o patrimônio anterior","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO alerts (id, customer_id, kind, rule, source_event_id, raised_at, payload)
VALUES ('01a0e3a4-9a44-76b8-8ac9-c7954c163203', '01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 'aporte', 'Aporte maior que o patrimônio anterior', '01a0e3a4-9a44-780e-a22c-0e402fabbd32', now() - interval '240 minutes', '{"after":68000,"amount":60000,"before":8000,"days":0,"from":"","kind":"aporte","reason":"Aporte de US$ 60.000,00, 7,5× o patrimônio","rule":"Aporte maior que o patrimônio anterior","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  rule = EXCLUDED.rule,
  source_event_id = EXCLUDED.source_event_id,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-76b8-8ac9-c7954c163203',
  'alert.raised',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-76b8-8ac9-c7954c163203'::text,
    'occurred_at', (now() - interval '240 minutes'),
    'customer_id', '01a0e3a4-9a44-75dd-b3a0-403a7a87836e'::text,
    'schema_version', 1,
    'payload', '{"after":68000,"amount":60000,"before":8000,"days":0,"from":"","kind":"aporte","reason":"Aporte de US$ 60.000,00, 7,5× o patrimônio","rule":"Aporte maior que o patrimônio anterior","to":""}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-780e-a22c-0e402fabbd32') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-76c1-abf5-e32044ae0362', '01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 'alert', now() - interval '238 minutes', '{"after":68000,"alert":"segmento","amount":0,"before":8000,"days":0,"from":"Essencial","reason":"Subiu de Essencial para Advance","rule":"Patrimônio cruzou a faixa de US$ 10 mil","to":"Advance"}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO alerts (id, customer_id, kind, rule, source_event_id, raised_at, payload)
VALUES ('01a0e3a4-9a44-76c1-abf5-e32044ae0362', '01a0e3a4-9a44-75dd-b3a0-403a7a87836e', 'segmento', 'Patrimônio cruzou a faixa de US$ 10 mil', '01a0e3a4-9a44-780e-a22c-0e402fabbd32', now() - interval '238 minutes', '{"after":68000,"amount":0,"before":8000,"days":0,"from":"Essencial","kind":"segmento","reason":"Subiu de Essencial para Advance","rule":"Patrimônio cruzou a faixa de US$ 10 mil","to":"Advance"}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  rule = EXCLUDED.rule,
  source_event_id = EXCLUDED.source_event_id,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-76c1-abf5-e32044ae0362',
  'alert.raised',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-76c1-abf5-e32044ae0362'::text,
    'occurred_at', (now() - interval '238 minutes'),
    'customer_id', '01a0e3a4-9a44-75dd-b3a0-403a7a87836e'::text,
    'schema_version', 1,
    'payload', '{"after":68000,"amount":0,"before":8000,"days":0,"from":"Essencial","kind":"segmento","reason":"Subiu de Essencial para Advance","rule":"Patrimônio cruzou a faixa de US$ 10 mil","to":"Advance"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-780e-a22c-0e402fabbd32') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-76cb-8607-7c79cbc05cde', '01a0e3a4-9a44-75e6-a8e6-8bb9dcdda086', 'alert', now() - interval '1440 minutes', '{"after":175000,"alert":"contato","amount":0,"before":175000,"days":94,"from":"","reason":"Último contato há 94 dias","rule":"Sem contato há mais de 90 dias","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO alerts (id, customer_id, kind, rule, source_event_id, raised_at, payload)
VALUES ('01a0e3a4-9a44-76cb-8607-7c79cbc05cde', '01a0e3a4-9a44-75e6-a8e6-8bb9dcdda086', 'contato', 'Sem contato há mais de 90 dias', '01a0e3a4-9a44-7816-9ee9-b9cffa56cafc', now() - interval '1440 minutes', '{"after":175000,"amount":0,"before":175000,"days":94,"from":"","kind":"contato","reason":"Último contato há 94 dias","rule":"Sem contato há mais de 90 dias","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  rule = EXCLUDED.rule,
  source_event_id = EXCLUDED.source_event_id,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-76cb-8607-7c79cbc05cde',
  'alert.raised',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-76cb-8607-7c79cbc05cde'::text,
    'occurred_at', (now() - interval '1440 minutes'),
    'customer_id', '01a0e3a4-9a44-75e6-a8e6-8bb9dcdda086'::text,
    'schema_version', 1,
    'payload', '{"after":175000,"amount":0,"before":175000,"days":94,"from":"","kind":"contato","reason":"Último contato há 94 dias","rule":"Sem contato há mais de 90 dias","to":""}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7816-9ee9-b9cffa56cafc') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-76d4-bcd2-a21f501cb1e9', '01a0e3a4-9a44-75ee-ad63-ef224b47aaeb', 'alert', now() - interval '6 minutes', '{"after":950000,"alert":"saque","amount":300000,"before":1250000,"days":0,"from":"","reason":"Saque de US$ 300.000,00, 24% do patrimônio","rule":"Saque acima de 20% do patrimônio em 24 horas","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO alerts (id, customer_id, kind, rule, source_event_id, raised_at, payload)
VALUES ('01a0e3a4-9a44-76d4-bcd2-a21f501cb1e9', '01a0e3a4-9a44-75ee-ad63-ef224b47aaeb', 'saque', 'Saque acima de 20% do patrimônio em 24 horas', '01a0e3a4-9a44-781f-aa8d-cacee20796c5', now() - interval '6 minutes', '{"after":950000,"amount":300000,"before":1250000,"days":0,"from":"","kind":"saque","reason":"Saque de US$ 300.000,00, 24% do patrimônio","rule":"Saque acima de 20% do patrimônio em 24 horas","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  rule = EXCLUDED.rule,
  source_event_id = EXCLUDED.source_event_id,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-76d4-bcd2-a21f501cb1e9',
  'alert.raised',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-76d4-bcd2-a21f501cb1e9'::text,
    'occurred_at', (now() - interval '6 minutes'),
    'customer_id', '01a0e3a4-9a44-75ee-ad63-ef224b47aaeb'::text,
    'schema_version', 1,
    'payload', '{"after":950000,"amount":300000,"before":1250000,"days":0,"from":"","kind":"saque","reason":"Saque de US$ 300.000,00, 24% do patrimônio","rule":"Saque acima de 20% do patrimônio em 24 horas","to":""}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-781f-aa8d-cacee20796c5') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO queue_signals (id, customer_id, kind, raised_at, payload)
VALUES ('01a0e3a4-9a44-76dd-8ef9-e0907926fdf2', '01a0e3a4-9a44-75fa-ac0a-25beade29a12', 'alert', now() - interval '300 minutes', '{"after":7500,"alert":"queda","amount":-2100,"before":9600,"days":0,"from":"","reason":"Patrimônio caiu 22% em 5 dias","rule":"Queda acima de 15% em 5 dias úteis","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO alerts (id, customer_id, kind, rule, source_event_id, raised_at, payload)
VALUES ('01a0e3a4-9a44-76dd-8ef9-e0907926fdf2', '01a0e3a4-9a44-75fa-ac0a-25beade29a12', 'queda', 'Queda acima de 15% em 5 dias úteis', '01a0e3a4-9a44-7827-843f-c87f97bc7066', now() - interval '300 minutes', '{"after":7500,"amount":-2100,"before":9600,"days":0,"from":"","kind":"queda","reason":"Patrimônio caiu 22% em 5 dias","rule":"Queda acima de 15% em 5 dias úteis","to":""}'::jsonb)
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  rule = EXCLUDED.rule,
  source_event_id = EXCLUDED.source_event_id,
  raised_at = EXCLUDED.raised_at,
  payload = EXCLUDED.payload;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-76dd-8ef9-e0907926fdf2',
  'alert.raised',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-76dd-8ef9-e0907926fdf2'::text,
    'occurred_at', (now() - interval '300 minutes'),
    'customer_id', '01a0e3a4-9a44-75fa-ac0a-25beade29a12'::text,
    'schema_version', 1,
    'payload', '{"after":7500,"amount":-2100,"before":9600,"days":0,"from":"","kind":"queda","reason":"Patrimônio caiu 22% em 5 dias","rule":"Queda acima de 15% em 5 dias úteis","to":""}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7827-843f-c87f97bc7066') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO notes (id, customer_id, kind, title, text, meta, occurred_at)
VALUES
  ('01a0e3a4-9a44-7763-bae8-4b24a2f3f3f2', '01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'nota', 'Nota do assessor', 'Cliente pretende comprar imóvel em Orlando no 1º semestre. Precisa de liquidez em março.', 'Ana Paula Ribeiro', now() - interval '2900 minutes'),
  ('01a0e3a4-9a44-77bb-bc23-a163dfc47fe5', '01a0e3a4-9a44-7566-b5de-eb2e365799f8', 'telefone', 'Ligação', 'Revisão semestral da carteira · 32 min', 'Ana Paula Ribeiro', now() - interval '43000 minutes')
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  kind = EXCLUDED.kind,
  title = EXCLUDED.title,
  text = EXCLUDED.text,
  meta = EXCLUDED.meta,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7763-bae8-4b24a2f3f3f2',
  'advisory.note.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7763-bae8-4b24a2f3f3f2'::text,
    'occurred_at', (now() - interval '2900 minutes'),
    'customer_id', '01a0e3a4-9a44-7566-b5de-eb2e365799f8'::text,
    'schema_version', 1,
    'payload', '{"kind":"nota","meta":"Ana Paula Ribeiro","text":"Cliente pretende comprar imóvel em Orlando no 1º semestre. Precisa de liquidez em março.","title":"Nota do assessor"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-77bb-bc23-a163dfc47fe5',
  'advisory.note.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-77bb-bc23-a163dfc47fe5'::text,
    'occurred_at', (now() - interval '43000 minutes'),
    'customer_id', '01a0e3a4-9a44-7566-b5de-eb2e365799f8'::text,
    'schema_version', 1,
    'payload', '{"kind":"telefone","meta":"Ana Paula Ribeiro","text":"Revisão semestral da carteira · 32 min","title":"Ligação"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

