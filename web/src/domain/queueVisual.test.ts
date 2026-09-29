import { describe, expect, it } from 'vitest';
import { ICONS, decorateSignal, motivoOptions } from './queueVisual';
import type { Signal } from './types';

const perfil: Signal = {
  id: 'perfil-1',
  kind: 'alert',
  client: 'customer-1',
  name: 'Fernanda Lima',
  segment: 'Essencial',
  alert: 'perfil',
  ago: 5,
};

describe('perfil alert', () => {
  it('decorates with its label and gauge icon', () => {
    const row = decorateSignal(perfil);
    expect(row.typeLabel).toBe('Compra acima do perfil');
    expect(row.icon).toBe(ICONS.perfil);
    expect(row.conf).toBeNull();
  });

  it('is a motivo option next to the other alert kinds', () => {
    const options = motivoOptions();
    expect(options).toContain('Compra acima do perfil');
    expect(options.indexOf('Compra acima do perfil')).toBe(options.indexOf('Sem contato há muito tempo') + 1);
  });
});
