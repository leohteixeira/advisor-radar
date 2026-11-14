import { render, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { AppShell } from './AppShell';

describe('AppShell navigation', () => {
  // jsdom has no layout: the second row at phone widths lives in app.css
  // (`@media (max-width: 719px)`). What is checked here is that every
  // destination is a link with its whole label and no truncating style.
  it('offers every team destination as a link with its full, untruncated name', () => {
    render(
      <MemoryRouter initialEntries={['/painel']}>
        <AppShell>
          <p>conteúdo</p>
        </AppShell>
      </MemoryRouter>,
    );

    const nav = screen.getByRole('navigation', { name: 'Persona' });
    const links = within(nav).getAllByRole('link');
    expect(links.map((link) => [link.textContent, link.getAttribute('href')])).toEqual([
      ['Fila', '/fila'],
      ['Revisão de triagem', '/revisao'],
      ['Painel da assessoria', '/painel'],
    ]);
    expect(within(nav).getByRole('link', { name: 'Painel da assessoria' })).toHaveAttribute('aria-current', 'page');
    for (const link of links) {
      expect(link.className).toMatch(/^persona-switch__btn( persona-switch__btn--active)?$/);
      expect(link).not.toHaveAttribute('style');
      expect(link).not.toHaveAttribute('title');
    }
  });
});
