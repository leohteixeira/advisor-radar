import { describe, expect, it } from 'vitest';
import {
  ICONS,
  caseOutcome,
  countLabel,
  decorateCase,
  decorateSignal,
  motivoOptions,
  signalSla,
  slaFacetsFromCards,
} from './queueVisual';
import type { CaseItem, Signal } from './types';

const perfil: Signal = {
  id: 'perfil-1',
  kind: 'alert',
  client: 'customer-1',
  name: 'Fernanda Lima',
  segment: 'Essencial',
  alert: 'perfil',
  ago: 5,
};

describe('perfil alert', () => {
  it('decorates with its label and gauge icon', () => {
    const row = decorateSignal(perfil);
    expect(row.typeLabel).toBe('Compra acima do perfil');
    expect(row.icon).toBe(ICONS.perfil);
    expect(row.conf).toBeNull();
  });

  it('is a motivo option next to the other alert kinds', () => {
    const options = motivoOptions();
    expect(options).toContain('Compra acima do perfil');
    expect(options.indexOf('Compra acima do perfil')).toBe(options.indexOf('Sem contato há muito tempo') + 1);
  });
});

const openCase: CaseItem = {
  id: 'case-1',
  client: 'customer-1',
  signal: 'signal-1',
  state: 0,
  openedAgo: 1200,
  slaTotal: 240,
  slaRemaining: 180,
  slaState: 'No prazo',
  escalated: false,
  history: [],
};

describe('decorateCase', () => {
  it.each([
    { name: 'on time', slaRemaining: 180, slaState: 'No prazo', countdown: '03:00:00', tone: 'ok' },
    // The state is the backend's even where a third of the SLA would say otherwise.
    { name: 'due soon by the backend', slaRemaining: 180, slaState: 'Vencendo', countdown: '03:00:00', tone: 'due' },
    { name: 'overdue', slaRemaining: -65, slaState: 'Vencido', countdown: '−01:05:00', tone: 'over' },
  ])('presents the SLA the BFF sends: $name', ({ slaRemaining, slaState, countdown, tone }) => {
    const row = decorateCase({ ...openCase, slaRemaining, slaState });
    expect(row.slaState).toBe(slaState);
    expect(row.countdown).toBe(countdown);
    expect(row.slaTone).toBe(tone);
  });

  it('shows no clock on a resolved case', () => {
    const row = decorateCase({ ...openCase, state: 3, slaRemaining: -65, slaState: 'Concluído' });
    expect(row.countdown).toBe('—');
    expect(row.slaTone).toBe('done');
  });

  it('names the origin only when the BFF sends one', () => {
    expect(decorateCase({ ...openCase, origin: 'saque relevante' }).origin).toBe('saque relevante');
    expect(decorateCase(openCase).origin).toBe('');
  });
});

describe('caseOutcome', () => {
  const joined: CaseItem = {
    ...openCase,
    id: 'case-2',
    signal: 'signal-0',
    history: [{ ago: 0, kind: 'signal', text: 'Sinal adicionado', signal: 'signal-2' }],
  };

  it.each([
    { name: 'a case opened for the signal', cases: [openCase], signal: 'signal-1', want: 'opened' },
    { name: 'a history line about the signal', cases: [openCase, joined], signal: 'signal-2', want: 'joined' },
    { name: 'a resolved case of the signal is not a new one', cases: [{ ...openCase, state: 3 }], signal: 'signal-1', want: null },
    { name: 'nothing yet', cases: [openCase, joined], signal: 'signal-9', want: null },
  ])('$name', ({ cases, signal, want }) => {
    expect(caseOutcome(cases, signal)).toBe(want);
  });
});

describe('signal SLA state', () => {
  // The BFF filters and counts with whole minutes: half of the SLA, and "due"
  // below 33% of it (at least 20 min). The card must land on the same side.
  it.each([
    { segment: 'Essencial', human: true, ago: 483, state: 'No prazo' }, // 720 total, 237 left, due below 237
    { segment: 'Essencial', human: true, ago: 484, state: 'Vencendo' },
    { segment: 'Advance', human: false, ago: 161, state: 'No prazo' }, // 240 total, 79 left, due below 79
    { segment: 'Advance', human: false, ago: 162, state: 'Vencendo' },
    { segment: 'Singular', human: true, ago: 10, state: 'No prazo' }, // 30 total, 20 left, due below 20
    { segment: 'Singular', human: true, ago: 11, state: 'Vencendo' },
    { segment: 'Singular', human: true, ago: 30, state: 'Vencendo' },
    { segment: 'Singular', human: true, ago: 31, state: 'Vencido' },
  ])('$segment, $ago min old is $state', ({ segment, human, ago, state }) => {
    const signal: Signal = { id: 's', kind: 'message', client: 'c', segment, human, ago };
    expect(signalSla(signal, segment).state).toBe(state);
  });
});

describe('SLA facets from the cards', () => {
  const server = [
    { label: 'No prazo', count: 15 },
    { label: 'Vencendo', count: 4 },
    { label: 'Vencido', count: 2 },
  ];

  it('counts every value from the cards while no SLA value is picked', () => {
    const cards = ['No prazo', 'Vencendo', 'Vencido', 'Vencido', 'Concluído'];
    expect(slaFacetsFromCards(server, cards, [])).toEqual([
      { label: 'No prazo', count: 1 },
      { label: 'Vencendo', count: 1 },
      { label: 'Vencido', count: 2 },
    ]);
  });

  it('counts only the picked values from the cards and keeps the others as sent', () => {
    expect(slaFacetsFromCards(server, ['Vencido', 'Vencido', 'Vencido'], ['Vencido'])).toEqual([
      { label: 'No prazo', count: 15 },
      { label: 'Vencendo', count: 4 },
      { label: 'Vencido', count: 3 },
    ]);
  });
});

describe('countLabel', () => {
  it('picks the singular only for one', () => {
    expect([0, 1, 2].map((n) => countLabel(n, 'sinal', 'sinais'))).toEqual(['sinais', 'sinal', 'sinais']);
  });
});
