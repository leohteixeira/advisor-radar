import { ALERT_LABELS, type CaseItem, type Signal } from './types';

export const FRUSTRATION = ['Calmo', 'Incomodado', 'Frustrado', 'Muito frustrado'] as const;

export const SEGMENT_SLA: Record<string, number> = {
  Essencial: 1440,
  Advance: 240,
  Singular: 60,
};

export const ICONS: Record<string, string> = {
  saque: 'M4 4l16 16M20 10v10H10',
  queda: 'M3 6l7 7 4-4 7 7M21 10v6h-6',
  aporte: 'M20 20L4 4M4 14V4h10',
  segmento: 'M6 4l6 6-6 6M13 4l6 6-6 6',
  contato: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 8v5l3 2',
  risco: 'M4 5h16v11H9l-5 4z',
  mensagem: 'M4 5h16v11H9l-5 4zM8 9h8M8 12h5',
  search: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zM20 20l-4-4',
  chevron: 'M6 9l6 6 6-6',
  close: 'M6 6l12 12M18 6L6 18',
  check: 'M4 12l5 5L20 7',
};

export const INTENT_OPTIONS = [
  'Operacional',
  'Câmbio',
  'Tributação',
  'Investimento',
  'Resgate',
  'Reclamação',
  'Encerramento',
  'Contato',
] as const;

export const SINAL_OPTIONS = [
  'Risco de saída',
  'Pedido humano',
  'Precisa de revisão',
  'Classificação simplificada',
] as const;

export const SLA_OPTIONS = ['No prazo', 'Vencendo', 'Vencido'] as const;

export const ANDAMENTO_OPTIONS = [
  'Novo sinal',
  'Aberto',
  'Em atendimento',
  'Aguardando cliente',
  'Resolvido',
] as const;

export type FilterKey = 'andamento' | 'sla' | 'sinal' | 'motivo' | 'segmento';

export interface QueueFilters {
  andamento: string[];
  sla: string[];
  sinal: string[];
  motivo: string[];
  segmento: string[];
}

export const EMPTY_FILTERS: QueueFilters = {
  andamento: [],
  sla: [],
  sinal: [],
  motivo: [],
  segmento: [],
};

export type SlaState = 'No prazo' | 'Vencendo' | 'Vencido' | 'Concluído';
export type Confidence = 'alta' | 'média' | 'baixa';

export interface DecoratedSignal {
  signal: Signal;
  name: string;
  segment: string;
  slaText: string;
  slaState: SlaState;
  slaTone: 'ok' | 'due' | 'over';
  accent: 'crit' | 'warn' | 'neutral';
  typeLabel: string;
  icon: string;
  agoText: string;
  conf: Confidence | null;
  confBars: number;
  needsReview: boolean;
  showFrust: boolean;
  fLabel: string;
  fLevel: number;
  score: number;
}

export interface DecoratedCase {
  caseItem: CaseItem;
  name: string;
  segment: string;
  slaState: SlaState;
  slaTone: 'ok' | 'due' | 'over' | 'done';
  countdown: string;
  origin: string;
  lastText: string;
  nextLabel: string;
  rem: number;
}

export function dateLabel(now = new Date()): string {
  const raw = now
    .toLocaleDateString('pt-BR', { weekday: 'short', day: 'numeric', month: 'short' })
    .replace('.', '');
  return raw.charAt(0).toUpperCase() + raw.slice(1);
}

export function dur(minutes: number): string {
  const abs = Math.abs(Math.round(minutes));
  if (abs < 60) {
    return `${abs} min`;
  }
  const h = Math.floor(abs / 60);
  const mm = abs % 60;
  if (abs < 1440) {
    return mm ? `${h} h ${mm}` : `${h} h`;
  }
  return `${Math.floor(abs / 1440)} d`;
}

export function agoText(ago?: number): string {
  if (ago == null || ago < 1) {
    return 'agora';
  }
  return `há ${dur(ago)}`;
}

