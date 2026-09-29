import { ActionControl } from '../actions';
import { useSdui } from '../context';
import type { SduiComponentProps } from '../registry';
import type { ProductRailProps } from '../types';
import { RiskBars } from './RiskBars';

/**
 * Investir `highlights`: the products the BFF picked for the profile, one
 * card each. The grid is one column on the phone and two on desktop
 * (Investir-Desktop); `data-columns` carries the count to the CSS.
 */
export function ProductRail({ props }: SduiComponentProps) {
  const p = props as unknown as ProductRailProps;
  const { wide } = useSdui();
  return (
    <section className="sdui-rail" aria-label={p.title}>
      <div className="sdui-rail__head">
        <h2>{p.title}</h2>
        {p.subtitle ? <p>{p.subtitle}</p> : null}
      </div>
      <div className="sdui-rail__grid" data-columns={wide ? 2 : 1}>
        {p.products.map((item, index) => (
          <article key={`${item.product_id}-${index}`} className="sdui-card sdui-product" aria-label={item.name}>
            <span className="sdui-kicker">{item.class_label}</span>
            <strong className="sdui-product__name">{item.name}</strong>
            <span className="sdui-product__risk">
              <RiskBars risk={item.risk} above={item.above_profile} />
              {item.risk_label}
            </span>
            <span className="sdui-product__return">{item.return_label}</span>
            <span className="sdui-product__min">{item.minimum}</span>
            {item.badge ? <span className="sdui-badge">{item.badge}</span> : null}
            <ActionControl action={item.action} className="sdui-btn sdui-btn--primary" />
          </article>
        ))}
      </div>
    </section>
  );
}
