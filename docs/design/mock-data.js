// Dados fictícios do Advisor Radar. Espelha o payload que o backend Go enviaria via SSE (evento "signal").
export const ICONS = {
  saque: 'M4 4l16 16M20 10v10H10',
  queda: 'M3 6l7 7 4-4 7 7M21 10v6h-6',
  aporte: 'M20 20L4 4M4 14V4h10',
  segmento: 'M6 4l6 6-6 6M13 4l6 6-6 6',
  contato: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 8v5l3 2',
  risco: 'M4 5h16v11H9l-5 4z',
  mensagem: 'M4 5h16v11H9l-5 4zM8 9h8M8 12h5',
  nota: 'M6 3h12v18H6zM9 8h6M9 12h6M9 16h3',
  caso: 'M4 7h16v13H4zM9 7V4h6v3',
  telefone: 'M5 4h4l2 5-3 2a10 10 0 0 0 5 5l2-3 5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2z',
  check: 'M4 12l5 5L20 7',
  chevron: 'M9 6l6 6-6 6',
  back: 'M15 6l-6 6 6 6',
  search: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zM20 20l-4-4',
  close: 'M6 6l12 12M18 6L6 18',
  radar: 'M12 3a9 9 0 1 0 9 9M12 7a5 5 0 1 0 5 5M12 12l6-6',
};

export const ALERT_TYPES = {
  saque: { label: 'Saque relevante', icon: 'saque' },
  queda: { label: 'Queda de patrimônio', icon: 'queda' },
  aporte: { label: 'Aporte grande', icon: 'aporte' },
  segmento: { label: 'Mudança de segmento', icon: 'segmento' },
  contato: { label: 'Sem contato há muito tempo', icon: 'contato' },
  risco: { label: 'Mensagem com risco', icon: 'risco' },
  mensagem: { label: 'Mensagem', icon: 'mensagem' },
};

export const INTENTS = ['Operacional', 'Câmbio', 'Tributação', 'Investimento', 'Resgate', 'Reclamação', 'Encerramento', 'Contato'];
export const FRUSTRATION = ['Calmo', 'Incomodado', 'Frustrado', 'Muito frustrado'];
export const SEGMENT_SLA = { Essencial: 1440, Advance: 240, Singular: 60 }; // minutos

export const ADVISORS = ['Ana Paula Ribeiro', 'Bruno Dias', 'Carla Menezes', 'Diego Rocha'];

