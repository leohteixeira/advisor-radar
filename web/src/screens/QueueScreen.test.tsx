import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { ICONS } from '../domain/queueVisual';
import type { CaseItem, ClientInfo, ListFacets, Signal } from '../domain/types';
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

const emptyFacets = {
  andamento: [],
  sla: [],
  sinal: [],
  motivo: [],
  segmento: [],
};

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
    slaRemaining: 95,
    slaState: 'No prazo',
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
    slaRemaining: 55,
    slaState: 'No prazo',
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
    slaRemaining: -340,
    slaState: 'Concluído',
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
        return new Response(JSON.stringify({ items: queue, facets: emptyFacets }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/v1/cases') && !url.includes('/advance')) {
        return new Response(JSON.stringify({ items: seedCases, states: [], facets: emptyFacets }), {
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
        const order = u.searchParams.get('order') ?? 'asc';
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
        items.sort((a, b) => a.ago - b.ago);
        if (order === 'desc') {
          items.reverse();
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
          return new Response(JSON.stringify({ items, facets: emptyFacets }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes('/v1/cases')) {
          return new Response(JSON.stringify({ items: seedCases, facets: emptyFacets }), {
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
    let liveAnnounced = false;

    const live: Signal = {
      id: SID.live,
      kind: 'message',
      client: CID.vanessa,
      name: 'Vanessa Moreira',
      segment: 'Advance',
      text: 'ao vivo',
      intent: 'Reclamação',
    };

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
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes('/v1/queue') && !url.includes('/stream') && (!init || !init.method || init.method === 'GET')) {
          const items = liveAnnounced ? [...seedSignals, live] : [...seedSignals];
          return new Response(JSON.stringify({ items, facets: emptyFacets }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes('/v1/cases')) {
          return new Response(JSON.stringify({ items: seedCases, facets: emptyFacets }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        return new Response('not found', { status: 404 });
      }),
    );
    renderQueue({ isDesktop: true, disableStream: false });

    await waitFor(() => {
      expect(screen.getByTestId('queue-board')).toBeInTheDocument();
      expect(FakeEventSource.current).not.toBeNull();
    });

    liveAnnounced = true;
    FakeEventSource.current?.listeners.get('signal')?.({
      data: JSON.stringify(live),
      lastEventId: SID.live,
    });

    const pill = await screen.findByRole('button', { name: '1 novo' });
    expect(screen.queryByText('ao vivo')).not.toBeInTheDocument();

    await user.click(pill);

    await waitFor(() => {
      expect(screen.getByText('ao vivo')).toBeInTheDocument();
    });
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
          return new Response(JSON.stringify({ items, facets: emptyFacets }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes('/v1/cases')) {
          return new Response(JSON.stringify({ items: seedCases, facets: emptyFacets }), {
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
          return new Response(JSON.stringify({ items, facets: emptyFacets }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        if (url.includes('/v1/cases')) {
          return new Response(JSON.stringify({ items: seedCases, facets: emptyFacets }), {
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

  it('renders a perfil alert as Compra acima do perfil with its reason and gauge icon', async () => {
    const perfil: Signal = {
      id: SID.fernandaPerfil,
      kind: 'alert',
      client: CID.fernanda,
      name: 'Fernanda Lima',
      segment: 'Essencial',
      alert: 'perfil',
      reason: 'Compra de US$ 1.000,00 em Cobalto Semicondutores, risco 5. Perfil conservador vai até risco 2.',
      rule: 'Compra acima do perfil de investidor',
      product_id: 'cobalto',
      risk: 5,
      profile: 'conservador',
      max_risk: 2,
    };
    mockFetch([...seedSignals, perfil]);
    renderQueue({ isDesktop: true });
    await waitFor(() => expect(screen.getByTestId(`signal-${SID.fernandaPerfil}`)).toBeInTheDocument());
    const card = screen.getByTestId(`signal-${SID.fernandaPerfil}`);
    expect(card).toHaveTextContent('Compra acima do perfil');
    expect(card).toHaveTextContent('Cobalto Semicondutores, risco 5. Perfil conservador vai até risco 2.');
    expect(card.querySelector(`path[d="${ICONS.perfil}"]`)).not.toBeNull();
  });

  it('shows Em atendimento on the Paulo case rail and keeps resolved cases off the board', async () => {
    const user = userEvent.setup();
    renderQueue({ isDesktop: true });
    await waitFor(() => expect(screen.getByTestId(`case-${KID.paulo}`)).toBeInTheDocument());
    expect(screen.queryByTestId(`case-${KID.patricia}`)).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Abrir caso 018F2C1A-7B3E de Paulo Henrique Souza' }));
    const rail = screen.getByTestId('case-rail');
    expect(within(rail).getByText('Em atendimento')).toBeInTheDocument();
    const current = within(rail).getByText('Em atendimento').closest('li');
    expect(current).toHaveAttribute('aria-current', 'step');
  });

  it('customer page opens with the most recent message and Mais recentes toggles to saque', async () => {
    const user = userEvent.setup();
    renderQueue({ isDesktop: true });
    await waitFor(() => expect(screen.getByTestId(`signal-${SID.marianaMsg}`)).toBeInTheDocument());
    await user.click(screen.getByRole('link', { name: 'Abrir Mariana Costa' }));
    await waitFor(() => expect(screen.getByTestId('client-360')).toBeInTheDocument());

    const list = screen.getByTestId('timeline-list');
    const items = within(list).getAllByRole('listitem');
    expect(items[0]).toHaveTextContent('Se isso não for resolvido hoje');

    await user.click(screen.getByRole('button', { name: 'Mais recentes' }));
    await waitFor(() => {
      const ordered = within(screen.getByTestId('timeline-list')).getAllByRole('listitem');
      expect(ordered[0]).toHaveTextContent('Saque');
    });

    const fetchMock = vi.mocked(fetch);
    const orderCalls = fetchMock.mock.calls
      .map((c) => String(c[0]))
      .filter((u) => u.includes('/timeline') && u.includes('order=desc'));
    expect(orderCalls.length).toBeGreaterThan(0);
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

/** A BFF whose lists change as the test tells it, with every request recorded. */
interface FakeBff {
  signals: Signal[];
  cases: CaseItem[];
  facets: ListFacets;
  /** Status of POST /v1/signals/{id}/case. */
  openCaseStatus: number;
  /** Runs when the command is accepted; cases acts on it later, when the test says so. */
  advance: (id: string) => Response;
  requests: { method: string; url: string; key: string | null }[];
}

const NEW_CASE = '018f2c1a-7b3e-7000-8000-000000000299';

function fakeBff(): FakeBff {
  return {
    signals: seedSignals.map((s) => ({ ...s })),
    cases: seedCases.map((c) => ({ ...c })),
    facets: emptyFacets,
    openCaseStatus: 202,
    advance: () => new Response('bad gateway', { status: 502 }),
    requests: [],
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

/** Answers the list routes the way the BFF filters by Andamento. */
function serve(bff: FakeBff) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'http://localhost');
      const method = init?.method ?? 'GET';
      const key = new Headers(init?.headers).get('Idempotency-Key');
      bff.requests.push({ method, url: url.pathname + url.search, key });
      const andamento = url.searchParams.getAll('andamento');

      if (method === 'GET' && url.pathname.endsWith('/v1/queue')) {
        const items = bff.signals.filter((s) => {
          const label = s.snoozed_until ? 'Adiados' : 'Novo sinal';
          return andamento.length === 0 ? label === 'Novo sinal' : andamento.includes(label);
        });
        return json({ items, facets: bff.facets });
      }
      if (method === 'GET' && url.pathname.endsWith('/v1/cases')) {
        const labels = ['Aberto', 'Em atendimento', 'Aguardando cliente', 'Resolvido'];
        const items = bff.cases.filter((c) =>
          andamento.length === 0 ? c.state < 3 : andamento.includes(labels[c.state] ?? ''),
        );
        return json({ items, states: labels, facets: bff.facets });
      }
      if (method === 'POST' && /\/v1\/signals\/[^/]+\/case$/.test(url.pathname)) {
        return bff.openCaseStatus === 202
          ? json({ request_id: 'req-1' }, 202)
          : new Response('refused', { status: bff.openCaseStatus });
      }
      if (method === 'POST' && url.pathname.endsWith('/advance')) {
        return bff.advance(url.pathname.split('/').at(-2) ?? '');
      }
      if (url.pathname.includes('/v1/actions/') && (method === 'PUT' || method === 'DELETE')) {
        const id = url.pathname.split('/').at(-1);
        if (method === 'DELETE') {
          bff.signals = bff.signals.map((s) =>
            s.id === id ? { ...s, contacted_at: undefined, snoozed_until: undefined } : s,
          );
        }
        return new Response(null, { status: 204 });
      }
      return new Response('not found', { status: 404 });
    }),
  );
}

function sent(bff: FakeBff, method: string, path: RegExp) {
  return bff.requests.filter((r) => r.method === method && path.test(r.url));
}

const OPEN_CASE = /\/v1\/signals\/[^/]+\/case$/;
const UNFILTERED_CASES = /\/v1\/cases$/;

/** Counts the re-reads of the unfiltered case list after the board first loaded. */
function pollCounter(bff: FakeBff) {
  const loaded = sent(bff, 'GET', UNFILTERED_CASES).length;
  return () => sent(bff, 'GET', UNFILTERED_CASES).length - loaded;
}

// Node's macrotask queue; the web tsconfig has no Node types.
declare const setImmediate: (fn: () => void) => unknown;

/** Lets pending fetches and their React updates finish; only setTimeout is faked. */
async function settle() {
  await act(async () => {
    for (let i = 0; i < 6; i += 1) {
      await new Promise<void>((resolve) => setImmediate(resolve));
    }
  });
}

/** Moves the polling clock forward. */
async function tick(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
  await settle();
}

async function boardReady() {
  await waitFor(() => expect(screen.getByTestId(`signal-${SID.marianaMsg}`)).toBeInTheDocument());
}

function openCaseButton(signalId: string): HTMLButtonElement {
  const card = screen.getByTestId(`signal-${signalId}`);
  return within(card).getByRole('button', { name: /Abrir caso|Abrindo caso/ }) as HTMLButtonElement;
}

/** What cases does with an accepted command: a new case for the signal, off the queue. */
function casesOpenIt(bff: FakeBff) {
  bff.signals = bff.signals.filter((s) => s.id !== SID.marianaMsg);
  bff.cases = [
    ...bff.cases,
    {
      id: NEW_CASE,
      client: CID.mariana,
      name: 'Mariana Costa',
      segment: 'Singular',
      signal: SID.marianaMsg,
      state: 0,
      openedAgo: 0,
      slaTotal: 30,
      slaRemaining: 30,
      slaState: 'No prazo',
      origin: 'mensagem · Reclamação',
      escalated: false,
      history: [{ ago: 0, kind: 'open', text: 'Caso aberto a partir de mensagem com risco' }],
    },
  ];
}

describe('QueueScreen: Abrir caso', () => {
  let bff: FakeBff;

  beforeEach(() => {
    bff = fakeBff();
    serve(bff);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('sends the command with an Idempotency-Key and shows the case the server opened, which survives a refetch', async () => {
    const first = renderQueue();
    await boardReady();
    const polls = pollCounter(bff);
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });

    fireEvent.click(openCaseButton(SID.marianaMsg));
    await settle();

    // Pending: the command went out once, nothing was drawn locally.
    const commands = sent(bff, 'POST', OPEN_CASE);
    expect(commands).toHaveLength(1);
    expect(commands[0]?.url).toContain(`/v1/signals/${SID.marianaMsg}/case`);
    expect(commands[0]?.key).toMatch(/^[0-9a-f-]{36}$/);
    expect(openCaseButton(SID.marianaMsg)).toBeDisabled();
    expect(openCaseButton(SID.marianaMsg)).toHaveTextContent('Abrindo caso…');
    expect(screen.getAllByTestId(/^case-/)).toHaveLength(2);
    expect(document.body.innerHTML).not.toContain('k-local');

    // First re-read: cases has not acted yet.
    await tick(1000);
    expect(polls()).toBe(1);
    expect(openCaseButton(SID.marianaMsg)).toBeDisabled();

    casesOpenIt(bff);
    await tick(1000);

    expect(screen.getByRole('status')).toHaveTextContent('Caso aberto para Mariana Costa');
    expect(screen.queryByTestId(`signal-${SID.marianaMsg}`)).not.toBeInTheDocument();
    const card = screen.getByTestId(`case-${NEW_CASE}`);
    expect(within(screen.getByRole('region', { name: 'Aberto' })).getByTestId(`case-${NEW_CASE}`)).toBe(card);
    // The clock and the SLA state are the server's, and the origin is named.
    expect(card).toHaveTextContent('00:30:00');
    expect(card).toHaveTextContent('SLA No prazo');
    expect(card).toHaveTextContent('mensagem · Reclamação');
    expect(card).not.toHaveTextContent('sinal arquivado');
    // Two re-reads found the outcome, one reload showed it; nothing after.
    expect(polls()).toBe(3);
    await tick(3000);
    expect(polls()).toBe(3);

    vi.useRealTimers();
    first.unmount();
    renderQueue();
    await waitFor(() => expect(screen.getByTestId(`case-${NEW_CASE}`)).toBeInTheDocument());
    expect(screen.queryByTestId(`signal-${SID.marianaMsg}`)).not.toBeInTheDocument();
  });

  it('says the signal joined the case the client already had open', async () => {
    // Roberto already has a case, opened from another signal.
    const robertoCase: CaseItem = {
      ...seedCases[0]!,
      id: NEW_CASE,
      client: CID.roberto,
      name: 'Roberto Nascimento',
      segment: 'Essencial',
      signal: SID.live,
      state: 1,
    };
    bff.cases = [...bff.cases, robertoCase];
    renderQueue();
    await boardReady();
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });

    fireEvent.click(openCaseButton(SID.robertoMsg));
    await settle();
    bff.signals = bff.signals.filter((s) => s.id !== SID.robertoMsg);
    bff.cases = bff.cases.map((c) =>
      c.id === NEW_CASE
        ? { ...c, history: [{ ago: 0, kind: 'signal', text: 'Mensagem adicionada ao caso', signal: SID.robertoMsg }] }
        : c,
    );
    await tick(1000);

    expect(screen.getByRole('status')).toHaveTextContent(
      'Roberto Nascimento já tinha um caso aberto. O sinal foi adicionado ao histórico desse caso.',
    );
    expect(screen.queryByTestId(`signal-${SID.robertoMsg}`)).not.toBeInTheDocument();
    expect(screen.getAllByTestId(/^case-/)).toHaveLength(3);
    expect(screen.getByTestId(`case-${NEW_CASE}`)).toHaveTextContent('Mensagem adicionada ao caso');
  });

  it.each([
    { name: 'the BFF fails', status: 502, message: 'Não foi possível abrir o caso de Mariana Costa. O sinal continua na fila.' },
    { name: 'the signal left the queue', status: 404, message: 'Este sinal não está mais na fila. Nenhum caso foi aberto.' },
    { name: 'the open-case budget is spent (429)', status: 429, message: 'Muitos casos abertos em pouco tempo. Aguarde alguns minutos e tente de novo.' },
  ])('keeps the signal and retries with the same key when $name', async ({ status, message }) => {
    bff.openCaseStatus = status;
    renderQueue();
    await boardReady();
    const polls = pollCounter(bff);
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });

    fireEvent.click(openCaseButton(SID.marianaMsg));
    await settle();

    expect(screen.getByRole('status')).toHaveTextContent(message);
    expect(openCaseButton(SID.marianaMsg)).toBeEnabled();
    expect(openCaseButton(SID.marianaMsg)).toHaveTextContent('Abrir caso');
    expect(screen.getAllByTestId(/^case-/)).toHaveLength(2);
    expect(polls()).toBe(0);

    bff.openCaseStatus = 202;
    fireEvent.click(screen.getByRole('button', { name: 'Tentar novamente' }));
    await settle();
    casesOpenIt(bff);
    await tick(1000);

    const commands = sent(bff, 'POST', OPEN_CASE);
    expect(commands).toHaveLength(2);
    expect(commands[1]?.key).toBe(commands[0]?.key);
    expect(screen.getByTestId(`case-${NEW_CASE}`)).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Caso aberto para Mariana Costa');
  });

  it('gives up after ten seconds without an outcome, keeps the signal and retries with a new key', async () => {
    renderQueue();
    await boardReady();
    const polls = pollCounter(bff);
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });

    fireEvent.click(openCaseButton(SID.marianaMsg));
    await settle();
    for (let second = 0; second < 9; second += 1) {
      await tick(1000);
    }
    expect(openCaseButton(SID.marianaMsg)).toBeDisabled();
    await tick(1000);

    expect(polls()).toBe(10);
    expect(screen.getByRole('status')).toHaveTextContent(
      'O caso de Mariana Costa ainda não foi confirmado. O sinal continua na fila.',
    );
    expect(openCaseButton(SID.marianaMsg)).toBeEnabled();
    expect(screen.getAllByTestId(/^case-/)).toHaveLength(2);
    await tick(5000);
    expect(polls()).toBe(10);

    fireEvent.click(screen.getByRole('button', { name: 'Tentar novamente' }));
    await settle();
    // The accepted key is known to advisory, which would publish nothing for it again.
    const commands = sent(bff, 'POST', OPEN_CASE);
    expect(commands).toHaveLength(2);
    expect(commands[1]?.key).toMatch(/^[0-9a-f-]{36}$/);
    expect(commands[1]?.key).not.toBe(commands[0]?.key);
  });

  it('uses a new key for another signal', async () => {
    renderQueue();
    await boardReady();
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });

    fireEvent.click(openCaseButton(SID.marianaMsg));
    fireEvent.click(openCaseButton(SID.robertoMsg));
    await settle();

    const commands = sent(bff, 'POST', OPEN_CASE);
    expect(commands).toHaveLength(2);
    expect(commands[0]?.key).not.toBe(commands[1]?.key);
  });

  it('stops re-reading the cases when the screen goes away', async () => {
    const view = renderQueue();
    await boardReady();
    const polls = pollCounter(bff);
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });

    fireEvent.click(openCaseButton(SID.marianaMsg));
    await settle();
    await tick(1000);
    expect(polls()).toBe(1);

    view.unmount();
    await tick(5000);
    expect(polls()).toBe(1);
  });
});

describe('QueueScreen: cases and filters', () => {
  let bff: FakeBff;

  beforeEach(() => {
    bff = fakeBff();
    serve(bff);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('keeps the server state when an advance fails and offers a retry', async () => {
    const user = userEvent.setup();
    renderQueue();
    await waitFor(() => expect(screen.getByTestId(`case-${KID.paulo}`)).toBeInTheDocument());
    const column = () => screen.getByRole('region', { name: 'Em atendimento' });

    await user.click(within(screen.getByTestId(`case-${KID.paulo}`)).getByRole('button', { name: 'Aguardar cliente →' }));

    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Não foi possível avançar o caso. Nada foi alterado.'),
    );
    expect(within(column()).getByTestId(`case-${KID.paulo}`)).toBeInTheDocument();
    expect(within(screen.getByTestId(`case-${KID.paulo}`)).getByRole('button', { name: 'Aguardar cliente →' })).toBeInTheDocument();

    bff.advance = (id) => {
      bff.cases = bff.cases.map((c) => (c.id === id ? { ...c, state: 2 } : c));
      return json(bff.cases.find((c) => c.id === id));
    };
    await user.click(screen.getByRole('button', { name: 'Tentar novamente' }));

    await waitFor(() =>
      expect(
        within(screen.getByRole('region', { name: 'Aguardando cliente' })).getByTestId(`case-${KID.paulo}`),
      ).toBeInTheDocument(),
    );
    expect(sent(bff, 'POST', /\/advance$/)).toHaveLength(2);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('takes the header count and the menu counts from the backend facets', async () => {
    const user = userEvent.setup();
    bff.facets = {
      ...emptyFacets,
      andamento: [
        { label: 'Novo sinal', count: 4 },
        { label: 'Adiados', count: 1 },
        { label: 'Aberto', count: 0 },
        { label: 'Em atendimento', count: 1 },
        { label: 'Aguardando cliente', count: 1 },
        { label: 'Resolvido', count: 2 },
      ],
    };
    renderQueue();
    await boardReady();

    // The default lists carry no resolved case; the count is the facet's.
    expect(screen.queryByTestId(`case-${KID.patricia}`)).not.toBeInTheDocument();
    expect(screen.getByText('resolvidos').closest('span')).toHaveTextContent('2 resolvidos');

    await user.click(screen.getByRole('button', { name: 'Andamento' }));
    const menu = screen.getByRole('group', { name: 'Andamento' });
    expect(within(menu).getByRole('button', { name: /Resolvido/ })).toHaveTextContent('Resolvido2');
    expect(within(menu).getByRole('button', { name: /Adiados/ })).toHaveTextContent('Adiados1');
  });

  it('closes the open filter menu on Esc and returns focus to its button', async () => {
    const user = userEvent.setup();
    renderQueue();
    await boardReady();

    const trigger = screen.getByRole('button', { name: 'SLA' });
    await user.click(trigger);
    const option = within(screen.getByRole('group', { name: 'SLA' })).getByRole('button', { name: /Vencido/ });
    option.focus();

    await user.keyboard('{Escape}');

    expect(screen.queryByRole('group', { name: 'SLA' })).not.toBeInTheDocument();
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(trigger).toHaveFocus();
  });

  it('shows the case history, oldest first, from the case card', async () => {
    const user = userEvent.setup();
    bff.cases = bff.cases.map((c) =>
      c.id === KID.paulo
        ? {
            ...c,
            history: [
              { ago: 25, kind: 'open', text: 'Caso aberto a partir de mensagem com reclamação' },
              { ago: 10, kind: 'state', text: 'Caso passou para Em atendimento' },
              { ago: 0, kind: 'message', text: 'Uma cobrança que não reconheço' },
            ],
          }
        : c,
    );
    renderQueue();
    await waitFor(() => expect(screen.getByTestId(`case-${KID.paulo}`)).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'Abrir caso 018F2C1A-7B3E de Paulo Henrique Souza' }));

    const dialog = screen.getByRole('dialog', { name: 'Paulo Henrique Souza' });
    expect(within(dialog).getByRole('heading', { name: 'Histórico (3)' })).toBeInTheDocument();
    const lines = within(within(dialog).getByTestId('case-history')).getAllByRole('listitem');
    expect(lines.map((li) => li.textContent)).toEqual([
      'Caso aberto a partir de mensagem com reclamaçãohá 25 min',
      'Caso passou para Em atendimentohá 10 min',
      'Uma cobrança que não reconheçoagora',
    ]);
  });

  it('says so when a case has no history', async () => {
    const user = userEvent.setup();
    renderQueue();
    await waitFor(() => expect(screen.getByTestId(`case-${KID.ana}`)).toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Abrir caso 018F2C1A-7B3E de Ana Beatriz Oliveira' }));
    const dialog = screen.getByRole('dialog', { name: 'Ana Beatriz Oliveira' });
    expect(within(dialog).getByRole('heading', { name: 'Histórico (0)' })).toBeInTheDocument();
    expect(within(dialog).getByText('Nenhum registro neste caso.')).toBeInTheDocument();
  });

  it('shows the SLA state and the countdown the BFF sends, not slaTotal minus openedAgo', async () => {
    bff.cases = [
      // Opened 20 h ago; cases says 3 h 20 left and already due soon.
      { ...seedCases[0]!, openedAgo: 1200, slaTotal: 240, slaRemaining: 200, slaState: 'Vencendo' },
      { ...seedCases[1]!, slaRemaining: -5, slaState: 'Vencido' },
    ];
    renderQueue();
    await waitFor(() => expect(screen.getByTestId(`case-${KID.paulo}`)).toBeInTheDocument());

    const due = screen.getByTestId(`case-${KID.paulo}`);
    expect(due).toHaveTextContent('03:20:00');
    expect(due).toHaveTextContent('SLA Vencendo');
    expect(due).toHaveClass('case-card--due');
    const over = screen.getByTestId(`case-${KID.ana}`);
    expect(over).toHaveTextContent('−00:05:00');
    expect(over).toHaveTextContent('SLA Vencido');
    expect(over).toHaveClass('case-card--over');
  });

  it('shows the short protocol instead of the case UUID on the card and in the detail', async () => {
    const user = userEvent.setup();
    renderQueue();
    const card = await screen.findByTestId(`case-${KID.paulo}`);
    expect(card.querySelector('.case-card__id')).toHaveTextContent(/^018F2C1A-7B3E$/);
    expect(card).not.toHaveTextContent(KID.paulo);

    await user.click(screen.getByRole('button', { name: 'Abrir caso 018F2C1A-7B3E de Paulo Henrique Souza' }));

    const detail = within(screen.getByRole('dialog', { name: 'Paulo Henrique Souza' })).getByTestId('case-detail');
    expect(detail).toHaveTextContent('Caso 018F2C1A-7B3E');
    expect(detail).not.toHaveTextContent(KID.paulo);
  });

  it('explains a halved SLA on the card and in the detail, and says nothing otherwise', async () => {
    const user = userEvent.setup();
    const why = 'Risco de saída, frustração, pedido de atendimento humano ou saque relevante reduzem o prazo à metade.';
    bff.cases = [
      { ...seedCases[0]!, segment: 'Singular', slaTotal: 30, slaRemaining: 24, slaHalved: true },
      { ...seedCases[1]!, slaHalved: false },
      { ...seedCases[0]!, id: NEW_CASE, name: 'Rafael Monteiro' },
    ];
    renderQueue();
    const halved = await screen.findByTestId(`case-${KID.paulo}`);

    const note = within(halved).getByTestId('sla-halved');
    expect(note).toHaveTextContent(/^Prazo prioritário: metade do SLA Singular$/);
    expect(note).toHaveAttribute('title', why);
    // Neither `false` nor a missing field shows the note.
    expect(within(screen.getByTestId(`case-${KID.ana}`)).queryByTestId('sla-halved')).not.toBeInTheDocument();
    expect(within(screen.getByTestId(`case-${NEW_CASE}`)).queryByTestId('sla-halved')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Abrir caso 018F2C1A-7B3E de Paulo Henrique Souza' }));
    const dialog = screen.getByRole('dialog', { name: 'Paulo Henrique Souza' });
    expect(within(dialog).getByTestId('sla-halved')).toHaveTextContent(`Prazo prioritário: metade do SLA Singular. ${why}`);

    await user.click(within(dialog).getByRole('button', { name: 'Fechar' }));
    await user.click(screen.getByRole('button', { name: 'Abrir caso 018F2C1A-7B3E de Ana Beatriz Oliveira' }));
    expect(
      within(screen.getByRole('dialog', { name: 'Ana Beatriz Oliveira' })).queryByTestId('sla-halved'),
    ).not.toBeInTheDocument();
  });

  it('requests motivo=Contato alone when Contato is picked beside Sem contato há muito tempo', async () => {
    const user = userEvent.setup();
    bff.facets = {
      ...emptyFacets,
      motivo: [
        { label: 'Sem contato há muito tempo', count: 1 },
        { label: 'Contato', count: 2 },
      ],
    };
    renderQueue();
    await boardReady();

    await user.click(screen.getByRole('button', { name: 'Motivo' }));
    const menu = screen.getByRole('group', { name: 'Motivo' });
    // Each option has an exact name, so "Contato" is not a substring match.
    expect(within(menu).getAllByRole('button').map((b) => b.getAttribute('aria-label'))).toEqual([
      'Sem contato há muito tempo (1)',
      'Contato (2)',
    ]);
    await user.click(within(menu).getByRole('button', { name: 'Contato (2)' }));

    await waitFor(() => expect(sent(bff, 'GET', /\/v1\/queue\?/)).toHaveLength(1));
    for (const list of ['queue', 'cases']) {
      const filtered = sent(bff, 'GET', new RegExp(`/v1/${list}\\?`)).map((r) => r.url);
      expect(filtered).toHaveLength(1);
      expect(filtered[0]).toMatch(new RegExp(`/v1/${list}\\?motivo=Contato$`));
    }
    expect(within(menu).getByRole('button', { name: 'Contato (2)' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(menu).getByRole('button', { name: 'Sem contato há muito tempo (1)' })).toHaveAttribute('aria-pressed', 'false');
  });

  it('lists snoozed signals under Adiados and reactivates one after a reload', async () => {
    const user = userEvent.setup();
    bff.signals = bff.signals.map((s) =>
      s.id === SID.sergioAlert ? { ...s, snoozed_until: '2026-11-13T18:30:00Z' } : s,
    );
    renderQueue();
    await boardReady();
    expect(screen.queryByTestId(`signal-${SID.sergioAlert}`)).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Andamento' }));
    await user.click(within(screen.getByRole('group', { name: 'Andamento' })).getByRole('button', { name: /Adiados/ }));

    const card = await screen.findByTestId(`signal-${SID.sergioAlert}`);
    expect(card).toHaveTextContent(/Adiado até \d{2}:\d{2}/);
    expect(within(card).queryByRole('button', { name: 'Adiar 1 h' })).not.toBeInTheDocument();
    expect(screen.queryByTestId(`signal-${SID.marianaMsg}`)).not.toBeInTheDocument();

    await user.click(within(card).getByRole('button', { name: 'Reativar' }));

    await waitFor(() => expect(screen.queryByTestId(`signal-${SID.sergioAlert}`)).not.toBeInTheDocument());
    expect(sent(bff, 'DELETE', new RegExp(`/v1/actions/${SID.sergioAlert}$`))).toHaveLength(1);
    // Back on the default board it is a new signal again.
    await user.click(screen.getByRole('button', { name: 'Limpar filtros' }));
    const back = await screen.findByTestId(`signal-${SID.sergioAlert}`);
    expect(within(back).getByRole('button', { name: 'Adiar 1 h' })).toBeInTheDocument();
  });

  it('unmarks a contacted signal from its card after a reload', async () => {
    const user = userEvent.setup();
    bff.signals = bff.signals.map((s) =>
      s.id === SID.pauloMsg ? { ...s, contacted_at: '2026-11-13T15:00:00Z' } : s,
    );
    renderQueue();
    await boardReady();

    const card = screen.getByTestId(`signal-${SID.pauloMsg}`);
    expect(within(card).queryByRole('button', { name: 'Contatado' })).not.toBeInTheDocument();
    await user.click(within(card).getByRole('button', { name: 'Desfazer contato' }));

    await waitFor(() =>
      expect(
        within(screen.getByTestId(`signal-${SID.pauloMsg}`)).getByRole('button', { name: 'Contatado' }),
      ).toBeInTheDocument(),
    );
    expect(sent(bff, 'DELETE', new RegExp(`/v1/actions/${SID.pauloMsg}$`))).toHaveLength(1);
    expect(screen.getByTestId(`signal-${SID.pauloMsg}`).querySelector('.signal-card__contacted')).toBeNull();
  });

  it('offers a retry when undoing an action fails', async () => {
    const user = userEvent.setup();
    bff.signals = bff.signals.map((s) =>
      s.id === SID.pauloMsg ? { ...s, contacted_at: '2026-11-13T15:00:00Z' } : s,
    );
    renderQueue();
    await boardReady();
    const fetchMock = vi.mocked(fetch);
    const real = fetchMock.getMockImplementation();
    fetchMock.mockImplementationOnce(async () => new Response('bad gateway', { status: 502 }));

    await user.click(
      within(screen.getByTestId(`signal-${SID.pauloMsg}`)).getByRole('button', { name: 'Desfazer contato' }),
    );
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Não foi possível desfazer. Nada foi alterado.'),
    );
    expect(real).toBeDefined();
    expect(
      within(screen.getByTestId(`signal-${SID.pauloMsg}`)).getByRole('button', { name: 'Desfazer contato' }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Tentar novamente' }));
    await waitFor(() =>
      expect(
        within(screen.getByTestId(`signal-${SID.pauloMsg}`)).getByRole('button', { name: 'Contatado' }),
      ).toBeInTheDocument(),
    );
  });

  it.each([
    { signals: 1, resolved: 1, text: '1 sinal·1 resolvido' },
    { signals: 2, resolved: 0, text: '2 sinais·0 resolvidos' },
    { signals: 0, resolved: 2, text: '0 sinais·2 resolvidos' },
  ])('writes the header counters in the matching number: $text', async ({ signals, resolved, text }) => {
    bff.signals = bff.signals.slice(0, signals);
    bff.facets = { ...emptyFacets, andamento: [{ label: 'Resolvido', count: resolved }] };
    renderQueue();
    await waitFor(() => expect(screen.getByTestId(`case-${KID.paulo}`)).toBeInTheDocument());

    const stats = document.querySelector('.queue-stats') as HTMLElement;
    const counters = Array.from(stats.querySelectorAll('span'))
      .filter((span) => span.querySelector('strong'))
      .map((span) => span.textContent);
    expect(counters.join('·')).toBe(text);
  });

  it('counts the SLA menu from the cards on the board, not from a facet read at another instant', async () => {
    const user = userEvent.setup();
    // The facet was counted before Sérgio's deadline passed; his card arrives overdue.
    bff.facets = {
      ...emptyFacets,
      sla: [
        { label: 'No prazo', count: 5 },
        { label: 'Vencendo', count: 1 },
        { label: 'Vencido', count: 0 },
      ],
    };
    bff.signals = bff.signals.map((s) => (s.id === SID.sergioAlert ? { ...s, ago: 45 } : s));
    bff.cases = bff.cases.map((c) => (c.id === KID.ana ? { ...c, slaRemaining: 12, slaState: 'Vencendo' } : c));
    renderQueue();
    await boardReady();

    await user.click(screen.getByRole('button', { name: 'SLA' }));
    const menu = screen.getByRole('group', { name: 'SLA' });
    const counts = within(menu).getAllByRole('button').map((b) => b.getAttribute('aria-label'));
    const over = document.querySelectorAll('.signal-card__sla--over').length;
    expect(over).toBe(1);
    // Four signals and two open cases: three on time, Ana's case due, Sérgio overdue.
    expect(counts).toEqual(['No prazo (4)', 'Vencendo (1)', 'Vencido (1)']);
  });

  it('says how many resolved cases match a search that finds nothing, and shows them on request', async () => {
    const user = userEvent.setup();
    bff.signals = [];
    bff.cases = bff.cases.filter((c) => c.id === KID.patricia);
    bff.facets = { ...emptyFacets, andamento: [{ label: 'Resolvido', count: 1 }] };
    renderQueue();
    await waitFor(() => expect(screen.getByText('Fila em dia')).toBeInTheDocument());
    // Nothing is searched yet: no hint.
    expect(screen.queryByRole('button', { name: 'Mostrar resolvidos' })).not.toBeInTheDocument();

    await user.type(screen.getByRole('textbox', { name: 'Buscar cliente' }), 'Patricia');
    expect(await screen.findByText('1 caso resolvido corresponde a esta busca.')).toBeInTheDocument();
    expect(screen.queryByTestId(`case-${KID.patricia}`)).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Mostrar resolvidos' }));
    expect(await screen.findByTestId(`case-${KID.patricia}`)).toBeInTheDocument();
    expect(screen.queryByText('1 caso resolvido corresponde a esta busca.')).not.toBeInTheDocument();
    expect(sent(bff, 'GET', /\/v1\/cases\?/).at(-1)?.url).toMatch(/q=Patricia&andamento=Resolvido$/);
  });

  it('pluralizes the resolved-match hint', async () => {
    const user = userEvent.setup();
    bff.signals = [];
    bff.cases = [];
    bff.facets = { ...emptyFacets, andamento: [{ label: 'Resolvido', count: 3 }] };
    renderQueue();
    await waitFor(() => expect(screen.getByText('Fila em dia')).toBeInTheDocument());

    await user.type(screen.getByRole('textbox', { name: 'Buscar cliente' }), 'a');
    expect(await screen.findByText('3 casos resolvidos correspondem a esta busca.')).toBeInTheDocument();
  });
});
