import { Route, Routes } from 'react-router-dom';
import { AppShell } from './components/AppShell';
import { CustomerScreen } from './screens/CustomerScreen';
import { ManagerScreen } from './screens/ManagerScreen';
import { QueueScreen } from './screens/QueueScreen';
import { ReviewScreen } from './screens/ReviewScreen';
import { SelectionScreen } from './screens/SelectionScreen';

export function App() {
  return (
    <Routes>
      <Route index element={<SelectionScreen />} />
      <Route element={<AppShell />}>
        <Route path="fila" element={<QueueScreen />} />
        <Route path="revisao" element={<ReviewScreen />} />
        <Route path="painel" element={<ManagerScreen />} />
        <Route path="clientes/:id" element={<CustomerScreen />} />
      </Route>
    </Routes>
  );
}
