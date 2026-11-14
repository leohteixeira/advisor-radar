import { Link } from 'react-router-dom';
import type { DecoratedSignal } from '../domain/queueVisual';

interface SignalCardProps {
  row: DecoratedSignal;
  onOpenCase: (id: string) => void;
  onContacted: (id: string) => void;
  onSnooze: (id: string) => void;
  /** Removes the advisor's mark from the signal: reactivates a snoozed one, unmarks a contacted one. */
  onUndoAction: (id: string) => void;
  /** Abrir caso was sent and its outcome is not known yet. */
  pending?: boolean;
  justArrived?: boolean;
}

function clock(iso: string): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) {
    return '';
  }
  return at.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
}

function Bars({ filled, heights }: { filled: number; heights: number[] }) {
  return (
    <span className="mini-bars" aria-hidden>
      {heights.map((h, i) => (
        <span key={h} className={i < filled ? 'mini-bars__on' : ''} style={{ height: h }} />
      ))}
    </span>
  );
}

export function SignalCard({
  row,
  onOpenCase,
  onContacted,
  onSnooze,
  onUndoAction,
  pending = false,
  justArrived,
}: SignalCardProps) {
  const { signal } = row;
  const snoozedUntil = signal.snoozed_until ? clock(signal.snoozed_until) : '';
  const tone = row.accent === 'crit' ? 'crit' : row.accent === 'warn' ? 'warn' : 'neutral';

  return (
    <article
      className={justArrived ? `signal-card signal-card--${tone} signal-card--arrive` : `signal-card signal-card--${tone}`}
      data-testid={`signal-${signal.id}`}
      aria-busy={pending}
    >
      <Link
        to={`/clientes/${signal.client}`}
        className="signal-card__open"
        aria-label={`Abrir ${row.name}`}
      >
        <div className="signal-card__top">
          <span className="signal-card__who">
            <strong className="signal-card__name">{row.name}</strong>
            <span className={`signal-card__segment signal-card__segment--${row.segment.toLowerCase()}`}>
              {row.segment}
            </span>
          </span>
          <span className={`signal-card__sla signal-card__sla--${row.slaTone}`}>{row.slaText}</span>
        </div>
        <p className="signal-card__type">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <path d={row.icon} />
          </svg>
          {row.typeLabel} · {row.agoText}
        </p>
        {signal.contacted_at ? <span className="signal-card__contacted">Contatado</span> : null}
        {signal.snoozed_until ? (
          <span className="signal-card__snoozed">{snoozedUntil ? `Adiado até ${snoozedUntil}` : 'Adiado'}</span>
        ) : null}
        {signal.kind === 'message' && signal.text ? (
          <p className="signal-card__quote">{signal.text}</p>
        ) : null}
        {signal.kind === 'alert' && signal.reason ? (
          <p className="signal-card__reason">{signal.reason}</p>
        ) : null}
        <div className="signal-card__tags">
          {signal.churn ? (
            <span className="signal-tag signal-tag--churn">Risco de saída · certeza {signal.churnConf}</span>
          ) : null}
          {signal.kind === 'message' && signal.intent ? (
            <span className="signal-tag">
              {signal.intent}
              <Bars filled={row.confBars} heights={[5, 7, 9]} />
            </span>
          ) : null}
          {row.showFrust ? (
            <span className={`signal-tag signal-tag--f${row.fLevel}`}>
              <Bars filled={row.fLevel + 1} heights={[4, 6, 8, 10]} />
              {row.fLabel}
            </span>
          ) : null}
          {signal.human ? <span className="signal-tag signal-tag--human">Pediu atendimento humano</span> : null}
          {row.needsReview ? <span className="signal-tag signal-tag--review">Precisa de revisão</span> : null}
          {signal.fallback ? <span className="signal-tag signal-tag--fallback">Classificação simplificada</span> : null}
        </div>
      </Link>
      {signal.snoozed_until ? (
        <div className="signal-card__actions">
          <button type="button" onClick={() => onUndoAction(signal.id)}>
            Reativar
          </button>
        </div>
      ) : (
        <div className="signal-card__actions">
          <button type="button" disabled={pending} onClick={() => onOpenCase(signal.id)}>
            {pending ? 'Abrindo caso…' : 'Abrir caso'}
          </button>
          {signal.contacted_at ? (
            <button type="button" disabled={pending} onClick={() => onUndoAction(signal.id)}>
              Desfazer contato
            </button>
          ) : (
            <button type="button" disabled={pending} onClick={() => onContacted(signal.id)}>
              Contatado
            </button>
          )}
          <button
            type="button"
            className="signal-card__snooze"
            disabled={pending}
            onClick={() => onSnooze(signal.id)}
          >
            Adiar 1 h
          </button>
        </div>
      )}
    </article>
  );
}
