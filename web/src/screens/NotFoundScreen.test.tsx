import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { App } from '../App';
import { ROUTER_BASENAME } from '../test/fixtures';

describe('unknown route', () => {
  it.each(['/rota-que-nao-existe', '/fila/extra/segmento'])('%s says the page was not found and links to the main views', (path) => {
    render(
      <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[`${ROUTER_BASENAME}${path}`]}>
        <App />
      </MemoryRouter>,
    );

    expect(screen.getByRole('heading', { name: 'Página não encontrada' })).toBeInTheDocument();
    const links = within(screen.getByRole('navigation', { name: 'Telas principais' })).getAllByRole('link');
    expect(links.map((link) => [link.textContent, link.getAttribute('href')])).toEqual([
      ['Fila do time', `${ROUTER_BASENAME}/fila`],
      ['Revisão de triagem', `${ROUTER_BASENAME}/revisao`],
      ['Painel da assessoria', `${ROUTER_BASENAME}/painel`],
      ['Visão do cliente', `${ROUTER_BASENAME}/client-pov`],
    ]);
  });
});
