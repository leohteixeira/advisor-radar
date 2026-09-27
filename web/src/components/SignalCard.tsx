import { clientName, clientSegment } from '../domain/clients';
import { ALERT_LABELS } from '../domain/types';
import type { Signal } from '../domain/types';

interface SignalCardProps {
  signal: Signal;
  onOpen: (id: string) => void;
  onOpenCase: (id: string) => void;
  onContacted: (id: string) => void;
  onSnooze: (id: string) => void;
}

function borderTone(signal: Signal): 'crit' | 'warn' | 'neutral' {
  if (signal.churn || signal.alert === 'saque' || signal.frustration === 3) {
    return 'crit';
  }
  if (
    signal.frustration === 2 ||
    signal.alert === 'queda' ||
    signal.alert === 'contato' ||
    signal.human
  ) {
    return 'warn';
  }
  return 'neutral';
}

function whyHere(signal: Signal): string {
  if (signal.kind === 'alert') {
    return ALERT_LABELS[signal.alert ?? ''] ?? signal.reason ?? signal.rule ?? 'Alerta';
  }
  if (signal.churn) {
    return 'Mensagem com risco';
  }
  return signal.intent ?? 'Mensagem';
}

export function SignalCard({
  signal,
  onOpen,
  onOpenCase,
  onContacted,
  onSnooze,
}: SignalCardProps) {
  const tone = borderTone(signal);
  const name = clientName(signal.client);
  const segment = clientSegment(signal.client);

  return (
    <article className={`signal-card signal-card--${tone}`} data-testid={`signal-${signal.id}`}>
      <button
        type="button"
        className="signal-card__open"
        onClick={() => onOpen(signal.id)}
        aria-label={`Abrir ${name}`}
      >
        <div className="signal-card__top">
          <strong className="signal-card__name">{name}</strong>
          <span className="signal-card__segment">{segment}</span>
        </div>
        <p className="signal-card__why">{whyHere(signal)}</p>
        {signal.contacted_at ? (
          <span className="signal-card__contacted">Contatado</span>
        ) : null}
        {signal.kind === 'message' && signal.text ? (
          <p className="signal-card__quote">{signal.text}</p>
        ) : null}
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
