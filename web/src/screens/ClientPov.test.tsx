import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { ROUTER_BASENAME } from '../test/fixtures';
import { fernandaPerfil } from '../test/perfilFixtures';
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
  if (url.endsWith('/screens/perfil')) {
    return json(fernandaPerfil());
  }
  if (url.endsWith('/v1/client-pov/simulation')) {
    return json({ sim_day: 2 });
  }
  return json(fernandaPhase2());
}

/** Sets the layout the client app reads from `(min-width: 900px)`. */
function setWide(wide: boolean) {
  window.matchMedia = (query: string) =>
    ({
      matches: wide && query === '(min-width: 900px)',
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
    }) as MediaQueryList;
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
  vi.useRealTimers();
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
    expect(await screen.findByRole('heading', { level: 1, name: 'Perfil' })).toBeInTheDocument();
    expect(screen.queryByText(/não entra nesta simulação/)).not.toBeInTheDocument();
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

describe('simulation strip', () => {
  const ADVANCE = '/v1/client-pov/simulation/advance-day';

  it('shows the day, advances it with a fresh key, and re-fetches the screen', async () => {
    const user = userEvent.setup();
    setWide(false);
    const keys: string[] = [];
    const answers: ((res: Response) => void)[] = [];
    let homeReads = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (String(url).endsWith(ADVANCE)) {
          expect(init?.method).toBe('POST');
          keys.push(new Headers(init?.headers).get('Idempotency-Key') ?? '');
          return new Promise<Response>((resolve) => answers.push(resolve));
        }
        if (String(url).endsWith('/screens/home')) {
          homeReads += 1;
        }
        return answerGet(String(url));
      }),
    );
    renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByText('Dia simulado 2')).toBeInTheDocument();
    expect(await screen.findByRole('heading', { name: 'Olá, Fernanda' })).toBeInTheDocument();
    const reads = homeReads;

    const advance = screen.getByRole('button', { name: '+1 dia' });
    expect(advance).toHaveAttribute('title', 'Avançar um dia');
    await user.click(advance);
    expect(advance).toBeDisabled();
    expect(advance).toHaveAttribute('aria-busy', 'true');
    answers[0]?.(json({ sim_day: 3, event_id: EVENT }, 202));
    expect(await screen.findByText('Dia simulado 3')).toBeInTheDocument();
    await waitFor(() => expect(homeReads).toBe(reads + 1));
    expect(advance).toBeEnabled();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    await user.click(advance);
    answers[1]?.(json({ sim_day: 4, event_id: EVENT }, 202));
    expect(await screen.findByText('Dia simulado 4')).toBeInTheDocument();
    expect(keys).toHaveLength(2);
    expect(keys[0]).not.toBe('');
    expect(keys[1]).not.toBe(keys[0]);
  });

  it('keeps the key until a definite answer, shows the strip errors, and re-reads the day after a failure', async () => {
    const user = userEvent.setup();
    setWide(false);
    let simNow = 2;
    const keys: string[] = [];
    const replies: (() => Response)[] = [
      () => json({ error: 'ten_minutes' }, 429),
      () => {
        // The deadline hit after account-sim committed: the day moved.
        simNow = 3;
        return new Response('down', { status: 502 });
      },
      () => json({ sim_day: 3, event_id: EVENT }, 202),
      () => json({ error: 'invalid' }, 400),
      () => json({ sim_day: 'x' }, 202),
      () => {
        simNow = 4;
        return json({ sim_day: 4, event_id: EVENT }, 202);
      },
    ];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (String(url).endsWith(ADVANCE)) {
          keys.push(new Headers(init?.headers).get('Idempotency-Key') ?? '');
          return (replies.shift() ?? (() => new Response('none', { status: 500 })))();
        }
        if (String(url).endsWith('/v1/client-pov/simulation')) {
          return json({ sim_day: simNow });
        }
        return answerGet(String(url));
      }),
    );
    renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByText('Dia simulado 2')).toBeInTheDocument();
    const advance = screen.getByRole('button', { name: '+1 dia' });

    await user.click(advance);
    expect(await screen.findByRole('alert')).toHaveTextContent('Muitos dias avançados em pouco tempo. Espere até 10 minutos e tente de novo.');
    expect(screen.getByText('Dia simulado 2')).toBeInTheDocument();

    await user.click(advance);
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível registrar a ação.');
    // The failure re-reads the day, which the uncertain advance did move.
    expect(await screen.findByText('Dia simulado 3')).toBeInTheDocument();

    // The retry replays the same key instead of advancing again.
    await user.click(advance);
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
    expect(screen.getByText('Dia simulado 3')).toBeInTheDocument();
    expect(keys).toHaveLength(3);
    expect(new Set(keys).size).toBe(1);

    // After the 202 a press takes a fresh key; a 400 is definite and drops it.
    await user.click(advance);
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível registrar a ação.');
    expect(keys[3]).not.toBe(keys[2]);
    // A 202 without a whole day is not a definite answer: the key is kept.
    await user.click(advance);
    await waitFor(() => expect(keys).toHaveLength(5));
    expect(keys[4]).not.toBe(keys[3]);
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível registrar a ação.');
    await user.click(advance);
    expect(await screen.findByText('Dia simulado 4')).toBeInTheDocument();
    expect(keys[5]).toBe(keys[4]);
  });

  it('reads the screen and the day once more after the advance settles', async () => {
    vi.useFakeTimers();
    setWide(false);
    let simNow = 2;
    let homeReads = 0;
    let carteiraReads = 0;
    const fetchMock = vi.fn(async (url: string) => {
      if (String(url).endsWith(ADVANCE)) {
        simNow += 1;
        return json({ sim_day: simNow, event_id: EVENT }, 202);
      }
      if (String(url).endsWith('/v1/client-pov/simulation')) {
        return json({ sim_day: simNow });
      }
      if (String(url).endsWith('/screens/home')) {
        homeReads += 1;
      }
      if (String(url).endsWith('/screens/carteira')) {
        carteiraReads += 1;
      }
      return answerGet(String(url));
    });
    vi.stubGlobal('fetch', fetchMock);
    const settle = async (ms = 0) => {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(ms);
      });
      for (let i = 0; i < 5; i++) {
        await act(async () => {
          await vi.advanceTimersByTimeAsync(0);
        });
      }
    };
    const view = renderAt(`/client-pov/${FERNANDA}`);
    await settle();
    expect(screen.getByText('Dia simulado 2')).toBeInTheDocument();
    const reads = homeReads;

    fireEvent.click(screen.getByRole('button', { name: '+1 dia' }));
    await settle();
    expect(screen.getByText('Dia simulado 3')).toBeInTheDocument();
    expect(homeReads).toBe(reads + 1);

    // Another viewer advances before the second read.
    simNow = 5;
    await settle(1_400);
    expect(homeReads).toBe(reads + 1);
    expect(screen.getByText('Dia simulado 3')).toBeInTheDocument();
    await settle(200);
    expect(homeReads).toBe(reads + 2);
    expect(screen.getByText('Dia simulado 5')).toBeInTheDocument();
    await settle(5_000);
    expect(homeReads).toBe(reads + 2);

    // Leaving the screen cancels the second read.
    fireEvent.click(screen.getByRole('button', { name: '+1 dia' }));
    await settle();
    expect(homeReads).toBe(reads + 3);
    fireEvent.click(screen.getByRole('button', { name: 'Carteira' }));
    await settle();
    expect(carteiraReads).toBe(1);
    await settle(3_000);
    expect(carteiraReads).toBe(1);
    expect(homeReads).toBe(reads + 3);

    // So does leaving the app.
    fireEvent.click(screen.getByRole('button', { name: '+1 dia' }));
    await settle();
    expect(carteiraReads).toBe(2);
    view.unmount();
    const calls = fetchMock.mock.calls.length;
    await settle(3_000);
    expect(fetchMock.mock.calls.length).toBe(calls);
  });

  it('labels the desktop button in full and hides a day it could not read', async () => {
    setWide(true);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (String(url).endsWith('/v1/client-pov/simulation')) {
          return new Response('down', { status: 502 });
        }
        return answerGet(String(url));
      }),
    );
    renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByRole('button', { name: 'Avançar um dia' })).not.toHaveAttribute('title');
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Fernanda' })).toBeInTheDocument();
    expect(screen.queryByText(/Dia simulado/)).not.toBeInTheDocument();
  });

  it('hides a day the BFF did not send as a whole number', async () => {
    setWide(false);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (String(url).endsWith('/v1/client-pov/simulation')) {
          return json({ sim_day: 'três' });
        }
        return answerGet(String(url));
      }),
    );
    renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByRole('heading', { name: 'Olá, Fernanda' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '+1 dia' })).toBeEnabled();
    expect(screen.queryByText(/Dia simulado/)).not.toBeInTheDocument();
  });
});
