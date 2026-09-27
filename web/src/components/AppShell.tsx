import type { ReactNode } from 'react';

interface AppShellProps {
  children: ReactNode;
}

export function AppShell({ children }: AppShellProps) {
  return (
    <div className="app-shell">
      <header className="app-shell__header">
        <div className="app-shell__brand">Advisor Radar</div>
        <div className="app-shell__persona" aria-label="Persona">
          Fila
        </div>
      </header>
      <main className="app-shell__main">{children}</main>
    </div>
  );
}
