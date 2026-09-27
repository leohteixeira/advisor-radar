import type { DecoratedCase, DecoratedSignal } from '../domain/queueVisual';
import { CaseCard } from './CaseCard';
import { SignalCard } from './SignalCard';

export const COLUMN_LABELS = [
  'Novos sinais',
  'Aberto',
  'Em atendimento',
  'Aguardando cliente',
] as const;

const ACCENTS = ['crit', 'info', 'brand', 'warn'] as const;

interface QueueBoardProps {
  signals: DecoratedSignal[];
  columns: { key: string; label: string; accent: (typeof ACCENTS)[number] | 'ok'; cases: DecoratedCase[] }[];
  showSignals: boolean;
  desktop: boolean;
  loading: boolean;
  error: boolean;
  emptyTitle: string;
  emptyText: string;
  onRetry: () => void;
  onViewCase: (id: string) => void;
  onOpenCase: (id: string) => void;
  onContacted: (id: string) => void;
  onSnooze: (id: string) => void;
  onAdvanceCase: (id: string) => void;
}

export function QueueBoard({
  signals,
  columns,
  showSignals,
  desktop,
  loading,
  error,
  emptyTitle,
  emptyText,
  onRetry,
  onViewCase,
  onOpenCase,
  onContacted,
  onSnooze,
  onAdvanceCase,
}: QueueBoardProps) {
  const visible: {
    key: string;
    label: string;
    accent: 'crit' | 'info' | 'brand' | 'warn' | 'ok';
    kind: 'signals' | 'cases';
    cases: DecoratedCase[];
  }[] = [
    ...(showSignals
      ? [
          {
            key: 'novos',
            label: COLUMN_LABELS[0],
            accent: 'crit' as const,
            kind: 'signals' as const,
            cases: [] as DecoratedCase[],
          },
        ]
      : []),
    ...columns.map((col) => ({
      key: col.key,
      label: col.label,
      accent: col.accent,
      kind: 'cases' as const,
      cases: col.cases,
    })),
  ];

  return (
    <div
      className={desktop ? 'queue-board queue-board--desktop' : 'queue-board queue-board--phone'}
      data-testid="queue-board"
      style={desktop ? { gridTemplateColumns: `repeat(${Math.max(visible.length, 1)}, minmax(0, 1fr))` } : undefined}
    >
      {visible.map((col) => {
        const count = col.kind === 'signals' ? signals.length : col.cases.length;
        const isEmpty =
          col.kind === 'signals' ? !loading && !error && signals.length === 0 : col.cases.length === 0 && !loading;
        return (
          <section key={col.key} className={`queue-column queue-column--${col.accent}`} aria-label={col.label}>
            <header className="queue-column__header">
              <span>{col.label}</span>
              <span className="queue-column__count">{loading && col.kind === 'signals' ? '' : count}</span>
            </header>
            <div className="queue-column__body">
              {col.kind === 'signals' && loading
                ? [0, 1, 2, 3].map((n) => (
                    <div key={n} className="queue-skeleton" aria-hidden>
                      <div className="queue-skeleton__row">
                        <span style={{ width: '45%' }} />
                        <span style={{ width: '18%' }} />
                      </div>
                      <span className="queue-skeleton__line" style={{ width: '35%' }} />
                      <span className="queue-skeleton__line queue-skeleton__line--body" style={{ width: '90%' }} />
                      <div className="queue-skeleton__row">
                        <span className="queue-skeleton__chip" style={{ width: '30%' }} />
                        <span className="queue-skeleton__chip" style={{ width: '22%' }} />
                      </div>
                    </div>
                  ))
                : null}
              {col.kind === 'signals' && error ? (
                <div className="queue-error">
                  <div className="queue-error__mark">!</div>
                  <strong>Não foi possível carregar a fila</strong>
                  <p>O serviço de prioridade não respondeu. Seus casos abertos continuam acessíveis pela visão do cliente.</p>
                  <button type="button" onClick={onRetry}>
                    Tentar novamente
                  </button>
                </div>
              ) : null}
              {isEmpty ? (
                <div className={col.kind === 'signals' ? 'queue-empty queue-empty--signals' : 'queue-empty'}>
                  {col.kind === 'signals' ? (
                    <div className="queue-empty__mark" aria-hidden>
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M4 12l5 5L20 7" />
                      </svg>
                    </div>
                  ) : null}
                  <strong>{col.kind === 'signals' ? emptyTitle : 'Nenhum caso'}</strong>
                  <p>
                    {col.kind === 'signals'
                      ? emptyText
                      : col.label === 'Aberto'
                        ? 'Abra um caso a partir de um sinal.'
                        : col.label === 'Em atendimento'
                          ? 'Nenhum caso em atendimento.'
                          : col.label === 'Aguardando cliente'
                            ? 'Nenhum caso aguardando resposta.'
                            : 'Nenhum caso resolvido hoje.'}
                  </p>
                </div>
              ) : null}
              {col.kind === 'signals' && !loading && !error
                ? signals.map((row) => (
                    <SignalCard
                      key={row.signal.id}
                      row={row}
                      onOpenCase={onOpenCase}
                      onContacted={onContacted}
                      onSnooze={onSnooze}
                    />
                  ))
                : null}
              {col.kind === 'cases'
                ? col.cases.map((row) => (
                    <CaseCard
                      key={row.caseItem.id}
                      row={row}
                      onAdvance={onAdvanceCase}
                      onOpen={onViewCase}
                    />
                  ))
                : null}
            </div>
          </section>
        );
      })}
    </div>
  );
}
