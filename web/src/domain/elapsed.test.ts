import { describe, expect, it } from 'vitest';
import { formatTimelineAgo } from './elapsed';

describe('formatTimelineAgo', () => {
  it.each([
    [0, 'há 0 min'],
    [45, 'há 45 min'],
    [60, 'há 1h00min'],
    [123, 'há 2h03min'],
    [1440, 'há 1 dia'],
    [21002, 'há 14 dias'],
    [43200, 'há 1 mês'],
    [11 * 43200, 'há 11 meses'],
    [2 * 518400, 'há 2 anos'],
    [-5, 'há 0 min'],
  ] as const)('formats %i minutes as %s', (minutes, expected) => {
    expect(formatTimelineAgo(minutes)).toBe(expected);
  });
});
