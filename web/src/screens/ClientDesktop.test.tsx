import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { ROUTER_BASENAME } from '../test/fixtures';
import { fernandaPerfil } from '../test/perfilFixtures';
import { fernandaPhase2, marianaCarteira, marianaPhase2, SEED, thiagoCarteira, thiagoInvestir, thiagoPhase2 } from '../test/sduiFixtures';
import type { Screen } from '../sdui/types';

const XRAY_KEY = 'advisor-radar.pov-xray';

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

/** The app's useWide query answers `wide`. */
function useLayout(wide: boolean) {
  window.matchMedia = (query: string) =>
    ({
      matches: wide && query === '(min-width: 900px)',
      media: query,
      addEventListener: () => undefined,
      removeListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      dispatchEvent: () => false,
      onchange: null,
    }) as MediaQueryList;
}

/** A BFF that answers `screens` by slug and the phase-2 customer read with `phase2`. */
function stubBFF(screens: Record<string, () => Screen>, phase2: () => unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      const slug = /\/screens\/(\w+)$/.exec(url)?.[1];
      const build = slug ? screens[slug] : undefined;
      if (slug) {
        return build ? json(build()) : json({ error: 'not_found' }, 404);
      }
      return json(phase2());
    }),
  );
}

function renderAt(path: string) {
  return render(
    <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[`${ROUTER_BASENAME}${path}`]}>
      <App />
    </MemoryRouter>,
  );
}

/** Each rendered section as [id, span], in grid order. */
function spans(): [string | undefined, string | null][] {
  return Array.from(document.querySelectorAll<HTMLElement>('.sdui-grid > .sdui-section')).map((node) => [node.dataset.section, node.getAttribute('data-span')]);
}

beforeEach(() => {
  localStorage.removeItem(XRAY_KEY);
});

afterEach(() => {
  localStorage.removeItem(XRAY_KEY);
  window.matchMedia = undefined as unknown as typeof window.matchMedia;
  vi.unstubAllGlobals();
});

describe('Investir on desktop (Investir-Desktop)', () => {
  it('spans cash and the rail across the grid, lays the rail in two columns, and the lists one each', async () => {
    useLayout(true);
    stubBFF({ investir: thiagoInvestir }, thiagoPhase2);
    renderAt(`/client-pov/${SEED.thiago}/investir`);
    const rail = await screen.findByRole('region', { name: 'Para o seu perfil arrojado' });
    expect(document.querySelector('.pov-app')).toHaveAttribute('data-layout', 'desktop');
    expect(spans()).toEqual([
      ['cash', '2'],
      ['highlights', '2'],
      ['fixed_income', '1'],
      ['etfs', '1'],
      ['stocks', '1'],
    ]);
    const grid = rail.querySelector('.sdui-rail__grid');
    expect(grid).toHaveAttribute('data-columns', '2');
    expect(within(grid as HTMLElement).getAllByRole('article')).toHaveLength(2);
  });

  it('keeps the rail in one column on the phone', async () => {
    useLayout(false);
    stubBFF({ investir: thiagoInvestir }, thiagoPhase2);
    renderAt(`/client-pov/${SEED.thiago}/investir`);
    const rail = await screen.findByRole('region', { name: 'Para o seu perfil arrojado' });
    expect(document.querySelector('.pov-app')).toHaveAttribute('data-layout', 'phone');
    expect(rail.querySelector('.sdui-rail__grid')).toHaveAttribute('data-columns', '1');
  });
});

