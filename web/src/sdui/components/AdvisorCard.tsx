import { ActionControl } from '../actions';
import type { SduiComponentProps } from '../registry';
import type { AdvisorCardProps } from '../types';

/** Home and perfil `advisor`: who answers the client and how fast. */
export function AdvisorCard({ props }: SduiComponentProps) {
  const p = props as unknown as AdvisorCardProps;
  return (
    <section className="sdui-card sdui-advisor" aria-label={p.kicker}>
      <span className="sdui-advisor__avatar" aria-hidden="true">
        {p.initials}
      </span>
      <div className="sdui-advisor__body">
        <span className="sdui-kicker">{p.kicker}</span>
        <strong>{p.name}</strong>
        <span>{p.meta}</span>
      </div>
      {p.action ? <ActionControl action={p.action} className="sdui-btn sdui-btn--secondary" /> : null}
    </section>
  );
}
