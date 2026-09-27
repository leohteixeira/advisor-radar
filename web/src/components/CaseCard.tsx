import { clientName, clientSegment } from '../domain/clients';
import { CASE_STATES } from '../domain/types';
import type { CaseItem } from '../domain/types';

interface CaseCardProps {
  caseItem: CaseItem;
  onAdvance: (id: string) => void;
}

export function CaseCard({ caseItem, onAdvance }: CaseCardProps) {
  const name = clientName(caseItem.client);
  const segment = clientSegment(caseItem.client);
  const stateLabel = CASE_STATES[caseItem.state] ?? 'Aberto';
  const canAdvance = caseItem.state < CASE_STATES.length - 1;

  return (
    <article className="case-card" data-testid={`case-${caseItem.id}`}>
      <div className="case-card__top">
        <strong className="case-card__name">{name}</strong>
        <span className="case-card__id">{caseItem.id}</span>
      </div>
      <p className="case-card__meta">
        {segment} · {stateLabel}
        {caseItem.escalated ? ' · Escalonado' : ''}
      </p>
      {canAdvance ? (
        <button type="button" onClick={() => onAdvance(caseItem.id)}>
          Avançar
        </button>
      ) : null}
    </article>
  );
}
