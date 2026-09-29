import type { ComponentType } from 'react';
import { ActionGrid } from './components/ActionGrid';
import { ActivityList } from './components/ActivityList';
import { AdvisorCard } from './components/AdvisorCard';
import { MomentCard } from './components/MomentCard';
import { WealthSummary } from './components/WealthSummary';

/** What every registered component receives from the envelope. */
export interface SduiComponentProps {
  /** Informational only: it never drives rendering (specs/http/bff.md). */
  variant: string;
  props: Record<string, unknown>;
}

/**
 * The component registry: the only place a `type` is added. A new type needs
 * a component here and a row in the component table of specs/http/bff.md.
 */
const REGISTRY: Readonly<Record<string, ComponentType<SduiComponentProps>>> = {
  moment_card: MomentCard,
  wealth_summary: WealthSummary,
  action_grid: ActionGrid,
  advisor_card: AdvisorCard,
  activity_list: ActivityList,
};

/** Finds the component for a type, or undefined when web does not know it. */
export function lookup(type: string): ComponentType<SduiComponentProps> | undefined {
  return Object.hasOwn(REGISTRY, type) ? REGISTRY[type] : undefined;
}
