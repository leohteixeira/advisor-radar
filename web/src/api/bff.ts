import type { CaseItem, Signal } from '../domain/types';

async function readJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    throw new Error(`http ${res.status}`);
  }
  return (await res.json()) as T;
}

export async function fetchQueue(): Promise<Signal[]> {
  const body = await readJSON<{ items: Signal[] }>(await fetch('/v1/queue'));
  return body.items ?? [];
}

export async function fetchCases(): Promise<CaseItem[]> {
  const body = await readJSON<{ items: CaseItem[] }>(await fetch('/v1/cases'));
  return body.items ?? [];
}

export async function putAction(id: string, action: 'contact' | 'snooze'): Promise<void> {
  const res = await fetch(`/v1/actions/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action }),
  });
  if (!res.ok) {
    throw new Error(`http ${res.status}`);
  }
}

export async function deleteAction(id: string): Promise<void> {
  const res = await fetch(`/v1/actions/${id}`, { method: 'DELETE' });
  if (!res.ok) {
    throw new Error(`http ${res.status}`);
  }
}

export async function advanceCase(id: string): Promise<CaseItem> {
  return readJSON<CaseItem>(
    await fetch(`/v1/cases/${id}/advance`, { method: 'POST' }),
  );
}
