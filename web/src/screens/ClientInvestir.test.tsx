import { act, fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import { formatCents } from '../api/pov';
import { ROUTER_BASENAME } from '../test/fixtures';
import { fernandaInvestir, fernandaPhase2, SEED, thiagoHome, thiagoIdleCashHome, thiagoInvestir, thiagoPhase2 } from '../test/sduiFixtures';
import type { Screen } from '../sdui/types';

const EVENT = '01a0e3b1-7c2d-7f3e-9a1b-5c8e2d4f6a70';

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function LocationProbe() {
  return <div data-testid="location">{useLocation().pathname}</div>;
}

/** The browser back and forward buttons, which change the tab without the tab bar. */
function HistoryProbe() {
  const navigate = useNavigate();
  return (
    <>
      <button type="button" onClick={() => void navigate(-1)}>
        history back
      </button>
      <button type="button" onClick={() => void navigate(1)}>
        history forward
      </button>
    </>
  );
}

function renderAt(path: string) {
  return render(
    <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[`${ROUTER_BASENAME}${path}`]}>
      <App />
      <LocationProbe />
      <HistoryProbe />
    </MemoryRouter>,
  );
}

/** A response the test settles by hand. */
function deferred() {
  let resolve: (response: Response) => void = () => undefined;
  const promise = new Promise<Response>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
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

interface Post {
  url: string;
  key: string | null;
  body: Record<string, unknown>;
}

interface Stub {
  home: () => Screen;
  investir: () => Screen;
  phase2: () => unknown;
  /** Answers each purchase in turn; the last answer repeats. */
  purchases?: (() => Response | Promise<Response>)[];
}

/** A BFF with the screen routes, the phase-2 customer read, and the story 9 purchase route. */
function stubBFF(stub: Stub) {
  const gets: string[] = [];
  const posts: Post[] = [];
  const answers = stub.purchases ?? [() => json({ event_id: EVENT }, 202)];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        const headers = new Headers(init.headers);
        posts.push({ url, key: headers.get('Idempotency-Key'), body: JSON.parse(String(init.body)) as Record<string, unknown> });
        const answer = answers.length > 1 ? answers.shift() : answers[0];
        return answer ? answer() : json({ event_id: EVENT }, 202);
      }
      gets.push(url);
      if (url.endsWith('/screens/home')) {
        return json(stub.home());
      }
      if (url.endsWith('/screens/investir')) {
        return json(stub.investir());
      }
      return json(stub.phase2());
    }),
  );
  return { gets, posts };
}

/**
 * An EventSource that reports `states` for EVENT as soon as the confirmation
 * subscribes, after a row of another event; with no states it stays silent.
 */
function stubStream(states: string[]) {
  const ids = ['outbox', 'broker', 'evaluated', 'queue'];
  vi.stubGlobal(
    'EventSource',
    class {
      addEventListener(type: string, fn: (ev: Event) => void) {
        if (type === 'bastidores') {
          const steps = states.map((state, index) => ({ id: ids[index], label: `bff label ${index}`, state }));
          fn({ data: JSON.stringify({ event_id: 'another', steps: [] }) } as MessageEvent);
          if (states.length > 0) {
            fn({ data: JSON.stringify({ event_id: EVENT, steps }) } as MessageEvent);
          }
        }
      }
      removeEventListener() {}
      close() {}
    },
  );
}

const fernanda: Stub = { home: thiagoHome, investir: fernandaInvestir, phase2: fernandaPhase2 };

async function openCobaltoForFernanda() {
  const user = userEvent.setup();
  renderAt(`/client-pov/${SEED.fernanda}/investir`);
  const stocks = await screen.findByRole('region', { name: 'Ações' });
  await user.click(within(stocks).getByRole('button', { name: /Cobalto Semicondutores/ }));
  return { user, form: screen.getByRole('form', { name: 'Investir em Cobalto Semicondutores' }) };
}

/** formatCents output as a text matcher sees it: getByText collapses the no-break space. */
function shown(cents: number): string {
  return formatCents(cents).replace(/\u00a0/g, ' ');
}

