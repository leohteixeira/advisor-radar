import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './bff';
import { formatCents, postAdvanceDay, readAmount } from './pov';

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

describe('readAmount', () => {
  it.each([
    ['1', 100],
    ['1000', 100_000],
    ['1.000', 100_000],
    ['1.000,5', 100_050],
    ['1.000,50', 100_050],
    ['1.234.567,89', 123_456_789],
    ['0,01', 1],
    ['007', 700],
    ['5,', 500],
    ['US$ 1.000,00', 100_000],
    ['US$1.000,00', 100_000],
    ['  250,00  ', 25_000],
    [formatCents(114_800), 114_800],
  ])('reads %j as %d cents', (text, cents) => {
    expect(readAmount(text)).toEqual({ cents, problem: null });
  });

  it.each(['', '   '])('reports %j as empty', (text) => {
    expect(readAmount(text)).toEqual({ cents: 0, problem: 'empty' });
  });

  it.each(['0', '0,00', '0.000', 'US$ 0,00'])('reports %j as zero', (text) => {
    expect(readAmount(text)).toEqual({ cents: 0, problem: 'zero' });
  });

  it.each([
    '-5',
    '+5',
    '1e3',
    'abc',
    '5 reais',
    '1,234',
    '1.5',
    '1.00',
    '1.0000',
    '10,5,0',
    ',50',
    '1 000',
    'US$',
    'R$ 5',
    '$5',
    '５',
    '99999999999999999999',
  ])('reports %j as unrecognised', (text) => {
    expect(readAmount(text)).toEqual({ cents: 0, problem: 'unrecognised' });
  });
});
