import { ActionControl } from '../actions';
import { ArrowIcon, hasIcon, SduiIcon } from '../icons';
import type { SduiComponentProps } from '../registry';
import { toneClass } from '../style';
import type { MomentCardProps } from '../types';

const TONES = ['neg', 'info', 'gold', 'neutral'] as const;

/** Home `moment`: what matters for this client now, with one optional action. */
export function MomentCard({ props }: SduiComponentProps) {
  const p = props as unknown as MomentCardProps;
  return (
    <section className={`sdui-moment ${toneClass(p.tone, TONES)}`} aria-label={p.kicker}>
      <div className="sdui-moment__head">
        {hasIcon(p.icon) ? (
          <span className="sdui-moment__icon" aria-hidden="true">
            <SduiIcon name={p.icon} size={18} />
          </span>
        ) : null}
        <span className="sdui-kicker">{p.kicker}</span>
      </div>
      <h2>{p.title}</h2>
      <p>{p.body}</p>
      {p.meta ? <p className="sdui-moment__meta">{p.meta}</p> : null}
      {p.action ? (
        <ActionControl
          action={p.action}
          className="sdui-btn sdui-btn--primary"
          render={(label) => (
            <>
              {label}
              <ArrowIcon />
            </>
          )}
        />
      ) : null}
    </section>
  );
}
