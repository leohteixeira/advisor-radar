export type Segment = 'Essencial' | 'Advance' | 'Singular';

export type AlertType = 'saque' | 'queda' | 'aporte' | 'segmento' | 'contato' | 'risco' | 'perfil';

export type Intent =
  | 'Operacional'
  | 'Câmbio'
  | 'Tributação'
  | 'Investimento'
  | 'Resgate'
  | 'Reclamação'
  | 'Encerramento'
  | 'Contato';

export type CaseState = 'Aberto' | 'Em atendimento' | 'Aguardando cliente' | 'Resolvido';

export interface Signal {
  id: string;
  kind: 'message' | 'alert';
  /** Customer UUIDv7. */
  client: string;
  /** Display name from the BFF book lookup. */
  name?: string;
  /** Segment from the BFF book lookup. */
  segment?: Segment | string;
  ago?: number;
  text?: string;
  intent?: Intent | string;
  dist?: Record<string, number>;
  frustration?: number;
  churn?: boolean;
  churnConf?: string;
  human?: boolean;
  channel?: string;
  fallback?: boolean;
  alert?: AlertType | string;
  amount?: number;
  before?: number;
  after?: number;
  from?: string;
  to?: string;
  days?: number;
  rule?: string;
  reason?: string;
  contacted_at?: string;
  /** Catalog product of a `perfil` alert. */
  product_id?: string;
  /** Product risk (1–5) of a `perfil` alert. */
  risk?: number;
  /** Investor profile of a `perfil` alert. */
  profile?: string;
  /** Highest risk the profile allows. */
  max_risk?: number;
}

export interface CaseHistoryEntry {
  ago: number;
  kind: string;
  text: string;
}

export interface CaseItem {
  id: string;
  /** Customer UUIDv7. */
  client: string;
  /** Display name from the BFF book lookup. */
  name?: string;
  /** Segment from the BFF book lookup. */
  segment?: Segment | string;
  signal: string;
  state: number;
  openedAgo: number;
  slaTotal: number;
  escalated: boolean;
  history: CaseHistoryEntry[];
}

export interface ClientInfo {
  id: string;
  name: string;
  segment: Segment;
  aum: number;
  advisor: string;
  since: string;
}

export interface TimelineEntry {
  event_id: string;
  customer_id: string;
  kind: string;
  title: string;
  text: string;
  meta: string;
  ago: number;
}

export interface ReviewRow {
  id: string;
  /** Customer UUIDv7. */
  client: string;
  /** Display name from the BFF book lookup. */
  name?: string;
  /** Segment from the BFF book lookup. */
  segment?: Segment | string;
  text: string;
  dist: Record<string, number>;
  ago: number;
  fallback?: boolean;
  intent: Intent | string;
  corrected?: boolean;
}

export interface ManagerBacklog {
  advisor: string;
  open: number;
  risk: number;
  overdue: number;
}

export interface ManagerAtRisk {
  id: string;
  client: string;
  advisor: string;
  segment: string;
  remaining: number;
}

export interface FacetCount {
  label: string;
  count: number;
}

export interface ListFacets {
  andamento: FacetCount[];
  sla: FacetCount[];
  sinal: FacetCount[];
  motivo: FacetCount[];
  segmento: FacetCount[];
}

export interface ManagerSnapshot {
  backlog: ManagerBacklog[];
  avgFirstContactMin: number;
  avgFirstContactYesterday: number;
  reviewPct: number;
  fallbackPct: number;
  intents: Record<string, number>;
  atRisk: ManagerAtRisk[];
}

export const CASE_STATES: CaseState[] = [
  'Aberto',
  'Em atendimento',
  'Aguardando cliente',
  'Resolvido',
];

export const ALERT_LABELS: Record<string, string> = {
  saque: 'Saque relevante',
  queda: 'Queda de patrimônio',
  aporte: 'Aporte grande',
  segmento: 'Mudança de segmento',
  contato: 'Sem contato há muito tempo',
  risco: 'Mensagem com risco',
  perfil: 'Compra acima do perfil',
};