function confirmButton(cents: number) {
  return screen.getByRole('button', { name: `Confirmar compra de ${formatCents(cents)}` });
}

beforeEach(() => {
  window.matchMedia = undefined as unknown as typeof window.matchMedia;
  stubStream([]);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('Investir route', () => {
  it('renders the investir envelope with one screen request', async () => {
    const { gets } = stubBFF({ home: thiagoHome, investir: thiagoInvestir, phase2: thiagoPhase2 });
    renderAt(`/client-pov/${SEED.thiago}/investir`);
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
    expect(screen.getByText('Produtos fictícios · preço fixo da simulação')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Disponível para investir' })).toHaveTextContent('US$ 60.520,00');
    expect(screen.getByText('Perfil arrojado')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Para o seu perfil arrojado' })).toBeInTheDocument();
    for (const title of ['Renda fixa', 'ETFs', 'Ações']) {
      expect(screen.getByRole('region', { name: title })).toBeInTheDocument();
    }
    expect(within(screen.getByRole('navigation', { name: 'Navegação principal' })).getByRole('button', { name: 'Investir' })).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByText(/não entra nesta simulação/)).not.toBeInTheDocument();
    expect(gets.filter((url) => url.includes('/screens/'))).toEqual([expect.stringMatching(new RegExp(`/v1/client-pov/customers/${SEED.thiago}/screens/investir$`))]);
  });

  it('shows the error state and retries the investir screen', async () => {
    const user = userEvent.setup();
    const screens = [() => json({ nope: true }), () => json(thiagoInvestir())];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => (url.endsWith('/screens/investir') ? (screens.shift() ?? screens[0])!() : json(thiagoPhase2()))),
    );
    renderAt(`/client-pov/${SEED.thiago}/investir`);
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível montar sua tela.');
    expect(screen.getByRole('heading', { level: 1, name: 'Não foi possível montar sua tela.' })).toHaveFocus();
    await user.click(screen.getByRole('button', { name: 'Tentar de novo' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
  });
});

describe('purchase form', () => {
  it('reads the product from the envelope and warns above the profile without blocking', async () => {
    stubBFF(fernanda);
    const { form } = await openCobaltoForFernanda();
    expect(within(form).getByRole('heading', { level: 1, name: 'Investir em Cobalto Semicondutores' })).toHaveFocus();
    expect(within(form).getByText('Ação')).toBeInTheDocument();
    expect(within(form).getByText('Risco 5 de 5')).toBeInTheDocument();
    expect(within(form).getByText('+27,1% em 12 meses')).toBeInTheDocument();
    expect(within(form).getByText('Preço fixo da simulação, sem cotação. Rentabilidade fictícia.')).toBeInTheDocument();
    expect(form.querySelector('.sdui-risk')).toHaveAttribute('data-above', 'true');
    expect(within(form).getByText('Disponível no caixa: US$ 1.148,00')).toHaveAttribute('id', 'valor-compra-caixa');
    expect(within(form).getByText('Mínimo US$ 10')).toHaveAttribute('id', 'valor-compra-minimo');
    const note = within(form).getByRole('note');
    expect(within(form).getByLabelText('Quanto investir')).toHaveAttribute('aria-describedby', 'valor-compra-caixa valor-compra-minimo valor-compra-aviso');
    expect(note).toHaveTextContent('Acima do seu perfil');
    expect(note).toHaveTextContent('Este produto tem risco 5. Seu perfil é conservador, que vai até risco 2. Você pode investir mesmo assim, e a sua assessora será avisada.');
    expect(within(form).getByLabelText('Quanto investir')).toHaveValue(formatCents(100_000));
    expect(within(form).getByRole('button', { name: 'US$ 1.000' })).toHaveAttribute('aria-pressed', 'true');
    expect(confirmButton(100_000)).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Cancelar' })).toBeInTheDocument();
    expect(document.querySelector('.sdui-screen')).toBeNull();
    expect(screen.queryByRole('button', { name: /Raio-X/ })).not.toBeInTheDocument();
  });

  it('shows no warning for a product within the profile', async () => {
    const user = userEvent.setup();
    stubBFF(fernanda);
    renderAt(`/client-pov/${SEED.fernanda}/investir`);
    await user.click(await screen.findByRole('button', { name: /Orla T-Bill 6 meses/ }));
    const form = screen.getByRole('form', { name: 'Investir em Orla T-Bill 6 meses' });
    expect(within(form).queryByRole('note')).not.toBeInTheDocument();
    expect(within(form).getByLabelText('Quanto investir')).toHaveAttribute('aria-describedby', 'valor-compra-caixa valor-compra-minimo');
    expect(form.querySelector('.sdui-risk')).toHaveAttribute('data-above', 'false');
  });

  it('fills the amount from the chips and caps it at the cash', async () => {
    stubBFF(fernanda);
    const { user, form } = await openCobaltoForFernanda();
    const input = within(form).getByLabelText('Quanto investir');

    await user.click(within(form).getByRole('button', { name: 'US$ 250' }));
    expect(input).toHaveValue(formatCents(25_000));
    expect(within(form).getByRole('button', { name: 'US$ 250' })).toHaveAttribute('aria-pressed', 'true');
    expect(within(form).getByRole('button', { name: 'US$ 1.000' })).toHaveAttribute('aria-pressed', 'false');
    expect(confirmButton(25_000)).toBeInTheDocument();

    await user.click(within(form).getByRole('button', { name: 'Tudo' }));
    expect(input).toHaveValue(formatCents(114_800));
    expect(within(form).getByRole('button', { name: 'Tudo' })).toHaveAttribute('aria-pressed', 'true');
    expect(confirmButton(114_800)).toBeInTheDocument();

    await user.clear(input);
    expect(within(form).getByRole('button', { name: 'Tudo' })).toHaveAttribute('aria-pressed', 'false');
    expect(confirmButton(0)).toBeDisabled();
    await user.type(input, '300,5');
    expect(input).toHaveValue('300,5');
    expect(confirmButton(30_050)).toBeEnabled();

    await user.clear(input);
    await user.type(input, '5000');
    expect(input).toHaveValue(formatCents(114_800));
    expect(confirmButton(114_800)).toBeInTheDocument();
  });

  it('caps the default and the chips when the cash is below them', async () => {
    const user = userEvent.setup();
    const poor = fernandaInvestir();
    const summary = poor.sections[0]?.components[0]?.props as Record<string, unknown>;
    summary.cash = 'US$ 120,00';
    summary.cash_cents = 12_000;
    stubBFF({ ...fernanda, investir: () => poor });
    renderAt(`/client-pov/${SEED.fernanda}/investir`);
    await user.click(await screen.findByRole('button', { name: /Orla T-Bill 6 meses/ }));
    const input = screen.getByLabelText('Quanto investir');
    expect(input).toHaveValue(formatCents(12_000));
    expect(screen.getByRole('button', { name: 'Tudo' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'US$ 1.000' })).toHaveAttribute('aria-pressed', 'false');
    await user.click(screen.getByRole('button', { name: 'US$ 250' }));
    expect(input).toHaveValue(formatCents(12_000));
  });

  it('has no cap and no "Tudo" when the cash section was omitted', async () => {
    const user = userEvent.setup();
    const noCash = fernandaInvestir();
    noCash.sections = noCash.sections.filter((section) => section.id !== 'cash');
    noCash.omitted = [{ id: 'cash', type: 'invest_summary', reason: 'account-sim' }];
    stubBFF({ ...fernanda, investir: () => noCash });
    renderAt(`/client-pov/${SEED.fernanda}/investir`);
    await user.click(await screen.findByRole('button', { name: /Orla T-Bill 6 meses/ }));
    expect(screen.queryByRole('button', { name: 'Tudo' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'US$ 1.000' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByLabelText('Quanto investir')).toHaveAttribute('aria-describedby', 'valor-compra-minimo');
    expect(screen.queryByText(/Disponível no caixa/)).not.toBeInTheDocument();
    const input = screen.getByLabelText('Quanto investir');
    await user.clear(input);
    await user.type(input, '5000');
    expect(confirmButton(500_000)).toBeEnabled();
  });

  it('cancels back to Investir and fetches the screen again', async () => {
    const { gets } = stubBFF(fernanda);
    const { user } = await openCobaltoForFernanda();
    await user.click(screen.getByRole('button', { name: 'Cancelar' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
    expect(screen.queryByRole('form')).not.toBeInTheDocument();

    await user.click(within(screen.getByRole('region', { name: 'Ações' })).getByRole('button', { name: /Farol Saúde/ }));
    await user.click(screen.getByRole('button', { name: 'Voltar para Investir' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
    expect(gets.filter((url) => url.endsWith('/screens/investir'))).toHaveLength(3);
  });

  // A definite refusal (any 4xx but 429) ends its Idempotency-Key; a retry
  // after an unknown outcome or a rate limit reuses it.
  it.each([
    ['422 insufficient', () => json({ error: 'insufficient' }, 422), 'Valor acima do caixa disponível.', 'fresh'],
    ['422 invalid', () => json({ error: 'invalid' }, 422), 'Confira o valor e tente de novo.', 'fresh'],
    ['a 422 without a code', () => new Response('no', { status: 422 }), 'Confira o valor e tente de novo.', 'fresh'],
    ['429', () => json({ error: 'rate_limited' }, 429), 'Muitas ações em pouco tempo. Espere um minuto e tente de novo.', 'same'],
    ['502', () => new Response('bad gateway', { status: 502 }), 'Não foi possível registrar a ação.', 'same'],
    ['a network failure', () => Promise.reject(new TypeError('failed to fetch')), 'Não foi possível registrar a ação.', 'same'],
  ])('shows the %s refusal and stays on the form', async (_name, answer, message, key) => {
    const { posts } = stubBFF({ ...fernanda, purchases: [answer, () => json({ event_id: EVENT }, 202)] });
    const { user, form } = await openCobaltoForFernanda();
    await user.click(confirmButton(100_000));
    expect(await within(form).findByRole('alert')).toHaveTextContent(message);
    expect(screen.queryByRole('heading', { name: 'Compra enviada' })).not.toBeInTheDocument();

    await user.click(confirmButton(100_000));
    expect(await screen.findByRole('heading', { level: 1, name: 'Compra enviada' })).toBeInTheDocument();
    expect(posts).toHaveLength(2);
    expect(posts[0]?.key).toMatch(/^[0-9a-f-]{36}$/);
    expect(posts[1]?.key).toMatch(/^[0-9a-f-]{36}$/);
    if (key === 'same') {
      expect(posts[1]?.key).toBe(posts[0]?.key);
    } else {
      expect(posts[1]?.key).not.toBe(posts[0]?.key);
    }
  });

  it('makes a fresh Idempotency-Key when the amount changes or the form reopens', async () => {
    const failed = () => new Response('bad gateway', { status: 502 });
    const { posts } = stubBFF({ ...fernanda, purchases: [failed, failed, failed, () => json({ event_id: EVENT }, 202)] });
    const { user, form } = await openCobaltoForFernanda();
    await user.click(confirmButton(100_000));
    expect(await within(form).findByRole('alert')).toHaveTextContent('Não foi possível registrar a ação.');
    await user.click(within(form).getByRole('button', { name: 'US$ 250' }));
    await user.click(confirmButton(25_000));
    await within(form).findByRole('alert');
    expect(posts).toHaveLength(2);
    expect(posts[1]?.key).not.toBe(posts[0]?.key);

    await user.click(screen.getByRole('button', { name: 'Cancelar' }));
    await user.click(within(await screen.findByRole('region', { name: 'Ações' })).getByRole('button', { name: /Cobalto Semicondutores/ }));
    await user.click(screen.getByRole('button', { name: 'US$ 250' }));
    await user.click(confirmButton(25_000));
    expect(await screen.findByRole('alert')).toHaveTextContent('Não foi possível registrar a ação.');
    expect(posts).toHaveLength(3);
    expect(posts[2]?.key).not.toBe(posts[1]?.key);

    await user.click(confirmButton(25_000));
    expect(await screen.findByRole('heading', { level: 1, name: 'Compra enviada' })).toBeInTheDocument();
    expect(posts[3]?.key).toBe(posts[2]?.key);
  });

  it('shows the minimum and keeps Confirm disabled below it', async () => {
    stubBFF(fernanda);
    const { user, form } = await openCobaltoForFernanda();
    expect(within(form).getByText('Mínimo US$ 10')).toBeInTheDocument();
    const input = within(form).getByLabelText('Quanto investir');
    await user.clear(input);
    await user.type(input, '9,99');
    expect(confirmButton(999)).toBeDisabled();
    await user.clear(input);
    await user.type(input, '10');
    expect(confirmButton(1_000)).toBeEnabled();
  });

  it('sends one purchase while the first is pending', async () => {
    const pending = deferred();
    const { posts } = stubBFF({ ...fernanda, purchases: [() => pending.promise] });
    const { user, form } = await openCobaltoForFernanda();
    await user.click(confirmButton(100_000));
    expect(confirmButton(100_000)).toBeDisabled();
    await user.click(confirmButton(100_000));
    fireEvent.submit(form);
    expect(posts).toHaveLength(1);
    await act(async () => pending.resolve(json({ event_id: EVENT }, 202)));
    expect(await screen.findByRole('heading', { level: 1, name: 'Compra enviada' })).toBeInTheDocument();
    expect(posts).toHaveLength(1);
  });

  it('sends one purchase when the form is submitted twice at once', async () => {
    const pending = deferred();
    const { posts } = stubBFF({ ...fernanda, purchases: [() => pending.promise] });
    const { form } = await openCobaltoForFernanda();
    act(() => {
      fireEvent.submit(form);
      fireEvent.submit(form);
    });
    expect(posts).toHaveLength(1);
    await act(async () => pending.resolve(json({ event_id: EVENT }, 202)));
    expect(await screen.findByRole('heading', { level: 1, name: 'Compra enviada' })).toBeInTheDocument();
  });

  it.each([
    ['a 202', () => json({ event_id: EVENT }, 202)],
    ['a refusal', () => json({ error: 'insufficient' }, 422)],
  ])('drops %s that arrives after the form closed', async (_name, answer) => {
    const pending = deferred();
    stubBFF({ ...fernanda, purchases: [() => pending.promise] });
    const { user } = await openCobaltoForFernanda();
    await user.click(confirmButton(100_000));
    await user.click(screen.getByRole('button', { name: 'Voltar para Investir' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
    await act(async () => pending.resolve(answer()));
    expect(screen.queryByRole('heading', { name: 'Compra enviada' })).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
  });

  it('drops a late answer and closes the form on browser back and forward', async () => {
    const pending = deferred();
    stubBFF({ home: thiagoHome, investir: thiagoInvestir, phase2: thiagoPhase2, purchases: [() => pending.promise] });
    const user = userEvent.setup();
    renderAt(`/client-pov/${SEED.thiago}`);
    await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' });
    await user.click(screen.getByRole('button', { name: 'Investir' }));
    const etfs = await screen.findByRole('region', { name: 'ETFs' });
    await user.click(within(etfs).getByRole('button', { name: /Maré Ações Globais ETF/ }));
    await user.click(confirmButton(100_000));

    await user.click(screen.getByRole('button', { name: 'history back' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(screen.queryByRole('form')).not.toBeInTheDocument();
    await act(async () => pending.resolve(json({ event_id: EVENT }, 202)));
    expect(screen.queryByRole('heading', { name: 'Compra enviada' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'history forward' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
    expect(screen.queryByRole('form')).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Compra enviada' })).not.toBeInTheDocument();
  });

  it('masks the cash and the amount when the eye toggle is on', async () => {
    const user = userEvent.setup();
    stubBFF({ home: thiagoHome, investir: thiagoInvestir, phase2: thiagoPhase2 });
    renderAt(`/client-pov/${SEED.thiago}`);
    await user.click(await screen.findByRole('button', { name: 'Esconder valores' }));
    await user.click(screen.getByRole('button', { name: 'Investir' }));
    const etfs = await screen.findByRole('region', { name: 'ETFs' });
    await user.click(within(etfs).getByRole('button', { name: /Maré Ações Globais ETF/ }));
    const form = screen.getByRole('form', { name: 'Investir em Maré Ações Globais ETF' });
    expect(within(form).getByText('Disponível no caixa: US$ ••••••')).toBeInTheDocument();
    expect(within(form).queryByText(/60\.520/)).not.toBeInTheDocument();
    await user.click(within(form).getByRole('button', { name: 'Confirmar compra de US$ ••••••' }));
    const sent = await screen.findByRole('region', { name: 'Compra enviada' });
    expect(within(sent).getByText('US$ •••••• em Maré Ações Globais ETF. O valor saiu do caixa e já aparece na sua carteira.')).toBeInTheDocument();
    expect(within(sent).getByRole('heading', { level: 1, name: 'Compra enviada' })).toHaveFocus();
  });

  it('logs and opens no form for a product that is not on the screen', async () => {
    const user = userEvent.setup();
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const ghost = fernandaInvestir();
    const stocks = ghost.sections.find((section) => section.id === 'stocks')?.components[0]?.props as { products: { action: { product_id: string } }[] };
    stocks.products[0]!.action.product_id = 'ghost';
    stubBFF({ ...fernanda, investir: () => ghost });
    renderAt(`/client-pov/${SEED.fernanda}/investir`);
    await user.click(within(await screen.findByRole('region', { name: 'Ações' })).getAllByRole('button')[0]!);
    expect(error).toHaveBeenCalledWith('sdui: purchase product ghost is not on the screen');
    expect(screen.queryByRole('form')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
  });
});

describe('client app shell', () => {
  it('shows the skeleton, not the home, while the Investir screen is pending', async () => {
    const user = userEvent.setup();
    const pending = deferred();
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/screens/investir')) {
          return pending.promise;
        }
        return url.endsWith('/screens/home') ? json(thiagoHome()) : json(thiagoPhase2());
      }),
    );
    renderAt(`/client-pov/${SEED.thiago}`);
    await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' });
    await user.click(screen.getByRole('button', { name: 'Investir' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Montando sua tela…');
    expect(screen.queryByRole('heading', { level: 1, name: 'Olá, Thiago' })).not.toBeInTheDocument();
    await act(async () => pending.resolve(json(thiagoInvestir())));
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
  });

  it('keeps the shell when the customer re-read fails after a panel closes', async () => {
    let reads = 0;
    stubBFF({
      ...fernanda,
      phase2: () => {
        reads += 1;
        if (reads > 1) {
          throw new TypeError('failed to fetch');
        }
        return fernandaPhase2();
      },
    });
    const { user } = await openCobaltoForFernanda();
    await user.click(screen.getByRole('button', { name: 'Cancelar' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
    expect(reads).toBe(2);
    expect(screen.queryByText('Não foi possível abrir este cliente.')).not.toBeInTheDocument();
  });
});

describe('purchase confirmation', () => {
  it('posts the purchase and shows the rule step above the profile', async () => {
    stubStream(['feito', 'feito', 'agora', 'aguardando']);
    const { posts } = stubBFF(fernanda);
    const { user } = await openCobaltoForFernanda();
    await user.click(screen.getByRole('button', { name: 'US$ 250' }));
    await user.click(confirmButton(25_000));

    const sent = await screen.findByRole('region', { name: 'Compra enviada' });
    expect(posts).toEqual([
      {
        url: expect.stringMatching(new RegExp(`/v1/client-pov/customers/${SEED.fernanda}/purchases$`)),
        key: expect.any(String),
        body: { product_id: 'cobalto', amount_cents: 25_000 },
      },
    ]);
    expect(within(sent).getByRole('heading', { level: 1, name: 'Compra enviada' })).toBeInTheDocument();
    expect(within(sent).getByText(`${shown(25_000)} em Cobalto Semicondutores. O valor saiu do caixa e já aparece na sua carteira.`)).toBeInTheDocument();
    expect(within(sent).getByText('01A0E3B1-7C2D')).toBeInTheDocument();
    expect(within(sent).getByText('account.event.recorded · kind aplicacao · schema 3')).toBeInTheDocument();
    expect(within(sent).getByText(`event_id ${EVENT}`)).toBeInTheDocument();
    expect(within(sent).getAllByRole('listitem').map((li) => [li.querySelector('span')?.textContent, li.getAttribute('data-state')])).toEqual([
      ['Gravado na outbox do account-sim', 'feito'],
      ['Publicado no RabbitMQ', 'feito'],
      ['Regra: compra acima do perfil', 'agora'],
      ['Na fila da assessoria', 'aguardando'],
    ]);
    expect(within(sent).getByRole('link', { name: 'Ver na fila do time' })).toHaveAttribute('href', '/advisor-radar/fila');
  });

  it('shows "Avaliado pelas regras" within the profile, before the stream reports', async () => {
    const user = userEvent.setup();
    stubBFF(fernanda);
    renderAt(`/client-pov/${SEED.fernanda}/investir`);
    await user.click(await screen.findByRole('button', { name: /Orla T-Bill 6 meses/ }));
    await user.click(confirmButton(100_000));
    const sent = await screen.findByRole('region', { name: 'Compra enviada' });
    expect(within(sent).getAllByRole('listitem').map((li) => [li.querySelector('span')?.textContent, li.getAttribute('data-state')])).toEqual([
      ['Gravado na outbox do account-sim', 'feito'],
      ['Publicado no RabbitMQ', 'aguardando'],
      ['Avaliado pelas regras', 'aguardando'],
      ['Na fila da assessoria, se uma regra disparar', 'aguardando'],
    ]);
  });

  it('lets Thiago buy "Tudo" in acoesg, after which his home has no idle cash', async () => {
    const user = userEvent.setup();
    let bought = false;
    const { posts, gets } = stubBFF({
      home: () => (bought ? thiagoHome() : thiagoIdleCashHome()),
      investir: thiagoInvestir,
      phase2: thiagoPhase2,
      purchases: [
        () => {
          bought = true;
          return json({ event_id: EVENT }, 202);
        },
      ],
    });
    renderAt(`/client-pov/${SEED.thiago}`);
    expect(await screen.findByRole('region', { name: 'Caixa parado' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Ver produtos' }));
    const etfs = await screen.findByRole('region', { name: 'ETFs' });
    await user.click(within(etfs).getByRole('button', { name: /Maré Ações Globais ETF/ }));
    await user.click(screen.getByRole('button', { name: 'Tudo' }));
    expect(screen.getByLabelText('Quanto investir')).toHaveValue(formatCents(6_052_000));
    expect(screen.queryByRole('note')).not.toBeInTheDocument();
    await user.click(confirmButton(6_052_000));

    expect(await screen.findByRole('heading', { level: 1, name: 'Compra enviada' })).toBeInTheDocument();
    expect(posts.map((post) => post.body)).toEqual([{ product_id: 'acoesg', amount_cents: 6_052_000 }]);
    expect(screen.getByText(`${shown(6_052_000)} em Maré Ações Globais ETF. O valor saiu do caixa e já aparece na sua carteira.`)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Voltar ao início' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent(new RegExp(`^/client-pov/${SEED.thiago}$`));
    expect(screen.queryByRole('region', { name: 'Caixa parado' })).not.toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Tudo em dia' })).toBeInTheDocument();
    expect(gets.filter((url) => url.endsWith('/screens/home'))).toHaveLength(2);
  });

  it('keeps the form and "Compra enviada" in the screen area on the phone, with no side panel', async () => {
    stubBFF(fernanda);
    const { user, form } = await openCobaltoForFernanda();
    expect(document.querySelector('.pov-app')).toHaveAttribute('data-layout', 'phone');
    expect(screen.queryByRole('complementary', { name: 'Ação' })).not.toBeInTheDocument();
    expect(form.closest('.pov-app__main')).not.toBeNull();
    expect(within(form).getByRole('button', { name: 'Voltar para Investir' })).toBeInTheDocument();

    await user.click(confirmButton(100_000));
    const sent = await screen.findByRole('region', { name: 'Compra enviada' });
    expect(screen.queryByRole('complementary', { name: 'Ação' })).not.toBeInTheDocument();
    expect(sent.closest('.pov-app__main')).not.toBeNull();
  });

  it('opens the form and its confirmation in the side panel on desktop, over an inert Investir screen', async () => {
    useDesktop();
    const { posts, gets } = stubBFF(fernanda);
    const { user } = await openCobaltoForFernanda();
    expect(document.querySelector('.pov-app')).toHaveAttribute('data-layout', 'desktop');
    const panel = screen.getByRole('complementary', { name: 'Ação' });
    const form = within(panel).getByRole('form', { name: 'Investir em Cobalto Semicondutores' });
    expect(within(form).getByRole('heading', { level: 1, name: 'Investir em Cobalto Semicondutores' })).toHaveFocus();
    expect(within(form).getByRole('button', { name: 'Fechar' })).toBeInTheDocument();
    expect(within(form).queryByRole('button', { name: 'Voltar para Investir' })).not.toBeInTheDocument();
    expect(form.closest('.pov-app__main')).toBeNull();
    // The screen stays on view behind the panel, out of reach, as with the other coded panels.
    const main = document.querySelector('.pov-app__main');
    expect(main?.querySelector('.sdui-screen[data-slug="investir"]')).not.toBeNull();
    expect(main).toHaveAttribute('inert');
    expect(screen.queryByRole('button', { name: 'Raio-X SDUI' })).not.toBeInTheDocument();

    await user.click(confirmButton(100_000));
    const sent = await within(panel).findByRole('region', { name: 'Compra enviada' });
    expect(within(sent).getByText('01A0E3B1-7C2D')).toBeInTheDocument();
    expect(posts).toHaveLength(1);
    expect(screen.queryByRole('form')).not.toBeInTheDocument();
    expect(document.querySelector('.pov-app__main')).toHaveAttribute('inert');
    expect(screen.queryByRole('button', { name: 'Raio-X SDUI' })).not.toBeInTheDocument();

    await user.click(within(sent).getByRole('button', { name: 'Fechar' }));
    expect(screen.queryByRole('complementary', { name: 'Ação' })).not.toBeInTheDocument();
    expect(document.querySelector('.pov-app__main')).not.toHaveAttribute('inert');
    expect(screen.getByRole('button', { name: 'Raio-X SDUI' })).toBeInTheDocument();
    await vi.waitFor(() => expect(gets.filter((url) => url.endsWith('/screens/investir'))).toHaveLength(2));
  });

  it('closes the desktop form panel from its close control, the scrim, and on navigate', async () => {
    useDesktop();
    stubBFF(fernanda);
    const { user } = await openCobaltoForFernanda();
    await user.click(within(screen.getByRole('complementary', { name: 'Ação' })).getByRole('button', { name: 'Fechar' }));
    expect(screen.queryByRole('complementary', { name: 'Ação' })).not.toBeInTheDocument();
    expect(document.querySelector('.pov-app__main')).not.toHaveAttribute('inert');

    const reopen = async () => {
      const stocks = await screen.findByRole('region', { name: 'Ações' });
      await user.click(within(stocks).getByRole('button', { name: /Cobalto Semicondutores/ }));
      expect(within(screen.getByRole('complementary', { name: 'Ação' })).getByRole('form')).toBeInTheDocument();
    };
    await reopen();
    await user.click(screen.getByRole('button', { name: 'Fechar painel' }));
    expect(screen.queryByRole('complementary', { name: 'Ação' })).not.toBeInTheDocument();

    await reopen();
    await user.click(within(screen.getByRole('navigation', { name: 'Navegação principal' })).getByRole('button', { name: 'Início' }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(screen.queryByRole('form')).not.toBeInTheDocument();
    expect(screen.queryByRole('complementary', { name: 'Ação' })).not.toBeInTheDocument();
  });
});
