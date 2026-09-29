export interface WalkStep {
  title: string;
  tag: string;
  where: string;
  detail: string;
  nodes: string[];
  graph: string[];
  edges: string[];
}

export const WALK_INTERVAL_MS = 2600;

export const WALK_STEPS: WalkStep[] = [
  {
    title: 'O cliente abre uma reclamação no app',
    tag: 'POST /v1/client-pov/customers/{id}/complaints',
    where: 'web → bff · HTTP',
    detail:
      'O app React envia o pedido ao BFF por HTTP/JSON versionado. O BFF não guarda nada: só valida e repassa.',
    nodes: ['web', 'bff'],
    graph: ['web', 'bff'],
    edges: ['webbff'],
  },
  {
    title: 'O BFF entrega o comando ao account-sim',
    tag: 'gRPC · deadline propagado',
    where: 'bff → account-sim · gRPC',
    detail:
      'Chamada gRPC com deadline e interceptors de tracing e retry. O trace nasce no clique e acompanha o evento até a fila.',
    nodes: ['bff', 'account-sim'],
    graph: ['bff', 'acct'],
    edges: ['bffacct'],
  },
  {
    title: 'Mensagem e evento gravados na mesma transação',
    tag: 'PostgreSQL · transactional outbox',
    where: 'account-sim · PostgreSQL',
    detail:
      'O account-sim salva a mensagem e o evento na outbox de uma vez. Se o broker estiver fora, o evento espera; nada se perde.',
    nodes: ['account-sim'],
    graph: ['acct'],
    edges: [],
  },
  {
    title: 'O relay publica no RabbitMQ',
    tag: 'message.received',
    where: 'account-sim → RabbitMQ',
    detail:
      'event_id, occurred_at, customer_id e schema_version no corpo; o contexto de trace vai nos headers.',
    nodes: ['account-sim', 'RabbitMQ'],
    graph: ['acct', 'bus'],
    edges: ['acctbus'],
  },
  {
    title: 'A triagem classifica a mensagem',
    tag: 'Jev · fallback heurístico',
    where: 'triage · Jev',
    detail:
      'Inbox idempotente por event_id. O Jev decide intenção, frustração, risco de saída e pedido humano. Se o modelo cair, a heurística responde e o resultado sai marcado como degradado.',
    nodes: ['RabbitMQ', 'triage', 'Jev'],
    graph: ['bus', 'triage', 'jev'],
    edges: ['bustriage', 'triagejev'],
  },
  {
    title: 'Uma regra gera o alerta e o caso ganha SLA',
    tag: 'message.triaged → alert.raised',
    where: 'advisory · cases',
    detail:
      'Cada regra do advisory é uma interface pequena. O cases abre o caso com SLA do segmento; o vencimento escala por fila com TTL e dead-letter exchange.',
    nodes: ['RabbitMQ', 'advisory', 'cases'],
    graph: ['bus', 'advisory', 'cases'],
    edges: ['busadv', 'buscases'],
  },
  {
    title: 'A visão 360 é atualizada',
    tag: 'Elasticsearch',
    where: 'timeline-indexer · Elasticsearch',
    detail:
      'O timeline-indexer grava eventos, mensagens triadas e casos para a busca textual da visão 360 do cliente.',
    nodes: ['RabbitMQ', 'timeline-indexer', 'Elasticsearch'],
    graph: ['bus', 'indexer', 'es'],
    edges: ['busidx', 'idxes'],
  },
  {
    title: 'O alerta aparece na fila do assessor',
    tag: 'SSE · alert.raised',
    where: 'bff → web · SSE',
    detail:
      'O BFF consome alert.raised e empurra por SSE, sem recarregar a página. Meta: p95 abaixo de 2 s do evento à tela.',
    nodes: ['RabbitMQ', 'bff', 'web'],
    graph: ['bus', 'bff', 'web'],
    edges: ['busbff', 'bffweb'],
  },
];

export function walkStep(index: number): WalkStep {
  const step = WALK_STEPS[index];
  if (!step) {
    throw new Error(`walk step ${index} is missing`);
  }
  return step;
}

export const GRAPH_NODES = [
  'web',
  'bff',
  'account-sim',
  'RabbitMQ',
  'triage',
  'Jev',
  'advisory',
  'cases',
  'timeline-indexer',
  'Elasticsearch',
] as const;
