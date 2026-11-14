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

describe('complaint panel and sidebar', () => {
  function stubNetwork(home: () => unknown = fernandaPhase2) {
    const posts: { url: string; body: string }[] = [];
    vi.stubGlobal(
      'EventSource',
      class {
        addEventListener() {}
        removeEventListener() {}
        close() {}
      },
    );
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === 'POST') {
          posts.push({ url: String(url), body: String(init.body) });
          return json({ event_id: EVENT });
        }
        return String(url).endsWith(`/customers/${FERNANDA}`) ? json(home()) : answerGet(String(url));
      }),
    );
    return posts;
  }

  it('shows the chosen preset in the text field and sends what the field shows', async () => {
    const user = userEvent.setup();
    const posts = stubNetwork();
    renderAt(`/client-pov/${FERNANDA}`);
    await user.click(await screen.findByRole('button', { name: 'Reclamar' }));

    const field = screen.getByPlaceholderText('O que aconteceu?');
    expect(field).toHaveValue('');
    expect(screen.getByRole('button', { name: 'Enviar reclamação' })).toBeDisabled();

    const preset = screen.getByRole('button', { name: /Minha transferência está atrasada/ });
    await user.click(preset);
    const presetText = 'Pedi uma transferência há dias e até agora não caiu. Preciso de uma posição hoje.';
    expect(field).toHaveValue(presetText);
    expect(preset).toHaveAttribute('aria-pressed', 'true');

    await user.type(field, ' Obrigada.');
    expect(preset).toHaveAttribute('aria-pressed', 'false');

    await user.click(screen.getByRole('button', { name: 'Enviar reclamação' }));
    expect(await screen.findByRole('heading', { name: 'Reclamação registrada' })).toBeInTheDocument();
    const sent = posts.find((post) => post.url.includes('/complaints'));
    expect(JSON.parse(sent?.body ?? '{}')).toEqual({ text: `${presetText} Obrigada.` });
  });

  it('shows the segment in the desktop sidebar only when the BFF sends one', async () => {
    setWide(true);
    stubNetwork();
    const view = renderAt(`/client-pov/${FERNANDA}`);
    expect(await screen.findByText('Cliente Essencial')).toBeInTheDocument();
    view.unmount();

    stubNetwork(() => ({ ...fernandaPhase2(), segment: '', sla: '' }));
    renderAt(`/client-pov/${FERNANDA}`);
    const who = (await screen.findByText('Fernanda Lima')).parentElement as HTMLElement;
    expect(who.textContent).toBe('Fernanda Lima');
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

/** Lets pending promises and effects run, after moving the fake clock by `ms`. */
async function elapse(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
  }
}

class SilentEventSource {
  addEventListener() {}
  removeEventListener() {}
  close() {}
}

describe('amount fields', () => {
  const UNRECOGNISED = 'Valor não reconhecido. Use o formato 1.000,00.';
  const ZERO = 'O valor precisa ser maior que zero.';
  const REJECTED: [string, string][] = [
    ['-5', UNRECOGNISED],
    ['1e3', UNRECOGNISED],
    ['abc', UNRECOGNISED],
    ['1,234', UNRECOGNISED],
    ['0', ZERO],
    ['0,00', ZERO],
  ];

  function stubPosts(answer: () => Response = () => json({ event_id: EVENT }, 202)) {
    const posts: { url: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal('EventSource', SilentEventSource);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === 'POST') {
          posts.push({ url: String(url), body: JSON.parse(String(init.body)) as Record<string, unknown> });
          return answer();
        }
        return answerGet(String(url));
      }),
    );
    return posts;
  }

  it.each([
    ['Depositar', /Quanto você quer depositar/, 'Confirmar depósito'],
    ['Sacar', /Quanto você quer sacar/, 'Confirmar saque'],
  ])('%s says why an amount is not accepted and sends nothing', async (action, label, confirm) => {
    const user = userEvent.setup();
    const posts = stubPosts();
    renderAt(`/client-pov/${FERNANDA}`);
    await user.click(await screen.findByRole('button', { name: action }));
    const input = screen.getByLabelText(label);
    const form = input.closest('form') as HTMLFormElement;

    // Untouched and empty: the sentence is helper text, not an error.
    expect(screen.getByText('Informe um valor maior que zero.')).toHaveAttribute('id', input.getAttribute('aria-describedby'));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(input).toHaveAttribute('aria-invalid', 'false');
    expect(screen.getByRole('button', { name: confirm })).toBeDisabled();
    fireEvent.submit(form);

    for (const [typed, sentence] of REJECTED) {
      await user.clear(input);
      await user.type(input, typed);
      expect(input).toHaveValue(typed);
      expect(screen.getByRole('alert')).toHaveTextContent(sentence);
      expect(input).toHaveAttribute('aria-invalid', 'true');
      expect(screen.getByRole('button', { name: confirm })).toBeDisabled();
      fireEvent.submit(form);
    }

    await user.clear(input);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByText('Informe um valor maior que zero.')).toBeInTheDocument();
    expect(posts).toHaveLength(0);
  });

  it('sends the cents of a valid amount, with or without the US$ prefix', async () => {
    const user = userEvent.setup();
    const posts = stubPosts();
    renderAt(`/client-pov/${FERNANDA}`);
    await user.click(await screen.findByRole('button', { name: 'Depositar' }));
    await user.type(screen.getByLabelText(/Quanto você quer depositar/), '1.000,50');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.queryByText('Informe um valor maior que zero.')).not.toBeInTheDocument();
    expect(screen.getByLabelText(/Quanto você quer depositar/)).not.toHaveAttribute('aria-describedby');
    await user.click(screen.getByRole('button', { name: 'Confirmar depósito' }));
    expect(await screen.findByRole('heading', { name: 'Depósito solicitado' })).toBeInTheDocument();
    expect(screen.getByText('kind: aporte · amount: 1000.5')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Voltar ao início' }));
    await user.click(screen.getByRole('button', { name: 'Sacar' }));
    await user.type(screen.getByLabelText(/Quanto você quer sacar/), 'US$ 1.000,00');
    await user.click(screen.getByRole('button', { name: 'Confirmar saque' }));
    expect(await screen.findByRole('heading', { name: 'Saque solicitado' })).toBeInTheDocument();
    expect(posts.map((post) => [post.url.split('/').pop(), post.body.amount])).toEqual([
      ['deposits', 100_050],
      ['withdrawals', 100_000],
    ]);
  });

  it.each([
    ['invalid', 'Confira o valor e tente de novo.'],
    ['insufficient', 'Valor acima do disponível para saque'],
  ])('shows the BFF refusal of an amount (422 %s)', async (code, sentence) => {
    const user = userEvent.setup();
    const posts = stubPosts(() => json({ error: code }, 422));
    renderAt(`/client-pov/${FERNANDA}`);
    await user.click(await screen.findByRole('button', { name: 'Sacar' }));
    await user.type(screen.getByLabelText(/Quanto você quer sacar/), '10');
    await user.click(screen.getByRole('button', { name: 'Confirmar saque' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(sentence);
    expect(posts).toHaveLength(1);
    expect(screen.queryByRole('heading', { name: 'Saque solicitado' })).not.toBeInTheDocument();
  });
});

describe('after an accepted command', () => {
  /** Fernanda's home once advisory has applied a US$ 10.000,00 deposit. */
  function homeAfter() {
    return JSON.parse(JSON.stringify(fernandaHome()).replaceAll('Essencial', 'Advance')) as unknown;
  }

  function carteiraAfter() {
    return JSON.parse(JSON.stringify(fernandaCarteira()).replaceAll('US$ 1.148,00', 'US$ 11.148,00')) as unknown;
  }

  /**
   * A BFF whose reads show the deposit only once `backend.settled` is set,
   * as when the pipeline is still running at the first re-read.
   */
  function stubPipeline(post: () => Response = () => json({ event_id: EVENT }, 202)) {
    const backend = { settled: false, home: 0, carteira: 0, shell: 0, posts: 0 };
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      const path = String(url);
      if (init?.method === 'POST') {
        backend.posts += 1;
        return post();
      }
      if (path.endsWith('/screens/home')) {
        backend.home += 1;
        return json(backend.settled ? homeAfter() : fernandaHome());
      }
      if (path.endsWith('/screens/carteira')) {
        backend.carteira += 1;
        return json(backend.settled ? carteiraAfter() : fernandaCarteira());
      }
      if (path.endsWith(`/customers/${FERNANDA}`)) {
        backend.shell += 1;
        return json(backend.settled ? { ...fernandaPhase2(), segment: 'Advance', assets: 1_820_000, caixa: 1_114_800 } : fernandaPhase2());
      }
      return answerGet(path);
    });
    vi.stubGlobal('fetch', fetchMock);
    vi.stubGlobal('EventSource', SilentEventSource);
    return { backend, fetchMock };
  }

  /** Deposits US$ 10.000,00 and leaves the confirmation at once. */
  async function depositAndClose() {
    fireEvent.click(screen.getByRole('button', { name: 'Depositar' }));
    fireEvent.change(screen.getByLabelText(/Quanto você quer depositar/), { target: { value: '10.000,00' } });
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar depósito' }));
    await elapse();
    expect(screen.getByRole('heading', { name: 'Depósito solicitado' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Voltar ao início' }));
    await elapse();
  }

  it('re-reads the shell and the screen until the pipeline has settled, then stops', async () => {
    vi.useFakeTimers();
    setWide(true);
    const { backend } = stubPipeline();
    renderAt(`/client-pov/${FERNANDA}`);
    await elapse();
    expect(screen.getByText('Cliente Essencial')).toBeInTheDocument();
    const home = backend.home;
    const shell = backend.shell;

    // The read on closing the panel races the pipeline and still sees the old state.
    await depositAndClose();
    expect(backend.home).toBe(home + 1);
    expect(backend.shell).toBe(shell + 1);
    expect(screen.getByText('Cliente Essencial')).toBeInTheDocument();
    expect(screen.getByText('Cliente Essencial desde 2024')).toBeInTheDocument();

    await elapse(1_400);
    expect(backend.home).toBe(home + 1);
    await elapse(200);
    expect(backend.home).toBe(home + 2);
    expect(backend.shell).toBe(shell + 2);
    expect(screen.getByText('Cliente Essencial')).toBeInTheDocument();

    // advisory applies the deposit; the next re-read shows it with no user action.
    backend.settled = true;
    await elapse(2_300);
    expect(backend.home).toBe(home + 2);
    await elapse(200);
    expect(backend.home).toBe(home + 3);
    expect(screen.getByText('Cliente Advance')).toBeInTheDocument();
    expect(screen.getByText('Cliente Advance desde 2024')).toBeInTheDocument();
    expect(screen.queryByText('Cliente Essencial')).not.toBeInTheDocument();

    await elapse(4_000);
    expect(backend.home).toBe(home + 4);
    await elapse(7_000);
    expect(backend.home).toBe(home + 5);
    expect(backend.shell).toBe(shell + 5);
    // The series is bounded: nothing is read after its last offset.
    await elapse(120_000);
    expect(backend.home).toBe(home + 5);
    expect(backend.shell).toBe(shell + 5);
    expect(backend.posts).toBe(1);
  });

  it('keeps re-reading across a tab change, refreshing the tab on view, until the app unmounts', async () => {
    vi.useFakeTimers();
    setWide(false);
    const { backend, fetchMock } = stubPipeline();
    const view = renderAt(`/client-pov/${FERNANDA}`);
    await elapse();
    await depositAndClose();
    const home = backend.home;

    fireEvent.click(screen.getByRole('button', { name: 'Carteira' }));
    await elapse();
    expect(backend.carteira).toBe(1);
    expect(screen.queryByText(/11\.148,00/)).not.toBeInTheDocument();

    backend.settled = true;
    await elapse(1_500);
    expect(backend.carteira).toBe(2);
    expect(backend.home).toBe(home);
    expect(screen.queryAllByText(/11\.148,00/)).not.toHaveLength(0);
    await elapse(2_500);
    expect(backend.carteira).toBe(3);

    view.unmount();
    const calls = fetchMock.mock.calls.length;
    await elapse(60_000);
    expect(fetchMock.mock.calls.length).toBe(calls);
  });

  it('restarts the series with each accepted command', async () => {
    vi.useFakeTimers();
    setWide(false);
    const { backend } = stubPipeline();
    renderAt(`/client-pov/${FERNANDA}`);
    await elapse();
    await depositAndClose();
    await elapse(1_000);
    await depositAndClose();
    const home = backend.home;

    // 1.5 s after the first deposit: its series is gone.
    await elapse(600);
    expect(backend.home).toBe(home);
    // 1.5 s after the second.
    await elapse(900);
    expect(backend.home).toBe(home + 1);
    // Then 4 s, 8 s and 15 s after the second, and no more.
    for (const ms of [2_500, 4_000, 7_000]) {
      await elapse(ms);
    }
    expect(backend.home).toBe(home + 4);
    await elapse(120_000);
    expect(backend.home).toBe(home + 4);
    expect(backend.posts).toBe(2);
  });

  it('starts nothing when the command is refused', async () => {
    vi.useFakeTimers();
    setWide(false);
    const { backend, fetchMock } = stubPipeline(() => json({ error: 'invalid' }, 422));
    renderAt(`/client-pov/${FERNANDA}`);
    await elapse();
    fireEvent.click(screen.getByRole('button', { name: 'Depositar' }));
    fireEvent.change(screen.getByLabelText(/Quanto você quer depositar/), { target: { value: '10.000,00' } });
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar depósito' }));
    await elapse();
    expect(screen.getByRole('alert')).toHaveTextContent('Confira o valor e tente de novo.');
    expect(backend.posts).toBe(1);

    const calls = fetchMock.mock.calls.length;
    await elapse(60_000);
    expect(fetchMock.mock.calls.length).toBe(calls);
    // Closing the form reads once, as before, and nothing follows.
    fireEvent.click(screen.getByRole('button', { name: 'Voltar' }));
    await elapse();
    const home = backend.home;
    await elapse(60_000);
    expect(backend.home).toBe(home);
  });
});

describe('action panel as a dialog', () => {
  function stub() {
    vi.stubGlobal('EventSource', SilentEventSource);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) =>
        init?.method === 'POST' ? json({ event_id: EVENT }, 202) : answerGet(String(url)),
      ),
    );
  }

  it.each([
    { layout: 'phone', wide: false, action: 'Depositar', field: /Quanto você quer depositar/, close: 'Voltar' },
    { layout: 'phone', wide: false, action: 'Sacar', field: /Quanto você quer sacar/, close: 'Voltar' },
    { layout: 'desktop', wide: true, action: 'Depositar', field: /Quanto você quer depositar/, close: 'Fechar' },
    { layout: 'desktop', wide: true, action: 'Sacar', field: /Quanto você quer sacar/, close: 'Fechar' },
  ])('$layout $action: focus enters the panel, Tab stays in it, and Esc returns focus to the trigger', async ({ wide, action, field, close }) => {
    setWide(wide);
    const user = userEvent.setup();
    stub();
    renderAt(`/client-pov/${FERNANDA}`);
    const trigger = await screen.findByRole('button', { name: action });
    trigger.focus();
    await user.keyboard('{Enter}');

    const panel = screen.getByRole('complementary', { name: 'Ação' });
    expect(screen.getByLabelText(field)).toBeInTheDocument();
    expect(panel).toHaveFocus();

    // Forward from the panel reaches its first control; back from there wraps to the last.
    await user.tab();
    expect(screen.getByRole('button', { name: close })).toHaveFocus();
    await user.tab({ shift: true });
    const last = Array.from(panel.querySelectorAll<HTMLElement>('button:not([disabled]), input')).at(-1);
    expect(last).toHaveFocus();
    // And forward from the last wraps to the first: focus never leaves the panel.
    await user.tab();
    expect(screen.getByRole('button', { name: close })).toHaveFocus();
    for (let i = 0; i < 12; i += 1) {
      await user.tab();
      expect(panel).toContainElement(document.activeElement as HTMLElement);
    }

    await user.keyboard('{Escape}');
    expect(screen.queryByLabelText(field)).not.toBeInTheDocument();
    expect(await screen.findByRole('button', { name: action })).toHaveFocus();
  });

  it('keeps focus in the panel across the confirmation and returns it to the trigger on close', async () => {
    setWide(false);
    const user = userEvent.setup();
    stub();
    renderAt(`/client-pov/${FERNANDA}`);
    (await screen.findByRole('button', { name: 'Depositar' })).focus();
    await user.keyboard('{Enter}');
    await user.type(screen.getByLabelText(/Quanto você quer depositar/), '100');
    await user.click(screen.getByRole('button', { name: 'Confirmar depósito' }));

    expect(await screen.findByRole('heading', { name: 'Depósito solicitado' })).toBeInTheDocument();
    const panel = screen.getByRole('complementary', { name: 'Ação' });
    expect(panel).toContainElement(document.activeElement as HTMLElement);

    await user.click(screen.getByRole('button', { name: 'Voltar ao início' }));
    expect(await screen.findByRole('button', { name: 'Depositar' })).toHaveFocus();
  });

  it('Shift+Tab from the panel itself goes to its last control', async () => {
    setWide(false);
    const user = userEvent.setup();
    stub();
    renderAt(`/client-pov/${FERNANDA}`);
    (await screen.findByRole('button', { name: 'Depositar' })).focus();
    await user.keyboard('{Enter}');

    const panel = screen.getByRole('complementary', { name: 'Ação' });
    expect(panel).toHaveFocus();
    await user.tab({ shift: true });
    expect(panel).toContainElement(document.activeElement as HTMLElement);
    expect(document.activeElement).not.toBe(panel);
  });

  it('on the phone returns focus to the very control that opened the panel when two share a label', async () => {
    setWide(false);
    const user = userEvent.setup();
    vi.stubGlobal('EventSource', SilentEventSource);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (!String(url).endsWith('/screens/home')) {
          return answerGet(String(url));
        }
        // Two sections offer "Depositar", as a moment card and the action grid do.
        const home = fernandaHome();
        const actions = home.sections.find((section) => section.id === 'actions');
        return json({ ...home, sections: [...home.sections, { ...actions, id: 'actions-again' }] });
      }),
    );
    renderAt(`/client-pov/${FERNANDA}`);
    await waitFor(() => expect(screen.getAllByRole('button', { name: 'Depositar' })).toHaveLength(2));

    for (const index of [1, 0]) {
      screen.getAllByRole('button', { name: 'Depositar' })[index]?.focus();
      await user.keyboard('{Enter}');
      expect(screen.getByRole('complementary', { name: 'Ação' })).toHaveFocus();
      await user.keyboard('{Escape}');
      // The phone rebuilt the screen behind the panel: these are new buttons.
      await waitFor(() => expect(screen.getAllByRole('button', { name: 'Depositar' })[index]).toHaveFocus());
    }
  });
});

