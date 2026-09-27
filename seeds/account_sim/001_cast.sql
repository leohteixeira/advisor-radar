-- account_sim outbox: Mariana timeline lines + market-day burst + alert facts.

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-774f-b570-c0fc7e50e00c',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-774f-b570-c0fc7e50e00c'::text,
    'occurred_at', (now() - interval '12 minutes'),
    'customer_id', '01a0e3a4-9a44-7566-b5de-eb2e365799f8'::text,
    'schema_version', 1,
    'payload', '{"channel":"chat","meta":"Reclamação · Frustrado · risco de saída","text":"Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.","title":"Mensagem · chat"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7759-805d-04d97179d63d',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7759-805d-04d97179d63d'::text,
    'occurred_at', (now() - interval '1500 minutes'),
    'customer_id', '01a0e3a4-9a44-7566-b5de-eb2e365799f8'::text,
    'schema_version', 1,
    'payload', '{"channel":"e-mail","meta":"Operacional · Incomodado","text":"A transferência que pedi na segunda ainda não caiu. Podem verificar?","title":"Mensagem · e-mail"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-776b-95ab-7e661669bb0d',
  'account.event.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-776b-95ab-7e661669bb0d'::text,
    'occurred_at', (now() - interval '4400 minutes'),
    'customer_id', '01a0e3a4-9a44-7566-b5de-eb2e365799f8'::text,
    'schema_version', 1,
    'payload', '{"after":248300,"amount":20000,"before":268300,"kind":"saque","meta":"Sem alerta · 7% do patrimônio","text":"US$ 20.000,00 para conta nos EUA","title":"Saque"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-777d-8b59-ccf9bdef57c5',
  'account.event.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-777d-8b59-ccf9bdef57c5'::text,
    'occurred_at', (now() - interval '21000 minutes'),
    'customer_id', '01a0e3a4-9a44-7566-b5de-eb2e365799f8'::text,
    'schema_version', 1,
    'payload', '{"after":248300,"amount":45000,"before":203300,"kind":"aporte","meta":"Câmbio a 5,42","text":"US$ 45.000,00","title":"Aporte"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-77c4-b12e-b525c28fc927',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-77c4-b12e-b525c28fc927'::text,
    'occurred_at', (now() - interval '0 minutes'),
    'customer_id', '01a0e3a4-9a44-760b-8bad-caa0bc7cab10'::text,
    'schema_version', 1,
    'payload', '{"channel":"email","text":"Ninguém me responde há dois dias. Vou abrir reclamação no Reclame Aqui."}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-77cd-81c8-5c6c767ee85e',
  'account.event.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-77cd-81c8-5c6c767ee85e'::text,
    'occurred_at', (now() - interval '0 minutes'),
    'customer_id', '01a0e3a4-9a44-7615-a44d-8902d124530a'::text,
    'schema_version', 1,
    'payload', '{"after":141000,"amount":55000,"before":196000,"kind":"withdrawal"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-77d5-ba0a-54e58c61238d',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-77d5-ba0a-54e58c61238d'::text,
    'occurred_at', (now() - interval '0 minutes'),
    'customer_id', '01a0e3a4-9a44-7603-bb01-1a44dd2d450c'::text,
    'schema_version', 1,
    'payload', '{"channel":"chat","text":"Bom dia! Como faço para ver o informe de rendimentos de 2025?"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-77df-8a71-d077ff827f97',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-77df-8a71-d077ff827f97'::text,
    'occurred_at', (now() - interval '0 minutes'),
    'customer_id', '01a0e3a4-9a44-7622-b16b-5c5dcfe5a501'::text,
    'schema_version', 1,
    'payload', '{"channel":"chat","text":"Preciso falar com uma pessoa, não com robô."}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-77e8-8195-7fa5735d5e49',
  'account.event.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-77e8-8195-7fa5735d5e49'::text,
    'occurred_at', (now() - interval '0 minutes'),
    'customer_id', '01a0e3a4-9a44-7636-8319-a6d8d039bfbe'::text,
    'schema_version', 1,
    'payload', '{"after":67000,"amount":-12000,"before":79000,"kind":"asset_drop"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-77f3-8878-39b428b5177d',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-77f3-8878-39b428b5177d'::text,
    'occurred_at', (now() - interval '0 minutes'),
    'customer_id', '01a0e3a4-9a44-762d-8db2-85b1e2c338af'::text,
    'schema_version', 1,
    'payload', '{"channel":"email","text":"Como declaro os dividendos recebidos em dólar?"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-77fc-a69d-3b5115c6333e',
  'account.event.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-77fc-a69d-3b5115c6333e'::text,
    'occurred_at', (now() - interval '22 minutes'),
    'customer_id', '01a0e3a4-9a44-75c7-91c3-f5c3250f406c'::text,
    'schema_version', 1,
    'payload', '{"after":450000,"amount":190000,"before":640000,"kind":"withdrawal"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7805-a11e-75352ed7001e',
  'account.event.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7805-a11e-75352ed7001e'::text,
    'occurred_at', (now() - interval '180 minutes'),
    'customer_id', '01a0e3a4-9a44-75d0-bb2f-41cc8876e922'::text,
    'schema_version', 1,
    'payload', '{"after":108000,"amount":-22000,"before":130000,"kind":"asset_drop"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-780e-a22c-0e402fabbd32',
  'account.event.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-780e-a22c-0e402fabbd32'::text,
    'occurred_at', (now() - interval '240 minutes'),
    'customer_id', '01a0e3a4-9a44-75dd-b3a0-403a7a87836e'::text,
    'schema_version', 1,
    'payload', '{"after":68000,"amount":60000,"before":8000,"kind":"deposit"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-781f-aa8d-cacee20796c5',
  'account.event.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-781f-aa8d-cacee20796c5'::text,
    'occurred_at', (now() - interval '6 minutes'),
    'customer_id', '01a0e3a4-9a44-75ee-ad63-ef224b47aaeb'::text,
    'schema_version', 1,
    'payload', '{"after":950000,"amount":300000,"before":1250000,"kind":"withdrawal"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7827-843f-c87f97bc7066',
  'account.event.recorded',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7827-843f-c87f97bc7066'::text,
    'occurred_at', (now() - interval '300 minutes'),
    'customer_id', '01a0e3a4-9a44-75fa-ac0a-25beade29a12'::text,
    'schema_version', 1,
    'payload', '{"after":7500,"amount":-2100,"before":9600,"kind":"asset_drop"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7831-8b59-09e3634d198d',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7831-8b59-09e3634d198d'::text,
    'occurred_at', (now() - interval '12 minutes'),
    'customer_id', '01a0e3a4-9a44-7566-b5de-eb2e365799f8'::text,
    'schema_version', 1,
    'payload', '{"channel":"chat","text":"Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora."}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-783a-82d1-a8e32168c44c',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-783a-82d1-a8e32168c44c'::text,
    'occurred_at', (now() - interval '25 minutes'),
    'customer_id', '01a0e3a4-9a44-7571-9cd6-29d603ed75d1'::text,
    'schema_version', 1,
    'payload', '{"channel":"e-mail","text":"Já é a terceira vez que eu explico o mesmo problema e ninguém resolve. Um absurdo."}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7844-ac70-f9372bd01f7c',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7844-ac70-f9372bd01f7c'::text,
    'occurred_at', (now() - interval '8 minutes'),
    'customer_id', '01a0e3a4-9a44-757a-ac8f-dab7db5eb068'::text,
    'schema_version', 1,
    'payload', '{"channel":"chat","text":"Consegue pedir para o meu assessor me ligar?"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-784e-a54a-ac95cd58ec5c',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-784e-a54a-ac95cd58ec5c'::text,
    'occurred_at', (now() - interval '100 minutes'),
    'customer_id', '01a0e3a4-9a44-7585-bc68-ce1a803f76a7'::text,
    'schema_version', 1,
    'payload', '{"channel":"e-mail","text":"Vendi ações com lucro em julho. Tenho que pagar DARF?"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7857-b585-39ae3bc52365',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7857-b585-39ae3bc52365'::text,
    'occurred_at', (now() - interval '33 minutes'),
    'customer_id', '01a0e3a4-9a44-7590-b715-07e7c4798c60'::text,
    'schema_version', 1,
    'payload', '{"channel":"chat","text":"Preciso sacar 5 mil dólares e trazer de volta para o Brasil."}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7860-83bd-04ce7e7b8777',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7860-83bd-04ce7e7b8777'::text,
    'occurred_at', (now() - interval '50 minutes'),
    'customer_id', '01a0e3a4-9a44-7599-93ae-de61ea009668'::text,
    'schema_version', 1,
    'payload', '{"channel":"chat","text":"Meu cartão foi recusado na viagem, o que eu faço?"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-786a-af65-90053c789720',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-786a-af65-90053c789720'::text,
    'occurred_at', (now() - interval '65 minutes'),
    'customer_id', '01a0e3a4-9a44-75a2-b139-40376b3a76ea'::text,
    'schema_version', 1,
    'payload', '{"channel":"e-mail","text":"Quero encerrar minha conta. Como faço para transferir os ativos?"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7875-933d-50bb6c416da5',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7875-933d-50bb6c416da5'::text,
    'occurred_at', (now() - interval '120 minutes'),
    'customer_id', '01a0e3a4-9a44-75ab-9a7d-ce6feb41b137'::text,
    'schema_version', 1,
    'payload', '{"channel":"chat","text":"Qual a cotação que vocês usam no câmbio? Está muito diferente do Google."}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-787f-9d84-2f6659d73c0b',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-787f-9d84-2f6659d73c0b'::text,
    'occurred_at', (now() - interval '185 minutes'),
    'customer_id', '01a0e3a4-9a44-75b5-b359-8d4428470a99'::text,
    'schema_version', 1,
    'payload', '{"channel":"e-mail","text":"Vocês têm alguma recomendação de renda fixa com vencimento em 2028?"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7888-b69f-7ddfec06a76e',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7888-b69f-7ddfec06a76e'::text,
    'occurred_at', (now() - interval '18 minutes'),
    'customer_id', '01a0e3a4-9a44-75be-a38e-f2f87b8089f1'::text,
    'schema_version', 1,
    'payload', '{"channel":"chat","text":"Estou frustrado com a demora pra liberar a transferência. Alguém pode me explicar?"}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7894-a788-33bad1a7f8ed',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7894-a788-33bad1a7f8ed'::text,
    'occurred_at', (now() - interval '44 minutes'),
    'customer_id', '01a0e3a4-9a44-762d-8db2-85b1e2c338af'::text,
    'schema_version', 1,
    'payload', '{"channel":"chat","text":"Quero mandar dinheiro pra minha filha que estuda fora."}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-789d-acec-2c36f903b28a',
  'message.received',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-789d-acec-2c36f903b28a'::text,
    'occurred_at', (now() - interval '71 minutes'),
    'customer_id', '01a0e3a4-9a44-7636-8319-a6d8d039bfbe'::text,
    'schema_version', 1,
    'payload', '{"channel":"e-mail","text":"Isso aqui não está batendo com o extrato."}'::jsonb
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

