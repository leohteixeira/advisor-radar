import { MASKED_MONEY, useSdui } from '../context';
import type { SduiComponentProps } from '../registry';
import { toneClass } from '../style';
import type { PositionListProps } from '../types';

const RETURN_TONES = ['pos', 'neg', 'neutral'] as const;

/** Carteira `positions_stocks`, `positions_etf`, and `positions_fixed_income`: one class with its subtotal. */
export function PositionList({ props }: SduiComponentProps) {
  const p = props as unknown as PositionListProps;
  const { masked } = useSdui();
  const money = (value: string) => (masked ? MASKED_MONEY : value);
  return (
    <section className="sdui-card sdui-positions" aria-label={p.title}>
      <div className="sdui-positions__head">
        <h2>{p.title}</h2>
        <b>{money(p.subtotal)}</b>
      </div>
      <ul>
        {p.items.map((item, index) => (
          <li key={`${item.product_id}-${index}`}>
            <div>
              <strong>{item.name}</strong>
              <span>
                {p.applied_label} {money(item.applied)}
              </span>
            </div>
            <div className="sdui-positions__value">
              <strong>{money(item.value)}</strong>
              <span className={toneClass(item.return_tone, RETURN_TONES)}>{item.return}</span>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
