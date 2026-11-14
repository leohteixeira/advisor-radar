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
  perfil: 'M4 17a8 8 0 0 1 16 0M12 17l4-6',
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
  'Adiados',
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
    total = Math.floor(total / 2);
  }
  const rem = total - (signal.ago ?? 0);
  // Whole minutes, as the BFF computes the state its filters and facets use.
  const dueBelow = Math.max(Math.floor((total * 33) / 100), 20);
  const state: SlaState = rem < 0 ? 'Vencido' : rem < dueBelow ? 'Vencendo' : 'No prazo';
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

/** How Abrir caso ended, read from the unfiltered case list. */
export type CaseOutcome = 'opened' | 'joined';

/**
 * Finds what cases did with the Abrir caso command for a signal: a case that
 * is not resolved and names the signal was opened for it; a history line that
 * names the signal means it joined the case the client already had open.
 */
export function caseOutcome(cases: CaseItem[], signalId: string): CaseOutcome | null {
  if (cases.some((c) => c.state < 3 && c.signal === signalId)) {
    return 'opened';
  }
  if (cases.some((c) => c.history.some((line) => line.signal === signalId))) {
    return 'joined';
  }
  return null;
}

/** Presents a case card. State and remaining minutes of the SLA come from the BFF. */
/** Why cases halves an SLA; the rule itself stays in the backend. */
export const SLA_HALVED_WHY =
  'Risco de saída, frustração, pedido de atendimento humano ou saque relevante reduzem o prazo à metade.';

/** The note shown beside the clock of a case whose SLA was halved. */
export function slaHalvedNote(segment: string): string {
  return `Prazo prioritário: metade do SLA ${segment}`;
}

export function decorateCase(caseItem: CaseItem): DecoratedCase {
  const segment = caseItem.segment ?? 'Essencial';
  const name = caseItem.name ?? caseItem.client;
  const done = caseItem.state === 3;
  const rem = caseItem.slaRemaining;
  const slaState = caseItem.slaState as SlaState;
  const secs = Math.max(0, Math.floor(Math.abs(rem) * 60));
  const hh = Math.floor(secs / 3600);
  const mm = Math.floor((secs % 3600) / 60);
  const ss = secs % 60;
  const last = caseItem.history[caseItem.history.length - 1];
  const next = ['Iniciar atendimento', 'Aguardar cliente', 'Marcar resolvido'][caseItem.state] ?? '';

  return {
    caseItem,
    name,
    segment,
    slaState,
    slaTone: done ? 'done' : slaState === 'Vencido' ? 'over' : slaState === 'Vencendo' ? 'due' : 'ok',
    countdown: done ? '—' : `${rem < 0 ? '−' : ''}${pad(hh)}:${pad(mm)}:${pad(ss)}`,
    origin: caseItem.origin ?? '',
    lastText: last?.text ?? '',
    nextLabel: next,
    rem,
  };
}

/** "1 sinal", "2 sinais": a count with the noun in the matching number. */
export function countLabel(count: number, singular: string, plural: string): string {
  return count === 1 ? singular : plural;
}

/**
 * The SLA facet counts for the cards on the board. The BFF counts each value
 * at the instant of its own response, and the signal and case lists are two
 * responses, so a deadline that passes between them would leave a count one
 * card away from the board. Every count the cards can answer is taken from
 * them: all values while no SLA value is picked, and the picked values
 * otherwise (the board then holds only those). The other counts stay as the
 * BFF sent them.
 */
export function slaFacetsFromCards(
  facets: { label: string; count: number }[],
  cardStates: readonly string[],
  picked: readonly string[],
): { label: string; count: number }[] {
  return facets.map((facet) =>
    picked.length === 0 || picked.includes(facet.label)
      ? { label: facet.label, count: cardStates.filter((state) => state === facet.label).length }
      : facet,
  );
}

export function motivoOptions(): string[] {
  return [
    ...(['saque', 'queda', 'aporte', 'segmento', 'contato', 'perfil'] as const)
      .map((k) => ALERT_LABELS[k])
      .filter((label): label is string => Boolean(label)),
    ...INTENT_OPTIONS,
  ];
}

/** Whether the board shows the column of a case state under the Andamento filter. */
export function columnVisible(label: string, filters: QueueFilters, state: number): boolean {
  if (filters.andamento.length === 0) {
    return state < 3;
  }
  return filters.andamento.includes(label);
}

/** New and snoozed signals share the signals column. */
export function signalsColumnVisible(filters: QueueFilters): boolean {
  return (
    filters.andamento.length === 0 ||
    filters.andamento.includes('Novo sinal') ||
    filters.andamento.includes('Adiados')
  );
}
