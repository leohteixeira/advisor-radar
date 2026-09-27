import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { QueueScreen } from '../screens/QueueScreen';
import type { CaseItem, Signal } from '../domain/types';

const seedSignals: Signal[] = [
  {
    id: 's01',
    kind: 'message',
    client: 'c01',
    text: 'Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.',
    intent: 'Reclamação',
    churn: true,
  },
  {
    id: 's02',
    kind: 'message',
    client: 'c02',
    text: 'Já é a terceira vez que eu explico o mesmo problema e ninguém resolve. Um absurdo.',
    intent: 'Reclamação',
    frustration: 3,
  },
];

const seedCases: CaseItem[] = [
  {
    id: 'k1042',
    client: 'c02',
    signal: 's02',
    state: 1,
    openedAgo: 25,
    slaTotal: 120,
    escalated: false,
    history: [],
  },
  {
    id: 'k1038',
    client: 'c07',
    signal: 's07',
    state: 2,
    openedAgo: 65,
    slaTotal: 120,
    escalated: true,
    history: [],
  },
  {
    id: 'k1031',
    client: 'c09',
    signal: 's09',
    state: 3,
    openedAgo: 400,
    slaTotal: 60,
    escalated: false,
    history: [],
  },
];

function mockFetch(queue: Signal[] = seedSignals) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/v1/queue') && (!init || !init.method || init.method === 'GET')) {
        return new Response(JSON.stringify({ items: queue }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.endsWith('/v1/cases')) {
        return new Response(JSON.stringify({ items: seedCases, states: [] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/v1/actions/') && init?.method === 'PUT') {
        return new Response(null, { status: 204 });
      }
      if (url.includes('/v1/actions/') && init?.method === 'DELETE') {
        return new Response(null, { status: 204 });
      }
      return new Response('not found', { status: 404 });
    }),
  );
}

