/** Risk levels of a catalog product: one bar per level. */
export const RISK_LEVELS = 5;

/**
 * Five bars filled up to `risk`, in the warning color when the BFF marked the
 * product above the customer's profile. A risk outside 0–5 is kept inside it.
 */
export function RiskBars({ risk, above }: { risk: unknown; above: unknown }) {
  const level = typeof risk === 'number' && Number.isFinite(risk) ? Math.min(RISK_LEVELS, Math.max(0, Math.round(risk))) : 0;
  return (
    <span className="sdui-risk" data-above={above === true ? 'true' : 'false'} aria-hidden="true">
      {Array.from({ length: RISK_LEVELS }, (_, index) => (
        <i key={index} data-on={index < level ? 'true' : 'false'} />
      ))}
    </span>
  );
}
