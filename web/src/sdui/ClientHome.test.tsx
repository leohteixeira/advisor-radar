import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { ROUTER_BASENAME } from '../test/fixtures';
import { SEED, thiagoHome, thiagoIdleCashHome, thiagoInvestir, thiagoPhase2 } from '../test/sduiFixtures';

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function LocationProbe() {
  return <div data-testid="location">{useLocation().pathname}</div>;
}

function renderAt(path: string) {
  return render(
    <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[`${ROUTER_BASENAME}${path}`]}>
      <App />
      <LocationProbe />
    </MemoryRouter>,
  );
}

function useDesktop() {
  window.matchMedia = (query: string) =>
    ({
      matches: query === '(min-width: 900px)',
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
    }) as MediaQueryList;
}

/**
 * Serves the home screen with `screenResponse`, the Investir screen with
 * Thiago's envelope, and every other GET with the phase-2 home.
 */
function stubBFF(screenResponse: (init?: RequestInit) => Response | Promise<Response>) {
  const calls: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push(url);
      if (url.endsWith('/screens/investir')) {
        return json(thiagoInvestir());
      }
      if (url.includes('/screens/')) {
        return screenResponse(init);
      }
      return json(thiagoPhase2());
    }),
  );
  return calls;
}

const idleCashHome = thiagoIdleCashHome;