describe('QueueScreen', () => {
  beforeEach(() => {
    mockFetch();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('on phone, opening s01 replaces the board', async () => {
    const user = userEvent.setup();
    render(<QueueScreen isDesktop={false} disableStream />);

    await waitFor(() => {
      expect(screen.getByTestId('queue-board')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Abrir Mariana Costa' }));

    expect(screen.queryByTestId('queue-board')).not.toBeInTheDocument();
    expect(screen.getByTestId('signal-detail')).toBeInTheDocument();
    expect(screen.getByText('Mariana Costa')).toBeInTheDocument();
  });

  it('on desktop, opening s01 keeps the board and shows a dialog', async () => {
    const user = userEvent.setup();
    render(<QueueScreen isDesktop disableStream />);

    await waitFor(() => {
      expect(screen.getByTestId('queue-board')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Abrir Mariana Costa' }));

    expect(screen.getByTestId('queue-board')).toBeInTheDocument();
    expect(screen.getByTestId('detail-modal')).toBeInTheDocument();
    expect(within(screen.getByTestId('detail-modal')).getByText('Mariana Costa')).toBeInTheDocument();
  });

  it('shows Contatado on s02 after contact and reload', async () => {
    const user = userEvent.setup();
    let contacted = false;

    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith('/v1/queue') && (!init || !init.method || init.method === 'GET')) {
          const items = seedSignals.map((s) =>
            s.id === 's02' && contacted
              ? { ...s, contacted_at: '2026-09-27T15:00:00Z' }
              : s,
          );
          return new Response(JSON.stringify({ items }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.endsWith('/v1/cases')) {
          return new Response(JSON.stringify({ items: seedCases }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes('/v1/actions/s02') && init?.method === 'PUT') {
          contacted = true;
          return new Response(null, { status: 204 });
        }
        return new Response('not found', { status: 404 });
      }),
    );

    const first = render(<QueueScreen isDesktop disableStream />);
    await waitFor(() => expect(first.getByTestId('signal-s02')).toBeInTheDocument());

    const card = first.getByTestId('signal-s02');
    await user.click(within(card).getByRole('button', { name: 'Contatado' }));
    await waitFor(() => {
      expect(within(first.getByTestId('signal-s02')).getByText((_, el) => {
        return el?.classList.contains('signal-card__contacted') === true;
      })).toBeInTheDocument();
    });

    first.unmount();
    const second = render(<QueueScreen isDesktop disableStream />);
    await waitFor(() => expect(second.getByTestId('signal-s02')).toBeInTheDocument());
    expect(
      within(second.getByTestId('signal-s02')).getByText((_, el) => {
        return el?.classList.contains('signal-card__contacted') === true;
      }),
    ).toBeInTheDocument();
  });

  it('keeps a live signal on the pill until it is tapped', async () => {
    const user = userEvent.setup();

    class FakeEventSource {
      static current: FakeEventSource | null = null;
      listeners = new Map<string, (ev: { data: string; lastEventId: string }) => void>();

      constructor(_url: string) {
        FakeEventSource.current = this;
      }

      addEventListener(
        type: string,
        fn: (ev: { data: string; lastEventId: string }) => void,
      ) {
        this.listeners.set(type, fn);
      }

      close() {}
    }

    vi.stubGlobal('EventSource', FakeEventSource);
    render(<QueueScreen isDesktop />);

    await waitFor(() => {
      expect(screen.getByTestId('queue-board')).toBeInTheDocument();
      expect(FakeEventSource.current).not.toBeNull();
    });

    FakeEventSource.current?.listeners.get('signal')?.({
      data: JSON.stringify({
        id: 'n01',
        kind: 'message',
        client: 'c18',
        text: 'ao vivo',
        intent: 'Reclamação',
      }),
      lastEventId: 'n01',
    });

    const pill = await screen.findByRole('button', { name: '1 novo' });
    expect(screen.queryByText('ao vivo')).not.toBeInTheDocument();

    await user.click(pill);

    expect(screen.getByText('ao vivo')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '1 novo' })).not.toBeInTheDocument();
  });

  it('Adiar 1 h removes s01 and Desfazer restores it', async () => {
    const user = userEvent.setup();
    let snoozed = false;
    let deleted = false;

    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith('/v1/queue') && (!init || !init.method || init.method === 'GET')) {
          const items = snoozed && !deleted ? seedSignals.filter((s) => s.id !== 's01') : seedSignals;
          return new Response(JSON.stringify({ items }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.endsWith('/v1/cases')) {
          return new Response(JSON.stringify({ items: seedCases }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes('/v1/actions/s01') && init?.method === 'PUT') {
          snoozed = true;
          deleted = false;
          return new Response(null, { status: 204 });
        }
        if (url.includes('/v1/actions/s01') && init?.method === 'DELETE') {
          deleted = true;
          snoozed = false;
          return new Response(null, { status: 204 });
        }
        return new Response('not found', { status: 404 });
      }),
    );

    render(<QueueScreen isDesktop disableStream />);
    await waitFor(() => expect(screen.getByTestId('signal-s01')).toBeInTheDocument());

    await user.click(
      within(screen.getByTestId('signal-s01')).getByRole('button', { name: 'Adiar 1 h' }),
    );
    await waitFor(() => {
      expect(screen.queryByTestId('signal-s01')).not.toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Desfazer' }));
    await waitFor(() => {
      expect(screen.getByTestId('signal-s01')).toBeInTheDocument();
    });
  });

  it('keeps a pill signal off the board after Desfazer reload', async () => {
    const user = userEvent.setup();
    let snoozed = false;
    let afterUndo = false;

    class FakeEventSource {
      static current: FakeEventSource | null = null;
      listeners = new Map<string, (ev: { data: string; lastEventId: string }) => void>();

      constructor(_url: string) {
        FakeEventSource.current = this;
      }

      addEventListener(
        type: string,
        fn: (ev: { data: string; lastEventId: string }) => void,
      ) {
        this.listeners.set(type, fn);
      }

      close() {}
    }

    const live: Signal = {
      id: 'n01',
      kind: 'message',
      client: 'c18',
      text: 'ao vivo',
      intent: 'Reclamação',
    };

    vi.stubGlobal('EventSource', FakeEventSource);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith('/v1/queue') && (!init || !init.method || init.method === 'GET')) {
          let items = snoozed ? seedSignals.filter((s) => s.id !== 's01') : [...seedSignals];
          if (afterUndo) {
            items = [...items, live];
          }
          return new Response(JSON.stringify({ items }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.endsWith('/v1/cases')) {
          return new Response(JSON.stringify({ items: seedCases }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes('/v1/actions/s01') && init?.method === 'PUT') {
          snoozed = true;
          return new Response(null, { status: 204 });
        }
        if (url.includes('/v1/actions/s01') && init?.method === 'DELETE') {
          snoozed = false;
          afterUndo = true;
          return new Response(null, { status: 204 });
        }
        return new Response('not found', { status: 404 });
      }),
    );

    render(<QueueScreen isDesktop />);

    await waitFor(() => {
      expect(screen.getByTestId('queue-board')).toBeInTheDocument();
      expect(FakeEventSource.current).not.toBeNull();
    });

    FakeEventSource.current?.listeners.get('signal')?.({
      data: JSON.stringify(live),
      lastEventId: 'n01',
    });

    await screen.findByRole('button', { name: '1 novo' });
    expect(screen.queryByText('ao vivo')).not.toBeInTheDocument();

    await user.click(
      within(screen.getByTestId('signal-s01')).getByRole('button', { name: 'Adiar 1 h' }),
    );
    await waitFor(() => {
      expect(screen.queryByTestId('signal-s01')).not.toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Desfazer' }));
    await waitFor(() => {
      expect(screen.getByTestId('signal-s01')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: '1 novo' })).toBeInTheDocument();
    expect(screen.queryByText('ao vivo')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: '1 novo' }));
    expect(screen.getAllByText('ao vivo')).toHaveLength(1);
    expect(screen.queryByRole('button', { name: '1 novo' })).not.toBeInTheDocument();
  });
});
