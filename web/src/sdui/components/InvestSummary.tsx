import { MASKED_MONEY, useSdui } from '../context';
import type { SduiComponentProps } from '../registry';
import type { InvestSummaryProps } from '../types';

/** Investir `cash`: the cash available to invest and the profile chip. */
export function InvestSummary({ props }: SduiComponentProps) {
  const p = props as unknown as InvestSummaryProps;
  const { masked } = useSdui();
  return (
    <section className="sdui-card sdui-invest" aria-label={p.cash_label}>
      <div className="sdui-invest__cash">
        <span>{p.cash_label}</span>
        <strong>{masked ? MASKED_MONEY : p.cash}</strong>
      </div>
      {p.profile_chip ? <span className="sdui-invest__chip">{p.profile_chip}</span> : null}
    </section>
  );
}
