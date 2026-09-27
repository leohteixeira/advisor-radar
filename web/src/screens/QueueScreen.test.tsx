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
    dist: { Reclamação: 0.82, Encerramento: 0.11, Operacional: 0.04, Resgate: 0.03 },
    frustration: 2,
    churn: true,
    churnConf: 'alta',
  },
  {
    id: 's02',
    kind: 'message',
    client: 'c02',
    text: 'Já é a terceira vez que eu explico o mesmo problema e ninguém resolve. Um absurdo.',
    intent: 'Reclamação',
    frustration: 3,
  },
  {
    id: 's06',
    kind: 'message',
    client: 'c06',
    text: 'Meu cartão foi recusado na viagem, o que eu faço?',
    intent: 'Operacional',
    dist: { Operacional: 0.71, Reclamação: 0.19, Contato: 0.1 },
    frustration: 1,
    fallback: true,
  },
  {
    id: 's11',
    kind: 'alert',
    client: 'c11',
    alert: 'saque',
    reason: 'Saque de US$ 190.000,00, 30% do patrimônio',
    rule: 'Saque acima de 20% do patrimônio em 24 horas',
  },
];

const timelineC01 = [
  {
    event_id: 'tl-c01-msg-1',
    customer_id: 'c01',
    kind: 'mensagem',
    title: 'Mensagem · chat',
    text: 'Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.',
    meta: 'Reclamação · Frustrado · risco de saída',
    ago: 12,
  },
  {
    event_id: 'tl-c01-nota-1',
    customer_id: 'c01',
    kind: 'nota',
    title: 'Nota do assessor',
    text: 'Cliente pretende comprar imóvel em Orlando no 1º semestre. Precisa de liquidez em março.',
    meta: 'Ana Paula Ribeiro',
    ago: 2900,
  },
  {
    event_id: 'tl-c01-saque-1',
    customer_id: 'c01',
    kind: 'saque',
    title: 'Saque',
    text: 'US$ 20.000,00 para conta nos EUA',
    meta: 'Sem alerta · 7% do patrimônio',
    ago: 4400,
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
      if (url.includes('/v1/customers/') && url.includes('/timeline')) {
        const u = new URL(url, 'http://localhost');
        const q = (u.searchParams.get('q') ?? '').toLowerCase();
        const kind = u.searchParams.get('kind') ?? '';
        let items = [...timelineC01];
        if (kind === 'notas') {
          items = items.filter((i) => i.kind === 'nota');
        }
        if (q) {
          items = items.filter(
            (i) =>
              i.title.toLowerCase().includes(q) ||
              i.text.toLowerCase().includes(q) ||
              i.meta.toLowerCase().includes(q),
          );
        }
        return new Response(JSON.stringify({ items }), {
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

  it('shows review and risk labels on s01 detail', async () => {
    const user = userEvent.setup();
    render(<QueueScreen isDesktop disableStream />);
    await waitFor(() => expect(screen.getByTestId('signal-s01')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Abrir Mariana Costa' }));
    const detail = screen.getByTestId('signal-detail');
    expect(detail).toHaveTextContent('Se isso não for resolvido hoje');
    expect(detail).toHaveTextContent('Reclamação');
    expect(detail).toHaveTextContent('Frustrado');
    expect(detail).toHaveTextContent('Mensagem com risco');
    expect(detail).toHaveTextContent('Precisa de revisão');
  });

  it('shows Classificação simplificada on s06', async () => {
    const user = userEvent.setup();
    render(<QueueScreen isDesktop disableStream />);
    await waitFor(() => expect(screen.getByTestId('signal-s06')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Abrir Roberto Nascimento' }));
    expect(screen.getByTestId('fallback-badge')).toHaveTextContent('Classificação simplificada');
  });

  it('shows Saque relevante and reason on s11', async () => {
    const user = userEvent.setup();
    render(<QueueScreen isDesktop disableStream />);
    await waitFor(() => expect(screen.getByTestId('signal-s11')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Abrir Sérgio Cardoso' }));
    expect(screen.getByTestId('alert-label')).toHaveTextContent('Saque relevante');
    expect(screen.getByTestId('alert-reason')).toHaveTextContent(
      'Saque de US$ 190.000,00, 30% do patrimônio',
    );
  });

  it('shows Em atendimento on the k1042 case rail and keeps k1031 off the board', async () => {
    const user = userEvent.setup();
    render(<QueueScreen isDesktop disableStream />);
    await waitFor(() => expect(screen.getByTestId('case-k1042')).toBeInTheDocument());
    expect(screen.queryByTestId('case-k1031')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Abrir caso k1042' }));
    const rail = screen.getByTestId('case-rail');
    expect(within(rail).getByText('Em atendimento')).toBeInTheDocument();
    const current = within(rail).getByText('Em atendimento').closest('li');
    expect(current).toHaveAttribute('aria-current', 'step');
  });

  it('c01 Orlando search shows the note and hides the saque', async () => {
    const user = userEvent.setup();
    render(<QueueScreen isDesktop disableStream />);
    await waitFor(() => expect(screen.getByTestId('signal-s01')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Abrir Mariana Costa' }));
    await user.click(screen.getByRole('button', { name: /Ver visão 360 de Mariana/ }));
    await waitFor(() => expect(screen.getByTestId('client-360')).toBeInTheDocument());
    const search = screen.getByTestId('timeline-search');
    await user.clear(search);
    await user.type(search, 'Orlando');
    await waitFor(() => {
      const list = screen.getByTestId('timeline-list');
      expect(list).toHaveTextContent('Orlando');
      expect(list).not.toHaveTextContent('US$ 20.000,00');
    });
  });

  it('c01 Notas chip shows the note and hides the saque', async () => {
    const user = userEvent.setup();
    render(<QueueScreen isDesktop disableStream />);
    await waitFor(() => expect(screen.getByTestId('signal-s01')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Abrir Mariana Costa' }));
    await user.click(screen.getByRole('button', { name: /Ver visão 360 de Mariana/ }));
    await waitFor(() => expect(screen.getByTestId('client-360')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Notas' }));
    await waitFor(() => {
      const list = screen.getByTestId('timeline-list');
      expect(list).toHaveTextContent('Orlando');
      expect(list).not.toHaveTextContent('US$ 20.000,00');
    });
  });
});
