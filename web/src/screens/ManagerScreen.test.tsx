import { render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ManagerSnapshot } from '../domain/types';
import { ManagerScreen } from './ManagerScreen';

function snapshot(over: Partial<ManagerSnapshot> = {}): ManagerSnapshot {
  return {
    backlog: [],
    avgFirstContactMin: 14,
    avgFirstContactYesterday: 19,
    reviewPct: 83,
    fallbackPct: 61,
    intents: { Tributação: 33, Reclamação: 50, Operacional: 17 },
    atRisk: [],
    ...over,
  };
}

function stubManager(snap: ManagerSnapshot) {
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(JSON.stringify(snap), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    ),
  );
}

async function contactCard() {
  const label = await screen.findByText('Alerta → primeiro contato');
  return label.parentElement as HTMLElement;
}

describe('ManagerScreen', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('lists each intent once with its share, largest first', async () => {
    stubManager(snapshot());
    render(<ManagerScreen />);

    const heading = await screen.findByRole('heading', { name: 'Intenções no dia' });
    const rows = Array.from((heading.parentElement as HTMLElement).querySelectorAll('.manager-intent'));
    expect(rows.map((row) => row.textContent)).toEqual(['Reclamação50%', 'Tributação33%', 'Operacional17%']);
  });

  it('shows the share of messages awaiting review', async () => {
    stubManager(snapshot());
    render(<ManagerScreen />);

    const label = await screen.findByText('Mensagens em revisão humana');
    const card = within(label.parentElement as HTMLElement);
    expect(card.getByText('83%')).toBeInTheDocument();
    expect(card.getByText('das mensagens aguardam revisão')).toBeInTheDocument();
  });

  it.each([
    { today: 14, yesterday: 19, text: '−5 min vs. ontem', tone: 'metric-card__note--ok' },
    { today: 24, yesterday: 19, text: '+5 min vs. ontem', tone: 'metric-card__note--crit' },
    { today: 19, yesterday: 19, text: 'igual a ontem', tone: null },
    { today: 24, yesterday: 0, text: 'sem contatos ontem', tone: null },
  ])('renders the first-contact delta $today vs $yesterday with a single sign', async ({ today, yesterday, text, tone }) => {
    stubManager(snapshot({ avgFirstContactMin: today, avgFirstContactYesterday: yesterday }));
    render(<ManagerScreen />);

    const note = within(await contactCard()).getByText(text);
    expect(note.textContent).toBe(text);
    expect(note.className).toBe(tone ? `metric-card__note ${tone}` : 'metric-card__note');
  });

  it.each([
    { pct: 42, note: 'das mensagens classificadas sem o modelo externo' },
    { pct: 0, note: 'todas as mensagens passaram pelo modelo externo' },
  ])('describes a $pct% simplified-classification share without calling the model healthy', async ({ pct, note }) => {
    stubManager(snapshot({ fallbackPct: pct }));
    render(<ManagerScreen />);

    const label = await screen.findByText('Classificação simplificada');
    const card = within(label.parentElement as HTMLElement);
    expect(card.getByText(`${pct}%`)).toBeInTheDocument();
    expect(card.getByText(note)).toBeInTheDocument();
    expect(screen.queryByText(/saudável/)).not.toBeInTheDocument();
  });

  it('says so when no case is at risk', async () => {
    stubManager(snapshot());
    render(<ManagerScreen />);

    const heading = await screen.findByRole('heading', { name: 'Casos com SLA em risco' });
    const panel = within(heading.parentElement as HTMLElement);
    expect(panel.getByText('Nenhum caso com SLA em risco agora.')).toBeInTheDocument();
    expect(screen.getByText('0 vencidos')).toBeInTheDocument();
  });

  it('names an at-risk case by its short code, never by the raw id', async () => {
    const id = '01a0e3a4-9a44-76f2-9263-f892becdd32a';
    stubManager(
      snapshot({
        atRisk: [{ id, client: 'Ana Beatriz Oliveira', advisor: 'Ana Paula Ribeiro', segment: 'Advance', remaining: -5 }],
      }),
    );
    render(<ManagerScreen />);

    const row = await screen.findByTestId(`risk-${id}`);
    expect(row).toHaveTextContent('Ana Beatriz Oliveira Advance');
    expect(row).toHaveTextContent('Ana Paula Ribeiro · Caso 01A0E3A4-9A44');
    expect(row).not.toHaveTextContent(id);
    expect(screen.queryByText('Nenhum caso com SLA em risco agora.')).not.toBeInTheDocument();
    expect(screen.getByText('1 vencido')).toBeInTheDocument();
  });
});