export const CLIENTS = {
  c01: { id: 'c01', name: 'Mariana Costa', segment: 'Singular', aum: 248300, advisor: 'Ana Paula Ribeiro', since: '2021' },
  c02: { id: 'c02', name: 'Paulo Henrique Souza', segment: 'Advance', aum: 96400, advisor: 'Ana Paula Ribeiro', since: '2022' },
  c03: { id: 'c03', name: 'Fernanda Lima', segment: 'Essencial', aum: 8200, advisor: 'Ana Paula Ribeiro', since: '2024' },
  c04: { id: 'c04', name: 'Carlos Eduardo Ramos', segment: 'Advance', aum: 142000, advisor: 'Ana Paula Ribeiro', since: '2020' },
  c05: { id: 'c05', name: 'Juliana Martins', segment: 'Singular', aum: 512900, advisor: 'Ana Paula Ribeiro', since: '2019' },
  c06: { id: 'c06', name: 'Roberto Nascimento', segment: 'Essencial', aum: 7800, advisor: 'Ana Paula Ribeiro', since: '2025' },
  c07: { id: 'c07', name: 'Ana Beatriz Oliveira', segment: 'Advance', aum: 190500, advisor: 'Ana Paula Ribeiro', since: '2021' },
  c08: { id: 'c08', name: 'Lucas Pereira', segment: 'Essencial', aum: 6100, advisor: 'Ana Paula Ribeiro', since: '2023' },
  c09: { id: 'c09', name: 'Patrícia Gomes', segment: 'Singular', aum: 780000, advisor: 'Ana Paula Ribeiro', since: '2018' },
  c10: { id: 'c10', name: 'Marcelo Ferreira', segment: 'Advance', aum: 88000, advisor: 'Ana Paula Ribeiro', since: '2022' },
  c11: { id: 'c11', name: 'Sérgio Cardoso', segment: 'Singular', aum: 450000, advisor: 'Ana Paula Ribeiro', since: '2019' },
  c12: { id: 'c12', name: 'Helena Barbosa', segment: 'Advance', aum: 108000, advisor: 'Ana Paula Ribeiro', since: '2023' },
  c13: { id: 'c13', name: 'Thiago Azevedo', segment: 'Advance', aum: 68000, advisor: 'Ana Paula Ribeiro', since: '2024' },
  c14: { id: 'c14', name: 'Camila Rodrigues', segment: 'Advance', aum: 175000, advisor: 'Ana Paula Ribeiro', since: '2020' },
  c15: { id: 'c15', name: 'Rafael Monteiro', segment: 'Singular', aum: 950000, advisor: 'Ana Paula Ribeiro', since: '2017' },
  c16: { id: 'c16', name: 'Beatriz Santana', segment: 'Essencial', aum: 7500, advisor: 'Ana Paula Ribeiro', since: '2025' },
  c17: { id: 'c17', name: 'Diego Carvalho', segment: 'Essencial', aum: 8900, advisor: 'Ana Paula Ribeiro', since: '2024' },
  c18: { id: 'c18', name: 'Vanessa Moreira', segment: 'Advance', aum: 119000, advisor: 'Ana Paula Ribeiro', since: '2021' },
  c19: { id: 'c19', name: 'Gustavo Teixeira', segment: 'Advance', aum: 141000, advisor: 'Ana Paula Ribeiro', since: '2022' },
  c20: { id: 'c20', name: 'Isabela Nunes', segment: 'Essencial', aum: 9400, advisor: 'Ana Paula Ribeiro', since: '2025' },
  c21: { id: 'c21', name: 'Otávio Freitas', segment: 'Essencial', aum: 5800, advisor: 'Bruno Dias', since: '2024' },
  c22: { id: 'c22', name: 'Renata Albuquerque', segment: 'Advance', aum: 67000, advisor: 'Carla Menezes', since: '2023' },
};

