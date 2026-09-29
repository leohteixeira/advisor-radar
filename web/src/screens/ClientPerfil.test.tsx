import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { ROUTER_BASENAME } from '../test/fixtures';
import { BETA_ON, fernandaPerfil } from '../test/perfilFixtures';
import { fernandaPhase2, SEED } from '../test/sduiFixtures';
import type { Preferences } from '../sdui/types';

const THEME_KEY = 'advisor-radar.pov-theme';

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

interface Put {
  url: string;
  headers: Headers;
  body: Preferences;
}

/**
 * A BFF holding Fernanda's preferences: the Perfil screen shows the stored
 * values and the PUT stores them, or answers `fail` without storing.
 */
function stubBFF(fail?: number) {
  let stored: Preferences = { channel: 'chat', beta: false };
  const gets: string[] = [];
  const puts: Put[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === 'PUT') {
        const body = JSON.parse(String(init.body)) as Preferences;
        puts.push({ url, headers: new Headers(init.headers), body });
        if (fail) {
          return json(fail === 422 ? { error: 'invalid' } : {}, fail);
        }
        stored = body;
        return json(stored);
      }
      gets.push(url);
      if (url.endsWith('/screens/perfil')) {
        return json(fernandaPerfil(stored));
      }
      return json(fernandaPhase2());
    }),
  );
  return { gets, puts, perfilReads: () => gets.filter((url) => url.endsWith('/screens/perfil')).length };
}

function renderPerfil() {
  return render(
    <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[`${ROUTER_BASENAME}/client-pov/${SEED.fernanda}/perfil`]}>
      <App />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  window.matchMedia = undefined as unknown as typeof window.matchMedia;
  localStorage.removeItem(THEME_KEY);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  localStorage.removeItem(THEME_KEY);
});

describe('Perfil route', () => {
  it('renders the perfil envelope without the tab note', async () => {
    const { perfilReads } = stubBFF();
    renderPerfil();
    expect(await screen.findByRole('heading', { level: 1, name: 'Perfil' })).toBeInTheDocument();
    expect(screen.getByText('Seus dados, seu perfil de investidor e suas preferências')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Fernanda Lima' })).toHaveTextContent('Conta 3301-7 · Orla Invest');
    expect(screen.getByRole('listitem', { current: true })).toHaveTextContent('Conservador');
    expect(screen.getByRole('region', { name: 'Dados cadastrais' })).toHaveTextContent('fernanda.lima@example.com');
    expect(screen.getByRole('radio', { name: 'Chat' })).toBeChecked();
    expect(screen.queryByText(/não entra nesta simulação/)).toBeNull();
    expect(perfilReads()).toBe(1);
  });

  it('stores the channel and beta, then re-reads the screen', async () => {
    const user = userEvent.setup();
    const { puts, perfilReads } = stubBFF();
    renderPerfil();
    await user.click(await screen.findByRole('radio', { name: 'E-mail' }));
    await waitFor(() => expect(perfilReads()).toBe(2));
    expect(puts).toHaveLength(1);
    expect(puts[0]?.url).toContain(`v1/client-pov/customers/${SEED.fernanda}/preferences`);
    expect(puts[0]?.body).toEqual({ channel: 'email', beta: false });
    expect(puts[0]?.headers.get('Idempotency-Key')).toBeNull();
    expect(puts[0]?.headers.get('Content-Type')).toBe('application/json');
    expect(screen.getByRole('radio', { name: 'E-mail' })).toBeChecked();

    await user.click(screen.getByRole('switch', { name: 'Programa beta' }));
    await waitFor(() => expect(screen.getByRole('switch', { name: 'Programa beta' })).toHaveAccessibleDescription(BETA_ON));
    expect(puts[1]?.body).toEqual({ channel: 'email', beta: true });
    expect(perfilReads()).toBe(3);
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it.each([502, 422, 429])('keeps the previous value when the PUT answers %i', async (status) => {
    const user = userEvent.setup();
    const { puts, perfilReads } = stubBFF(status);
    renderPerfil();
    await user.click(await screen.findByRole('switch', { name: 'Programa beta' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível salvar. Tente de novo.');
    expect(screen.getByRole('switch', { name: 'Programa beta' })).toHaveAttribute('aria-checked', 'false');
    expect(puts).toHaveLength(1);
    expect(perfilReads()).toBe(1);
  });

  it('switches the local theme from the preference list', async () => {
    const user = userEvent.setup();
    const { puts } = stubBFF();
    const { container } = renderPerfil();
    const prefs = await screen.findByRole('region', { name: 'Preferências' });
    await user.click(within(prefs).getByRole('button', { name: 'Tema escuro', pressed: true }));
    expect(container.querySelector('.pov-app')).toHaveClass('pov-app--light');
    expect(localStorage.getItem(THEME_KEY)).toBe('light');
    expect(within(prefs).getByRole('button', { name: 'Tema escuro', pressed: false })).toHaveTextContent('Claro');
    expect(puts).toHaveLength(0);
  });
});
