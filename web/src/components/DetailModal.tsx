import { useEffect, type ReactNode } from 'react';
import type { CaseItem, Signal } from '../domain/types';
import { ALERT_LABELS, CASE_STATES } from '../domain/types';
import { CLIENTS, clientName, clientSegment } from '../domain/clients';
import { Timeline360 } from './Timeline360';

const FRUSTRATION = ['Calmo', 'Incomodado', 'Frustrado', 'Muito frustrado'];

export type DetailSelection =
  | { type: 'signal'; signal: Signal }
  | { type: 'case'; caseItem: CaseItem }
  | { type: 'client'; clientId: string };

interface DetailModalProps {
  selection: DetailSelection;
  onClose: () => void;
  onOpenClient: (clientId: string) => void;
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

function firstName(clientId: string): string {
  return clientName(clientId).split(' ')[0] ?? clientName(clientId);
}

function MessageDetail({
  signal,
  onOpenClient,
}: {
  signal: Signal;
  onOpenClient: (id: string) => void;
}) {
  const name = clientName(signal.client);
  const segment = clientSegment(signal.client);
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
      <button
        type="button"
        className="detail__360"
        onClick={() => onOpenClient(signal.client)}
      >
        Ver visão 360 de {firstName(signal.client)} →
      </button>
    </div>
  );
}

function AlertDetail({
  signal,
  onOpenClient,
}: {
  signal: Signal;
  onOpenClient: (id: string) => void;
}) {
  const name = clientName(signal.client);
  const segment = clientSegment(signal.client);
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
      <button
        type="button"
        className="detail__360"
        onClick={() => onOpenClient(signal.client)}
      >
        Ver visão 360 de {firstName(signal.client)} →
      </button>
    </div>
  );
}

function CaseDetail({
  caseItem,
  onOpenClient,
}: {
  caseItem: CaseItem;
  onOpenClient: (id: string) => void;
}) {
  const name = clientName(caseItem.client);
  const segment = clientSegment(caseItem.client);
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
      <button
        type="button"
        className="detail__360"
        onClick={() => onOpenClient(caseItem.client)}
      >
        Ver visão 360 de {firstName(caseItem.client)} →
      </button>
    </div>
  );
}

export function DetailModal({ selection, onClose, onOpenClient, desktop }: DetailModalProps) {
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
  if (selection.type === 'client') {
    const info = CLIENTS[selection.clientId];
    dialogLabel = info?.name ?? selection.clientId;
    body = (
      <Timeline360
        clientId={selection.clientId}
        name={info?.name ?? selection.clientId}
        segment={info?.segment ?? 'Essencial'}
        aum={info?.aum}
        advisor={info?.advisor}
        since={info?.since}
      />
    );
  } else if (selection.type === 'case') {
    dialogLabel = clientName(selection.caseItem.client);
    body = <CaseDetail caseItem={selection.caseItem} onOpenClient={onOpenClient} />;
  } else if (selection.signal.kind === 'alert') {
    dialogLabel = clientName(selection.signal.client);
    body = <AlertDetail signal={selection.signal} onOpenClient={onOpenClient} />;
  } else {
    dialogLabel = clientName(selection.signal.client);
    body = <MessageDetail signal={selection.signal} onOpenClient={onOpenClient} />;
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
