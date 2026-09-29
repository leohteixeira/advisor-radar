import type { Tone } from './types';

/**
 * Maps a server `tone` to its color class. A tone outside `accepted` (the
 * tones the component type lists) renders as neutral.
 */
export function toneClass(tone: unknown, accepted: readonly Tone[]): string {
  const known = accepted.find((value) => value === tone);
  return `sdui-tone--${known ?? 'neutral'}`;
}

const ALLOCATION_CLASSES = ['stocks', 'etfs', 'fixed_income', 'cash'] as const;

/** Maps an allocation `class` to its color class; an unknown class is neutral. */
export function allocationClass(value: unknown): string {
  const known = ALLOCATION_CLASSES.find((name) => name === value);
  return `sdui-alloc--${known ?? 'other'}`;
}

/** Keeps a server `bar_width` inside 0–100 for use as a CSS width. */
export function barWidth(value: unknown): string {
  const n = typeof value === 'number' && Number.isFinite(value) ? Math.min(100, Math.max(0, value)) : 0;
  return `${n}%`;
}
