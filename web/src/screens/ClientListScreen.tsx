import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { fetchPOVClients, formatCents, type POVClient } from '../api/pov';

const SEGMENT_RANK: Record<string, number> = { Essencial: 0, Advance: 1, Singular: 2 };

function initials(name: string): string {
  return name
    .split(' ')
    .slice(0, 2)
    .map((part) => part[0] ?? '')
    .join('')
    .toUpperCase();
}

export function ClientListScreen() {
  const [items, setItems] = useState<POVClient[] | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    let gone = false;
    fetchPOVClients()
      .then((rows) => {
        if (!gone) {
          setItems(
            [...rows].sort((a, b) => (SEGMENT_RANK[a.segment] ?? 9) - (SEGMENT_RANK[b.segment] ?? 9)),
          );
        }
      })
      .catch(() => {
        if (!gone) {
          setError(true);
        }
      });
    return () => {
      gone = true;
    };
  }, []);

  return (
    <main className="pov-list">
      <header className="pov-list__bar">
        <Link to="/" className="pov-list__back" aria-label="Trocar visão">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
            <path d="M15 6l-6 6 6 6" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
          <span>Trocar visão</span>
        </Link>
        <span className="pov-list__brand">Advisor Radar</span>
        <span className="pov-list__badge">Visão do cliente</span>
      </header>

      <section className="pov-list__intro">
        <div>
          <p className="pov-list__eyebrow">Um cliente fictício por segmento</p>
          <h1>Entre como um cliente</h1>
          <p>
            Cada um já tem histórico na visão 360. O que ele fizer no app vira evento no RabbitMQ e chega à
            fila da assessoria.
          </p>
        </div>
        <ol className="pov-list__steps">
          <li>
            <span>1</span>Escolha um cliente abaixo
          </li>
          <li>
            <span>2</span>Saque, deposite, escreva ou reclame
          </li>
          <li>
            <span>3</span>Veja o alerta chegar na fila do time
          </li>
        </ol>
      </section>

      {error ? <p role="alert">Não foi possível carregar os clientes.</p> : null}
      {items === null && !error ? <p className="pov-list__status">Carregando clientes…</p> : null}
      {items?.length === 0 ? <p className="pov-list__status">Nenhum cliente semeado.</p> : null}

      <nav className="pov-list__grid" aria-label="Clientes">
        {items?.map((client) => (
          <Link key={client.customer_id} className="pov-card" to={`/client-pov/${client.customer_id}`} data-segment={client.segment}>
            <span className="pov-card__avatar" aria-hidden="true">
              {initials(client.name)}
            </span>
            <span className="pov-card__who">
              <strong>{client.name}</strong>
              <span>
                {client.since ? `Cliente desde ${client.since} · ` : ''}
                {client.advisor}
              </span>
            </span>
            <span className="pov-card__meta">
              <span className="pov-card__segment">{client.segment}</span>
              <span className="pov-card__money">{formatCents(client.assets)}</span>
              <span className="pov-card__sla">SLA base {client.sla}</span>
            </span>
            <p>{client.hint}</p>
          </Link>
        ))}
      </nav>

      <footer>Nenhuma mensagem é enviada a pessoas reais. Os eventos entram na mesma fila que o time vê.</footer>
    </main>
  );
}
