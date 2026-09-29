import type { SduiComponentProps } from '../registry';
import type { ProfileHeaderProps } from '../types';

/** Perfil `header`: who the client is and, when registration answered, the account. */
export function ProfileHeader({ props }: SduiComponentProps) {
  const p = props as unknown as ProfileHeaderProps;
  return (
    <section className="sdui-card sdui-profile" aria-label={p.name}>
      <span className="sdui-profile__avatar" aria-hidden="true">
        {p.initials}
      </span>
      <div className="sdui-profile__body">
        <strong>{p.name}</strong>
        <span>{p.subtitle}</span>
        {p.account ? <span className="sdui-profile__account">{p.account}</span> : null}
      </div>
    </section>
  );
}
