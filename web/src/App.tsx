import { Navigate, Route, Routes } from 'react-router-dom';
import { AppShell } from './components/AppShell';
import { CustomerScreen } from './screens/CustomerScreen';
import { ManagerScreen } from './screens/ManagerScreen';
import { QueueScreen } from './screens/QueueScreen';
import { ReviewScreen } from './screens/ReviewScreen';

export function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="fila" replace />} />
        <Route path="fila" element={<QueueScreen />} />
        <Route path="revisao" element={<ReviewScreen />} />
        <Route path="painel" element={<ManagerScreen />} />
        <Route path="clientes/:id" element={<CustomerScreen />} />
      </Route>
    </Routes>
  );
}
