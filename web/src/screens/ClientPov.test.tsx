import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { ROUTER_BASENAME } from '../test/fixtures';

const FERNANDA = '01a0e3a4-9a44-757a-ac8f-dab7db5eb068';
const EVENT = '01a0e3a5-2f4c-7b1e-9d3a-6c0b8e41f27a';

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

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('phone client pov', () => {
  it('lists seeded clients and opens one', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/v1/client-pov/customers')) {
          return json({
            items: [
              {
                customer_id: FERNANDA,
                name: 'Fernanda Lima',
                segment: 'Essencial',
                assets: 820000,
                sla: '24 h',
                advisor: 'Ana Paula Ribeiro',
              },
            ],
          });
        }
        return json({
          customer_id: FERNANDA,
          name: 'Fernanda Lima',
          segment: 'Essencial',
          advisor: 'Ana Paula Ribeiro',
          sla: '24 h',
          assets: 820000,
          caixa: 114800,
          allocation: { acoes: 164000, etfs: 369000, renda_fixa: 172200, caixa: 114800 },
          activity: [],
          messages: [],
        });
      }),
    );
    renderAt('/client-pov');
    expect(await screen.findByRole('link', { name: /Fernanda Lima/ })).toBeInTheDocument();
    expect(screen.getByText(/Perto do teto da faixa/)).toBeInTheDocument();
    await user.click(screen.getByRole('link', { name: /Fernanda Lima/ }));
    expect(await screen.findByRole('heading', { name: 'Olá, Fernanda' })).toBeInTheDocument();
    expect(screen.getByText('Nenhuma movimentação nos últimos 30 dias.')).toBeInTheDocument();
  });

  it('shows a list error', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('no', { status: 502 })));
    renderAt('/client-pov');
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível carregar os clientes.');
  });

  it('deposits, withdraws, and files the preset complaint', async () => {
    const user = userEvent.setup();
    const posts: string[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === 'POST') {
          posts.push(String(url));
          if (String(url).includes('/withdrawals') && String(init.body).includes('99999900')) {
            return json({ error: 'insufficient' }, 422);
          }
          return json({ event_id: EVENT });
        }
        return json({
          customer_id: FERNANDA,
          name: 'Fernanda Lima',
          segment: 'Essencial',
          advisor: 'Ana Paula Ribeiro',
          sla: '24 h',
          assets: 820000,
          caixa: 114800,
          allocation: { acoes: 1, etfs: 1, renda_fixa: 1, caixa: 114800 },
          activity: [],
          messages: [],
        });
      }),
    );
    renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByRole('heading', { name: 'Olá, Fernanda' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Depositar' }));
    await user.click(screen.getByRole('button', { name: 'US$ 10.000' }));
    await user.click(screen.getByRole('button', { name: 'Confirmar' }));
    expect(await screen.findByText('Protocolo 01A0E3A5-2F4C')).toBeInTheDocument();
    expect(screen.getByText(EVENT)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Ver na fila do time' })).toHaveAttribute('href', '/advisor-radar/fila');

    await user.click(screen.getByRole('button', { name: 'Voltar ao início' }));
    await user.click(screen.getByRole('button', { name: 'Sacar' }));
    await user.type(screen.getByLabelText('Valor em USD'), '999999');
    expect(screen.getByRole('button', { name: 'Confirmar' })).toBeDisabled();
    expect(screen.getByRole('alert')).toHaveTextContent('Valor acima do disponível para saque');

    await user.click(screen.getByRole('button', { name: 'Voltar' }));
    await user.click(screen.getByRole('button', { name: 'Reclamar' }));
    await user.click(screen.getByRole('button', { name: 'Estou pensando em sair' }));
    expect(await screen.findByRole('heading', { name: 'Reclamação registrada' })).toBeInTheDocument();
    expect(posts.some((url) => url.includes('/complaints'))).toBe(true);
    await waitFor(() => expect(posts.some((url) => url.includes('/deposits'))).toBe(true));
  });

  it('shows the client error state', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('no', { status: 404 })));
    renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível abrir este cliente.');
  });
});