export function confidenceOf(dist?: Record<string, number>, fallback?: boolean): Confidence | null {
  if (fallback) {
    return 'média';
  }
  if (!dist || Object.keys(dist).length === 0) {
    return null;
  }
  const top = Math.max(...Object.values(dist));
  if (top >= 0.75) {
    return 'alta';
  }
  if (top >= 0.5) {
    return 'média';
  }
  return 'baixa';
}

function slaToneOf(rem: number, state: SlaState): 'ok' | 'due' | 'over' {
  if (rem < 0 || state === 'Vencido') {
    return 'over';
  }
  if (state === 'Vencendo') {
    return 'due';
  }
  return 'ok';
}

export function signalSla(signal: Signal, segment: string): { rem: number; state: SlaState; text: string } {
  let total = SEGMENT_SLA[segment] ?? SEGMENT_SLA.Essencial ?? 1440;
  const frustration = signal.frustration ?? 0;
  if (signal.churn || frustration >= 2 || signal.human || signal.alert === 'saque') {
    total = Math.round(total / 2);
  }
  const rem = total - (signal.ago ?? 0);
  const state: SlaState = rem < 0 ? 'Vencido' : rem < Math.max(total * 0.33, 20) ? 'Vencendo' : 'No prazo';
  const text = rem < 0 ? `Vencido há ${dur(rem)}` : `SLA ${dur(rem)}`;
  return { rem, state, text };
}

export function decorateSignal(signal: Signal): DecoratedSignal {
  const segment = signal.segment ?? 'Essencial';
  const name = signal.name ?? signal.client;
  const sla = signalSla(signal, segment);
  const isMsg = signal.kind === 'message';
  const frustration = signal.frustration ?? 0;
  const crit = Boolean(signal.churn) || frustration === 3 || sla.state === 'Vencido';
  const warn = frustration === 2 || sla.state === 'Vencendo' || Boolean(signal.human);
  const conf = isMsg ? confidenceOf(signal.dist, signal.fallback) : null;
  const typeKey = isMsg
    ? signal.churn || frustration >= 2
      ? 'risco'
      : 'mensagem'
    : (signal.alert ?? 'mensagem');
  const typeLabel = isMsg
    ? typeKey === 'risco'
      ? 'Mensagem com risco'
      : 'Mensagem'
    : (ALERT_LABELS[String(signal.alert ?? '')] ?? 'Alerta');
  const score =
    (signal.churn ? 100 : 0) +
    (signal.human ? 30 : 0) +
    frustration * 15 +
    (sla.rem < 0 ? 60 : sla.state === 'Vencendo' ? 40 : 0) +
    (segment === 'Singular' ? 20 : segment === 'Advance' ? 10 : 0) +
    (signal.alert === 'saque' ? 25 : 0);

  return {
    signal,
    name,
    segment,
    slaText: sla.text,
    slaState: sla.state,
    slaTone: slaToneOf(sla.rem, sla.state),
    accent: crit ? 'crit' : warn ? 'warn' : 'neutral',
    typeLabel,
    icon: ICONS[typeKey] ?? ICONS.mensagem ?? '',
    agoText: agoText(signal.ago),
    conf,
    confBars: conf === 'alta' ? 3 : conf === 'média' ? 2 : conf === 'baixa' ? 1 : 0,
    needsReview: isMsg && conf !== 'alta' && conf !== null,
    showFrust: isMsg && frustration >= 1,
    fLabel: FRUSTRATION[Math.min(3, Math.max(0, frustration))] ?? 'Calmo',
    fLevel: frustration,
    score,
  };
}

function pad(n: number): string {
  return String(n).padStart(2, '0');
}

