import { ApiError } from './bff';
import { apiPath } from './base';

export interface POVClient {
  customer_id: string;
  name: string;
  segment: string;
  assets: number;
  sla: string;
  advisor: string;
  hint?: string;
}

export interface POVHome extends POVClient {
  caixa: number;
  allocation: { acoes: number; etfs: number; renda_fixa: number; caixa: number };
  activity: unknown[];
  messages: unknown[];
}

export interface POVAccepted {
  event_id: string;
}

const HINTS: Record<string, string> = {
  'Fernanda Lima':
    'Perto do teto da faixa. Um depósito pode subir o segmento; uma reclamação mostra o SLA mais longo.',
  'Thiago Azevedo': 'Acabou de subir de Essencial para Advance com um depósito grande.',
  'Mariana Costa':
    'Já reclamou de uma transferência atrasada. Um saque grande ou uma ameaça de saída sobem a prioridade na hora.',
};

export function protocolOf(eventID: string): string {
  const [a, b] = eventID.split('-');
  return `${a ?? ''}-${b ?? ''}`.toUpperCase();
}

export function formatCents(cents: number): string {
  return (cents / 100).toLocaleString('pt-BR', { style: 'currency', currency: 'USD' });
}

async function read<T>(res: Response): Promise<T> {
  if (!res.ok) {
    throw new ApiError(res.status);
  }
  return (await res.json()) as T;
}

export async function fetchPOVClients(): Promise<POVClient[]> {
  const body = await read<{ items: POVClient[] }>(await fetch(apiPath('v1/client-pov/customers')));
  return body.items.map((item) => ({ ...item, hint: HINTS[item.name] ?? '' }));
}

export async function fetchPOVHome(id: string): Promise<POVHome> {
  return read<POVHome>(await fetch(apiPath(`v1/client-pov/customers/${id}`)));
}

export async function postPOV(
  id: string,
  kind: 'deposits' | 'withdrawals' | 'complaints' | 'messages',
  body: Record<string, unknown>,
  key: string,
): Promise<POVAccepted> {
  const res = await fetch(apiPath(`v1/client-pov/customers/${id}/${kind}`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key },
    body: JSON.stringify(body),
  });
  return read<POVAccepted>(res);
}
