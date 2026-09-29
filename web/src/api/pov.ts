import { ApiError } from './bff';
import { apiPath } from './base';

export interface POVClient {
  customer_id: string;
  name: string;
  segment: string;
  assets: number;
  sla: string;
  advisor: string;
  since: string;
  hint: string;
}

export interface POVActivity {
  title: string;
  meta: string;
  value: string;
  tone: '' | 'pos' | 'neg';
  icon: string;
}

export interface POVMessage {
  text: string;
  meta: string;
}

export interface POVHome extends POVClient {
  caixa: number;
  allocation: { acoes: number; etfs: number; renda_fixa: number; caixa: number };
  activity: POVActivity[];
  messages: POVMessage[];
}

export interface POVAccepted {
  event_id: string;
}

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

export async function fetchPOVClients(signal?: AbortSignal): Promise<POVClient[]> {
  const body = await read<{ items: POVClient[] }>(await fetch(apiPath('v1/client-pov/customers'), { signal }));
  return body.items.map((item) => ({
    ...item,
    since: item.since ?? '',
    hint: item.hint ?? '',
  }));
}

export async function fetchPOVHome(id: string): Promise<POVHome> {
  const home = await read<POVHome>(await fetch(apiPath(`v1/client-pov/customers/${id}`)));
  return {
    ...home,
    since: home.since ?? '',
    hint: home.hint ?? '',
    activity: asActivity(home.activity),
    messages: asMessages(home.messages),
  };
}

function asActivity(value: unknown): POVActivity[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const rows: POVActivity[] = [];
  for (const row of value) {
    if (!row || typeof row !== 'object') {
      continue;
    }
    const item = row as Record<string, unknown>;
    if (typeof item.title !== 'string') {
      continue;
    }
    rows.push({
      title: item.title,
      meta: typeof item.meta === 'string' ? item.meta : '',
      value: typeof item.value === 'string' ? item.value : '',
      tone: item.tone === 'pos' || item.tone === 'neg' ? item.tone : '',
      icon: typeof item.icon === 'string' ? item.icon : 'in',
    });
  }
  return rows;
}

function asMessages(value: unknown): POVMessage[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const rows: POVMessage[] = [];
  for (const row of value) {
    if (!row || typeof row !== 'object') {
      continue;
    }
    const item = row as Record<string, unknown>;
    if (typeof item.text !== 'string') {
      continue;
    }
    rows.push({ text: item.text, meta: typeof item.meta === 'string' ? item.meta : '' });
  }
  return rows;
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
