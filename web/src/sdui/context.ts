import { createContext, useContext } from 'react';
import type { Slug } from './types';

/** Panels the client app can open from a panel action today. */
export type OpenPanel = 'deposit' | 'withdraw' | 'message' | 'complaint';

/** What SDUI components need from the screen that hosts them. */
export interface SduiContextValue {
  /** Opens another screen of the same customer, closing any open panel first. */
  onNavigate: (slug: Slug) => void;
  /** The eye toggle: money props render masked. Presentation only. */
  masked: boolean;
  onToggleMask: () => void;
  onPanel: (panel: OpenPanel) => void;
}

export const SduiContext = createContext<SduiContextValue | null>(null);

export function useSdui(): SduiContextValue {
  const value = useContext(SduiContext);
  if (!value) {
    throw new Error('sdui: component rendered outside SduiContext');
  }
  return value;
}

/** Masked money, as the eye toggle shows it. */
export const MASKED_MONEY = 'US$ ••••••';
