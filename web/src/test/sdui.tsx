import { render } from '@testing-library/react';
import type { ReactNode } from 'react';
import { vi } from 'vitest';
import { SduiContext, type SduiContextValue } from '../sdui/context';

/** Renders SDUI UI inside an SduiContext with spy callbacks. */
export function renderSdui(ui: ReactNode, overrides: Partial<SduiContextValue> = {}) {
  const value: SduiContextValue = {
    onNavigate: vi.fn(),
    masked: false,
    onToggleMask: vi.fn(),
    onPanel: vi.fn(),
    onPurchase: vi.fn(),
    ...overrides,
  };
  const tree = (node: ReactNode, ctx: SduiContextValue) => <SduiContext value={ctx}>{node}</SduiContext>;
  const view = render(tree(ui, value));
  return {
    ...view,
    value,
    rerenderWith: (node: ReactNode, next: Partial<SduiContextValue> = {}) => view.rerender(tree(node, { ...value, ...next })),
  };
}
