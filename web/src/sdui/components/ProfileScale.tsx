import type { SduiComponentProps } from '../registry';
import type { ProfileScaleProps } from '../types';
import { RiskBars } from './RiskBars';

/**
 * Perfil `suitability`: the three investor profile levels with the client's
 * own marked. Each level's `max_risk` comes from advisory and fills the bars;
 * web holds no max-risk table. One column on the phone, three on desktop (CSS).
 */
export function ProfileScale({ props }: SduiComponentProps) {
  const p = props as unknown as ProfileScaleProps;
  return (
    <section className="sdui-card sdui-scale" aria-label={p.title}>
      <div className="sdui-rail__head">
        <h2>{p.title}</h2>
        <p>{p.subtitle}</p>
      </div>
      <ul className="sdui-scale__levels">
        {p.levels.map((level, index) => (
          <li key={`${level.key}-${index}`} className="sdui-level" data-current={level.current ? 'true' : 'false'} aria-current={level.current ? 'true' : undefined}>
            <div className="sdui-level__head">
              <strong>{level.label}</strong>
              {level.current ? <span className="sdui-level__chip">{p.current_label}</span> : null}
            </div>
            <RiskBars risk={level.max_risk} above={false} />
            <span className="sdui-level__desc">{level.description}</span>
            <span className="sdui-level__limit">{level.limit}</span>
          </li>
        ))}
      </ul>
      <p className="sdui-scale__footer">{p.footer}</p>
    </section>
  );
}
