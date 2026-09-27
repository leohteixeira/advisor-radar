import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import type { ManagerSnapshot, ReviewRow, Signal } from '../domain/types';
import { ManagerScreen } from './ManagerScreen';
import { ReviewScreen } from './ReviewScreen';

const reviewSeed: ReviewRow[] = [
  {
    id: 'r01',
    client: 'c05',
    text: 'Preciso sacar 5 mil dólares e trazer de volta para o Brasil.',
    dist: { Resgate: 0.54, Câmbio: 0.31, Operacional: 0.1, Encerramento: 0.05 },
    ago: 33,
    intent: 'Resgate',
  },
  {
    id: 'r05',
    client: 'c01',
    text: 'Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.',
    dist: { Reclamação: 0.82, Encerramento: 0.11, Operacional: 0.04, Resgate: 0.03 },
    ago: 12,
    intent: 'Reclamação',
  },
  {
    id: 'r06',
    client: 'c06',
    text: 'Meu cartão foi recusado na viagem, o que eu faço?',
    dist: { Operacional: 0.71, Reclamação: 0.19, Contato: 0.1 },
    ago: 50,
    fallback: true,
    intent: 'Operacional',
  },
];

const queueSeed: Signal[] = [
  {
    id: 's01',
    kind: 'message',
    client: 'c01',
    text: 'Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.',
    intent: 'Reclamação',
  },
];

const managerSeed: ManagerSnapshot = {
  backlog: [
    { advisor: 'Ana Paula Ribeiro', open: 17, risk: 4, overdue: 1 },
    { advisor: 'Bruno Dias', open: 9, risk: 1, overdue: 0 },
  ],
  avgFirstContactMin: 14,
  avgFirstContactYesterday: 19,
  reviewPct: 18,
  fallbackPct: 6,
  intents: { Operacional: 31, Tributação: 22 },
  atRisk: [
    {
      id: 'k1042',
      client: 'Paulo Henrique Souza',
      advisor: 'Ana Paula Ribeiro',
      segment: 'Advance',
      remaining: 46,
    },
    {
      id: 'k1044',
      client: 'Sérgio Cardoso',
      advisor: 'Ana Paula Ribeiro',
      segment: 'Singular',
      remaining: -8,
    },
  ],
};

function mockFetch(
  rows: ReviewRow[] = structuredClone(reviewSeed),
  opts: { failCorrect?: boolean } = {},
) {
  const store = { rows };

  class FakeEventSource {
    close = vi.fn();
    addEventListener = vi.fn();
  }
  vi.stubGlobal('EventSource', FakeEventSource);

  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';

      if (url.endsWith('/v1/review') && method === 'GET') {
        return new Response(JSON.stringify({ items: store.rows }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }

      const correct = url.match(/\/v1\/review\/([^/?]+)$/);
      if (correct && method === 'PUT') {
        if (opts.failCorrect) {
          return new Response('', { status: 500 });
        }
        const id = correct[1];
        const body = JSON.parse(String(init?.body ?? '{}')) as { intent?: string };
        const current = store.rows.find((r) => r.id === id);
        if (!current) {
          return new Response('', { status: 404 });
        }
        const updated: ReviewRow = {
          id: current.id,
          client: current.client,
          text: current.text,
          dist: current.dist,
          ago: current.ago,
          fallback: current.fallback,
          intent: body.intent ?? current.intent,
          corrected: true,
        };
        store.rows = store.rows.map((r) => (r.id === id ? updated : r));
        return new Response(JSON.stringify(updated), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }

      if (url.endsWith('/v1/queue') && method === 'GET') {
        return new Response(JSON.stringify({ items: queueSeed }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }

      if (url.endsWith('/v1/cases') && method === 'GET') {
        return new Response(JSON.stringify({ items: [], states: [] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }

      if (url.endsWith('/v1/manager') && method === 'GET') {
        return new Response(JSON.stringify(managerSeed), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }

      return new Response('not found', { status: 404 });
    }),
  );
  return store;
}

describe('ReviewScreen', () => {
  beforeEach(() => {
    mockFetch();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('corrects r05 to Operacional and confirms feedback', async () => {
    const user = userEvent.setup();
    render(<ReviewScreen />);

    const card = await screen.findByTestId('review-r05');
    expect(within(card).getAllByText('Reclamação').length).toBeGreaterThan(0);
    expect(within(card).getByText('82%')).toBeTruthy();

    await user.click(within(card).getByRole('button', { name: /Operacional/i }));

    await waitFor(() => {
      expect(within(card).getByRole('status')).toHaveTextContent('Corrigido para Operacional');
    });
    expect(screen.getByText(/Obrigado pelo feedback/)).toBeTruthy();
  });

  it('confirms the suggested intent on r05', async () => {
    const user = userEvent.setup();
    render(<ReviewScreen />);

    const card = await screen.findByTestId('review-r05');
    await user.click(within(card).getByRole('button', { name: 'Confirmar Reclamação' }));

    await waitFor(() => {
      expect(within(card).getByRole('status')).toHaveTextContent('Confirmado como Reclamação');
    });
    expect(screen.getByText('Confirmado: Reclamação')).toBeTruthy();
  });

  it('keeps cards and shows a toast when correction fails', async () => {
    mockFetch(structuredClone(reviewSeed), { failCorrect: true });
    const user = userEvent.setup();
    render(<ReviewScreen />);

    const card = await screen.findByTestId('review-r05');
    await user.click(within(card).getByRole('button', { name: /Operacional/i }));

    await waitFor(() => {
      expect(screen.getByText('Não foi possível corrigir. Tente de novo.')).toBeTruthy();
    });
    expect(screen.getByTestId('review-r05')).toBeTruthy();
    expect(screen.getByTestId('review-r01')).toBeTruthy();
    expect(within(card).queryByRole('status')).toBeNull();
  });

  it('returns to the advisor queue when persona switches back to Fila', async () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });

    const user = userEvent.setup();
    render(<App />);

    expect(await screen.findByText('Novos sinais')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Revisão de triagem' }));
    expect(await screen.findByTestId('review-r05')).toBeTruthy();
    expect(screen.queryByText('Novos sinais')).toBeNull();

    await user.click(screen.getByRole('button', { name: 'Painel da assessoria' }));
    expect(await screen.findByTestId('backlog-Ana Paula Ribeiro')).toBeTruthy();
    expect(screen.getByText('6%')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Fila' }));
    expect(await screen.findByText('Novos sinais')).toBeTruthy();
    expect(screen.queryByTestId('review-r05')).toBeNull();
    expect(screen.queryByTestId('backlog-Ana Paula Ribeiro')).toBeNull();
  });

  it('shows backlog, first contact, review rate, and k1042', async () => {
    render(<ManagerScreen />);

    const backlog = await screen.findByTestId('backlog-Ana Paula Ribeiro');
    expect(within(backlog).getByText('17')).toBeTruthy();
    expect(screen.getByText(/14/)).toBeTruthy();
    expect(screen.getByText('18%')).toBeTruthy();
    expect(screen.getByText('6%')).toBeTruthy();
    expect(screen.getByTestId('risk-k1042')).toHaveTextContent('46 min');
    expect(screen.getByTestId('risk-k1044')).toHaveTextContent('vencido há 8 min');
  });
});