export function decorateCase(caseItem: CaseItem, signals: Signal[]): DecoratedCase {
  const segment = caseItem.segment ?? 'Essencial';
  const name = caseItem.name ?? caseItem.client;
  const done = caseItem.state === 3;
  const rem = caseItem.slaTotal - caseItem.openedAgo;
  const slaState: SlaState = done
    ? 'Concluído'
    : rem < 0
      ? 'Vencido'
      : rem < Math.max(caseItem.slaTotal * 0.33, 20)
        ? 'Vencendo'
        : 'No prazo';
  const secs = Math.max(0, Math.floor(Math.abs(rem) * 60));
  const hh = Math.floor(secs / 3600);
  const mm = Math.floor((secs % 3600) / 60);
  const ss = secs % 60;
  const sig = signals.find((s) => s.id === caseItem.signal);
  const origin = sig
    ? sig.kind === 'message'
      ? `mensagem · ${sig.intent ?? ''}`.trim()
      : (ALERT_LABELS[String(sig.alert ?? '')] ?? 'alerta').toLowerCase()
    : 'sinal arquivado';
  const last = caseItem.history[caseItem.history.length - 1];
  const next = ['Iniciar atendimento', 'Aguardar cliente', 'Marcar resolvido'][caseItem.state] ?? '';

  return {
    caseItem,
    name,
    segment,
    slaState,
    slaTone: done ? 'done' : slaToneOf(rem, slaState),
    countdown: done ? '—' : `${rem < 0 ? '−' : ''}${pad(hh)}:${pad(mm)}:${pad(ss)}`,
    origin,
    lastText: last?.text ?? '',
    nextLabel: next,
    rem,
  };
}

function has(filters: QueueFilters, key: FilterKey): boolean {
  return filters[key].length > 0;
}

function sinalOn(row: DecoratedSignal, option: string): boolean {
  if (option === 'Risco de saída') {
    return Boolean(row.signal.churn);
  }
  if (option === 'Pedido humano') {
    return Boolean(row.signal.human);
  }
  if (option === 'Precisa de revisão') {
    return row.needsReview;
  }
  return Boolean(row.signal.fallback);
}

export function signalMatches(
  row: DecoratedSignal,
  query: string,
  filters: QueueFilters,
  skip?: FilterKey,
): boolean {
  const q = query.trim().toLowerCase();
  if (q && !row.name.toLowerCase().includes(q)) {
    return false;
  }
  if (skip !== 'sinal' && has(filters, 'sinal') && !filters.sinal.some((o) => sinalOn(row, o))) {
    return false;
  }
  if (skip !== 'sla' && has(filters, 'sla') && !filters.sla.includes(row.slaState)) {
    return false;
  }
  if (
    skip !== 'motivo' &&
    has(filters, 'motivo') &&
    !filters.motivo.includes(row.typeLabel) &&
    !filters.motivo.includes(String(row.signal.intent ?? ''))
  ) {
    return false;
  }
  if (skip !== 'segmento' && has(filters, 'segmento') && !filters.segmento.includes(row.segment)) {
    return false;
  }
  return true;
}

export function caseMatches(
  row: DecoratedCase,
  signals: DecoratedSignal[],
  query: string,
  filters: QueueFilters,
  skip?: FilterKey,
): boolean {
  const q = query.trim().toLowerCase();
  if (q && !row.name.toLowerCase().includes(q)) {
    return false;
  }
  if (skip !== 'sla' && has(filters, 'sla') && !filters.sla.includes(row.slaState)) {
    return false;
  }
  if (skip !== 'segmento' && has(filters, 'segmento') && !filters.segmento.includes(row.segment)) {
    return false;
  }
  const sig = signals.find((s) => s.signal.id === row.caseItem.signal);
  if (skip !== 'sinal' && has(filters, 'sinal')) {
    if (!sig || !filters.sinal.some((o) => sinalOn(sig, o))) {
      return false;
    }
  }
  if (skip !== 'motivo' && has(filters, 'motivo')) {
    if (
      !sig ||
      (!filters.motivo.includes(sig.typeLabel) && !filters.motivo.includes(String(sig.signal.intent ?? '')))
    ) {
      return false;
    }
  }
  return true;
}

export function motivoOptions(): string[] {
  return [
    ...(['saque', 'queda', 'aporte', 'segmento', 'contato'] as const)
      .map((k) => ALERT_LABELS[k])
      .filter((label): label is string => Boolean(label)),
    ...INTENT_OPTIONS,
  ];
}

export function columnVisible(label: string, filters: QueueFilters, state: number | null): boolean {
  if (filters.andamento.length === 0) {
    return state == null || state < 3;
  }
  return filters.andamento.includes(label);
}
