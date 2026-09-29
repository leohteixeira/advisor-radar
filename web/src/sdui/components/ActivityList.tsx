import { MASKED_MONEY, useSdui } from '../context';
import { SduiIcon } from '../icons';
import type { SduiComponentProps } from '../registry';
import { toneClass } from '../style';
import type { ActivityListProps } from '../types';

const TONES = ['pos', 'neg', 'neutral'] as const;

/** Home `activity` and carteira `history`: recent client-facing movements. */
export function ActivityList({ props }: SduiComponentProps) {
  const p = props as unknown as ActivityListProps;
  const { masked } = useSdui();
  return (
    <section className="sdui-card sdui-activity" aria-label={p.title}>
      <h2>{p.title}</h2>
      {p.items.length === 0 && p.empty_text ? <p className="sdui-activity__empty">{p.empty_text}</p> : null}
      {p.items.length > 0 ? (
        <ul>
          {p.items.map((item, index) => {
            const tone = toneClass(item.tone, TONES);
            return (
              <li key={`${item.title}-${index}`}>
                <span className={`sdui-activity__icon ${tone}`} aria-hidden="true">
                  <SduiIcon name={item.icon} size={18} />
                </span>
                <div>
                  <strong>{item.title}</strong>
                  <span>{item.meta}</span>
                </div>
                {item.value ? <b className={tone}>{masked ? MASKED_MONEY : item.value}</b> : null}
              </li>
            );
          })}
        </ul>
      ) : null}
    </section>
  );
}
