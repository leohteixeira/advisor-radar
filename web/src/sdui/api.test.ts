import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiPath } from '../api/base';
import { ApiError } from '../api/bff';
import { SEED, thiagoHome, thiagoPhase2 } from '../test/sduiFixtures';
import { fetchScreen, InvalidScreenError, isScreen } from './api';

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function stubFetch(response: Response) {
  const fetchMock = vi.fn(async (_url: string) => response);
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function live(): AbortSignal {
  return new AbortController().signal;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('fetchScreen', () => {
  it('requests one screen and returns the envelope', async () => {
    const fetchMock = stubFetch(json(thiagoHome()));
    const screen = await fetchScreen(SEED.thiago, 'home', live());
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]?.[0]).toBe(apiPath(`v1/client-pov/customers/${SEED.thiago}/screens/home`));
    expect(screen).toEqual(thiagoHome());
  });

  it('throws ApiError on a failed request', async () => {
    stubFetch(new Response('bad gateway', { status: 502 }));
    await expect(fetchScreen(SEED.thiago, 'home', live())).rejects.toEqual(new ApiError(502));
  });

  it.each([
    ['the phase-2 home', thiagoPhase2()],
    ['no schema_version', { ...thiagoHome(), schema_version: undefined }],
    ['a string schema_version', { ...thiagoHome(), schema_version: '1' }],
    ['no sections', { ...thiagoHome(), sections: undefined }],
    ['null', null],
    ['an array', []],
    ['no slug', { ...thiagoHome(), slug: undefined }],
    ['another screen slug', { ...thiagoHome(), slug: 'carteira' }],
    ['an Object.prototype slug', { ...thiagoHome(), slug: 'constructor' }],
  ])('throws InvalidScreenError for %s', async (_name, body) => {
    stubFetch(json(body));
    await expect(fetchScreen(SEED.thiago, 'home', live())).rejects.toBeInstanceOf(InvalidScreenError);
  });

  it('fills defaults and drops malformed sections and components', async () => {
    stubFetch(
      json({
        schema_version: 1,
        slug: 'home',
        subtitle: '',
        sections: [
          'moment',
          { id: 7, components: [] },
          { id: 'no-components' },
          { id: 'kept', components: [null, 3, { type: 9, variant: 'x', props: {} }, { variant: 'y' }, { type: 'moment_card', variant: 'welcome', props: { title: 't' } }] },
        ],
        omitted: [
          { id: 'activity', type: 'activity_list', reason: 'timeline' },
          'bad',
          { type: 'advisor_card', reason: 'advisory' },
          { id: 'wealth', reason: 'account-sim' },
          { id: 'moment', type: 'moment_card' },
        ],
      }),
    );
    const screen = await fetchScreen('a/b', 'home', live());
    expect(screen).toEqual({
      schema_version: 1,
      slug: 'home',
      revision: '',
      title: '',
      subtitle: undefined,
      sections: [
        {
          id: 'kept',
          components: [{ type: 'moment_card', variant: 'welcome', props: { title: 't' } }],
        },
      ],
      omitted: [
        { id: 'activity', type: 'activity_list', reason: 'timeline' },
        { id: 'moment', type: 'moment_card', reason: '' },
      ],
    });
  });

  it('fills a missing variant and non-object props', async () => {
    stubFetch(json({ schema_version: 1, slug: 'home', sections: [{ id: 's', components: [{ type: 'moment_card', variant: 3, props: [] }] }] }));
    const screen = await fetchScreen('a', 'home', live());
    expect(screen.sections[0]?.components).toEqual([{ type: 'moment_card', variant: '', props: {} }]);
  });

  it('throws InvalidScreenError for a 200 that is not JSON', async () => {
    stubFetch(new Response('<html>proxy error</html>', { status: 200 }));
    await expect(fetchScreen(SEED.thiago, 'home', live())).rejects.toBeInstanceOf(InvalidScreenError);
  });

  it('aborts on the caller signal and after the client timeout', async () => {
    const timer = new AbortController();
    const timeout = vi.spyOn(AbortSignal, 'timeout').mockReturnValue(timer.signal);
    const signals: AbortSignal[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise<Response>((_resolve, reject) => {
            const signal = init?.signal as AbortSignal;
            signals.push(signal);
            signal.addEventListener('abort', () => reject(signal.reason));
          }),
      ),
    );
    const byTimeout = fetchScreen(SEED.thiago, 'home', live());
    expect(timeout).toHaveBeenCalledWith(3000);
    timer.abort(new DOMException('timed out', 'TimeoutError'));
    await expect(byTimeout).rejects.toMatchObject({ name: 'TimeoutError' });

    timeout.mockReturnValue(new AbortController().signal);
    const caller = new AbortController();
    const byCaller = fetchScreen(SEED.thiago, 'home', caller.signal);
    caller.abort();
    await expect(byCaller).rejects.toMatchObject({ name: 'AbortError' });
    expect(signals.every((signal) => signal.aborted)).toBe(true);
  });

  it('treats a missing omitted list as empty', async () => {
    const fetchMock = stubFetch(json({ schema_version: 1, slug: 'carteira', sections: [], omitted: 'x' }));
    const screen = await fetchScreen('a/b', 'carteira', live());
    expect(fetchMock.mock.calls[0]?.[0]).toBe(apiPath('v1/client-pov/customers/a%2Fb/screens/carteira'));
    expect(screen.slug).toBe('carteira');
    expect(screen.omitted).toEqual([]);
  });
});

describe('isScreen', () => {
  it('accepts an envelope and rejects anything else', () => {
    expect(isScreen(thiagoHome())).toBe(true);
    expect(isScreen(thiagoPhase2())).toBe(false);
    expect(isScreen('home')).toBe(false);
  });
});