// Sinais na fila. ago = minutos desde o evento. Probabilidades ficam no payload, mas a UI mostra só rótulo + certeza.
export const SIGNALS = [
  { id: 's01', kind: 'message', client: 'c01', ago: 12, text: 'Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.', intent: 'Reclamação', dist: { 'Reclamação': 0.82, 'Encerramento': 0.11, 'Operacional': 0.04, 'Resgate': 0.03 }, frustration: 2, churn: true, churnConf: 'alta', human: false, channel: 'chat' },
  { id: 's02', kind: 'message', client: 'c02', ago: 25, text: 'Já é a terceira vez que eu explico o mesmo problema e ninguém resolve. Um absurdo.', intent: 'Reclamação', dist: { 'Reclamação': 0.91, 'Operacional': 0.06, 'Encerramento': 0.03 }, frustration: 3, churn: false, churnConf: 'média', human: false, channel: 'e-mail' },
  { id: 's03', kind: 'message', client: 'c03', ago: 8, text: 'Consegue pedir para o meu assessor me ligar?', intent: 'Contato', dist: { 'Contato': 0.94, 'Operacional': 0.04, 'Investimento': 0.02 }, frustration: 0, churn: false, churnConf: 'alta', human: true, channel: 'chat' },
  { id: 's04', kind: 'message', client: 'c04', ago: 100, text: 'Vendi ações com lucro em julho. Tenho que pagar DARF?', intent: 'Tributação', dist: { 'Tributação': 0.88, 'Investimento': 0.08, 'Operacional': 0.04 }, frustration: 0, churn: false, churnConf: 'alta', human: false, channel: 'e-mail' },
  { id: 's05', kind: 'message', client: 'c05', ago: 33, text: 'Preciso sacar 5 mil dólares e trazer de volta para o Brasil.', intent: 'Resgate', dist: { 'Resgate': 0.54, 'Câmbio': 0.31, 'Operacional': 0.10, 'Encerramento': 0.05 }, frustration: 0, churn: false, churnConf: 'média', human: false, channel: 'chat' },
  { id: 's06', kind: 'message', client: 'c06', ago: 50, text: 'Meu cartão foi recusado na viagem, o que eu faço?', intent: 'Operacional', dist: { 'Operacional': 0.71, 'Reclamação': 0.19, 'Contato': 0.10 }, frustration: 1, churn: false, churnConf: 'alta', human: false, channel: 'chat', fallback: true },
  { id: 's07', kind: 'message', client: 'c07', ago: 65, text: 'Quero encerrar minha conta. Como faço para transferir os ativos?', intent: 'Encerramento', dist: { 'Encerramento': 0.86, 'Resgate': 0.09, 'Operacional': 0.05 }, frustration: 0, churn: true, churnConf: 'alta', human: false, channel: 'e-mail' },
  { id: 's08', kind: 'message', client: 'c08', ago: 120, text: 'Qual a cotação que vocês usam no câmbio? Está muito diferente do Google.', intent: 'Câmbio', dist: { 'Câmbio': 0.79, 'Reclamação': 0.15, 'Operacional': 0.06 }, frustration: 1, churn: false, churnConf: 'alta', human: false, channel: 'chat' },
  { id: 's09', kind: 'message', client: 'c09', ago: 185, text: 'Vocês têm alguma recomendação de renda fixa com vencimento em 2028?', intent: 'Investimento', dist: { 'Investimento': 0.93, 'Operacional': 0.05, 'Tributação': 0.02 }, frustration: 0, churn: false, churnConf: 'alta', human: false, channel: 'e-mail' },
  { id: 's10', kind: 'message', client: 'c10', ago: 18, text: 'Estou frustrado com a demora pra liberar a transferência. Alguém pode me explicar?', intent: 'Operacional', dist: { 'Operacional': 0.58, 'Reclamação': 0.35, 'Contato': 0.07 }, frustration: 2, churn: false, churnConf: 'média', human: true, channel: 'chat' },
  { id: 's11', kind: 'alert', client: 'c11', ago: 22, alert: 'saque', amount: 190000, before: 640000, after: 450000, rule: 'Saque acima de 20% do patrimônio em 24 horas', reason: 'Saque de US$ 190.000,00, 30% do patrimônio' },
  { id: 's12', kind: 'alert', client: 'c12', ago: 180, alert: 'queda', amount: -22000, before: 130000, after: 108000, rule: 'Queda acima de 15% em 5 dias úteis', reason: 'Patrimônio caiu 17% em 5 dias' },
  { id: 's13', kind: 'alert', client: 'c13', ago: 240, alert: 'aporte', amount: 60000, before: 8000, after: 68000, rule: 'Aporte maior que o patrimônio anterior', reason: 'Aporte de US$ 60.000,00, 7,5× o patrimônio' },
  { id: 's14', kind: 'alert', client: 'c13', ago: 238, alert: 'segmento', before: 8000, after: 68000, from: 'Essencial', to: 'Advance', rule: 'Patrimônio cruzou a faixa de US$ 10 mil', reason: 'Subiu de Essencial para Advance' },
  { id: 's15', kind: 'alert', client: 'c14', ago: 1440, alert: 'contato', days: 94, before: 175000, after: 175000, rule: 'Sem contato há mais de 90 dias', reason: 'Último contato há 94 dias' },
  { id: 's16', kind: 'alert', client: 'c15', ago: 6, alert: 'saque', amount: 300000, before: 1250000, after: 950000, rule: 'Saque acima de 20% do patrimônio em 24 horas', reason: 'Saque de US$ 300.000,00, 24% do patrimônio' },
  { id: 's17', kind: 'alert', client: 'c16', ago: 300, alert: 'queda', amount: -2100, before: 9600, after: 7500, rule: 'Queda acima de 15% em 5 dias úteis', reason: 'Patrimônio caiu 22% em 5 dias' },
];

