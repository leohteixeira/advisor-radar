import type { SduiComponentProps } from '../registry';
import type { ProfileFieldListProps } from '../types';

/** Perfil `registration`: read-only fictional registration data, one row per field. */
export function ProfileFieldList({ props }: SduiComponentProps) {
  const p = props as unknown as ProfileFieldListProps;
  return (
    <section className="sdui-card sdui-fields" aria-label={p.title}>
      <h2>{p.title}</h2>
      <dl>
        {p.fields.map((field, index) => (
          <div key={`${field.label}-${index}`}>
            <dt>{field.label}</dt>
            <dd>{field.value}</dd>
          </div>
        ))}
      </dl>
      {p.footnote ? <p className="sdui-fields__note">{p.footnote}</p> : null}
    </section>
  );
}
