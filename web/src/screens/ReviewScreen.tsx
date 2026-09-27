import { useCallback, useEffect, useState } from 'react';
import { correctReview, fetchReview } from '../api/bff';
import { Toast } from '../components/Toast';
import { clientName, clientSegment } from '../domain/clients';
import type { Intent, ReviewRow } from '../domain/types';

function sortedDist(dist: Record<string, number>): [string, number][] {
  return Object.entries(dist).sort((a, b) => b[1] - a[1]);
}

function confidenceLabel(top: number): string {
  if (top >= 0.75) {
    return 'alta';
  }
  if (top >= 0.5) {
    return 'média';
  }
  return 'baixa';
}

function agoText(ago: number): string {
  if (ago < 60) {
    return `há ${ago} min`;
  }
  const h = Math.floor(ago / 60);
  const m = ago % 60;
  if (m === 0) {
    return `há ${h} h`;
  }
  return `há ${h} h ${m} min`;
}

export function ReviewScreen() {
  const [items, setItems] = useState<ReviewRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      setItems(await fetchReview());
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!toast) {
      return;
    }
    const t = window.setTimeout(() => setToast(null), 3500);
    return () => window.clearTimeout(t);
  }, [toast]);

  const onCorrect = async (row: ReviewRow, intent: Intent | string) => {
    setBusyId(row.id);
    try {
      const updated = await correctReview(row.id, intent);
      setItems((prev) => prev.map((r) => (r.id === row.id ? updated : r)));
      const top = sortedDist(row.dist)[0]?.[0];
      setToast(
        intent === top
          ? `Confirmado: ${intent}`
          : `Corrigido para ${intent}. Obrigado pelo feedback.`,
      );
    } catch {
      setToast('Não foi possível corrigir. Tente de novo.');
    } finally {
      setBusyId(null);
    }
  };

  if (loading) {
    return <div className="queue-status">Carregando revisão…</div>;
  }
  if (error) {
    return (
      <div className="queue-status">
        Não foi possível carregar a revisão.
        <button type="button" onClick={() => void load()}>
          Tentar de novo
        </button>
      </div>
    );
  }

  const pending = items.filter((r) => !r.corrected).length;

  return (
    <div className="review-screen">
      <div className="review-screen__head">
        <span>
          <strong>{pending}</strong> mensagens aguardando · intenção abaixo de 0,85
        </span>
        <span>Confirme a sugestão ou escolha a intenção correta</span>
      </div>

      {items.map((row) => {
        const entries = sortedDist(row.dist);
        const top = entries[0]?.[0] ?? row.intent;
        const topProb = entries[0]?.[1] ?? 0;
        const conf = confidenceLabel(topProb);
        const alts = entries.slice(1, 4);
        const doneLabel =
          row.intent === top ? `Confirmado como ${row.intent}` : `Corrigido para ${row.intent}`;

        return (
          <article
            key={row.id}
            className={row.corrected ? 'review-card review-card--done' : 'review-card'}
            data-testid={`review-${row.id}`}
          >
            <div className="review-card__top">
              <span className="review-card__name">{clientName(row.client)}</span>
              <span className="review-card__segment">{clientSegment(row.client)}</span>
              <span className="review-card__ago">{agoText(row.ago)}</span>
            </div>
            <blockquote className="review-card__quote">“{row.text}”</blockquote>

            {row.fallback ? (
              <span className="review-card__fallback">Classificação simplificada</span>
            ) : null}

            <div className="review-card__dist" aria-label="Distribuição de intenção">
              {entries.map(([label, value]) => (
                <div key={label} className="review-card__dist-row">
                  <span>{label}</span>
                  <div className="review-card__bar">
                    <div style={{ width: `${Math.round(value * 100)}%` }} />
                  </div>
                  <span>{Math.round(value * 100)}%</span>
                </div>
              ))}
            </div>

            {!row.corrected ? (
              <div className="review-card__actions">
                <div className="review-card__suggest">
                  <div>
                    <span className="review-card__label">Sugestão do modelo</span>
                    <div className="review-card__suggest-row">
                      <strong>{top}</strong>
                      <span className="review-card__conf">certeza {conf}</span>
                    </div>
                  </div>
                  <button
                    type="button"
                    aria-label={`Confirmar ${top}`}
                    disabled={busyId === row.id}
                    onClick={() => void onCorrect(row, top)}
                  >
                    Confirmar
                  </button>
                </div>
                <div>
                  <span className="review-card__label">Ou escolha outra intenção</span>
                  <div className="review-card__alts">
                    {alts.map(([label], i) => (
                      <button
                        key={label}
                        type="button"
                        disabled={busyId === row.id}
                        onClick={() => void onCorrect(row, label)}
                      >
                        <span>{label}</span>
                        <span>{i === 0 ? 'mais provável' : 'alternativa'}</span>
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            ) : (
              <div className="review-card__done" role="status">
                {doneLabel}
              </div>
            )}
          </article>
        );
      })}

      {pending === 0 ? (
        <div className="review-screen__empty">
          Tudo revisado. Novas mensagens com intenção abaixo de 0,85 aparecem aqui.
        </div>
      ) : null}

      {toast ? <Toast text={toast} /> : null}
    </div>
  );
}