describe('Carteira on desktop (Carteira-Desktop)', () => {
  it('lays every section one column wide, in the order walletSections draws them', async () => {
    useLayout(true);
    stubBFF({ carteira: marianaCarteira }, marianaPhase2);
    renderAt(`/client-pov/${SEED.mariana}/carteira`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Carteira' })).toBeInTheDocument();
    expect(document.querySelector('.pov-app')).toHaveAttribute('data-layout', 'desktop');
    expect(screen.getByText(/vendo como Mariana Costa/)).toBeInTheDocument();
    expect(spans()).toEqual([
      ['summary', '1'],
      ['allocation', '1'],
      ['positions_stocks', '1'],
      ['positions_etf', '1'],
      ['positions_fixed_income', '1'],
      ['history', '1'],
    ]);
  });

  it('leaves no slot for an omitted middle section: the next section takes its place', async () => {
    useLayout(true);
    const withoutStocks = (): Screen => {
      const envelope = marianaCarteira();
      return {
        ...envelope,
        sections: envelope.sections.filter((section) => section.id !== 'positions_stocks'),
        omitted: [{ id: 'positions_stocks', type: 'position_list', reason: 'catalog' }],
      };
    };
    stubBFF({ carteira: withoutStocks }, marianaPhase2);
    renderAt(`/client-pov/${SEED.mariana}/carteira`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Carteira' })).toBeInTheDocument();
    expect(spans()).toEqual([
      ['summary', '1'],
      ['allocation', '1'],
      ['positions_etf', '1'],
      ['positions_fixed_income', '1'],
      ['history', '1'],
    ]);
    expect(document.querySelector('[data-section="positions_stocks"]')).toBeNull();
  });

  it('lays Carteira in the one-column phone grid, every section span 1', async () => {
    useLayout(false);
    stubBFF({ carteira: marianaCarteira }, marianaPhase2);
    renderAt(`/client-pov/${SEED.mariana}/carteira`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Carteira' })).toBeInTheDocument();
    expect(document.querySelector('.pov-app')).toHaveAttribute('data-layout', 'phone');
    expect(spans()).toEqual([
      ['summary', '1'],
      ['allocation', '1'],
      ['positions_stocks', '1'],
      ['positions_etf', '1'],
      ['positions_fixed_income', '1'],
      ['history', '1'],
    ]);
  });

  it('outlines each section in the desktop grid with Raio-X, omitted ones as placeholders with their span', async () => {
    const user = userEvent.setup();
    useLayout(true);
    const omitted = (): Screen => {
      const envelope = thiagoCarteira();
      return {
        ...envelope,
        sections: envelope.sections.filter((section) => section.id !== 'history'),
        omitted: [{ id: 'history', type: 'activity_list', reason: 'timeline' }],
      };
    };
    stubBFF({ carteira: omitted }, thiagoPhase2);
    renderAt(`/client-pov/${SEED.thiago}/carteira`);
    await user.click(await screen.findByRole('button', { name: 'Raio-X SDUI' }));
    expect(document.querySelector('.sdui-screen')).toHaveAttribute('data-xray', 'on');
    expect(spans()).toEqual([
      ['summary', '1'],
      ['allocation', '1'],
      ['positions_stocks', '1'],
      ['positions_etf', '1'],
      ['history', '1'],
    ]);
    expect(document.querySelector('.sdui-section--omitted')).toHaveTextContent('history · activity_list · omitido');
  });
});

describe('Perfil on desktop (Perfil-Desktop)', () => {
  it('spans header, suitability, and advisor, lays the levels in three columns, and pairs registration with preferences', async () => {
    useLayout(true);
    stubBFF({ perfil: () => fernandaPerfil() }, fernandaPhase2);
    renderAt(`/client-pov/${SEED.fernanda}/perfil`);
    const scale = await screen.findByRole('region', { name: 'Seu perfil de investidor' });
    expect(document.querySelector('.pov-app')).toHaveAttribute('data-layout', 'desktop');
    expect(spans()).toEqual([
      ['header', '2'],
      ['suitability', '2'],
      ['registration', '1'],
      ['preferences', '1'],
      ['advisor', '2'],
    ]);
    const levels = within(scale).getByRole('list');
    expect(levels).toHaveAttribute('data-columns', '3');
    expect(within(levels).getAllByRole('listitem')).toHaveLength(3);
  });

  it('keeps the levels in one column on the phone', async () => {
    useLayout(false);
    stubBFF({ perfil: () => fernandaPerfil() }, fernandaPhase2);
    renderAt(`/client-pov/${SEED.fernanda}/perfil`);
    const scale = await screen.findByRole('region', { name: 'Seu perfil de investidor' });
    expect(document.querySelector('.pov-app')).toHaveAttribute('data-layout', 'phone');
    expect(within(scale).getByRole('list')).toHaveAttribute('data-columns', '1');
  });
});
