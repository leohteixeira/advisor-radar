import { useCallback, useEffect, useState } from 'react';
import { fetchManager } from '../api/bff';
import type { ManagerSnapshot } from '../domain/types';

function remainingText(remaining: number): string {
  const abs = Math.abs(remaining);
  if (remaining < 0) {
    return `vencido há ${abs} min`;
  }
  return `${remaining} min`;
}

export function ManagerScreen() {
  const [snap, setSnap] = useState<ManagerSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      setSnap(await fetchManager());
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  if (loading) {
    return <div className="queue-status">Carregando painel…</div>;
  }
  if (error || !snap) {
    return (
      <div className="queue-status">
        Não foi possível carregar o painel.
        <button type="button" onClick={() => void load()}>
          Tentar de novo
        </button>
      </div>
    );
  }

  const maxOpen = Math.max(...snap.backlog.map((b) => b.open), 1);
  const overdueCount = snap.atRisk.filter((k) => k.remaining < 0).length;
  const atRiskSorted = [...snap.atRisk].sort((a, b) => a.remaining - b.remaining);
  const maxIntent = Math.max(...Object.values(snap.intents), 1);
  const intents = Object.entries(snap.intents).sort((a, b) => b[1] - a[1]);
  const delta = snap.avgFirstContactYesterday - snap.avgFirstContactMin;

  return (
    <div className="manager-screen">
      <div className="manager-screen__metrics">
        <div className="metric-card metric-card--risk">
          <span className="metric-card__label">Casos com SLA em risco</span>
          <span className="metric-card__value">{snap.atRisk.length}</span>
          <span className="metric-card__note metric-card__note--crit">{overdueCount} vencidos</span>
        </div>
        <div className="metric-card">
          <span className="metric-card__label">Alerta → primeiro contato</span>
          <span className="metric-card__value">
            {snap.avgFirstContactMin} <span className="metric-card__unit">min</span>
          </span>
          <span className="metric-card__note metric-card__note--ok">−{delta} min vs. ontem</span>
        </div>
        <div className="metric-card">
          <span className="metric-card__label">Mensagens em revisão humana</span>
          <span className="metric-card__value">{snap.reviewPct}%</span>
          <span className="metric-card__note">taxa de revisão</span>
        </div>
        <div className="metric-card">
          <span className="metric-card__label">Classificação simplificada</span>
          <span className="metric-card__value">{snap.fallbackPct}%</span>
          <span className="metric-card__note">modelo externo saudável</span>
        </div>
      </div>

      <div className="manager-screen__grid">
        <section className="manager-panel">
          <h2>Backlog por assessor</h2>
          {snap.backlog.map((b) => {
            const ok = Math.max(b.open - b.risk - b.overdue, 0);
            return (
              <div key={b.advisor} className="manager-backlog" data-testid={`backlog-${b.advisor}`}>
                <div className="manager-backlog__row">
                  <span>{b.advisor}</span>
                  <span>
                    <strong>{b.open}</strong> abertos · {b.risk} em risco
                    {b.overdue > 0 ? ` · ${b.overdue} vencido${b.overdue > 1 ? 's' : ''}` : ''}
                  </span>
                </div>
                <div className="manager-backlog__bar" aria-hidden>
                  <div className="manager-backlog__ok" style={{ width: `${(ok / maxOpen) * 100}%` }} />
                  <div className="manager-backlog__risk" style={{ width: `${(b.risk / maxOpen) * 100}%` }} />
                  <div
                    className="manager-backlog__over"
                    style={{ width: `${(b.overdue / maxOpen) * 100}%` }}
                  />
                </div>
              </div>
            );
          })}
        </section>

        <section className="manager-panel">
          <h2>Casos com SLA em risco</h2>
          {atRiskSorted.map((k) => (
            <div key={k.id} className="manager-risk" data-testid={`risk-${k.id}`}>
              <div>
                <strong>
                  {k.client} <span>{k.segment}</span>
                </strong>
                <span>
                  {k.advisor} · <code>{k.id}</code>
                </span>
              </div>
              <span className={k.remaining < 0 ? 'manager-risk__rem--over' : 'manager-risk__rem'}>
                {remainingText(k.remaining)}
              </span>
            </div>
          ))}
        </section>

        <section className="manager-panel">
          <h2>Intenções no dia</h2>
          {intents.map(([label, pct]) => (
            <div key={label} className="manager-intent">
              <span>{label}</span>
              <div className="manager-intent__bar">
                <div
                  className={
                    label === 'Reclamação' || label === 'Encerramento'
                      ? 'manager-intent__fill manager-intent__fill--crit'
                      : 'manager-intent__fill'
                  }
                  style={{ width: `${(pct / maxIntent) * 100}%` }}
                />
              </div>
              <span>{pct}%</span>
            </div>
          ))}
        </section>
      </div>
    </div>
  );
}
