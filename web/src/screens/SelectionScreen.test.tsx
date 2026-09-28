import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { App } from '../App';
import { WALK_INTERVAL_MS, WALK_STEPS } from '../selection/steps';
import { ROUTER_BASENAME } from '../test/fixtures';

function installMedia(matches: Record<string, boolean>) {
  window.matchMedia = (query: string) =>
    ({
      matches: matches[query] ?? false,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }) as MediaQueryList;
}

function renderAt(entry: string) {
  return render(
    <MemoryRouter basename={ROUTER_BASENAME} initialEntries={[entry]}>
      <App />
    </MemoryRouter>,
  );
}

describe('selection screen', () => {
  beforeEach(() => {
    installMedia({
      '(min-width: 900px)': false,
      '(prefers-reduced-motion: reduce)': false,
    });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('renders selection at /advisor-radar instead of the queue', () => {
    renderAt(ROUTER_BASENAME);
    expect(
      screen.getByRole('heading', { level: 1, name: 'O mesmo sinal, visto dos dois lados da conversa.' }),
    ).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Fila' })).not.toBeInTheDocument();
  });

  it('opens the team queue and the client list from the two cards', async () => {
    const user = userEvent.setup();
    renderAt(ROUTER_BASENAME);

    expect(screen.getByRole('link', { name: /Visão do time/ })).toHaveAttribute('href', '/advisor-radar/fila');
    expect(screen.getByRole('link', { name: /Visão do cliente/ })).toHaveAttribute(
      'href',
      '/advisor-radar/client-pov',
    );

    vi.stubGlobal(
      'EventSource',
      class {
        addEventListener() {}
        close() {}
      },
    );
    await user.click(screen.getByRole('link', { name: /Visão do time/ }));
    expect(screen.getByRole('navigation', { name: 'Persona' })).toBeInTheDocument();
  });

  it('states the three pillars and all eight walkthrough steps', () => {
    renderAt(`${ROUTER_BASENAME}/`);
    expect(screen.getByRole('heading', { name: 'Alertas proativos' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Triagem de mensagens' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Casos com SLA' })).toBeInTheDocument();
    expect(screen.getByText('Saque relevante, queda de patrimônio, aporte grande, mudança de segmento e cliente sem contato.')).toBeInTheDocument();
    for (const step of WALK_STEPS) {
      expect(screen.getByRole('button', { name: new RegExp(step.title) })).toBeInTheDocument();
    }
    expect(screen.getByText('POST /v1/client-pov/customers/{id}/complaints')).toBeInTheDocument();
    expect(screen.getByText(/dados, nomes e marcas fictícios/)).toBeInTheDocument();
  });

  it('advances the walkthrough every 2.6s and pauses', () => {
    vi.useFakeTimers();
    renderAt(ROUTER_BASENAME);

    expect(screen.getByRole('button', { name: /Passo 1:/ })).toHaveAttribute('aria-current', 'step');

    act(() => {
      vi.advanceTimersByTime(WALK_INTERVAL_MS);
    });
    expect(screen.getByRole('button', { name: /Passo 2:/ })).toHaveAttribute('aria-current', 'step');

    act(() => {
      screen.getByRole('button', { name: 'Pausar' }).click();
    });
    act(() => {
      vi.advanceTimersByTime(WALK_INTERVAL_MS * 3);
    });
    expect(screen.getByRole('button', { name: /Passo 2:/ })).toHaveAttribute('aria-current', 'step');
    expect(screen.getByRole('button', { name: 'Reproduzir' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('does not move when reduced motion is requested', () => {
    vi.useFakeTimers();
    installMedia({
      '(min-width: 900px)': false,
      '(prefers-reduced-motion: reduce)': true,
    });
    renderAt(ROUTER_BASENAME);
    expect(screen.getByRole('button', { name: 'Reproduzir' })).toHaveAttribute('aria-pressed', 'true');
    act(() => {
      vi.advanceTimersByTime(WALK_INTERVAL_MS * 4);
    });
    expect(screen.getByRole('button', { name: /Passo 1:/ })).toHaveAttribute('aria-current', 'step');
  });

  it('shows the desktop cards, graph, and step jumpers', async () => {
    installMedia({
      '(min-width: 900px)': true,
      '(prefers-reduced-motion: reduce)': false,
    });
    const user = userEvent.setup();
    renderAt(ROUTER_BASENAME);

    expect(screen.getByText('Entrar como time')).toBeInTheDocument();
    expect(screen.getByText('Escolher cliente')).toBeInTheDocument();
    expect(screen.getByText('Fila do assessor')).toBeInTheDocument();
    expect(screen.getByText('Visão 360 do cliente')).toBeInTheDocument();
    expect(screen.getByText('Saque e wire-out')).toBeInTheDocument();
    expect(screen.getByText('Reclamações prontas')).toBeInTheDocument();
    expect(screen.getByText('Como uma reclamação chega à fila do assessor')).toBeInTheDocument();
    expect(screen.getByText('Docker Compose')).toBeInTheDocument();
    expect(screen.getByText(/Nenhuma mensagem é enviada a clientes de verdade/)).toBeInTheDocument();
    expect(screen.getByText('App e fila').closest('article')).toHaveClass('on');
    expect(screen.queryByRole('list', { name: 'Passos da reclamação' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Passo 4:/ }));
    expect(document.querySelector('.selection__readout code')).toHaveTextContent('message.received');
    expect(screen.getByText('O relay publica no RabbitMQ')).toBeInTheDocument();
    expect(document.querySelector('.selection__bus')).toHaveClass('on');
    expect(screen.getByRole('button', { name: 'Reproduzir' })).toHaveAttribute('aria-pressed', 'true');
  });
});
