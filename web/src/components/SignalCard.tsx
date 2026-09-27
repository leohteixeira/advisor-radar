import type { DecoratedSignal } from '../domain/queueVisual';

interface SignalCardProps {
  row: DecoratedSignal;
  onOpen: (id: string) => void;
  onOpenCase: (id: string) => void;
  onContacted: (id: string) => void;
  onSnooze: (id: string) => void;
  justArrived?: boolean;
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
  onOpen,
  onOpenCase,
  onContacted,
  onSnooze,
  justArrived,
}: SignalCardProps) {
  const { signal } = row;
  const tone = row.accent === 'crit' ? 'crit' : row.accent === 'warn' ? 'warn' : 'neutral';

  return (
    <article
      className={justArrived ? `signal-card signal-card--${tone} signal-card--arrive` : `signal-card signal-card--${tone}`}
      data-testid={`signal-${signal.id}`}
    >
      <button
        type="button"
        className="signal-card__open"
        onClick={() => onOpen(signal.id)}
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
      </button>
      <div className="signal-card__actions">
        <button type="button" onClick={() => onOpenCase(signal.id)}>
          Abrir caso
        </button>
        <button type="button" onClick={() => onContacted(signal.id)}>
          Contatado
        </button>
        <button type="button" className="signal-card__snooze" onClick={() => onSnooze(signal.id)}>
          Adiar 1 h
        </button>
      </div>
    </article>
  );
}
