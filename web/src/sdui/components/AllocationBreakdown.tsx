import { MASKED_MONEY, useSdui } from '../context';
import type { SduiComponentProps } from '../registry';
import { allocationClass, barWidth } from '../style';
import type { AllocationBreakdownProps } from '../types';

/** Carteira `allocation`: each class with its value, share, and bar. */
export function AllocationBreakdown({ props }: SduiComponentProps) {
  const p = props as unknown as AllocationBreakdownProps;
  const { masked } = useSdui();
  return (
    <section className="sdui-card sdui-breakdown" aria-label={p.title}>
      <h2>{p.title}</h2>
      <ul>
        {p.rows.map((row, index) => (
          <li key={`${row.class}-${index}`}>
            <div className="sdui-breakdown__row">
              <i className={allocationClass(row.class)} aria-hidden="true" />
              <span>{row.label}</span>
              <b>{masked ? MASKED_MONEY : row.value}</b>
              <em>{row.share}</em>
            </div>
            <div className="sdui-breakdown__track" aria-hidden="true">
              <span className={allocationClass(row.class)} style={{ width: barWidth(row.bar_width) }} />
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
