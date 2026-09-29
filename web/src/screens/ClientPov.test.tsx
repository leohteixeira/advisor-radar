import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { ROUTER_BASENAME } from '../test/fixtures';
import { fernandaCarteira, fernandaHome, fernandaInvestir, fernandaPhase2, SEED } from '../test/sduiFixtures';

const FERNANDA = SEED.fernanda;
const EVENT = '01a0e3a5-2f4c-7b1e-9d3a-6c0b8e41f27a';

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

/** A GET the client app sends: an SDUI screen, or the phase-2 customer read the panels use. */
function answerGet(url: string): Response {
  if (url.endsWith('/screens/home')) {
    return json(fernandaHome());
  }
  if (url.endsWith('/screens/investir')) {
    return json(fernandaInvestir());
  }
  if (url.endsWith('/screens/carteira')) {
    return json(fernandaCarteira());
  }
  return json(fernandaPhase2());
}

function renderAt(path: string) {
  return render(
    <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[`${ROUTER_BASENAME}${path}`]}>
      <App />
    </MemoryRouter>,
  );
}

afterEach(() => {
  localStorage.removeItem('advisor-radar.pov-theme');
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
                since: '2024',
                hint: 'Perto do teto da faixa. Um depósito pode subir o segmento; uma reclamação mostra o SLA mais longo.',
              },
            ],
          });
        }
        return answerGet(url);
      }),
    );
    renderAt('/client-pov');
    expect(await screen.findByRole('link', { name: /Fernanda Lima/ })).toBeInTheDocument();
    expect(screen.getByText(/Perto do teto da faixa/)).toBeInTheDocument();
    await user.click(screen.getByRole('link', { name: /Fernanda Lima/ }));
    expect(await screen.findByRole('heading', { name: 'Olá, Fernanda' })).toBeInTheDocument();
    expect(screen.getByText('Suas movimentações aparecem aqui assim que acontecerem.')).toBeInTheDocument();
  });

  it('shows a list error', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('no', { status: 502 })));
    renderAt('/client-pov');
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível carregar os clientes.');
  });

  it('deposits, withdraws, and files the preset complaint', async () => {
    const user = userEvent.setup();
    const posts: string[] = [];
    class FakeEventSource {
      addEventListener(type: string, fn: (ev: Event) => void) {
        if (type === 'bastidores') {
          fn({ data: JSON.stringify({ event_id: EVENT, steps: [{ id: 'outbox', label: 'Gravado na outbox do account-sim', state: 'feito' }] }) } as MessageEvent);
        }
      }
      removeEventListener() {}
      close() {}
    }
    vi.stubGlobal('EventSource', FakeEventSource);
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
        return answerGet(String(url));
      }),
    );
    renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByRole('heading', { name: 'Olá, Fernanda' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Depositar' }));
    await user.click(screen.getByRole('button', { name: 'US$ 10.000' }));
    await user.click(screen.getByRole('button', { name: 'Confirmar depósito' }));
    expect(await screen.findByText('Protocolo 01A0E3A5-2F4C')).toBeInTheDocument();
    expect(await screen.findByText(/Gravado na outbox do account-sim feito/)).toBeInTheDocument();
    expect(screen.getByText(EVENT)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Ver na fila do time' })).toHaveAttribute('href', '/advisor-radar/fila');

    await user.click(screen.getByRole('button', { name: 'Voltar ao início' }));
    await user.click(screen.getByRole('button', { name: 'Sacar' }));
    await user.type(screen.getByLabelText(/Quanto você quer sacar/), '999999');
    expect(screen.getByRole('button', { name: 'Confirmar saque' })).toBeDisabled();
    expect(screen.getByRole('alert')).toHaveTextContent('Valor acima do disponível para saque');

    await user.click(screen.getByRole('button', { name: 'Voltar' }));
    await user.click(screen.getByRole('button', { name: 'Reclamar' }));
    await user.click(screen.getByRole('button', { name: /Estou pensando em sair/ }));
    await user.click(screen.getByRole('button', { name: 'Enviar reclamação' }));
    expect(await screen.findByRole('heading', { name: 'Reclamação registrada' })).toBeInTheDocument();
    expect(posts.some((url) => url.includes('/complaints'))).toBe(true);
    await waitFor(() => expect(posts.some((url) => url.includes('/deposits'))).toBe(true));
  });

  it('uses a desktop panel, the light theme, and a free message', async () => {
    const user = userEvent.setup();
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
    const posts: { url: string; body: string }[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === 'POST') {
          posts.push({ url: String(url), body: String(init.body) });
          return json({ event_id: EVENT });
        }
        return answerGet(String(url));
      }),
    );
    vi.stubGlobal(
      'EventSource',
      class {
        addEventListener() {}
        removeEventListener() {}
        close() {}
      },
    );
    const view = renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByRole('button', { name: 'Tema claro' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Perfil' }));
    expect(screen.getByRole('status')).toHaveTextContent('Perfil não entra nesta simulação');
    await user.click(screen.getByRole('button', { name: 'Carteira' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Carteira' })).toBeInTheDocument();
    expect(screen.queryByText(/não entra nesta simulação/)).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Investir' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
    expect(screen.queryByText(/não entra nesta simulação/)).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Início' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Fernanda' })).toBeInTheDocument();
    expect(screen.queryByText(/não entra nesta simulação/)).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Tema claro' }));
    expect(screen.getByRole('button', { name: 'Tema escuro' })).toBeInTheDocument();
    expect(document.querySelector('.pov-app--light')).not.toBeNull();
    expect(localStorage.getItem('advisor-radar.pov-theme')).toBe('light');

    view.unmount();
    renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByRole('button', { name: 'Tema escuro' })).toBeInTheDocument();
    expect(document.querySelector('.pov-app--light')).not.toBeNull();

    await user.click(screen.getByRole('button', { name: 'Mensagem' }));
    expect(screen.getByRole('button', { name: 'Fechar' })).toBeInTheDocument();
    expect(screen.getByRole('complementary', { name: 'Ação' })).toHaveClass('pov-app__panel');
    expect(screen.getByText('Ainda não há mensagens. Escreva a primeira.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Enviar mensagem' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'E-mail' }));
    await user.type(screen.getByLabelText('Mensagem'), 'Olá, assessoria');
    await user.click(screen.getByRole('button', { name: 'Enviar mensagem' }));
    expect(await screen.findByRole('heading', { name: 'Mensagem enviada' })).toBeInTheDocument();
    expect(posts.some((post) => post.url.includes('/messages') && post.body.includes('e-mail'))).toBe(true);
  });

  it('shows the client error state', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('no', { status: 404 })));
    renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível abrir este cliente.');
  });
});
