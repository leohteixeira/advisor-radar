import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { ROUTER_BASENAME } from '../test/fixtures';
import { renderSdui } from '../test/sdui';
import { fernandaHome, fernandaHomeV2, SEED, thiagoHome, thiagoPhase2 } from '../test/sduiFixtures';
import { fetchScreen, fetchScreenResponse, InvalidScreenError, SUPPORTED_SCHEMA, UnsupportedSchemaError } from './api';
import { SduiScreen, SduiUpdate } from './SduiScreen';

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function live(): AbortSignal {
  return new AbortController().signal;
}

/** Answers every screen request with `screenResponse` and every other GET with the phase-2 home. */
function stubBFF(screenResponse: () => Response) {
  const inits: (RequestInit | undefined)[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.includes('/screens/')) {
        inits.push(init);
        return screenResponse();
      }
      return json(thiagoPhase2());
    }),
  );
  return inits;
}

/** Replaces window.location with a copy whose reload is a spy. */
function stubReload() {
  const reload = vi.fn();
  vi.stubGlobal('location', { ...window.location, reload });
  return reload;
}

function sectionIDs(container: HTMLElement): (string | null)[] {
  return Array.from(container.querySelectorAll('.sdui-section')).map((node) => node.getAttribute('data-section'));
}

beforeEach(() => {
  window.matchMedia = undefined as unknown as typeof window.matchMedia;
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('X-SDUI-Schema', () => {
  it('supports schema version 1', () => {
    expect(SUPPORTED_SCHEMA).toBe(1);
  });

  it.each([
    ['fetchScreen', () => fetchScreen(SEED.thiago, 'home', live())],
    ['fetchScreenResponse', () => fetchScreenResponse(SEED.thiago, 'home', live())],
  ])('%s sends the supported schema on the request', async (_name, request) => {
    const inits = stubBFF(() => json(thiagoHome()));
    await request();
    expect(inits).toHaveLength(1);
    expect(new Headers(inits[0]?.headers).get('X-SDUI-Schema')).toBe('1');
  });

  it('throws UnsupportedSchemaError on a 406', async () => {
    stubBFF(() => json({ supported: { min: 1, max: 1 } }, 406));
    await expect(fetchScreen(SEED.thiago, 'home', live())).rejects.toBeInstanceOf(UnsupportedSchemaError);
  });

  it.each([
    ['a newer version', { ...thiagoHome(), schema_version: 2 }],
    ['version 0', { ...thiagoHome(), schema_version: 0 }],
    ['a newer version of another shape', { schema_version: 2, screen: { blocks: [] } }],
  ])('throws UnsupportedSchemaError on a 200 envelope of %s', async (_name, body) => {
    stubBFF(() => json(body));
    await expect(fetchScreen(SEED.thiago, 'home', live())).rejects.toBeInstanceOf(UnsupportedSchemaError);
  });

  it('keeps InvalidScreenError for a body without a numeric schema_version', async () => {
    stubBFF(() => json({ ...thiagoHome(), schema_version: '2' }));
    await expect(fetchScreen(SEED.thiago, 'home', live())).rejects.toBeInstanceOf(InvalidScreenError);
  });
});

describe('SduiUpdate', () => {
  it('asks for an update, takes focus, and reloads the page', async () => {
    const user = userEvent.setup();
    const reload = stubReload();
    render(<SduiUpdate />);
    const heading = screen.getByRole('heading', { level: 1, name: 'Atualize o app para ver esta tela.' });
    expect(screen.getByRole('alert')).toContainElement(heading);
    expect(heading).toHaveFocus();
    await user.click(screen.getByRole('button', { name: 'Recarregar' }));
    expect(reload).toHaveBeenCalledTimes(1);
  });
});

describe('client app on an unsupported schema', () => {
  function renderHome() {
    return render(
      <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[`${ROUTER_BASENAME}/client-pov/${SEED.thiago}`]}>
        <App />
      </MemoryRouter>,
    );
  }

  it.each([
    ['a 406', () => json({ supported: { min: 1, max: 1 } }, 406)],
    ['a 200 envelope of schema 2', () => json({ ...thiagoHome(), schema_version: 2 })],
  ])('shows the update notice on %s and keeps the strip and the tabs', async (_name, response) => {
    const user = userEvent.setup();
    const reload = stubReload();
    stubBFF(response);
    renderHome();
    expect(await screen.findByRole('alert')).toHaveTextContent('Atualize o app para ver esta tela.');
    expect(screen.getByText(/vendo como Thiago Azevedo/)).toBeInTheDocument();
    const tabs = screen.getByRole('navigation', { name: 'Navegação principal' });
    expect(within(tabs).getByRole('button', { name: /Investir/ })).toBeInTheDocument();
    expect(document.querySelector('.sdui-screen')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Tentar de novo' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Recarregar' }));
    expect(reload).toHaveBeenCalledTimes(1);
  });
});

describe('home revision v2', () => {
  it('renders the highlights rail right after the moment through the registry', () => {
    const { container } = renderSdui(<SduiScreen screen={fernandaHomeV2()} />);
    expect(sectionIDs(container)).toEqual(['moment', 'highlights', 'wealth', 'actions', 'advisor', 'activity']);
    const rail = container.querySelector('[data-section="highlights"]');
    expect(rail).toHaveAttribute('data-span', '2');
    expect(within(rail as HTMLElement).getByText('Para o seu perfil conservador')).toBeInTheDocument();
    expect(within(rail as HTMLElement).getByText('Orla T-Bill 6 meses')).toBeInTheDocument();
    expect(within(rail as HTMLElement).getByText('Orla Corporate IG 2029')).toBeInTheDocument();
    expect(container.querySelector('.sdui-screen')).toHaveAttribute('data-revision', 'v2');
  });

  it('names the revision in the Raio-X banner', () => {
    renderSdui(<SduiScreen screen={fernandaHomeV2()} xray={{ customerID: SEED.fernanda }} />);
    expect(screen.getByText(/revision v2/)).toBeInTheDocument();
  });

  it('fetches a v2 envelope as served', async () => {
    stubBFF(() => json(fernandaHomeV2()));
    const got = await fetchScreen(SEED.fernanda, 'home', live());
    expect(got).toEqual(fernandaHomeV2());
    expect(got.sections.map((section) => section.id)).toEqual(['moment', 'highlights', ...fernandaHome().sections.slice(1).map((section) => section.id)]);
  });

  it('shows a served v2 home in the client app with its revision in Raio-X', async () => {
    const user = userEvent.setup();
    localStorage.clear();
    stubBFF(() => json(fernandaHomeV2()));
    const { container } = render(
      <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[`${ROUTER_BASENAME}/client-pov/${SEED.fernanda}`]}>
        <App />
      </MemoryRouter>,
    );
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Fernanda' })).toBeInTheDocument();
    expect(sectionIDs(container)).toEqual(['moment', 'highlights', 'wealth', 'actions', 'advisor', 'activity']);
    await user.click(screen.getByRole('button', { name: 'Raio-X' }));
    expect(screen.getByRole('region', { name: 'Raio-X SDUI' })).toHaveTextContent('slug home · revision v2 · schema 1 · 6 seções');
    localStorage.clear();
  });
});
