import { useEffect, useState, type ReactNode } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';

const PERSONAS = [
  { to: '/fila', label: 'Fila' },
  { to: '/revisao', label: 'Revisão de triagem' },
  { to: '/painel', label: 'Painel da assessoria' },
] as const;

interface AppShellProps {
  children?: ReactNode;
}

export function AppShell({ children }: AppShellProps) {
  const [dark, setDark] = useState(() => document.documentElement.dataset.theme === 'dark');
  const location = useLocation();
  const isQueue = location.pathname === '/fila' || location.pathname.endsWith('/fila');

  useEffect(() => {
    document.documentElement.dataset.theme = dark ? 'dark' : 'light';
  }, [dark]);

  return (
    <div className="app-shell">
      <header className="app-shell__header">
        <div className="app-shell__brand">
          <span className="app-shell__mark" aria-hidden>
            <span />
            <span />
            <span />
          </span>
          Advisor Radar
        </div>
        <nav className="app-shell__persona" aria-label="Persona">
          {PERSONAS.map((p) => (
            <NavLink
              key={p.to}
              to={p.to}
              className={({ isActive }) =>
                isActive ? 'persona-switch__btn persona-switch__btn--active' : 'persona-switch__btn'
              }
            >
              {p.label}
            </NavLink>
          ))}
        </nav>
        <button
          type="button"
          className={dark ? 'theme-toggle theme-toggle--dark' : 'theme-toggle'}
          aria-label={dark ? 'Mudar para tema claro' : 'Mudar para tema escuro'}
          onClick={() => setDark((value) => !value)}
        >
          <span className="theme-toggle__knob" />
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <circle cx="12" cy="12" r="4" />
            <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
          </svg>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
          </svg>
        </button>
      </header>
      <main className={isQueue ? 'app-shell__main app-shell__main--queue' : 'app-shell__main'}>
        {children ?? <Outlet />}
      </main>
    </div>
  );
}
