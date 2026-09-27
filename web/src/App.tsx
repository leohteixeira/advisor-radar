import { useState } from 'react';
import { AppShell } from './components/AppShell';
import type { Persona } from './domain/types';
import { ManagerScreen } from './screens/ManagerScreen';
import { QueueScreen } from './screens/QueueScreen';
import { ReviewScreen } from './screens/ReviewScreen';

export function App() {
  const [persona, setPersona] = useState<Persona>('queue');

  return (
    <AppShell persona={persona} onPersonaChange={setPersona}>
      {persona === 'queue' ? <QueueScreen /> : null}
      {persona === 'review' ? <ReviewScreen /> : null}
      {persona === 'manager' ? <ManagerScreen /> : null}
    </AppShell>
  );
}