// Sinais que chegam em tempo real ao "Simular dia de mercado" (ordem de chegada).
export const STREAM = [
  { id: 'n01', kind: 'message', client: 'c18', text: 'Ninguém me responde há dois dias. Vou abrir reclamação no Reclame Aqui.', intent: 'Reclamação', dist: { 'Reclamação': 0.89, 'Encerramento': 0.08, 'Contato': 0.03 }, frustration: 3, churn: true, churnConf: 'média', human: false, channel: 'e-mail' },
  { id: 'n02', kind: 'alert', client: 'c19', alert: 'saque', amount: 55000, before: 196000, after: 141000, rule: 'Saque acima de 20% do patrimônio em 24 horas', reason: 'Saque de US$ 55.000,00, 28% do patrimônio' },
  { id: 'n03', kind: 'message', client: 'c17', text: 'Bom dia! Como faço para ver o informe de rendimentos de 2025?', intent: 'Tributação', dist: { 'Tributação': 0.9, 'Operacional': 0.08, 'Investimento': 0.02 }, frustration: 0, churn: false, churnConf: 'alta', human: false, channel: 'chat' },
  { id: 'n04', kind: 'message', client: 'c20', text: 'Preciso falar com uma pessoa, não com robô.', intent: 'Contato', dist: { 'Contato': 0.87, 'Reclamação': 0.1, 'Operacional': 0.03 }, frustration: 1, churn: false, churnConf: 'alta', human: true, channel: 'chat' },
  { id: 'n05', kind: 'alert', client: 'c22', alert: 'queda', amount: -12000, before: 79000, after: 67000, rule: 'Queda acima de 15% em 5 dias úteis', reason: 'Patrimônio caiu 15% em 5 dias' },
  { id: 'n06', kind: 'message', client: 'c21', text: 'Como declaro os dividendos recebidos em dólar?', intent: 'Tributação', dist: { 'Tributação': 0.85, 'Investimento': 0.1, 'Operacional': 0.05 }, frustration: 0, churn: false, churnConf: 'alta', human: false, channel: 'e-mail' },
];

// Fila de revisão do analista (certeza baixa/média).
export const REVIEW = [
  { id: 'r01', client: 'c05', text: 'Preciso sacar 5 mil dólares e trazer de volta para o Brasil.', dist: { 'Resgate': 0.54, 'Câmbio': 0.31, 'Operacional': 0.10, 'Encerramento': 0.05 }, ago: 33 },
  { id: 'r02', client: 'c10', text: 'Estou frustrado com a demora pra liberar a transferência. Alguém pode me explicar?', dist: { 'Operacional': 0.58, 'Reclamação': 0.35, 'Contato': 0.07 }, ago: 18 },
  { id: 'r03', client: 'c21', text: 'Quero mandar dinheiro pra minha filha que estuda fora.', dist: { 'Câmbio': 0.41, 'Operacional': 0.38, 'Investimento': 0.12, 'Resgate': 0.09 }, ago: 44 },
  { id: 'r04', client: 'c22', text: 'Isso aqui não está batendo com o extrato.', dist: { 'Operacional': 0.45, 'Reclamação': 0.40, 'Tributação': 0.15 }, ago: 71 },
];

export const CASE_STATES = ['Aberto', 'Em atendimento', 'Aguardando cliente', 'Resolvido'];

