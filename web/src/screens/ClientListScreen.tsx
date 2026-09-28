import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { fetchPOVClients, formatCents, type POVClient } from '../api/pov';

export function ClientListScreen() {
  const [items, setItems] = useState<POVClient[] | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    let gone = false;
    fetchPOVClients()
      .then((rows) => {
        if (!gone) {
          setItems(rows);
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
        <Link to="/">Advisor Radar</Link>
        <span>Demo técnica · dados fictícios</span>
      </header>
      <h1>Escolha um cliente</h1>
      {error ? <p role="alert">Não foi possível carregar os clientes.</p> : null}
      {items === null && !error ? <p>Carregando clientes…</p> : null}
      {items?.length === 0 ? <p>Nenhum cliente semeado.</p> : null}
      <ul>
        {items?.map((client) => (
          <li key={client.customer_id}>
            <Link to={`/client-pov/${client.customer_id}`}>
              <strong>{client.name}</strong>
              <span>{client.segment}</span>
              <span>{formatCents(client.assets)}</span>
              <span>{client.sla}</span>
              <span>{client.advisor}</span>
              <p>{client.hint}</p>
            </Link>
          </li>
        ))}
      </ul>
      <footer>Nenhuma mensagem é enviada a pessoas reais. Os eventos entram na mesma fila que o time vê.</footer>
    </main>
  );
}
