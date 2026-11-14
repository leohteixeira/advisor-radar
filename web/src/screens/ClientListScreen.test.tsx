import { render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import type { POVClient } from '../api/pov';
import { ClientListScreen } from './ClientListScreen';
import { CID } from '../test/fixtures';

const known: POVClient = {
  customer_id: CID.mariana,
  name: 'Mariana Costa',
  segment: 'Singular',
  assets: 24_830_000,
  sla: '1 h',
  advisor: 'Ana Paula Ribeiro',
  since: '2021',
  hint: 'Reclama e pede um humano.',
};

// advisory could not answer for this one: the BFF sends an empty segment and SLA.
const unknown: POVClient = { ...known, customer_id: CID.fernanda, name: 'Fernanda Lima', segment: '', sla: '' };

function renderList(clients: POVClient[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ items: clients }), { status: 200 })),
  );
  return render(
    <MemoryRouter>
      <ClientListScreen />
    </MemoryRouter>,
  );
}

describe('ClientListScreen', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows the segment and the base SLA of a client advisory knows', async () => {
    renderList([known]);
    const card = await screen.findByRole('link', { name: /Mariana Costa/ });
    expect(within(card).getByText('Singular')).toHaveClass('pov-card__segment');
    expect(within(card).getByText('SLA base 1 h')).toBeInTheDocument();
  });

  it('leaves out the segment chip and the SLA line when advisory could not answer', async () => {
    renderList([known, unknown]);
    const card = await screen.findByRole('link', { name: /Fernanda Lima/ });
    expect(card.querySelector('.pov-card__segment')).toBeNull();
    expect(card.querySelector('.pov-card__sla')).toBeNull();
    expect(card).not.toHaveTextContent('SLA base');
    expect(card).toHaveTextContent('Fernanda Lima');
  });
});
