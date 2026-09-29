import { MASKED_MONEY, useSdui } from '../context';
import { EyeIcon } from '../icons';
import type { SduiComponentProps } from '../registry';
import { allocationClass, barWidth, toneClass } from '../style';
import type { WealthSummaryProps } from '../types';

const CHANGE_TONES = ['pos', 'neg', 'neutral'] as const;

/** Home `wealth`: patrimony, the allocation bar, and cash. */
export function WealthSummary({ props }: SduiComponentProps) {
  const p = props as unknown as WealthSummaryProps;
  const { masked, onToggleMask } = useSdui();
  return (
    <section className="sdui-card sdui-wealth" aria-label={p.total_label}>
      <div className="sdui-wealth__head">
        <span>{p.total_label}</span>
        <button type="button" aria-label={masked ? 'Mostrar valores' : 'Esconder valores'} aria-pressed={masked} onClick={onToggleMask}>
          <EyeIcon />
        </button>
      </div>
      <strong className="sdui-wealth__total">{masked ? MASKED_MONEY : p.total}</strong>
      {p.day_change ? (
        <span className={`sdui-pill ${toneClass(p.day_change_tone, CHANGE_TONES)}`}>{masked ? MASKED_MONEY : p.day_change}</span>
      ) : null}
      <div className="sdui-bar" aria-hidden="true">
        {p.allocation.map((row, index) => (
          <span key={`${row.class}-${index}`} className={allocationClass(row.class)} style={{ width: barWidth(row.bar_width) }} />
        ))}
      </div>
      <ul className="sdui-legend">
        {p.allocation.map((row, index) => (
          <li key={`${row.class}-${index}`}>
            <i className={allocationClass(row.class)} aria-hidden="true" />
            <span>{row.label}</span>
            <b>{row.share}</b>
          </li>
        ))}
      </ul>
      <p className="sdui-wealth__cash">
        <span>{p.cash_label}</span>
        <b>{masked ? MASKED_MONEY : p.cash}</b>
      </p>
    </section>
  );
}
