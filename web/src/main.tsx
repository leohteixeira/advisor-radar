import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { App } from './App';
import '../../docs/design/handoff/tokens.css';
import './styles/app.css';

const root = document.getElementById('root');
if (!root) {
  throw new Error('root element missing');
}

const basename = import.meta.env.BASE_URL.replace(/\/$/, '') || '/';

createRoot(root).render(
  <StrictMode>
    <BrowserRouter basename={basename}>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
