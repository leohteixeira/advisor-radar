import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { ROUTER_BASENAME } from '../test/fixtures';
import type { Screen } from '../sdui/types';
import { SEED, thiagoCarteira, thiagoCarteiraAfterTudo, thiagoHome, thiagoPhase2 } from '../test/sduiFixtures';

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

/** Thiago's Carteira with the catalog down: no position list, each listed in `omitted`. */
function catalogDown(): Screen {
  const envelope = thiagoCarteira();
  return {
    ...envelope,
    sections: envelope.sections.filter((section) => !section.id.startsWith('positions_')),
    omitted: [
      { id: 'positions_stocks', type: 'position_list', reason: 'catalog' },
      { id: 'positions_etf', type: 'position_list', reason: 'catalog' },
      { id: 'positions_fixed_income', type: 'position_list', reason: 'catalog' },
    ],
  };
}

/**
 * A BFF with Thiago's home and Carteira screens and the phase-2 customer read; it records every GET.
 * `carteira` answers the Carteira request, Thiago's day-0 envelope by default.
 */
function stubBFF(carteira: () => Response = () => json(thiagoCarteira())) {
  const gets: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      gets.push(url);
      if (url.endsWith('/screens/home')) {
        return json(thiagoHome());
      }
      if (url.endsWith('/screens/carteira')) {
        return carteira();
      }
      return json(thiagoPhase2());
    }),
  );
  return gets;
}

afterEach(() => {
  localStorage.removeItem('advisor-radar.pov-xray');
  vi.unstubAllGlobals();
});

describe('Carteira route', () => {
  it('renders the carteira envelope at /client-pov/{id}/carteira', async () => {
    const gets = stubBFF();
    renderAt(`/client-pov/${SEED.thiago}/carteira`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Carteira' })).toBeInTheDocument();
    expect(screen.getByText('Valores de mercado no dia simulado 0')).toBeInTheDocument();
    expect(screen.queryByText(/não entra nesta simulação/)).not.toBeInTheDocument();
    expect(gets.some((url) => url.endsWith(`/customers/${SEED.thiago}/screens/carteira`))).toBe(true);
    expect(gets.some((url) => url.endsWith('/screens/home'))).toBe(false);

    const sections = Array.from(document.querySelectorAll<HTMLElement>('.sdui-section')).map((section) => section.dataset.section);
    expect(sections).toEqual(['summary', 'allocation', 'positions_stocks', 'positions_etf', 'history']);
    expect(screen.getByRole('button', { name: 'Carteira' })).toHaveAttribute('aria-current', 'page');
  });

  it('masks every money value with the eye toggle and shows them again', async () => {
    const user = userEvent.setup();
    stubBFF();
    renderAt(`/client-pov/${SEED.thiago}/carteira`);
    const summary = await screen.findByRole('region', { name: 'Patrimônio total' });
    expect(within(summary).getByText('US$ 68.000,00')).toBeInTheDocument();

    await user.click(within(summary).getByRole('button', { name: 'Esconder valores' }));
    const main = document.querySelector('.sdui-screen') as HTMLElement;
    expect(within(main).queryByText(/US\$ \d/)).not.toBeInTheDocument();
    expect(within(main).getByText('+7,4%')).toBeInTheDocument();

    await user.click(within(summary).getByRole('button', { name: 'Mostrar valores' }));
    expect(within(summary).getByText('US$ 68.000,00')).toBeInTheDocument();
  });

  it('masks the amount of a history row with the eye toggle', async () => {
    const user = userEvent.setup();
    stubBFF(() => json(thiagoCarteiraAfterTudo()));
    renderAt(`/client-pov/${SEED.thiago}/carteira`);
    const history = await screen.findByRole('region', { name: 'Movimentações' });
    const purchase = within(history).getAllByRole('listitem')[0] as HTMLElement;
    expect(within(purchase).getByText('Compra · Maré Ações Globais ETF')).toBeInTheDocument();
    expect(within(purchase).getByText('−US$ 60.520,00')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Esconder valores' }));
    expect(within(purchase).queryByText('−US$ 60.520,00')).not.toBeInTheDocument();
    expect(within(purchase).getByText('US$ ••••••')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Mostrar valores' }));
    expect(within(purchase).getByText('−US$ 60.520,00')).toBeInTheDocument();
  });

  it('draws the omitted Carteira sections in Raio-X', async () => {
    const user = userEvent.setup();
    stubBFF(() => json(catalogDown()));
    renderAt(`/client-pov/${SEED.thiago}/carteira`);
    await screen.findByRole('heading', { level: 1, name: 'Carteira' });
    expect(document.querySelector('.sdui-section--omitted')).toBeNull();

    await user.click(screen.getByRole('button', { name: 'Raio-X' }));
    const placeholders = Array.from(document.querySelectorAll<HTMLElement>('.sdui-section--omitted'));
    expect(placeholders).toHaveLength(3);
    expect(within(placeholders[0] as HTMLElement).getAllByText('positions_stocks · position_list · omitido')).not.toHaveLength(0);
    expect(within(placeholders[1] as HTMLElement).getAllByText('positions_etf · position_list · omitido')).not.toHaveLength(0);
    expect(within(placeholders[2] as HTMLElement).getAllByText('positions_fixed_income · position_list · omitido')).not.toHaveLength(0);
    expect(screen.getByRole('region', { name: 'Raio-X SDUI' })).toHaveTextContent(`customers/${SEED.thiago}/screens/carteira`);
    expect(screen.queryByRole('region', { name: 'Ações' })).not.toBeInTheDocument();
  });

  it('shows the error state when the Carteira request fails and recovers on retry', async () => {
    const user = userEvent.setup();
    const answers = [new Response('bad gateway', { status: 502 })];
    const gets = stubBFF(() => answers.shift() ?? json(thiagoCarteira()));
    renderAt(`/client-pov/${SEED.thiago}/carteira`);
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível montar sua tela.');
    expect(screen.queryByRole('heading', { level: 1, name: 'Carteira' })).not.toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: 'Navegação principal' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Tentar de novo' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Carteira' })).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(gets.filter((url) => url.endsWith('/screens/carteira'))).toHaveLength(2);
  });

  it('opens Carteira from the Início tab', async () => {
    const user = userEvent.setup();
    const gets = stubBFF();
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Carteira' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Carteira' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Movimentações' })).toBeInTheDocument();
    expect(gets.filter((url) => url.endsWith('/screens/carteira'))).toHaveLength(1);
  });
});
