import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import type { CaseItem, ClientInfo, Signal } from '../domain/types';
import { CustomerScreen } from '../screens/CustomerScreen';
import { QueueScreen } from '../screens/QueueScreen';
import { CID, KID, ROUTER_BASENAME, SID } from '../test/fixtures';

const seedSignals: Signal[] = [
  {
    id: SID.marianaMsg,
    kind: 'message',
    client: CID.mariana,
    name: 'Mariana Costa',
    segment: 'Singular',
    text: 'Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.',
    intent: 'Reclamação',
    dist: { Reclamação: 0.82, Encerramento: 0.11, Operacional: 0.04, Resgate: 0.03 },
    frustration: 2,
    churn: true,
    churnConf: 'alta',
  },
  {
    id: SID.pauloMsg,
    kind: 'message',
    client: CID.paulo,
    name: 'Paulo Henrique Souza',
    segment: 'Advance',
    text: 'Já é a terceira vez que eu explico o mesmo problema e ninguém resolve. Um absurdo.',
    intent: 'Reclamação',
    frustration: 3,
  },
  {
    id: SID.robertoMsg,
    kind: 'message',
    client: CID.roberto,
    name: 'Roberto Nascimento',
    segment: 'Essencial',
    text: 'Meu cartão foi recusado na viagem, o que eu faço?',
    intent: 'Operacional',
    dist: { Operacional: 0.71, Reclamação: 0.19, Contato: 0.1 },
    frustration: 1,
    fallback: true,
  },
  {
    id: SID.sergioAlert,
    kind: 'alert',
    client: CID.sergio,
    name: 'Sérgio Cardoso',
    segment: 'Singular',
    alert: 'saque',
    reason: 'Saque de US$ 190.000,00, 30% do patrimônio',
    rule: 'Saque acima de 20% do patrimônio em 24 horas',
  },
];

const marianaCustomer: ClientInfo = {
  id: CID.mariana,
  name: 'Mariana Costa',
  segment: 'Singular',
  aum: 248300,
  advisor: 'Ana Paula Ribeiro',
  since: '2021',
};

const timelineMariana = [
  {
    event_id: '018f2c1a-7b3e-7000-8000-000000000401',
    customer_id: CID.mariana,
    kind: 'mensagem',
    title: 'Mensagem · chat',
    text: 'Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.',
    meta: 'Reclamação · Frustrado · risco de saída',
    ago: 12,
  },
  {
    event_id: '018f2c1a-7b3e-7000-8000-000000000402',
    customer_id: CID.mariana,
    kind: 'nota',
    title: 'Nota do assessor',
    text: 'Cliente pretende comprar imóvel em Orlando no 1º semestre. Precisa de liquidez em março.',
    meta: 'Ana Paula Ribeiro',
    ago: 2900,
  },
  {
    event_id: '018f2c1a-7b3e-7000-8000-000000000403',
    customer_id: CID.mariana,
    kind: 'saque',
    title: 'Saque',
    text: 'US$ 20.000,00 para conta nos EUA',
    meta: 'Sem alerta · 7% do patrimônio',
    ago: 4400,
  },
];

