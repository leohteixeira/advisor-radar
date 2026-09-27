const MINUTES_PER_HOUR = 60;
const MINUTES_PER_DAY = 1440;
const MINUTES_PER_MONTH = 43_200;
const MINUTES_PER_YEAR = 518_400;

/** Formats timeline age (minutes) as a Portuguese relative label with the `há` prefix. */
export function formatTimelineAgo(minutes: number): string {
  const ago = Math.max(0, Math.trunc(Math.round(minutes)));

  if (ago < MINUTES_PER_HOUR) {
    return `há ${ago} min`;
  }

  if (ago < MINUTES_PER_DAY) {
    const hours = Math.floor(ago / MINUTES_PER_HOUR);
    const mins = ago % MINUTES_PER_HOUR;
    return `há ${hours}h${String(mins).padStart(2, '0')}min`;
  }

  if (ago < MINUTES_PER_MONTH) {
    const days = Math.floor(ago / MINUTES_PER_DAY);
    return days === 1 ? 'há 1 dia' : `há ${days} dias`;
  }

  if (ago < MINUTES_PER_YEAR) {
    const months = Math.floor(ago / MINUTES_PER_MONTH);
    return months === 1 ? 'há 1 mês' : `há ${months} meses`;
  }

  const years = Math.floor(ago / MINUTES_PER_YEAR);
  return years === 1 ? 'há 1 ano' : `há ${years} anos`;
}
