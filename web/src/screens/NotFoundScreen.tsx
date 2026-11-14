import { Link } from 'react-router-dom';

const VIEWS = [
  { to: '/fila', label: 'Fila do time' },
  { to: '/revisao', label: 'Revisão de triagem' },
  { to: '/painel', label: 'Painel da assessoria' },
  { to: '/client-pov', label: 'Visão do cliente' },
] as const;

/** Any address that is not a screen: says so and links back to the main views. */
export function NotFoundScreen() {
  return (
    <main className="not-found">
      <p className="not-found__brand">Advisor Radar</p>
      <h1>Página não encontrada</h1>
      <p>Este endereço não corresponde a nenhuma tela. Escolha para onde ir:</p>
      <nav aria-label="Telas principais">
        <ul>
          {VIEWS.map((view) => (
            <li key={view.to}>
              <Link to={view.to}>{view.label}</Link>
            </li>
          ))}
        </ul>
      </nav>
    </main>
  );
}