const seedCases: CaseItem[] = [
  {
    id: KID.paulo,
    client: CID.paulo,
    name: 'Paulo Henrique Souza',
    segment: 'Advance',
    signal: SID.pauloMsg,
    state: 1,
    openedAgo: 25,
    slaTotal: 120,
    escalated: false,
    history: [],
  },
  {
    id: KID.ana,
    client: CID.anaBeatriz,
    name: 'Ana Beatriz Oliveira',
    segment: 'Advance',
    signal: SID.anaMsg,
    state: 2,
    openedAgo: 65,
    slaTotal: 120,
    escalated: true,
    history: [],
  },
  {
    id: KID.patricia,
    client: CID.patricia,
    name: 'Patrícia Gomes',
    segment: 'Singular',
    signal: SID.patriciaMsg,
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
      if (url.includes('/v1/queue') && !url.includes('/stream') && (!init || !init.method || init.method === 'GET')) {
        return new Response(JSON.stringify({ items: queue }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/v1/cases') && !url.includes('/advance')) {
        return new Response(JSON.stringify({ items: seedCases, states: [] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes(`/v1/customers/${CID.mariana}`) && !url.includes('/timeline')) {
        return new Response(JSON.stringify(marianaCustomer), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/v1/customers/') && url.includes('/timeline')) {
        const u = new URL(url, 'http://localhost');
        const q = (u.searchParams.get('q') ?? '').toLowerCase();
        const kind = u.searchParams.get('kind') ?? '';
        let items = [...timelineMariana];
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

function renderQueue(opts: { isDesktop?: boolean; disableStream?: boolean } = {}) {
  const { isDesktop = true, disableStream = true } = opts;
  return render(
    <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[`${ROUTER_BASENAME}/fila`]}>
      <Routes>
        <Route
          path="/fila"
          element={<QueueScreen isDesktop={isDesktop} disableStream={disableStream} />}
        />
        <Route path="/clientes/:id" element={<CustomerScreen />} />
      </Routes>
    </MemoryRouter>,
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

  it('on phone, opening a signal navigates to the customer page', async () => {
    const user = userEvent.setup();
    renderQueue({ isDesktop: false });

    await waitFor(() => {
      expect(screen.getByTestId('queue-board')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('link', { name: 'Abrir Mariana Costa' }));

    await waitFor(() => {
      expect(screen.getByTestId('customer-page')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('queue-board')).not.toBeInTheDocument();
    expect(screen.getByText('Mariana Costa')).toBeInTheDocument();
  });

  it('on desktop, opening a signal navigates to the customer page', async () => {
    const user = userEvent.setup();
    renderQueue({ isDesktop: true });

    await waitFor(() => {
      expect(screen.getByTestId('queue-board')).toBeInTheDocument();
    });

    await user.click(screen.getByRole('link', { name: 'Abrir Mariana Costa' }));

    await waitFor(() => {
      expect(screen.getByTestId('customer-page')).toBeInTheDocument();
    });
    expect(screen.getByTestId('client-360')).toBeInTheDocument();
  });

  it('shows Contatado after contact and reload', async () => {
    const user = userEvent.setup();
    let contacted = false;

    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes('/v1/queue') && !url.includes('/stream') && (!init || !init.method || init.method === 'GET')) {
          const items = seedSignals.map((s) =>
            s.id === SID.pauloMsg && contacted
              ? { ...s, contacted_at: '2026-09-27T15:00:00Z' }
              : s,
          );
          return new Response(JSON.stringify({ items }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes('/v1/cases')) {
          return new Response(JSON.stringify({ items: seedCases }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes(`/v1/actions/${SID.pauloMsg}`) && init?.method === 'PUT') {
          contacted = true;
          return new Response(null, { status: 204 });
        }
        return new Response('not found', { status: 404 });
      }),
    );

    const first = renderQueue({ isDesktop: true });
    await waitFor(() => expect(first.getByTestId(`signal-${SID.pauloMsg}`)).toBeInTheDocument());

    const card = first.getByTestId(`signal-${SID.pauloMsg}`);
    await user.click(within(card).getByRole('button', { name: 'Contatado' }));
    await waitFor(() => {
      expect(within(first.getByTestId(`signal-${SID.pauloMsg}`)).getByText((_, el) => {
        return el?.classList.contains('signal-card__contacted') === true;
      })).toBeInTheDocument();
    });

    first.unmount();
    const second = renderQueue({ isDesktop: true });
    await waitFor(() => expect(second.getByTestId(`signal-${SID.pauloMsg}`)).toBeInTheDocument());
    expect(
      within(second.getByTestId(`signal-${SID.pauloMsg}`)).getByText((_, el) => {
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
    renderQueue({ isDesktop: true, disableStream: false });

    await waitFor(() => {
      expect(screen.getByTestId('queue-board')).toBeInTheDocument();
      expect(FakeEventSource.current).not.toBeNull();
    });

    FakeEventSource.current?.listeners.get('signal')?.({
      data: JSON.stringify({
        id: SID.live,
        kind: 'message',
        client: CID.vanessa,
        name: 'Vanessa Moreira',
        segment: 'Advance',
        text: 'ao vivo',
        intent: 'Reclamação',
      }),
      lastEventId: SID.live,
    });

    const pill = await screen.findByRole('button', { name: '1 novo' });
    expect(screen.queryByText('ao vivo')).not.toBeInTheDocument();

    await user.click(pill);

    expect(screen.getByText('ao vivo')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '1 novo' })).not.toBeInTheDocument();
  });

  it('Adiar 1 h removes a signal and Desfazer restores it', async () => {
    const user = userEvent.setup();
    let snoozed = false;
    let deleted = false;

    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes('/v1/queue') && !url.includes('/stream') && (!init || !init.method || init.method === 'GET')) {
          const items = snoozed && !deleted
            ? seedSignals.filter((s) => s.id !== SID.marianaMsg)
            : seedSignals;
          return new Response(JSON.stringify({ items }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes('/v1/cases')) {
          return new Response(JSON.stringify({ items: seedCases }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes(`/v1/actions/${SID.marianaMsg}`) && init?.method === 'PUT') {
          snoozed = true;
          deleted = false;
          return new Response(null, { status: 204 });
        }
        if (url.includes(`/v1/actions/${SID.marianaMsg}`) && init?.method === 'DELETE') {
          deleted = true;
          snoozed = false;
          return new Response(null, { status: 204 });
        }
        return new Response('not found', { status: 404 });
      }),
    );

    renderQueue({ isDesktop: true });
    await waitFor(() => expect(screen.getByTestId(`signal-${SID.marianaMsg}`)).toBeInTheDocument());

    await user.click(
      within(screen.getByTestId(`signal-${SID.marianaMsg}`)).getByRole('button', { name: 'Adiar 1 h' }),
    );
    await waitFor(() => {
      expect(screen.queryByTestId(`signal-${SID.marianaMsg}`)).not.toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Desfazer' }));
    await waitFor(() => {
      expect(screen.getByTestId(`signal-${SID.marianaMsg}`)).toBeInTheDocument();
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
      id: SID.live,
      kind: 'message',
      client: CID.vanessa,
      name: 'Vanessa Moreira',
      segment: 'Advance',
      text: 'ao vivo',
      intent: 'Reclamação',
    };

    vi.stubGlobal('EventSource', FakeEventSource);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes('/v1/queue') && !url.includes('/stream') && (!init || !init.method || init.method === 'GET')) {
          let items = snoozed ? seedSignals.filter((s) => s.id !== SID.marianaMsg) : [...seedSignals];
          if (afterUndo) {
            items = [...items, live];
          }
          return new Response(JSON.stringify({ items }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes('/v1/cases')) {
          return new Response(JSON.stringify({ items: seedCases }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes(`/v1/actions/${SID.marianaMsg}`) && init?.method === 'PUT') {
          snoozed = true;
          return new Response(null, { status: 204 });
        }
        if (url.includes(`/v1/actions/${SID.marianaMsg}`) && init?.method === 'DELETE') {
          snoozed = false;
          afterUndo = true;
          return new Response(null, { status: 204 });
        }
        return new Response('not found', { status: 404 });
      }),
    );

    renderQueue({ isDesktop: true, disableStream: false });

    await waitFor(() => {
      expect(screen.getByTestId('queue-board')).toBeInTheDocument();
      expect(FakeEventSource.current).not.toBeNull();
    });

    FakeEventSource.current?.listeners.get('signal')?.({
      data: JSON.stringify(live),
      lastEventId: SID.live,
    });

    await screen.findByRole('button', { name: '1 novo' });
    expect(screen.queryByText('ao vivo')).not.toBeInTheDocument();

    await user.click(
      within(screen.getByTestId(`signal-${SID.marianaMsg}`)).getByRole('button', { name: 'Adiar 1 h' }),
    );
    await waitFor(() => {
      expect(screen.queryByTestId(`signal-${SID.marianaMsg}`)).not.toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Desfazer' }));
    await waitFor(() => {
      expect(screen.getByTestId(`signal-${SID.marianaMsg}`)).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: '1 novo' })).toBeInTheDocument();
    expect(screen.queryByText('ao vivo')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: '1 novo' }));
    expect(screen.getAllByText('ao vivo')).toHaveLength(1);
    expect(screen.queryByRole('button', { name: '1 novo' })).not.toBeInTheDocument();
  });

  it('shows review and risk labels on the Mariana card', async () => {
    renderQueue({ isDesktop: true });
    await waitFor(() => expect(screen.getByTestId(`signal-${SID.marianaMsg}`)).toBeInTheDocument());
    const card = screen.getByTestId(`signal-${SID.marianaMsg}`);
    expect(card).toHaveTextContent('Se isso não for resolvido hoje');
    expect(card).toHaveTextContent('Reclamação');
    expect(card).toHaveTextContent('Frustrado');
    expect(card).toHaveTextContent('Mensagem com risco');
    expect(card).toHaveTextContent('Risco de saída');
  });

  it('shows Classificação simplificada on the Roberto card', async () => {
    renderQueue({ isDesktop: true });
    await waitFor(() => expect(screen.getByTestId(`signal-${SID.robertoMsg}`)).toBeInTheDocument());
    expect(screen.getByTestId(`signal-${SID.robertoMsg}`)).toHaveTextContent(
      'Classificação simplificada',
    );
  });

  it('shows Saque relevante reason on the Sérgio card', async () => {
    renderQueue({ isDesktop: true });
    await waitFor(() => expect(screen.getByTestId(`signal-${SID.sergioAlert}`)).toBeInTheDocument());
    const card = screen.getByTestId(`signal-${SID.sergioAlert}`);
    expect(card).toHaveTextContent('Saque relevante');
    expect(card).toHaveTextContent('Saque de US$ 190.000,00, 30% do patrimônio');
  });

  it('shows Em atendimento on the Paulo case rail and keeps resolved cases off the board', async () => {
    const user = userEvent.setup();
    renderQueue({ isDesktop: true });
    await waitFor(() => expect(screen.getByTestId(`case-${KID.paulo}`)).toBeInTheDocument());
    expect(screen.queryByTestId(`case-${KID.patricia}`)).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: `Abrir caso ${KID.paulo}` }));
    const rail = screen.getByTestId('case-rail');
    expect(within(rail).getByText('Em atendimento')).toBeInTheDocument();
    const current = within(rail).getByText('Em atendimento').closest('li');
    expect(current).toHaveAttribute('aria-current', 'step');
  });

  it('customer page Orlando search shows the note and hides the saque', async () => {
    const user = userEvent.setup();
    renderQueue({ isDesktop: true });
    await waitFor(() => expect(screen.getByTestId(`signal-${SID.marianaMsg}`)).toBeInTheDocument());
    await user.click(screen.getByRole('link', { name: 'Abrir Mariana Costa' }));
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

  it('customer page Notas chip shows the note and hides the saque', async () => {
    const user = userEvent.setup();
    renderQueue({ isDesktop: true });
    await waitFor(() => expect(screen.getByTestId(`signal-${SID.marianaMsg}`)).toBeInTheDocument());
    await user.click(screen.getByRole('link', { name: 'Abrir Mariana Costa' }));
    await waitFor(() => expect(screen.getByTestId('client-360')).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Notas' }));
    await waitFor(() => {
      const list = screen.getByTestId('timeline-list');
      expect(list).toHaveTextContent('Orlando');
      expect(list).not.toHaveTextContent('US$ 20.000,00');
    });
  });

  it('shows 404 when the customer is missing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('/v1/customers/')) {
          return new Response('', { status: 404 });
        }
        return new Response('not found', { status: 404 });
      }),
    );

    render(
      <MemoryRouter
        basename={ROUTER_BASENAME}
        initialEntries={[`${ROUTER_BASENAME}/clientes/${CID.mariana}`]}
      >
        <Routes>
          <Route path="/clientes/:id" element={<CustomerScreen />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId('customer-not-found')).toBeInTheDocument();
    });
    expect(screen.getByText('Cliente não encontrado.')).toBeInTheDocument();
  });

  it('shows 400 when the customer id is invalid', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('', { status: 400 })),
    );

    render(
      <MemoryRouter
        basename={ROUTER_BASENAME}
        initialEntries={[`${ROUTER_BASENAME}/clientes/not-a-uuid`]}
      >
        <Routes>
          <Route path="/clientes/:id" element={<CustomerScreen />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByTestId('customer-bad-id')).toBeInTheDocument();
    });
    expect(screen.getByText('Identificador de cliente inválido.')).toBeInTheDocument();
  });
});
