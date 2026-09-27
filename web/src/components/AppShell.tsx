import { useEffect, useState, type ReactNode } from 'react';
import type { Persona } from '../domain/types';

const PERSONAS: { id: Persona; label: string }[] = [
  { id: 'queue', label: 'Fila' },
  { id: 'review', label: 'Revisão de triagem' },
  { id: 'manager', label: 'Painel da assessoria' },
];

interface AppShellProps {
  children: ReactNode;
  persona: Persona;
  onPersonaChange: (persona: Persona) => void;
}

export function AppShell({ children, persona, onPersonaChange }: AppShellProps) {
  const [dark, setDark] = useState(() => document.documentElement.dataset.theme === 'dark');

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
          {PERSONAS.map((p) => {
            const active = p.id === persona;
            return (
              <button
                key={p.id}
                type="button"
                className={active ? 'persona-switch__btn persona-switch__btn--active' : 'persona-switch__btn'}
                aria-current={active ? 'page' : undefined}
                onClick={() => onPersonaChange(p.id)}
              >
                {p.label}
              </button>
            );
          })}
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
      <main className={persona === 'queue' ? 'app-shell__main app-shell__main--queue' : 'app-shell__main'}>
        {children}
      </main>
    </div>
  );
}
