import type { DecoratedCase } from '../domain/queueVisual';

interface CaseCardProps {
  row: DecoratedCase;
  onAdvance: (id: string) => void;
  onOpen: (id: string) => void;
}

export function CaseCard({ row, onAdvance, onOpen }: CaseCardProps) {
  const { caseItem } = row;
  const canAdvance = caseItem.state < 3 && row.nextLabel !== '';

  return (
    <article className={`case-card case-card--${row.slaTone}`} data-testid={`case-${caseItem.id}`}>
      <button
        type="button"
        className="case-card__open"
        onClick={() => onOpen(caseItem.id)}
        aria-label={`Abrir caso ${caseItem.id}`}
      >
        <div className="case-card__top">
          <strong className="case-card__name">{row.name}</strong>
          <span className={`case-card__count case-card__count--${row.slaTone}`}>{row.countdown}</span>
        </div>
        <div className="case-card__badges">
          <span className={`signal-card__segment signal-card__segment--${row.segment.toLowerCase()}`}>
            {row.segment}
          </span>
          <span className={`case-card__state case-card__state--${caseItem.state}`}>SLA {row.slaState}</span>
        </div>
        <p className="case-card__meta">
          <span className="case-card__id">{caseItem.id}</span> · {row.origin}
        </p>
        {caseItem.escalated ? (
          <span className="case-card__escalated">Escalonado automaticamente</span>
        ) : null}
        {row.lastText ? <p className="case-card__last">{row.lastText}</p> : null}
      </button>
      {canAdvance ? (
        <div className="case-card__actions">
          <button type="button" className="case-card__advance" onClick={() => onAdvance(caseItem.id)}>
            {row.nextLabel} →
          </button>
        </div>
      ) : null}
    </article>
  );
}
