export type Segment = 'Essencial' | 'Advance' | 'Singular';

export type AlertType = 'saque' | 'queda' | 'aporte' | 'segmento' | 'contato' | 'risco';

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
  client: string;
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
}

export interface CaseHistoryEntry {
  ago: number;
  kind: string;
  text: string;
}

export interface CaseItem {
  id: string;
  client: string;
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
};
