import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderSdui } from '../test/sdui';
import { ActionControl, resolveAction } from './actions';

afterEach(() => {
  vi.restoreAllMocks();
});

describe('resolveAction', () => {
  it.each([
    ['home', { type: 'navigate', label: 'Início', target: 'home' }, { kind: 'navigate', label: 'Início', slug: 'home' }],
    ['investir', { type: 'navigate', label: 'Ver produtos', target: 'investir' }, { kind: 'navigate', label: 'Ver produtos', slug: 'investir' }],
    ['carteira', { type: 'navigate', label: 'Ver carteira', target: 'carteira' }, { kind: 'navigate', label: 'Ver carteira', slug: 'carteira' }],
    ['perfil', { type: 'navigate', label: 'Perfil', target: 'perfil' }, { kind: 'navigate', label: 'Perfil', slug: 'perfil' }],
    ['deposit', { type: 'panel', label: 'Depositar', target: 'deposit' }, { kind: 'panel', label: 'Depositar', panel: 'deposit' }],
    ['withdraw', { type: 'panel', label: 'Sacar', target: 'withdraw' }, { kind: 'panel', label: 'Sacar', panel: 'withdraw' }],
    ['message', { type: 'panel', label: 'Mensagem', target: 'message' }, { kind: 'panel', label: 'Mensagem', panel: 'message' }],
    ['complaint', { type: 'panel', label: 'Reclamar', target: 'complaint' }, { kind: 'panel', label: 'Reclamar', panel: 'complaint' }],
    ['purchase', { type: 'panel', label: 'Investir', target: 'purchase', product_id: 'p1' }, { kind: 'hidden', problem: null }],
    ['note', { type: 'note', label: 'Saiba mais', text: 'Texto' }, { kind: 'note', label: 'Saiba mais', text: 'Texto' }],
    ['https link', { type: 'link', label: 'Site', href: 'https://example.com/a' }, { kind: 'link', label: 'Site', href: 'https://example.com/a' }],
  ])('resolves %s', (_name, action, expected) => {
    expect(resolveAction(action)).toEqual(expected);
  });

  it.each([
    ['null', null, 'sdui: action is not an object'],
    ['a string', 'navigate', 'sdui: action is not an object'],
    ['no label', { type: 'navigate', target: 'home' }, 'sdui: action navigate has no label'],
    ['an empty label', { type: 'panel', label: '', target: 'deposit' }, 'sdui: action panel has no label'],
    ['an unknown slug', { type: 'navigate', label: 'x', target: 'extrato' }, 'sdui: unknown navigate target extrato'],
    ['an unknown panel', { type: 'panel', label: 'x', target: 'transfer' }, 'sdui: unknown panel target transfer'],
    ['a note without text', { type: 'note', label: 'x' }, 'sdui: note action has no text'],
    ['an http link', { type: 'link', label: 'x', href: 'http://example.com' }, 'sdui: link refused, href is not https'],
    ['a javascript link', { type: 'link', label: 'x', href: 'javascript:alert(1)' }, 'sdui: link refused, href is not https'],
    ['a link that is not a URL', { type: 'link', label: 'x', href: 'nope' }, 'sdui: link refused, href is not https'],
    ['a link without href', { type: 'link', label: 'x' }, 'sdui: link refused, href is not https'],
    ['an unknown type', { type: 'teleport', label: 'x' }, 'sdui: unknown action type teleport'],
  ])('hides %s', (_name, action, problem) => {
    expect(resolveAction(action)).toEqual({ kind: 'hidden', problem });
  });
});

describe('ActionControl', () => {
  it('navigates to the target screen', async () => {
    const user = userEvent.setup();
    const { value } = renderSdui(<ActionControl action={{ type: 'navigate', label: 'Ver produtos', target: 'investir' }} className="x" />);
    await user.click(screen.getByRole('button', { name: 'Ver produtos' }));
    expect(value.onNavigate).toHaveBeenCalledWith('investir');
  });

  it('opens a coded panel', async () => {
    const user = userEvent.setup();
    const { value } = renderSdui(<ActionControl action={{ type: 'panel', label: 'Depositar', target: 'deposit' }} className="x" />);
    await user.click(screen.getByRole('button', { name: 'Depositar' }));
    expect(value.onPanel).toHaveBeenCalledWith('deposit');
  });

  it('shows a note in place', async () => {
    const user = userEvent.setup();
    renderSdui(<ActionControl action={{ type: 'note', label: 'Saiba mais', text: 'Preço fixo da simulação.' }} className="x" />);
    const button = screen.getByRole('button', { name: 'Saiba mais' });
    expect(button).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('Preço fixo da simulação.')).not.toBeInTheDocument();
    await user.click(button);
    expect(button).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('Preço fixo da simulação.')).toHaveAttribute('id', button.getAttribute('aria-controls'));
    await user.click(button);
    expect(screen.queryByText('Preço fixo da simulação.')).not.toBeInTheDocument();
  });

  it('opens an https link safely', () => {
    renderSdui(
      <ActionControl action={{ type: 'link', label: 'Regulamento', href: 'https://example.com/r' }} className="x" render={(label) => <em>{label} ↗</em>} />,
    );
    const link = screen.getByRole('link', { name: 'Regulamento ↗' });
    expect(link).toHaveAttribute('href', 'https://example.com/r');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');
    expect(link).toHaveAttribute('target', '_blank');
  });

  it.each([
    ['an unknown action', { type: 'teleport', label: 'Ir' }],
    ['a non-https link', { type: 'link', label: 'Ir', href: 'http://example.com' }],
    ['an unknown panel', { type: 'panel', label: 'Ir', target: 'transfer' }],
  ])('hides %s and reports it once', (_name, action) => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const { container, rerenderWith } = renderSdui(<ActionControl action={action} className="x" />);
    rerenderWith(<ActionControl action={action} className="x" />, { masked: true });
    expect(container.querySelector('.x')).toBeNull();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(error).toHaveBeenCalledTimes(1);
  });

  it('hides a purchase panel without reporting it', () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    renderSdui(<ActionControl action={{ type: 'panel', label: 'Investir', target: 'purchase', product_id: 'p1' }} className="x" />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    expect(error).not.toHaveBeenCalled();
  });
});
