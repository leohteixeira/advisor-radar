import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { ROUTER_BASENAME } from '../test/fixtures';
import { marianaHome, SEED, thiagoHome, thiagoInvestir, thiagoPhase2 } from '../test/sduiFixtures';
import type { Screen } from './types';

const XRAY_KEY = 'advisor-radar.pov-xray';

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function renderAt(path: string) {
  return render(
    <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[`${ROUTER_BASENAME}${path}`]}>
      <App />
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

function stubBFF(home: () => Screen | Response) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/screens/investir')) {
        return json(thiagoInvestir());
      }
      if (url.includes('/screens/')) {
        const body = home();
        return body instanceof Response ? body : json(body);
      }
      return json(thiagoPhase2());
    }),
  );
}

function timelineDown(): Screen {
  const home = thiagoHome();
  return {
    ...home,
    sections: home.sections.filter((section) => section.id !== 'activity'),
    omitted: [{ id: 'activity', type: 'activity_list', reason: 'timeline' }],
  };
}

beforeEach(() => {
  window.matchMedia = undefined as unknown as typeof window.matchMedia;
  localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  localStorage.clear();
});

describe('Raio-X SDUI toggle in the simulation strip', () => {
  it('is off by default and leaves nothing of Raio-X in the DOM', async () => {
    stubBFF(timelineDown);
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Raio-X' })).toHaveAttribute('aria-pressed', 'false');
    expect(screen.queryByRole('region', { name: 'Raio-X SDUI' })).not.toBeInTheDocument();
    expect(screen.queryByText(/omitido/)).not.toBeInTheDocument();
    expect(document.querySelector('.sdui-xray-label')).toBeNull();
  });

  it('outlines and labels Thiago’s sections, shows the banner, and turns off again', async () => {
    const user = userEvent.setup();
    stubBFF(thiagoHome);
    renderAt(`/client-pov/${SEED.thiago}`);
    const toggle = await screen.findByRole('button', { name: 'Raio-X' });
    await user.click(toggle);
    expect(toggle).toHaveAttribute('aria-pressed', 'true');
    expect(Array.from(document.querySelectorAll('.sdui-xray-label')).map((node) => node.textContent)).toEqual([
      'moment · moment_card · welcome',
      'wealth · wealth_summary · default',
      'actions · action_grid · default',
      'advisor · advisor_card · default',
      'activity · activity_list · recent',
    ]);
    const banner = screen.getByRole('region', { name: 'Raio-X SDUI' });
    expect(banner).toHaveTextContent(`GET /v1/client-pov/customers/${encodeURIComponent(SEED.thiago)}/screens/home`);
    expect(banner).toHaveTextContent('slug home · revision v1 · schema 1 · 5 seções');
    expect(localStorage.getItem(XRAY_KEY)).toBe('on');

    await user.click(toggle);
    expect(toggle).toHaveAttribute('aria-pressed', 'false');
    expect(screen.queryByRole('region', { name: 'Raio-X SDUI' })).not.toBeInTheDocument();
    expect(document.querySelector('.sdui-xray-label')).toBeNull();
    expect(localStorage.getItem(XRAY_KEY)).toBe('off');
  });

  it('draws the omitted activity and says the timeline is down', async () => {
    const user = userEvent.setup();
    stubBFF(timelineDown);
    renderAt(`/client-pov/${SEED.thiago}`);
    await user.click(await screen.findByRole('button', { name: 'Raio-X' }));
    const placeholder = document.querySelector('.sdui-section--omitted') as HTMLElement;
    expect(within(placeholder).getAllByText('activity · activity_list · omitido')).not.toHaveLength(0);
    expect(screen.getByRole('region', { name: 'Raio-X SDUI' })).toHaveTextContent('slug home · revision v1 · schema 1 · 4 seções · timeline fora do ar');
    expect(screen.queryByRole('region', { name: 'Atividade recente' })).not.toBeInTheDocument();
  });

  it('reads "Raio-X SDUI" on desktop and remembers the choice', async () => {
    localStorage.setItem(XRAY_KEY, 'on');
    useDesktop();
    stubBFF(marianaHome);
    renderAt(`/client-pov/${SEED.mariana}`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Mariana' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Raio-X SDUI' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText('advisor · advisor_card · dedicated')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Raio-X SDUI' })).toHaveTextContent(`customers/${SEED.mariana}/screens/home`);
  });

  it('hides the toggle while the screen loads', async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => undefined)));
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(screen.getByRole('status')).toHaveTextContent('Montando sua tela…');
    expect(screen.queryByRole('button', { name: /Raio-X/ })).not.toBeInTheDocument();
  });

  it('hides the toggle on the error state', async () => {
    localStorage.setItem(XRAY_KEY, 'on');
    stubBFF(() => new Response('bad gateway', { status: 502 }));
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível montar sua tela.');
    expect(screen.queryByRole('button', { name: /Raio-X/ })).not.toBeInTheDocument();
  });

  it('labels the Investir sections', async () => {
    localStorage.setItem(XRAY_KEY, 'on');
    stubBFF(thiagoHome);
    renderAt(`/client-pov/${SEED.thiago}/investir`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Raio-X' })).toHaveAttribute('aria-pressed', 'true');
    expect(Array.from(document.querySelectorAll('.sdui-xray-label')).map((node) => node.textContent)).toEqual([
      'cash · invest_summary · default',
      'highlights · product_rail · profile_arrojado',
      'fixed_income · product_list · fixed_income',
      'etfs · product_list · etf',
      'stocks · product_list · stocks',
    ]);
    expect(screen.getByRole('region', { name: 'Raio-X SDUI' })).toHaveTextContent(`customers/${SEED.thiago}/screens/investir`);
  });

  it('hides the toggle and the X-ray on a tab with no SDUI screen', async () => {
    const user = userEvent.setup();
    localStorage.setItem(XRAY_KEY, 'on');
    stubBFF(thiagoHome);
    renderAt(`/client-pov/${SEED.thiago}/carteira`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Raio-X/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Raio-X SDUI' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Início' }));
    expect(screen.getByRole('button', { name: 'Raio-X' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('region', { name: 'Raio-X SDUI' })).toBeInTheDocument();
  });

  it('keeps working in memory when storage is unavailable', async () => {
    const user = userEvent.setup();
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    stubBFF(thiagoHome);
    renderAt(`/client-pov/${SEED.thiago}`);
    const toggle = await screen.findByRole('button', { name: 'Raio-X' });
    await user.click(toggle);
    expect(toggle).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('region', { name: 'Raio-X SDUI' })).toBeInTheDocument();
  });

  it('is not part of the team routes', async () => {
    useDesktop();
    vi.stubGlobal(
      'EventSource',
      class {
        addEventListener() {}
        close() {}
      },
    );
    vi.stubGlobal('fetch', vi.fn(async () => json({ items: [], facets: {} })));
    renderAt('/fila');
    expect(await screen.findByRole('navigation', { name: 'Persona' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Raio-X/ })).not.toBeInTheDocument();
  });
});