export const CASES = [
  { id: 'k1042', client: 'c02', signal: 's02', state: 1, openedAgo: 25, slaTotal: 120, escalated: false, history: [
    { ago: 25, kind: 'caso', text: 'Caso aberto a partir de mensagem com reclamação' },
    { ago: 21, kind: 'caso', text: 'Em atendimento · Ana Paula Ribeiro' },
    { ago: 19, kind: 'nota', text: 'Problema é o bloqueio da transferência internacional desde 12/09. Compliance pediu comprovante de origem.' },
  ] },
  { id: 'k1038', client: 'c07', signal: 's07', state: 2, openedAgo: 65, slaTotal: 120, escalated: true, history: [
    { ago: 65, kind: 'caso', text: 'Caso aberto a partir de pedido de encerramento' },
    { ago: 60, kind: 'caso', text: 'Escalonado automaticamente: risco de saída alto em cliente Advance' },
    { ago: 58, kind: 'telefone', text: 'Ligação · 9 min · cliente insatisfeita com taxas de câmbio' },
    { ago: 55, kind: 'caso', text: 'Aguardando cliente · proposta de isenção enviada por e-mail' },
  ] },
  { id: 'k1031', client: 'c09', signal: 's09', state: 3, openedAgo: 400, slaTotal: 60, escalated: false, history: [
    { ago: 400, kind: 'caso', text: 'Caso aberto' },
    { ago: 380, kind: 'telefone', text: 'Ligação · 14 min · sugestão de Treasuries 2028' },
    { ago: 370, kind: 'caso', text: 'Resolvido' },
  ] },
];

// Timeline da visão 360 (Mariana Costa). ago em minutos.
export const TIMELINES = {
  c01: [
    { ago: 12, kind: 'mensagem', title: 'Mensagem · chat', text: 'Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.', meta: 'Reclamação · Frustrado · risco de saída' },
    { ago: 1500, kind: 'mensagem', title: 'Mensagem · e-mail', text: 'A transferência que pedi na segunda ainda não caiu. Podem verificar?', meta: 'Operacional · Incomodado' },
    { ago: 2900, kind: 'nota', title: 'Nota do assessor', text: 'Cliente pretende comprar imóvel em Orlando no 1º semestre. Precisa de liquidez em março.', meta: 'Ana Paula Ribeiro' },
    { ago: 4400, kind: 'saque', title: 'Saque', text: 'US$ 20.000,00 para conta nos EUA', meta: 'Sem alerta · 7% do patrimônio' },
    { ago: 10100, kind: 'caso', title: 'Caso k0977 resolvido', text: 'Dúvida sobre DARF de venda de ETF', meta: 'Tributação · 2 dias' },
    { ago: 21000, kind: 'aporte', title: 'Aporte', text: 'US$ 45.000,00', meta: 'Câmbio a 5,42' },
    { ago: 43000, kind: 'telefone', title: 'Ligação', text: 'Revisão semestral da carteira · 32 min', meta: 'Ana Paula Ribeiro' },
  ],
};

// Painel do gestor (dia atual).
export const MANAGER = {
  backlog: [
    { advisor: 'Ana Paula Ribeiro', open: 17, risk: 4, overdue: 1 },
    { advisor: 'Bruno Dias', open: 9, risk: 1, overdue: 0 },
    { advisor: 'Carla Menezes', open: 13, risk: 2, overdue: 2 },
    { advisor: 'Diego Rocha', open: 6, risk: 0, overdue: 0 },
  ],
  avgFirstContactMin: 14,
  avgFirstContactYesterday: 19,
  reviewPct: 18,
  fallbackPct: 6,
  intents: { 'Operacional': 31, 'Tributação': 22, 'Reclamação': 14, 'Câmbio': 11, 'Investimento': 9, 'Contato': 7, 'Resgate': 4, 'Encerramento': 2 },
  atRisk: [
    { id: 'k1042', client: 'Paulo Henrique Souza', advisor: 'Ana Paula Ribeiro', segment: 'Advance', remaining: 46 },
    { id: 'k1051', client: 'Renata Albuquerque', advisor: 'Carla Menezes', segment: 'Advance', remaining: 12 },
    { id: 'k1049', client: 'Otávio Freitas', advisor: 'Bruno Dias', segment: 'Essencial', remaining: 95 },
    { id: 'k1044', client: 'Sérgio Cardoso', advisor: 'Ana Paula Ribeiro', segment: 'Singular', remaining: -8 },
    { id: 'k1040', client: 'Marina Duarte', advisor: 'Carla Menezes', segment: 'Singular', remaining: -35 },
  ],
};
