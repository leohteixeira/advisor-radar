import type { ReactNode } from 'react';
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
  return (
    <div className="app-shell">
      <header className="app-shell__header">
        <div className="app-shell__brand">Advisor Radar</div>
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
      </header>
      <main className="app-shell__main">{children}</main>
    </div>
  );
}
