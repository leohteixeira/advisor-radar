import { useId, useState } from 'react';
import { useSdui } from '../context';
import type { SduiComponentProps } from '../registry';
import type { PreferenceListProps, Preferences } from '../types';

/** Coded control copy: the only words web adds to this component. */
const SAVE_FAILED = 'Não foi possível salvar. Tente de novo.';
const THEME_NAME = { light: 'Claro', dark: 'Escuro' } as const;
/** The theme button is a toggle for the dark theme: pressed while dark. */
const THEME_TOGGLE = 'escuro';

/**
 * Perfil `preferences`. The theme toggles the local theme and never reaches
 * the server. The channel radio group and the beta switch store both values
 * through the host, which then reloads the screen. While a change is saving
 * it shows as chosen, the list is busy, and both controls are disabled, so no
 * second change starts; when the save fails the stored value comes back and
 * an alert asks for another try. Each new screen envelope (new props) wins
 * over any choice still shown.
 */
export function PreferenceList({ props }: SduiComponentProps) {
  const p = props as unknown as PreferenceListProps;
  const { light, onToggleTheme, onPreferences } = useSdui();
  const id = useId();
  const stored: Preferences = { channel: p.channel.value, beta: p.beta.enabled };
  const [pending, setPending] = useState<Preferences | null>(null);
  const [seenProps, setSeenProps] = useState(props);
  const [failed, setFailed] = useState(false);
  const [saving, setSaving] = useState(false);
  if (seenProps !== props) {
    setSeenProps(props);
    setPending(null);
  }
  const shown = pending ?? stored;
  const themeName = light ? THEME_NAME.light : THEME_NAME.dark;

  async function save(next: Preferences) {
    setSaving(true);
    setFailed(false);
    setPending(next);
    try {
      await onPreferences(next);
    } catch {
      setPending(null);
      setFailed(true);
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="sdui-card sdui-prefs" aria-label={p.title} aria-busy={saving}>
      <h2>{p.title}</h2>
      <div className="sdui-prefs__row">
        <div className="sdui-prefs__text">
          <strong>{p.theme.label}</strong>
          <span>{p.theme.hint}</span>
        </div>
        <button
          type="button"
          className="sdui-btn sdui-btn--secondary"
          aria-label={`${p.theme.label} ${THEME_TOGGLE}`}
          aria-pressed={!light}
          onClick={onToggleTheme}
        >
          {themeName}
        </button>
      </div>
      <div className="sdui-prefs__row sdui-prefs__row--stack">
        <div className="sdui-prefs__text">
          <strong id={`${id}-channel`}>{p.channel.label}</strong>
          <span>{p.channel.hint}</span>
        </div>
        <div role="radiogroup" aria-labelledby={`${id}-channel`} className="sdui-seg">
          {p.channel.options.map((option) => (
            <label key={option.value} className="sdui-seg__opt" data-on={shown.channel === option.value ? 'true' : 'false'}>
              <input
                type="radio"
                name={`${id}-channel`}
                value={option.value}
                checked={shown.channel === option.value}
                disabled={saving}
                onChange={() => void save({ channel: option.value, beta: shown.beta })}
              />
              <span>{option.label}</span>
            </label>
          ))}
        </div>
      </div>
      <div className="sdui-prefs__row">
        <div className="sdui-prefs__text">
          <strong>{p.beta.label}</strong>
          <span id={`${id}-beta`}>{p.beta.hint}</span>
        </div>
        <button
          type="button"
          role="switch"
          className="sdui-switch"
          aria-checked={shown.beta}
          aria-label={p.beta.label}
          aria-describedby={`${id}-beta`}
          disabled={saving}
          onClick={() => void save({ channel: shown.channel, beta: !shown.beta })}
        >
          <span className="sdui-switch__track" aria-hidden="true">
            <span className="sdui-switch__knob" />
          </span>
        </button>
      </div>
      {failed ? (
        <p role="alert" className="sdui-prefs__error">
          {SAVE_FAILED}
        </p>
      ) : null}
    </section>
  );
}
