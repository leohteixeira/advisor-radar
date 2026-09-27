-- Cases cast: Paulo, Ana Beatriz, Patrícia with Ana's operator id snapshotted.

INSERT INTO cases (id, customer_id, signal_id, advisor_id, state, sla_total_minutes, escalated, opened_at)
VALUES ('01a0e3a4-9a44-76e9-9644-aec5cdb8be90', '01a0e3a4-9a44-7571-9cd6-29d603ed75d1', '01a0e3a4-9a44-764a-90c1-20f5fd0964b8', '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', 'Em atendimento', 120, false, now() - interval '25 minutes')
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  signal_id = EXCLUDED.signal_id,
  advisor_id = EXCLUDED.advisor_id,
  state = EXCLUDED.state,
  sla_total_minutes = EXCLUDED.sla_total_minutes,
  escalated = EXCLUDED.escalated,
  opened_at = EXCLUDED.opened_at;

INSERT INTO cases (id, customer_id, signal_id, advisor_id, state, sla_total_minutes, escalated, opened_at)
VALUES ('01a0e3a4-9a44-76f2-9263-f892becdd32a', '01a0e3a4-9a44-75a2-b139-40376b3a76ea', '01a0e3a4-9a44-767c-a2ae-1520073b09dd', '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', 'Aguardando cliente', 120, true, now() - interval '65 minutes')
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  signal_id = EXCLUDED.signal_id,
  advisor_id = EXCLUDED.advisor_id,
  state = EXCLUDED.state,
  sla_total_minutes = EXCLUDED.sla_total_minutes,
  escalated = EXCLUDED.escalated,
  opened_at = EXCLUDED.opened_at;

INSERT INTO cases (id, customer_id, signal_id, advisor_id, state, sla_total_minutes, escalated, opened_at)
VALUES ('01a0e3a4-9a44-76fb-9df9-047e1974d2f6', '01a0e3a4-9a44-75b5-b359-8d4428470a99', '01a0e3a4-9a44-768f-b9d1-3d7f36df3063', '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', 'Resolvido', 60, false, now() - interval '400 minutes')
ON CONFLICT (id) DO UPDATE SET
  customer_id = EXCLUDED.customer_id,
  signal_id = EXCLUDED.signal_id,
  advisor_id = EXCLUDED.advisor_id,
  state = EXCLUDED.state,
  sla_total_minutes = EXCLUDED.sla_total_minutes,
  escalated = EXCLUDED.escalated,
  opened_at = EXCLUDED.opened_at;

INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ('01a0e3a4-9a44-78a7-a68a-427155c2f05b', '01a0e3a4-9a44-76e9-9644-aec5cdb8be90', 'caso', 'Caso aberto a partir de mensagem com reclamação', now() - interval '25 minutes')
ON CONFLICT (id) DO UPDATE SET
  case_id = EXCLUDED.case_id,
  kind = EXCLUDED.kind,
  text = EXCLUDED.text,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ('01a0e3a4-9a44-78b2-ae12-1daf6e973e19', '01a0e3a4-9a44-76e9-9644-aec5cdb8be90', 'caso', 'Em atendimento · Ana Paula Ribeiro', now() - interval '21 minutes')
ON CONFLICT (id) DO UPDATE SET
  case_id = EXCLUDED.case_id,
  kind = EXCLUDED.kind,
  text = EXCLUDED.text,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ('01a0e3a4-9a44-78bc-8174-27091ac81530', '01a0e3a4-9a44-76e9-9644-aec5cdb8be90', 'nota', 'Problema é o bloqueio da transferência internacional desde 12/09. Compliance pediu comprovante de origem.', now() - interval '19 minutes')
ON CONFLICT (id) DO UPDATE SET
  case_id = EXCLUDED.case_id,
  kind = EXCLUDED.kind,
  text = EXCLUDED.text,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ('01a0e3a4-9a44-78c6-b9e2-1f7db4129596', '01a0e3a4-9a44-76f2-9263-f892becdd32a', 'caso', 'Caso aberto a partir de pedido de encerramento', now() - interval '65 minutes')
ON CONFLICT (id) DO UPDATE SET
  case_id = EXCLUDED.case_id,
  kind = EXCLUDED.kind,
  text = EXCLUDED.text,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ('01a0e3a4-9a44-78ce-a15a-63985e4d6077', '01a0e3a4-9a44-76f2-9263-f892becdd32a', 'caso', 'Escalonado automaticamente: risco de saída alto em cliente Advance', now() - interval '60 minutes')
ON CONFLICT (id) DO UPDATE SET
  case_id = EXCLUDED.case_id,
  kind = EXCLUDED.kind,
  text = EXCLUDED.text,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ('01a0e3a4-9a44-78d8-8865-3df82bf725ee', '01a0e3a4-9a44-76f2-9263-f892becdd32a', 'telefone', 'Ligação · 9 min · cliente insatisfeita com taxas de câmbio', now() - interval '58 minutes')
