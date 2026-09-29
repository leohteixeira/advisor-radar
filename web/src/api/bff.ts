import type {
  CaseItem,
  ClientInfo,
  Intent,
  ListFacets,
  ManagerSnapshot,
  ReviewRow,
  Signal,
  TimelineEntry,
} from '../domain/types';
import { apiPath } from './base';

export class ApiError extends Error {
  readonly status: number;
  /** The `error` code of a JSON error body (`insufficient`, `invalid`, …), or ''. */
  readonly code: string;

  constructor(status: number, code = '') {
    super(`http ${status}`);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

async function readJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    throw new ApiError(res.status);
  }
  return (await res.json()) as T;
}

export async function fetchQueue(params?: {
  q?: string;
  andamento?: string[];
  sla?: string[];
  sinal?: string[];
  motivo?: string[];
  segmento?: string[];
}): Promise<{ items: Signal[]; facets: ListFacets }> {
  const qs = buildListQuery(params);
  const body = await readJSON<{ items: Signal[]; facets: ListFacets }>(
    await fetch(apiPath(`v1/queue${qs}`)),
  );
  return {
    items: body.items ?? [],
    facets: body.facets ?? emptyFacets(),
  };
}

export async function fetchCases(params?: {
  q?: string;
  andamento?: string[];
  sla?: string[];
  sinal?: string[];
  motivo?: string[];
  segmento?: string[];
}): Promise<{ items: CaseItem[]; facets: ListFacets }> {
  const qs = buildListQuery(params);
  const body = await readJSON<{ items: CaseItem[]; states?: string[]; facets: ListFacets }>(
    await fetch(apiPath(`v1/cases${qs}`)),
  );
  return {
    items: body.items ?? [],
    facets: body.facets ?? emptyFacets(),
  };
}

function buildListQuery(params?: {
  q?: string;
  andamento?: string[];
  sla?: string[];
  sinal?: string[];
  motivo?: string[];
  segmento?: string[];
}): string {
  if (!params) {
    return '';
  }
  const sp = new URLSearchParams();
  if (params.q?.trim()) {
    sp.set('q', params.q.trim());
  }
  for (const key of ['andamento', 'sla', 'sinal', 'motivo', 'segmento'] as const) {
    for (const value of params[key] ?? []) {
      sp.append(key, value);
    }
  }
  const qs = sp.toString();
  return qs ? `?${qs}` : '';
}

function emptyFacets(): ListFacets {
  return { andamento: [], sla: [], sinal: [], motivo: [], segmento: [] };
}

export async function putAction(id: string, action: 'contact' | 'snooze'): Promise<void> {
  const res = await fetch(apiPath(`v1/actions/${id}`), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action }),
  });
  if (!res.ok) {
    throw new ApiError(res.status);
  }
}

export async function deleteAction(id: string): Promise<void> {
  const res = await fetch(apiPath(`v1/actions/${id}`), { method: 'DELETE' });
  if (!res.ok) {
    throw new ApiError(res.status);
  }
}

export async function advanceCase(id: string): Promise<CaseItem> {
  return readJSON<CaseItem>(
    await fetch(apiPath(`v1/cases/${id}/advance`), { method: 'POST' }),
  );
}

export async function fetchCustomer(id: string): Promise<ClientInfo> {
  return readJSON<ClientInfo>(await fetch(apiPath(`v1/customers/${id}`)));
}

export async function fetchTimeline(
  customerId: string,
  q = '',
  kind = '',
  order: 'asc' | 'desc' = 'asc',
): Promise<TimelineEntry[]> {
  const params = new URLSearchParams();
  if (q) {
    params.set('q', q);
  }
  if (kind) {
    params.set('kind', kind);
  }
  if (order === 'desc') {
    params.set('order', 'desc');
  }
  const qs = params.toString();
  const url = apiPath(`v1/customers/${customerId}/timeline${qs ? `?${qs}` : ''}`);
  const body = await readJSON<{ items: TimelineEntry[] }>(await fetch(url));
  return body.items ?? [];
}

export async function fetchReview(): Promise<ReviewRow[]> {
  const body = await readJSON<{ items: ReviewRow[] }>(await fetch(apiPath('v1/review')));
  return body.items ?? [];
}

export async function correctReview(id: string, intent: Intent | string): Promise<ReviewRow> {
  return readJSON<ReviewRow>(
    await fetch(apiPath(`v1/review/${id}`), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ intent }),
    }),
  );
}

export async function fetchManager(): Promise<ManagerSnapshot> {
  return readJSON<ManagerSnapshot>(await fetch(apiPath('v1/manager')));
}

export function queueStreamUrl(): string {
  return apiPath('v1/queue/stream');
}
