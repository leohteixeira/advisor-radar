import { useEffect, useState } from 'react';
import { fetchTimeline } from '../api/bff';
import { formatTimelineAgo } from '../domain/elapsed';
import type { TimelineEntry } from '../domain/types';

const CHIPS = [
  { value: '', label: 'Todos' },
  { value: 'conta', label: 'Conta' },
  { value: 'mensagens', label: 'Mensagens' },
  { value: 'casos', label: 'Casos' },
  { value: 'notas', label: 'Notas' },
] as const;

interface Timeline360Props {
  clientId: string;
  name: string;
  segment: string;
  aum?: number;
  advisor?: string;
  since?: string;
}

function fmtAum(n?: number): string {
  if (n == null) {
    return '—';
  }
  return new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'USD' }).format(n);
}

export function Timeline360({
  clientId,
  name,
  segment,
  aum,
  advisor,
  since,
}: Timeline360Props) {
  const [query, setQuery] = useState('');
  const [kind, setKind] = useState('');
  const [order, setOrder] = useState<'asc' | 'desc'>('asc');
  const [items, setItems] = useState<TimelineEntry[]>([]);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      try {
        const rows = await fetchTimeline(clientId, query, kind, order);
        if (!cancelled) {
          setItems(rows);
          setError(false);
        }
      } catch {
        if (!cancelled) {
          setItems([]);
          setError(true);
        }
      }
    };
    void run();
    return () => {
      cancelled = true;
    };
  }, [clientId, query, kind, order]);

  return (
    <div className="detail detail--360" data-testid="client-360">
      <p className="detail__eyebrow">Visão 360</p>
      <h2 className="detail__title detail__title--serif">{name}</h2>
      <p className="detail__sub">
        {segment}
        {aum != null ? ` · Patrimônio ${fmtAum(aum)}` : ''}
        {advisor ? ` · Assessora ${advisor}` : ''}
        {since ? ` · desde ${since}` : ''}
      </p>

      <label className="timeline-search">
        <span className="visually-hidden">Buscar na timeline</span>
        <input
          type="search"
          placeholder="Buscar na timeline"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          data-testid="timeline-search"
        />
      </label>

      <div className="timeline-chips" role="group" aria-label="Filtro da timeline">
        {CHIPS.map((c) => (
          <button
            key={c.label}
            type="button"
            className={kind === c.value ? 'chip chip--active' : 'chip'}
            aria-pressed={kind === c.value}
            onClick={() => setKind(c.value)}
          >
            {c.label}
          </button>
        ))}
        <button
          type="button"
          className="chip"
          aria-pressed
          data-testid="timeline-order"
          onClick={() => setOrder((cur) => (cur === 'asc' ? 'desc' : 'asc'))}
        >
          {order === 'asc' ? 'Mais recentes' : 'Mais antigos'}
        </button>
      </div>

      {error ? <p className="timeline-empty">Não foi possível carregar a timeline.</p> : null}

      <ul className="timeline-list" data-testid="timeline-list">
        {items.map((it) => (
          <li key={it.event_id} className="timeline-item" data-kind={it.kind}>
            <div className="timeline-item__head">
              <strong>{it.title}</strong>
              <span className="timeline-item__ago">{formatTimelineAgo(it.ago)}</span>
            </div>
            <p className="timeline-item__text">{it.text}</p>
            {it.meta ? <p className="timeline-item__meta">{it.meta}</p> : null}
          </li>
        ))}
      </ul>

      {!error && items.length === 0 ? (
        <p className="timeline-empty">Nada encontrado com esse filtro.</p>
      ) : null}
    </div>
  );
}
