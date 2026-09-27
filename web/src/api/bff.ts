import type {
  CaseItem,
  ClientInfo,
  Intent,
  ManagerSnapshot,
  ReviewRow,
  Signal,
  TimelineEntry,
} from '../domain/types';
import { apiPath } from './base';

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number) {
    super(`http ${status}`);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function readJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    throw new ApiError(res.status);
  }
  return (await res.json()) as T;
}

export async function fetchQueue(): Promise<Signal[]> {
  const body = await readJSON<{ items: Signal[] }>(await fetch(apiPath('v1/queue')));
  return body.items ?? [];
}

export async function fetchCases(): Promise<CaseItem[]> {
  const body = await readJSON<{ items: CaseItem[] }>(await fetch(apiPath('v1/cases')));
  return body.items ?? [];
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
): Promise<TimelineEntry[]> {
  const params = new URLSearchParams();
  if (q) {
    params.set('q', q);
  }
  if (kind) {
    params.set('kind', kind);
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
