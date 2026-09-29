import { createContext, useContext } from 'react';
import type { Preferences, Slug } from './types';

/** Coded panels a panel action opens without a product. */
export type OpenPanel = 'deposit' | 'withdraw' | 'message' | 'complaint';

/** What SDUI components need from the screen that hosts them. */
export interface SduiContextValue {
  /** Opens another screen of the same customer, closing any open panel first. */
  onNavigate: (slug: Slug) => void;
  /** The eye toggle: money props render masked. Presentation only. */
  masked: boolean;
  onToggleMask: () => void;
  onPanel: (panel: OpenPanel) => void;
  /** Opens the coded purchase form for one product of the screen on view. */
  onPurchase: (productID: string) => void;
  /** The local theme: true when light. It lives in the browser only. */
  light: boolean;
  onToggleTheme: () => void;
  /**
   * Stores the channel and beta flag, then reloads the screen. It rejects
   * when the write fails, so the caller keeps the previous value.
   */
  onPreferences: (next: Preferences) => Promise<void>;
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
