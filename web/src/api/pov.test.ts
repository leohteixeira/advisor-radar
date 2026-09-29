import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './bff';
import { postAdvanceDay } from './pov';

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('postAdvanceDay', () => {
  it('posts the key and returns the new day', async () => {
    const fetchMock = vi.fn(async () => json({ sim_day: 3, event_id: 'ev-1' }, 202));
    vi.stubGlobal('fetch', fetchMock);
    await expect(postAdvanceDay('k1')).resolves.toEqual({ sim_day: 3, event_id: 'ev-1' });
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toMatch(/v1\/client-pov\/simulation\/advance-day$/);
    expect(init.method).toBe('POST');
    expect(new Headers(init.headers).get('Idempotency-Key')).toBe('k1');
  });

  it('reads a missing event id as empty', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json({ sim_day: 0 }, 202)));
    await expect(postAdvanceDay('k1')).resolves.toEqual({ sim_day: 0, event_id: '' });
  });

  it.each([
    ['missing', {}],
    ['negative', { sim_day: -1 }],
    ['fractional', { sim_day: 1.5 }],
    ['a string', { sim_day: '3' }],
  ])('rejects a 202 whose sim_day is %s', async (_, body) => {
    vi.stubGlobal('fetch', vi.fn(async () => json(body, 202)));
    const err = await postAdvanceDay('k1').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 202, code: 'sim_day' });
  });

  it('rejects an error answer with its status and code', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json({ error: 'ten_minutes' }, 429)));
    await expect(postAdvanceDay('k1')).rejects.toMatchObject({ status: 429, code: 'ten_minutes' });
  });
});
