import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { App } from '../App';
import { apiPath } from '../api/base';
import type { Component, Screen } from '../sdui/types';
import { JSON_PREVIEW_LINES, ROSTER_TIMEOUT_MS } from '../selection/SduiShowcase';
import { SDUI_PRINCIPLES, SDUI_STEPS } from '../selection/sdui';
import { WALK_INTERVAL_MS, WALK_STEPS } from '../selection/steps';
import { ROUTER_BASENAME } from '../test/fixtures';
import { fernandaHome, marianaHome, SEED, thiagoHome } from '../test/sduiFixtures';

function installMedia(matches: Record<string, boolean>) {
  window.matchMedia = (query: string) =>
    ({
      matches: matches[query] ?? false,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }) as MediaQueryList;
}

function renderAt(entry: string) {
  return render(
    <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[entry]}>
      <App />
    </MemoryRouter>,
  );
}

describe('selection screen', () => {
  beforeEach(() => {
    installMedia({
      '(min-width: 900px)': false,
      '(prefers-reduced-motion: reduce)': false,
    });
    // The #sdui section asks the BFF for live screens; these cases run with it down.
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new TypeError('offline'))));
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('renders selection at /advisor-radar instead of the queue', () => {
    renderAt(ROUTER_BASENAME);
    expect(
      screen.getByRole('heading', { level: 1, name: 'O mesmo sinal, visto dos dois lados da conversa.' }),
    ).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Fila' })).not.toBeInTheDocument();
  });

  it('opens the team queue and the client list from the two cards', async () => {
    const user = userEvent.setup();
    renderAt(ROUTER_BASENAME);

    expect(screen.getByRole('link', { name: /Visão do time/ })).toHaveAttribute('href', '/advisor-radar/fila');
    expect(screen.getByRole('link', { name: /Visão do cliente/ })).toHaveAttribute(
      'href',
      '/advisor-radar/client-pov',
    );

    vi.stubGlobal(
      'EventSource',
      class {
        addEventListener() {}
        close() {}
      },
    );
    await user.click(screen.getByRole('link', { name: /Visão do time/ }));
    expect(screen.getByRole('navigation', { name: 'Persona' })).toBeInTheDocument();
  });

  it('states the three pillars and all eight walkthrough steps', () => {
    renderAt(`${ROUTER_BASENAME}/`);
    expect(screen.getByRole('heading', { name: 'Alertas proativos' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Triagem de mensagens' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Casos com SLA' })).toBeInTheDocument();
    expect(screen.getByText('Saque relevante, queda de patrimônio, aporte grande, mudança de segmento, cliente sem contato e compra acima do perfil.')).toBeInTheDocument();
    for (const step of WALK_STEPS) {
      expect(screen.getByRole('button', { name: new RegExp(step.title) })).toBeInTheDocument();
    }
    expect(screen.getByText('POST /v1/client-pov/customers/{id}/complaints')).toBeInTheDocument();
    expect(screen.getByText(/dados, nomes e marcas fictícios/)).toBeInTheDocument();
  });

  it('advances the walkthrough every 2.6s and pauses', () => {
    vi.useFakeTimers();
    renderAt(ROUTER_BASENAME);

    expect(screen.getByRole('button', { name: /Passo 1:/ })).toHaveAttribute('aria-current', 'step');

    act(() => {
      vi.advanceTimersByTime(WALK_INTERVAL_MS);
    });
    expect(screen.getByRole('button', { name: /Passo 2:/ })).toHaveAttribute('aria-current', 'step');

    act(() => {
      screen.getByRole('button', { name: 'Pausar' }).click();
    });
    act(() => {
      vi.advanceTimersByTime(WALK_INTERVAL_MS * 3);
    });
    expect(screen.getByRole('button', { name: /Passo 2:/ })).toHaveAttribute('aria-current', 'step');
    expect(screen.getByRole('button', { name: 'Reproduzir' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('does not move when reduced motion is requested', () => {
    vi.useFakeTimers();
    installMedia({
      '(min-width: 900px)': false,
      '(prefers-reduced-motion: reduce)': true,
    });
    renderAt(ROUTER_BASENAME);
    expect(screen.getByRole('button', { name: 'Reproduzir' })).toHaveAttribute('aria-pressed', 'true');
    act(() => {
      vi.advanceTimersByTime(WALK_INTERVAL_MS * 4);
    });
    expect(screen.getByRole('button', { name: /Passo 1:/ })).toHaveAttribute('aria-current', 'step');
  });

  it('shows the desktop cards, graph, and step jumpers', async () => {
    installMedia({
      '(min-width: 900px)': true,
      '(prefers-reduced-motion: reduce)': false,
    });
    const user = userEvent.setup();
    renderAt(ROUTER_BASENAME);

    expect(screen.getByText('Entrar como time')).toBeInTheDocument();
    expect(screen.getByText('Escolher cliente')).toBeInTheDocument();
    expect(screen.getByText('Fila do assessor')).toBeInTheDocument();
    expect(screen.getByText('Visão 360 do cliente')).toBeInTheDocument();
    expect(screen.getByText('Saque e wire-out')).toBeInTheDocument();
    expect(screen.getByText('Reclamações prontas')).toBeInTheDocument();
    expect(screen.getByText('Como uma reclamação chega à fila do assessor')).toBeInTheDocument();
    expect(screen.getByText('Docker Compose')).toBeInTheDocument();
    expect(screen.getByText(/Nenhuma mensagem é enviada a clientes de verdade/)).toBeInTheDocument();
    expect(screen.getByText('App e fila').closest('article')).toHaveClass('on');
    expect(screen.queryByRole('list', { name: 'Passos da reclamação' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Passo 4:/ }));
    expect(document.querySelector('.selection__readout code')).toHaveTextContent('message.received');
    expect(screen.getByText('O relay publica no RabbitMQ')).toBeInTheDocument();
    expect(document.querySelector('.selection__bus')).toHaveClass('on');
    expect(screen.getByRole('button', { name: 'Reproduzir' })).toHaveAttribute('aria-pressed', 'true');
  });
});

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function withMoment(home: Screen, variant: string, title: string): Screen {
  const moment: Component = { type: 'moment_card', variant, props: { kicker: 'K', title, body: 'B', tone: 'info' } };
  return { ...home, sections: [{ id: 'moment', components: [moment] }, ...home.sections.slice(1)] };
}

const ROSTER = [
  { customer_id: SEED.mariana, name: 'Mariana Costa', segment: 'Singular', assets: 1, sla: '1 h', advisor: 'Ana Paula Ribeiro', since: '', hint: '' },
  { customer_id: SEED.fernanda, name: 'Fernanda Lima', segment: 'Essencial', assets: 1, sla: '24 h', advisor: 'Ana Paula Ribeiro', since: '', hint: '' },
  { customer_id: SEED.thiago, name: 'Thiago Azevedo', segment: 'Advance', assets: 1, sla: '4 h', advisor: 'Ana Paula Ribeiro', since: '', hint: '' },
];

const HOMES: Record<string, Screen> = {
  [SEED.fernanda]: withMoment(fernandaHome(), 'segment_upgrade_near', 'Fernanda, faltam US$ 1.800,00 para o Advance'),
  [SEED.thiago]: withMoment(thiagoHome(), 'idle_cash', 'Thiago, 89% do seu patrimônio está em caixa'),
  [SEED.mariana]: withMoment(marianaHome(), 'portfolio_review', 'Mariana, sua revisão de carteira está disponível'),
};

/** Serves the POV list and each seed client's home; `override` replaces a response. */
function stubLiveBFF(override: (url: string) => Response | undefined = () => undefined) {
  const calls: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      calls.push(url);
      const replaced = override(url);
      if (replaced) {
        return replaced;
      }
      if (url.endsWith('/v1/client-pov/customers')) {
        return json({ items: ROSTER });
      }
      const id = /customers\/([^/]+)\/screens\/home$/.exec(url)?.[1] ?? '';
      const home = HOMES[id];
      return home ? json(home) : new Response('not found', { status: 404 });
    }),
  );
  return calls;
}

function sdui(): HTMLElement {
  return document.getElementById('sdui') as HTMLElement;
}

describe('selection screen, phase 3 Server-Driven UI', () => {
  beforeEach(() => {
    installMedia({
      '(min-width: 900px)': false,
      '(prefers-reduced-motion: reduce)': true,
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('adds the header anchors, the Novo band, and the fifth pillar', () => {
    stubLiveBFF();
    renderAt(ROUTER_BASENAME);
    const anchors = screen.getByRole('navigation', { name: 'Nesta página' });
    expect(within(anchors).getByRole('link', { name: 'Arquitetura' })).toHaveAttribute('href', '#arquitetura');
    const sduiLink = within(anchors).getByRole('link', { name: /SDUI/ });
    expect(sduiLink).toHaveAttribute('href', '#sdui');
    expect(within(sduiLink).getByText('Novo')).toBeInTheDocument();

    expect(screen.getByText('Novo nesta versão · Server-Driven UI')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Três clientes, três inícios diferentes, nenhuma linha de front escrita para cada um.' })).toBeInTheDocument();
    expect(screen.getByText('O bff devolve a tela como página, seções e componentes, já preenchida para o momento do cliente. O app só renderiza.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Ver como funciona' })).toHaveAttribute('href', '#sdui');
    const cards = screen.getByRole('region', { name: 'Visões' });
    const band = screen.getByText('Novo nesta versão · Server-Driven UI');
    expect(cards.compareDocumentPosition(band) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    expect(screen.getByRole('heading', { name: 'Telas pelo servidor' })).toBeInTheDocument();
    expect(
      screen.getByText('O app do cliente é server-driven. O backend escolhe a variante de cada seção pelo momento do cliente, e o front só desenha um catálogo fixo de componentes.'),
    ).toBeInTheDocument();
  });

  it('places #sdui after #arquitetura with its heading, steps, and principles', () => {
    stubLiveBFF();
    renderAt(ROUTER_BASENAME);
    const walk = document.getElementById('arquitetura') as HTMLElement;
    expect(within(walk).getByRole('heading', { name: 'Como uma reclamação chega à fila do assessor' })).toBeInTheDocument();
    expect(walk.compareDocumentPosition(sdui()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(sdui()).getByRole('heading', { level: 2, name: 'O backend monta a tela de cada cliente' })).toBeInTheDocument();
    const steps = within(sdui()).getAllByRole('listitem');
    expect(steps).toHaveLength(5);
    SDUI_STEPS.forEach((step, index) => {
      expect(steps[index]).toHaveTextContent(`0${index + 1}${step.title}${step.text}`);
    });
    for (const item of SDUI_PRINCIPLES) {
      expect(within(sdui()).getByRole('heading', { name: item.title })).toBeInTheDocument();
      expect(within(sdui()).getByText(item.text)).toBeInTheDocument();
    }
    expect(within(sdui()).queryByText('errgroup · deadline')).not.toBeInTheDocument();
  });

  it('fetches Thiago’s live home first and shows the response, the moment, its rule, and the rendered home', async () => {
    const calls = stubLiveBFF();
    renderAt(ROUTER_BASENAME);
    const tabs = within(sdui()).getByRole('group', { name: 'Escolha um cliente' });
    expect(within(tabs).getAllByRole('button').map((tab) => tab.textContent)).toEqual(['Fernanda · Essencial', 'Thiago · Advance', 'Mariana · Singular']);
    expect(within(tabs).getByRole('button', { name: 'Thiago · Advance' })).toHaveAttribute('aria-pressed', 'true');

    expect(await within(sdui()).findByText('200 · 1 requisição')).toBeInTheDocument();
    expect(calls.filter((url) => url.endsWith('/screens/home'))).toEqual([apiPath(`v1/client-pov/customers/${SEED.thiago}/screens/home`)]);
    const pre = sdui().querySelector('pre') as HTMLElement;
    expect(pre).toHaveTextContent('"schema_version": 1');
    expect(pre).toHaveTextContent('"variant": "idle_cash"');

    const moment = within(sdui()).getByText('Momento (advisory, gRPC)').parentElement as HTMLElement;
    expect(within(moment).getByText('idle_cash')).toBeInTheDocument();
    const rule = within(sdui()).getByText('A regra que escolheu a variante').parentElement as HTMLElement;
    expect(rule).toHaveTextContent('5 de 7 · idle_cash · advisory');
    expect(rule).toHaveTextContent('Patrimônio acima de zero e caixa igual ou acima de 50% do patrimônio.');

    const phone = within(sdui()).getByRole('region', { name: 'Início de Thiago no app' });
    expect(within(phone).getByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(within(phone).getByText('Thiago, 89% do seu patrimônio está em caixa')).toBeInTheDocument();
    expect(within(phone).getByText('US$ 68.000,00')).toBeInTheDocument();
    expect(within(phone).getByText('TA')).toBeInTheDocument();
    expect(phone.querySelector('.pov-app')).toHaveAttribute('inert');
  });

  it('switches clients with the tabs and fetches the picked client’s home', async () => {
    const user = userEvent.setup();
    const calls = stubLiveBFF();
    renderAt(ROUTER_BASENAME);
    await within(sdui()).findByRole('region', { name: 'Início de Thiago no app' });

    await user.click(within(sdui()).getByRole('button', { name: 'Mariana · Singular' }));
    expect(within(sdui()).getByRole('button', { name: 'Mariana · Singular' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(sdui()).getByRole('button', { name: 'Thiago · Advance' })).toHaveAttribute('aria-pressed', 'false');
    const phone = await within(sdui()).findByRole('region', { name: 'Início de Mariana no app' });
    expect(within(phone).getByRole('heading', { level: 1, name: 'Olá, Mariana' })).toBeInTheDocument();
    expect(within(sdui()).getByText('A regra que escolheu a variante').parentElement).toHaveTextContent('6 de 7 · portfolio_review · advisoryO cliente é Singular.');
    expect(calls).toContain(apiPath(`v1/client-pov/customers/${SEED.mariana}/screens/home`));

    await user.click(within(sdui()).getByRole('button', { name: 'Fernanda · Essencial' }));
    expect(await within(sdui()).findByRole('region', { name: 'Início de Fernanda no app' })).toBeInTheDocument();
    expect(within(sdui()).getByText('A regra que escolheu a variante').parentElement).toHaveTextContent('4 de 7 · segment_upgrade_near · advisory');
    expect(calls.filter((url) => url.endsWith('/screens/home'))).toHaveLength(3);
  });

  it('labels tabs with the segment from the POV list, falling back to the design copy', async () => {
    stubLiveBFF((url) =>
      url.endsWith('/v1/client-pov/customers') ? json({ items: [{ ...ROSTER[1], segment: 'Advance' }, ROSTER[2]] }) : undefined,
    );
    renderAt(ROUTER_BASENAME);
    expect(await within(sdui()).findByRole('button', { name: 'Fernanda · Advance' })).toBeInTheDocument();
    expect(within(sdui()).getByRole('button', { name: 'Mariana · Singular' })).toBeInTheDocument();
  });

  it('says a seed client is missing from the POV list without blaming the BFF', async () => {
    const user = userEvent.setup();
    stubLiveBFF((url) => (url.endsWith('/v1/client-pov/customers') ? json({ items: [ROSTER[2]] }) : undefined));
    renderAt(ROUTER_BASENAME);
    await within(sdui()).findByRole('region', { name: 'Início de Thiago no app' });
    await user.click(within(sdui()).getByRole('button', { name: 'Mariana · Singular' }));
    expect(within(sdui()).getByRole('status')).toHaveTextContent('Esse cliente não está na lista do bff agora.');
    expect(within(sdui()).queryByText(/O bff não respondeu agora/)).not.toBeInTheDocument();
  });

  it('asks for an update, not a BFF outage, when the home schema is not supported', async () => {
    stubLiveBFF((url) =>
      url.endsWith('/screens/home') ? json({ supported: { min: 2, max: 2 } }, 406) : undefined,
    );
    renderAt(ROUTER_BASENAME);
    expect(await within(sdui()).findByText('Atualize o app para ver esta tela.')).toBeInTheDocument();
    expect(within(sdui()).queryByText(/O bff não respondeu agora/)).not.toBeInTheDocument();
    expect(within(sdui()).queryByRole('region', { name: 'Início de Thiago no app' })).not.toBeInTheDocument();
  });

  it('matches seed clients by id and ignores malformed or renamed list items', async () => {
    const items = [
      'junk',
      { customer_id: SEED.mariana, name: 42, segment: { bad: true } },
      { ...ROSTER[2], name: 'Mariana Souza' },
      { ...ROSTER[1], name: 'Thiago Lima' },
    ];
    const calls = stubLiveBFF((url) => (url.endsWith('/v1/client-pov/customers') ? json({ items }) : undefined));
    renderAt(ROUTER_BASENAME);
    const phone = await within(sdui()).findByRole('region', { name: 'Início de Thiago no app' });
    expect(within(phone).getByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(within(phone).getByText('MS')).toBeInTheDocument();
    expect(calls.filter((url) => url.endsWith('/screens/home'))).toEqual([apiPath(`v1/client-pov/customers/${SEED.thiago}/screens/home`)]);
    expect(within(sdui()).getByRole('button', { name: 'Mariana · Singular' })).toBeInTheDocument();
    expect(within(sdui()).getByRole('button', { name: 'Fernanda · Essencial' })).toBeInTheDocument();
  });

  it('asks for the POV list again when a tab is clicked after it failed', async () => {
    const user = userEvent.setup();
    let rosterCalls = 0;
    stubLiveBFF((url) => {
      if (url.endsWith('/v1/client-pov/customers')) {
        rosterCalls += 1;
        return rosterCalls === 1 ? new Response('bad gateway', { status: 502 }) : undefined;
      }
      return undefined;
    });
    renderAt(ROUTER_BASENAME);
    expect(await within(sdui()).findByText(/O bff não respondeu agora/)).toBeInTheDocument();
    await user.click(within(sdui()).getByRole('button', { name: 'Mariana · Singular' }));
    expect(await within(sdui()).findByRole('region', { name: 'Início de Mariana no app' })).toBeInTheDocument();
    expect(rosterCalls).toBe(2);
  });

  it('gives up on a POV list that never answers after the roster timeout', async () => {
    const timer = new AbortController();
    vi.spyOn(AbortSignal, 'timeout').mockReturnValue(timer.signal);
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener('abort', () => reject(init.signal?.reason));
          }),
      ),
    );
    renderAt(ROUTER_BASENAME);
    expect(within(sdui()).getByRole('status')).toHaveTextContent('Buscando no bff a tela de Thiago…');
    expect(AbortSignal.timeout).toHaveBeenCalledWith(ROSTER_TIMEOUT_MS);
    act(() => {
      timer.abort(new DOMException('timed out', 'TimeoutError'));
    });
    expect(await within(sdui()).findByText(/O bff não respondeu agora/)).toBeInTheDocument();
    expect(within(sdui()).queryByText(/Buscando no bff/)).not.toBeInTheDocument();
    vi.restoreAllMocks();
  });

  it('keeps the picked client when an earlier client’s response arrives late', async () => {
    const user = userEvent.setup();
    const held: Record<string, (res: Response) => void> = {};
    stubLiveBFF((url) => {
      const id = /customers\/([^/]+)\/screens\/home$/.exec(url)?.[1];
      if (id === SEED.thiago || id === SEED.mariana) {
        return new Promise<Response>((resolve) => {
          held[id] = resolve;
        }) as unknown as Response;
      }
      return undefined;
    });
    renderAt(ROUTER_BASENAME);
    await waitFor(() => expect(held[SEED.thiago]).toBeDefined());
    await user.click(within(sdui()).getByRole('button', { name: 'Mariana · Singular' }));
    await waitFor(() => expect(held[SEED.mariana]).toBeDefined());
    held[SEED.mariana]?.(json(HOMES[SEED.mariana]));
    const phone = await within(sdui()).findByRole('region', { name: 'Início de Mariana no app' });
    held[SEED.thiago]?.(json(HOMES[SEED.thiago]));
    await new Promise((done) => setTimeout(done, 20));
    expect(within(phone).getByRole('heading', { level: 1, name: 'Olá, Mariana' })).toBeInTheDocument();
    expect(within(sdui()).queryByRole('region', { name: 'Início de Thiago no app' })).not.toBeInTheDocument();
    expect(within(sdui()).getByText('A regra que escolheu a variante').parentElement).toHaveTextContent('6 de 7 · portfolio_review · advisory');
  });

  it('collapses the response again when another client is picked', async () => {
    const user = userEvent.setup();
    stubLiveBFF();
    renderAt(ROUTER_BASENAME);
    await user.click(await within(sdui()).findByRole('button', { name: /Mostrar a resposta inteira/ }));
    expect(within(sdui()).getByRole('button', { name: 'Mostrar menos' })).toHaveAttribute('aria-expanded', 'true');
    await user.click(within(sdui()).getByRole('button', { name: 'Mariana · Singular' }));
    await within(sdui()).findByRole('region', { name: 'Início de Mariana no app' });
    expect(within(sdui()).getByRole('button', { name: /Mostrar a resposta inteira/ })).toHaveAttribute('aria-expanded', 'false');
  });

  it('shows the real HTTP status of the screen response', async () => {
    stubLiveBFF((url) => (url.endsWith(`${SEED.thiago}/screens/home`) ? json(HOMES[SEED.thiago], 203) : undefined));
    renderAt(ROUTER_BASENAME);
    expect(await within(sdui()).findByText('203 · 1 requisição')).toBeInTheDocument();
  });

  it('says when the moment section was omitted, with its reason', async () => {
    const home = thiagoHome();
    const omitted: Screen = { ...home, sections: home.sections.slice(1), omitted: [{ id: 'moment', type: 'moment_card', reason: 'moments' }] };
    stubLiveBFF((url) => (url.endsWith(`${SEED.thiago}/screens/home`) ? json(omitted) : undefined));
    renderAt(ROUTER_BASENAME);
    await within(sdui()).findByText('200 · 1 requisição');
    const moment = within(sdui()).getByText('Momento (advisory, gRPC)').parentElement as HTMLElement;
    expect(moment).toHaveTextContent('omitido');
    expect(within(sdui()).getByText(/A seção moment ficou fora da resposta \(motivo: moments\)/)).toBeInTheDocument();
    expect(within(sdui()).queryByText(/Sem linha documentada/)).not.toBeInTheDocument();
  });

  it('shows a dash for an empty moment variant', async () => {
    stubLiveBFF((url) => (url.endsWith(`${SEED.thiago}/screens/home`) ? json(withMoment(thiagoHome(), '', 'T')) : undefined));
    renderAt(ROUTER_BASENAME);
    await within(sdui()).findByText('200 · 1 requisição');
    const moment = within(sdui()).getByText('Momento (advisory, gRPC)').parentElement as HTMLElement;
    expect(within(moment).getByText('—')).toBeInTheDocument();
    expect(within(sdui()).getByText('Sem linha documentada na tabela de prioridade para esta variante.')).toBeInTheDocument();
  });

  it('shows a neutral note when the BFF is down and keeps the rest of the page', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new TypeError('offline'))));
    renderAt(ROUTER_BASENAME);
    expect(within(sdui()).getByRole('status')).toHaveTextContent('Buscando no bff a tela de Thiago…');
    expect(await within(sdui()).findByText(/O bff não respondeu agora, então a resposta ao vivo não aparece aqui\./)).toBeInTheDocument();
    expect(within(sdui()).queryByText('200 · 1 requisição')).not.toBeInTheDocument();
    expect(within(sdui()).getByRole('button', { name: 'Thiago · Advance' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(sdui()).getAllByRole('listitem')).toHaveLength(5);
    expect(screen.getByRole('heading', { level: 1, name: 'O mesmo sinal, visto dos dois lados da conversa.' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Visão do cliente/ })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Telas pelo servidor' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Como uma reclamação chega à fila do assessor' })).toBeInTheDocument();
  });

  it('shows the note when the screen request fails', async () => {
    stubLiveBFF((url) => (url.endsWith('/screens/home') ? new Response('bad gateway', { status: 502 }) : undefined));
    renderAt(ROUTER_BASENAME);
    expect(await within(sdui()).findByText(/O bff não respondeu agora/)).toBeInTheDocument();
    expect(sdui().querySelector('.sdui-screen')).toBeNull();
  });

  it('trims a long response behind a disclosure', async () => {
    const user = userEvent.setup();
    stubLiveBFF();
    renderAt(ROUTER_BASENAME);
    const more = await within(sdui()).findByRole('button', { name: /Mostrar a resposta inteira \(\d+ linhas\)/ });
    const pre = sdui().querySelector('pre') as HTMLElement;
    expect(more).toHaveAttribute('aria-expanded', 'false');
    expect(more).toHaveAttribute('aria-controls', pre.id);
    expect(pre.textContent?.split('\n')).toHaveLength(JSON_PREVIEW_LINES + 1);
    expect(pre.textContent?.endsWith('…')).toBe(true);
    expect(pre).not.toHaveTextContent('"activity"');

    await user.click(more);
    const less = within(sdui()).getByRole('button', { name: 'Mostrar menos' });
    expect(less).toHaveAttribute('aria-expanded', 'true');
    expect(pre).toHaveTextContent('"id": "activity"');
    expect(pre.textContent).toBe(JSON.stringify(HOMES[SEED.thiago], null, 2));
  });

  it('shows a short response whole', async () => {
    const short = { schema_version: 1, slug: 'home', revision: 'v1', title: 'Olá', sections: [], omitted: [] };
    stubLiveBFF((url) => (url.endsWith('/screens/home') ? json(short) : undefined));
    renderAt(ROUTER_BASENAME);
    expect(await within(sdui()).findByText('200 · 1 requisição')).toBeInTheDocument();
    expect(within(sdui()).queryByRole('button', { name: /Mostrar/ })).not.toBeInTheDocument();
    expect(sdui().querySelector('pre')?.textContent).toBe(JSON.stringify(short, null, 2));
    const moment = within(sdui()).getByText('Momento (advisory, gRPC)').parentElement as HTMLElement;
    expect(moment).toHaveTextContent('—');
    expect(within(sdui()).getByText('Sem linha documentada na tabela de prioridade para esta variante.')).toBeInTheDocument();
  });

  it.each([
    ['portfolio_drop', '1 de 7 · portfolio_drop · advisory'],
    ['case_open', '2 de 7 · case_open · cases'],
    ['segment_upgraded', '3 de 7 · segment_upgraded · advisory'],
    ['welcome', '7 de 7 · welcome · nenhuma fonte'],
  ])('shows the documented rule row of %s', async (variant, row) => {
    stubLiveBFF((url) => (url.endsWith(`${SEED.thiago}/screens/home`) ? json(withMoment(thiagoHome(), variant, 'T')) : undefined));
    renderAt(ROUTER_BASENAME);
    await within(sdui()).findByText('200 · 1 requisição');
    expect(within(sdui()).getByText('A regra que escolheu a variante').parentElement).toHaveTextContent(row);
  });

  it('marks a variant without a documented row', async () => {
    stubLiveBFF((url) => (url.endsWith(`${SEED.thiago}/screens/home`) ? json(withMoment(thiagoHome(), 'hero_moment', 'T')) : undefined));
    renderAt(ROUTER_BASENAME);
    await within(sdui()).findByText('200 · 1 requisição');
    expect(within(sdui()).getByText('hero_moment')).toBeInTheDocument();
    expect(within(sdui()).getByText('Sem linha documentada na tabela de prioridade para esta variante.')).toBeInTheDocument();
  });

  it('lays out the desktop section with the request line, the arrow, and the step tags', async () => {
    installMedia({
      '(min-width: 900px)': true,
      '(prefers-reduced-motion: reduce)': true,
    });
    stubLiveBFF();
    renderAt(ROUTER_BASENAME);
    const anchors = screen.getByRole('navigation', { name: 'Nesta página' });
    expect(within(anchors).getByRole('link', { name: /^Server-Driven UI/ })).toHaveAttribute('href', '#sdui');
    expect(screen.getByText('Demo técnica · dados fictícios')).toBeInTheDocument();
    expect(
      screen.getByText('Regras determinísticas sobre eventos de conta: saque relevante, queda de patrimônio, aporte grande, mudança de segmento, cliente sem contato e compra acima do perfil.'),
    ).toBeInTheDocument();
    expect(within(sdui()).getByText(/O front conhece um catálogo fixo de componentes e só desenha o que recebeu\./)).toBeInTheDocument();
    for (const step of SDUI_STEPS) {
      expect(within(sdui()).getByText(step.tag)).toBeInTheDocument();
    }
    expect(await within(sdui()).findByText('GET /v1/client-pov/customers/{id}/screens/home')).toBeInTheDocument();
    expect(within(sdui()).getByText('O que o app desenha')).toBeInTheDocument();
    expect(within(sdui()).getByText('por type')).toBeInTheDocument();
    await waitFor(() => expect(within(sdui()).getByRole('region', { name: 'Início de Thiago no app' })).toBeInTheDocument());
  });
});
