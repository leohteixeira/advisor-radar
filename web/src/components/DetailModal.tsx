import { useEffect } from 'react';
import { clientName, clientSegment } from '../domain/clients';
import { ALERT_LABELS } from '../domain/types';
import type { Signal } from '../domain/types';

interface DetailModalProps {
  signal: Signal;
  onClose: () => void;
  desktop: boolean;
}

export function DetailModal({ signal, onClose, desktop }: DetailModalProps) {
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

  const name = clientName(signal.client);
  const segment = clientSegment(signal.client);
  const title =
    signal.kind === 'alert'
      ? (ALERT_LABELS[signal.alert ?? ''] ?? 'Alerta')
      : (signal.intent ?? 'Mensagem');

  const body = (
    <div className="detail" data-testid="signal-detail">
      <div className="detail__header">
        <button type="button" className="detail__close" onClick={onClose} aria-label="Fechar">
          ✕
        </button>
        <h2 className="detail__title">{name}</h2>
        <p className="detail__sub">
          {segment} · {title}
        </p>
      </div>
      {signal.text ? <p className="detail__quote">{signal.text}</p> : null}
      {signal.reason ? <p className="detail__reason">{signal.reason}</p> : null}
      {signal.rule ? <p className="detail__rule">{signal.rule}</p> : null}
      {signal.contacted_at ? <p className="detail__contacted">Contatado</p> : null}
    </div>
  );

  if (!desktop) {
    return body;
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
        aria-label={name}
        onClick={(e) => e.stopPropagation()}
      >
        {body}
      </div>
    </div>
  );
}
