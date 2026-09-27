import type { ReactNode } from 'react';
import { CaseCard } from './CaseCard';
import { SignalCard } from './SignalCard';
import type { CaseItem, Signal } from '../domain/types';

export const COLUMN_LABELS = [
  'Novos sinais',
  'Aberto',
  'Em atendimento',
  'Aguardando cliente',
] as const;

interface QueueBoardProps {
  signals: Signal[];
  cases: CaseItem[];
  desktop: boolean;
  onOpen: (id: string) => void;
  onOpenCase: (id: string) => void;
  onContacted: (id: string) => void;
  onSnooze: (id: string) => void;
  onAdvanceCase: (id: string) => void;
}

export function QueueBoard({
  signals,
  cases,
  desktop,
  onOpen,
  onOpenCase,
  onContacted,
  onSnooze,
  onAdvanceCase,
}: QueueBoardProps) {
  const byState = (state: number) => cases.filter((c) => c.state === state);

  const columns: { key: string; label: string; body: ReactNode }[] = [
    {
      key: 'novos',
      label: COLUMN_LABELS[0],
      body: signals.map((s) => (
        <SignalCard
          key={s.id}
          signal={s}
          onOpen={onOpen}
          onOpenCase={onOpenCase}
          onContacted={onContacted}
          onSnooze={onSnooze}
        />
      )),
    },
    {
      key: 'aberto',
      label: COLUMN_LABELS[1],
      body: byState(0).map((c) => (
        <CaseCard key={c.id} caseItem={c} onAdvance={onAdvanceCase} />
      )),
    },
    {
      key: 'atendimento',
      label: COLUMN_LABELS[2],
      body: byState(1).map((c) => (
        <CaseCard key={c.id} caseItem={c} onAdvance={onAdvanceCase} />
      )),
    },
    {
      key: 'aguardando',
      label: COLUMN_LABELS[3],
      body: byState(2).map((c) => (
        <CaseCard key={c.id} caseItem={c} onAdvance={onAdvanceCase} />
      )),
    },
  ];

  return (
    <div
      className={desktop ? 'queue-board queue-board--desktop' : 'queue-board queue-board--phone'}
      data-testid="queue-board"
    >
      {columns.map((col) => (
        <section key={col.key} className="queue-column" aria-label={col.label}>
          <header className="queue-column__header">
            <span>{col.label}</span>
          </header>
          <div className="queue-column__body">{col.body}</div>
        </section>
      ))}
    </div>
  );
}
