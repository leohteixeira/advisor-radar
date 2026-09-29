import { ActionControl } from '../actions';
import type { SduiComponentProps } from '../registry';
import type { ProductListProps } from '../types';
import { RiskBars } from './RiskBars';

function Chevron() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M9 6l6 6-6 6" />
    </svg>
  );
}

/** Investir `fixed_income`, `etfs`, and `stocks`: one row per product, the whole row opens the purchase form. */
export function ProductList({ props }: SduiComponentProps) {
  const p = props as unknown as ProductListProps;
  return (
    <section className="sdui-card sdui-products" aria-label={p.title}>
      <h2>{p.title}</h2>
      <ul>
        {p.products.map((item, index) => (
          <li key={`${item.product_id}-${index}`}>
            <ActionControl
              action={item.action}
              className="sdui-products__row"
              render={(label) => (
                <>
                  <span className="sdui-products__body">
                    <strong>{item.name}</strong>
                    <span className="sdui-product__risk">
                      <RiskBars risk={item.risk} above={item.above_profile} />
                      {item.risk_label} · {item.return_label}
                    </span>
                    {item.badge ? <span className="sdui-badge">{item.badge}</span> : null}
                  </span>
                  <span className="visually-hidden">{label}</span>
                  <Chevron />
                </>
              )}
            />
          </li>
        ))}
      </ul>
    </section>
  );
}