beforeEach(() => {
  window.matchMedia = undefined as unknown as typeof window.matchMedia;
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('client app home from the screen route', () => {
  it('renders the SDUI home with one screen request', async () => {
    const calls = stubBFF(() => json(thiagoHome()));
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(screen.getByText('Cliente Advance desde 2024')).toBeInTheDocument();
    expect(screen.getByText('US$ 68.000,00')).toBeInTheDocument();
    expect(calls.filter((url) => url.endsWith(`/v1/client-pov/customers/${SEED.thiago}/screens/home`))).toHaveLength(1);
    expect(screen.queryByText('Nenhuma movimentação nos últimos 30 dias.')).not.toBeInTheDocument();
  });

  it('shows the skeleton while the screen is pending', async () => {
    let resolve: (res: Response) => void = () => undefined;
    stubBFF(() => new Promise<Response>((done) => (resolve = done)));
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(screen.getByRole('status')).toHaveTextContent('Montando sua tela…');
    expect(await screen.findByText(/vendo como Thiago Azevedo/)).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Montando sua tela…');
    resolve(json(thiagoHome()));
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(screen.queryByText('Montando sua tela…')).not.toBeInTheDocument();
  });

  it.each([
    ['a 502', () => new Response('bad gateway', { status: 502 })],
    ['a 200 that is not an envelope', () => json(thiagoPhase2())],
    ['an envelope of another slug', () => json(thiagoInvestir())],
  ])('shows the error state on %s and keeps the shell', async (_name, response) => {
    stubBFF(response);
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível montar sua tela.');
    expect(screen.getByRole('button', { name: 'Tentar de novo' })).toBeInTheDocument();
    expect(screen.getByText(/vendo como Thiago Azevedo/)).toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: 'Navegação principal' })).toBeInTheDocument();
    expect(document.querySelector('.sdui-screen')).toBeNull();
    expect(screen.queryByRole('heading', { level: 1, name: 'Olá, Thiago' })).not.toBeInTheDocument();
  });

  it('retries the screen request from the error state', async () => {
    const user = userEvent.setup();
    const answers = [new Response('bad gateway', { status: 502 }), json(thiagoHome())];
    const calls = stubBFF(() => answers.shift() ?? json(thiagoHome()));
    renderAt(`/client-pov/${SEED.thiago}`);
    await user.click(await screen.findByRole('button', { name: 'Tentar de novo' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(calls.filter((url) => url.endsWith('/screens/home'))).toHaveLength(2);
  });

  it('keeps the tabs working from the error state', async () => {
    const user = userEvent.setup();
    stubBFF(() => new Response('bad gateway', { status: 502 }));
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível montar sua tela.');
    await user.click(screen.getByRole('button', { name: 'Perfil' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível montar sua tela.');
    expect(screen.getByRole('status')).toHaveTextContent('Perfil não entra nesta simulação');
  });

  it('opens the coded deposit panel from a panel action', async () => {
    const user = userEvent.setup();
    stubBFF(() => json(thiagoHome()));
    renderAt(`/client-pov/${SEED.thiago}`);
    await user.click(await screen.findByRole('button', { name: 'Depositar' }));
    expect(screen.getByLabelText(/Quanto você quer depositar/)).toBeInTheDocument();
  });

  it('routes a navigate action to its screen route', async () => {
    const user = userEvent.setup();
    const calls = stubBFF(() => json(idleCashHome()));
    renderAt(`/client-pov/${SEED.thiago}`);
    await user.click(await screen.findByRole('button', { name: 'Ver produtos' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent(`/client-pov/${SEED.thiago}/investir`);
    expect(within(screen.getByRole('navigation', { name: 'Navegação principal' })).getByRole('button', { name: 'Investir' })).toHaveAttribute('aria-current', 'page');
    expect(calls.filter((url) => url.endsWith('/screens/investir'))).toHaveLength(1);
    await user.click(screen.getByRole('button', { name: 'Início' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
  });

  it('masks money with the eye toggle', async () => {
    const user = userEvent.setup();
    stubBFF(() => json(thiagoHome()));
    renderAt(`/client-pov/${SEED.thiago}`);
    await user.click(await screen.findByRole('button', { name: 'Esconder valores' }));
    expect(screen.getAllByText('US$ ••••••')).toHaveLength(2);
    expect(screen.queryByText('US$ 68.000,00')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Mostrar valores' }));
    expect(screen.getByText('US$ 68.000,00')).toBeInTheDocument();
  });

  it('shows the error state when the screen request times out', async () => {
    const timer = new AbortController();
    vi.spyOn(AbortSignal, 'timeout').mockReturnValue(timer.signal);
    stubBFF(
      (init) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => reject(init.signal?.reason));
        }),
    );
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(await screen.findByText(/vendo como Thiago Azevedo/)).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Montando sua tela…');
    timer.abort(new DOMException('timed out', 'TimeoutError'));
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível montar sua tela.');
    expect(screen.queryByText('Montando sua tela…')).not.toBeInTheDocument();
  });

  it('fetches the screen again when a panel returns to Início', async () => {
    const user = userEvent.setup();
    const later = thiagoHome();
    (later.sections[1]?.components[0]?.props as { total: string }).total = 'US$ 78.000,00';
    const screens = [thiagoHome(), later];
    const calls = stubBFF(() => json(screens.shift() ?? later));
    renderAt(`/client-pov/${SEED.thiago}`);
    await user.click(await screen.findByRole('button', { name: 'Depositar' }));
    await user.click(screen.getByRole('button', { name: 'Voltar' }));
    expect(await screen.findByText('US$ 78.000,00')).toBeInTheDocument();
    expect(screen.queryByText('US$ 68.000,00')).not.toBeInTheDocument();
    expect(calls.filter((url) => url.endsWith('/screens/home'))).toHaveLength(2);
  });

  it('renders the SDUI home on desktop and closes a panel on navigate', async () => {
    const user = userEvent.setup();
    useDesktop();
    stubBFF(() => json(idleCashHome()));
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(document.querySelector('.pov-app')).toHaveAttribute('data-layout', 'desktop');

    await user.click(screen.getByRole('button', { name: 'Depositar' }));
    expect(screen.getByRole('complementary', { name: 'Ação' })).toHaveTextContent(/Quanto você quer depositar/);
    expect(screen.getByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Ver produtos' }));
    expect(screen.queryByRole('complementary', { name: 'Ação' })).not.toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent(`/client-pov/${SEED.thiago}/investir`);
    const sidebar = screen.getByRole('navigation', { name: 'Navegação principal' });
    expect(within(sidebar).getByRole('button', { name: 'Investir' })).toHaveAttribute('aria-current', 'page');
  });

  it('redirects /home to the canonical home route', async () => {
    stubBFF(() => json(thiagoHome()));
    renderAt(`/client-pov/${SEED.thiago}/home`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent(new RegExp(`^/client-pov/${SEED.thiago}$`));
  });

  it('keeps an id that needs encoding in its routes', async () => {
    const user = userEvent.setup();
    stubBFF(() => json(thiagoHome()));
    renderAt('/client-pov/a%20b/extrato');
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent(/^\/client-pov\/a%20b$/);
    await user.click(screen.getByRole('button', { name: 'Carteira' }));
    expect(screen.getByTestId('location')).toHaveTextContent(/^\/client-pov\/a%20b\/carteira$/);
  });

  it('redirects an unknown tab to the home', async () => {
    stubBFF(() => json(thiagoHome()));
    renderAt(`/client-pov/${SEED.thiago}/extrato`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Início' })).toHaveAttribute('aria-current', 'page'));
  });
});