ON CONFLICT (id) DO UPDATE SET
  case_id = EXCLUDED.case_id,
  kind = EXCLUDED.kind,
  text = EXCLUDED.text,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ('01a0e3a4-9a44-78e2-ac0d-b5988756240e', '01a0e3a4-9a44-76f2-9263-f892becdd32a', 'caso', 'Aguardando cliente · proposta de isenção enviada por e-mail', now() - interval '55 minutes')
ON CONFLICT (id) DO UPDATE SET
  case_id = EXCLUDED.case_id,
  kind = EXCLUDED.kind,
  text = EXCLUDED.text,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ('01a0e3a4-9a44-78ec-b118-129671e9b7ac', '01a0e3a4-9a44-76fb-9df9-047e1974d2f6', 'caso', 'Caso aberto', now() - interval '400 minutes')
ON CONFLICT (id) DO UPDATE SET
  case_id = EXCLUDED.case_id,
  kind = EXCLUDED.kind,
  text = EXCLUDED.text,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ('01a0e3a4-9a44-78f7-b83a-d40c869877f8', '01a0e3a4-9a44-76fb-9df9-047e1974d2f6', 'telefone', 'Ligação · 14 min · sugestão de Treasuries 2028', now() - interval '380 minutes')
ON CONFLICT (id) DO UPDATE SET
  case_id = EXCLUDED.case_id,
  kind = EXCLUDED.kind,
  text = EXCLUDED.text,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ('01a0e3a4-9a44-7900-aff7-bb632e5b301c', '01a0e3a4-9a44-76fb-9df9-047e1974d2f6', 'caso', 'Resolvido', now() - interval '370 minutes')
ON CONFLICT (id) DO UPDATE SET
  case_id = EXCLUDED.case_id,
  kind = EXCLUDED.kind,
  text = EXCLUDED.text,
  occurred_at = EXCLUDED.occurred_at;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7774-b190-2aa228e74df7',
  'case.status.changed',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7774-b190-2aa228e74df7'::text,
    'occurred_at', (now() - interval '10100 minutes'),
    'customer_id', '01a0e3a4-9a44-7566-b5de-eb2e365799f8'::text,
    'schema_version', 1,
    'payload', jsonb_build_object(
      'case_id', 'k0977',
      'state', 'Resolvido',
      'text', 'Dúvida sobre DARF de venda de ETF',
      'meta', 'Tributação · 2 dias'
    )
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7909-a1e3-62cfcb6ed410',
  'case.opened',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7909-a1e3-62cfcb6ed410'::text,
    'occurred_at', (now() - interval '25 minutes'),
    'customer_id', '01a0e3a4-9a44-7571-9cd6-29d603ed75d1'::text,
    'schema_version', 1,
    'payload', jsonb_build_object(
      'case_id', '01a0e3a4-9a44-76e9-9644-aec5cdb8be90',
      'state', 'Em atendimento',
      'escalated', false,
      'sla_total_minutes', 120
    )
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7909-a1e3-62cfcb6ed410') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-7913-a9fc-e3f97f1ff85d',
  'case.opened',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-7913-a9fc-e3f97f1ff85d'::text,
    'occurred_at', (now() - interval '65 minutes'),
    'customer_id', '01a0e3a4-9a44-75a2-b139-40376b3a76ea'::text,
    'schema_version', 1,
    'payload', jsonb_build_object(
      'case_id', '01a0e3a4-9a44-76f2-9263-f892becdd32a',
      'state', 'Aguardando cliente',
      'escalated', true,
      'sla_total_minutes', 120
    )
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-7913-a9fc-e3f97f1ff85d') ON CONFLICT (event_id) DO NOTHING;

INSERT INTO outbox (event_id, routing_key, payload, published_at)
VALUES (
  '01a0e3a4-9a44-791c-9724-52d8a6b6eedd',
  'case.opened',
  jsonb_build_object(
    'event_id', '01a0e3a4-9a44-791c-9724-52d8a6b6eedd'::text,
    'occurred_at', (now() - interval '400 minutes'),
    'customer_id', '01a0e3a4-9a44-75b5-b359-8d4428470a99'::text,
    'schema_version', 1,
    'payload', jsonb_build_object(
      'case_id', '01a0e3a4-9a44-76fb-9df9-047e1974d2f6',
      'state', 'Resolvido',
      'escalated', false,
      'sla_total_minutes', 60
    )
  ),
  NULL
)
ON CONFLICT (event_id) DO UPDATE SET
  routing_key = EXCLUDED.routing_key,
  payload = EXCLUDED.payload,
  published_at = NULL;

INSERT INTO inbox (event_id) VALUES ('01a0e3a4-9a44-791c-9724-52d8a6b6eedd') ON CONFLICT (event_id) DO NOTHING;

