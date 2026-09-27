import { useEffect, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { CaseItem, Signal } from '../domain/types';
import { ALERT_LABELS, CASE_STATES } from '../domain/types';

const FRUSTRATION = ['Calmo', 'Incomodado', 'Frustrado', 'Muito frustrado'];

export type DetailSelection =
  | { type: 'signal'; signal: Signal }
  | { type: 'case'; caseItem: CaseItem };

interface DetailModalProps {
  selection: DetailSelection;
  onClose: () => void;
  desktop: boolean;
}

function maxProb(dist?: Record<string, number>): number {
  if (!dist) {
    return 1;
  }
  const vals = Object.values(dist);
  if (vals.length === 0) {
    return 1;
  }
  return Math.max(...vals);
}

function needsReview(signal: Signal): boolean {
  return signal.kind === 'message' && maxProb(signal.dist) < 0.85;
}

function typeLabel(signal: Signal): string {
  if (signal.kind === 'alert') {
    return ALERT_LABELS[signal.alert ?? ''] ?? 'Alerta';
  }
  if (signal.churn || (signal.frustration ?? 0) >= 2) {
    return 'Mensagem com risco';
  }
  return signal.intent ?? 'Mensagem';
}

function displayName(item: { name?: string; client: string }): string {
  return item.name ?? item.client;
}

function displaySegment(item: { segment?: string }): string {
  return item.segment ?? 'Essencial';
}

function firstName(item: { name?: string; client: string }): string {
  const full = displayName(item);
  return full.split(' ')[0] ?? full;
}

function MessageDetail({ signal }: { signal: Signal }) {
  const name = displayName(signal);
  const segment = displaySegment(signal);
  const frust = FRUSTRATION[signal.frustration ?? 0] ?? 'Calmo';
  const review = needsReview(signal);

  return (
    <div className="detail detail--message" data-testid="signal-detail">
      <p className="detail__eyebrow">Mensagem triada</p>
      {signal.fallback ? (
        <span className="badge badge--fallback" data-testid="fallback-badge">
          Classificação simplificada
        </span>
      ) : null}
      <h2 className="detail__title">{name}</h2>
      <p className="detail__sub">
        {segment}
        {signal.channel ? ` · ${signal.channel}` : ''}
      </p>
      {signal.text ? <p className="detail__quote">“{signal.text}”</p> : null}
      <div className="detail__tags">
        {signal.intent ? <span className="badge">{signal.intent}</span> : null}
        {(signal.frustration ?? 0) >= 1 ? (
          <span className="badge badge--frust">{frust}</span>
        ) : null}
        <span className="badge badge--type">{typeLabel(signal)}</span>
        {review ? (
          <span className="badge badge--review" data-testid="review-badge">
            Precisa de revisão
          </span>
        ) : null}
      </div>
      {signal.contacted_at ? <p className="detail__contacted">Contatado</p> : null}
      <Link to={`/clientes/${signal.client}`} className="detail__360">
        Ver visão 360 de {firstName(signal)} →
      </Link>
    </div>
  );
}

function AlertDetail({ signal }: { signal: Signal }) {
  const name = displayName(signal);
  const segment = displaySegment(signal);
  const label = ALERT_LABELS[signal.alert ?? ''] ?? 'Alerta';

  return (
    <div className="detail detail--alert" data-testid="signal-detail">
      <p className="detail__eyebrow">Alerta de conta</p>
      <h2 className="detail__title">{name}</h2>
      <p className="detail__sub">{segment}</p>
      <p className="detail__alert-label" data-testid="alert-label">
        {label}
      </p>
      {signal.reason ? (
        <p className="detail__reason" data-testid="alert-reason">
          {signal.reason}
        </p>
      ) : null}
      {signal.rule ? <p className="detail__rule">{signal.rule}</p> : null}
      {signal.contacted_at ? <p className="detail__contacted">Contatado</p> : null}
      <Link to={`/clientes/${signal.client}`} className="detail__360">
        Ver visão 360 de {firstName(signal)} →
      </Link>
    </div>
  );
}

function CaseDetail({ caseItem }: { caseItem: CaseItem }) {
  const name = displayName(caseItem);
  const segment = displaySegment(caseItem);
  const stateLabel = CASE_STATES[caseItem.state] ?? 'Aberto';

  return (
    <div className="detail detail--case" data-testid="case-detail">
      <p className="detail__eyebrow">
        Caso <span className="detail__case-id">{caseItem.id}</span>
      </p>
      <h2 className="detail__title">{name}</h2>
      <p className="detail__sub">
        {segment} · {stateLabel}
      </p>
      <ol className="case-rail" data-testid="case-rail" aria-label="Etapas do caso">
        {CASE_STATES.map((label, i) => {
          const done = i < caseItem.state;
          const current = i === caseItem.state;
          return (
            <li
              key={label}
              className={
                current
                  ? 'case-rail__step case-rail__step--current'
                  : done
                    ? 'case-rail__step case-rail__step--done'
                    : 'case-rail__step'
              }
              aria-current={current ? 'step' : undefined}
            >
              <span className="case-rail__dot" />
              <span className="case-rail__label">{label}</span>
            </li>
          );
        })}
      </ol>
      <Link to={`/clientes/${caseItem.client}`} className="detail__360">
        Ver visão 360 de {firstName(caseItem)} →
      </Link>
    </div>
  );
}

export function DetailModal({ selection, onClose, desktop }: DetailModalProps) {
  useEffect(() => {
    if (!desktop) {
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [desktop, onClose]);

  let body: ReactNode;
  let dialogLabel = 'Detalhe';
  if (selection.type === 'case') {
    dialogLabel = displayName(selection.caseItem);
    body = <CaseDetail caseItem={selection.caseItem} />;
  } else if (selection.signal.kind === 'alert') {
    dialogLabel = displayName(selection.signal);
    body = <AlertDetail signal={selection.signal} />;
  } else {
    dialogLabel = displayName(selection.signal);
    body = <MessageDetail signal={selection.signal} />;
  }

  const wrapped = (
    <div className="detail-shell">
      <button type="button" className="detail__close" onClick={onClose} aria-label="Fechar">
        ✕
      </button>
      {body}
    </div>
  );

  if (!desktop) {
    return wrapped;
  }

  return (
    <div
      className="detail-modal-backdrop"
      data-testid="detail-modal"
      onClick={onClose}
      role="presentation"
    >
      <div
        className="detail-modal"
        role="dialog"
        aria-modal="true"
        aria-label={dialogLabel}
        onClick={(e) => e.stopPropagation()}
      >
        {wrapped}
      </div>
    </div>
  );
}
