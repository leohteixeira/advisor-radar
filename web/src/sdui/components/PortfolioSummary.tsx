import { MASKED_MONEY, useSdui } from '../context';
import { EyeIcon } from '../icons';
import type { SduiComponentProps } from '../registry';
import { toneClass } from '../style';
import type { PortfolioSummaryProps } from '../types';

const STAT_TONES = ['pos', 'neg', 'neutral'] as const;

/** Carteira `summary`: patrimony at market value, the day-change pill, and the portfolio stats. */
export function PortfolioSummary({ props }: SduiComponentProps) {
  const p = props as unknown as PortfolioSummaryProps;
  const { masked, onToggleMask } = useSdui();
  return (
    <section className="sdui-card sdui-portfolio" aria-label={p.total_label}>
      <div className="sdui-wealth__head">
        <span>{p.total_label}</span>
        <button type="button" aria-label={masked ? 'Mostrar valores' : 'Esconder valores'} aria-pressed={masked} onClick={onToggleMask}>
          <EyeIcon />
        </button>
      </div>
      <strong className="sdui-wealth__total">{masked ? MASKED_MONEY : p.total}</strong>
      {p.day_change ? (
        <span className={`sdui-pill ${toneClass(p.day_change_tone, STAT_TONES)}`}>{masked ? MASKED_MONEY : p.day_change}</span>
      ) : null}
      <dl className="sdui-stats">
        {p.stats.map((stat, index) => (
          <div key={`${stat.label}-${index}`}>
            <dt>{stat.label}</dt>
            <dd className={stat.tone ? toneClass(stat.tone, STAT_TONES) : undefined}>{masked && stat.money ? MASKED_MONEY : stat.value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
