import { act, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { BETA_OFF, BETA_ON, fernandaPerfil, preferenceList, profileFields, profileHeader, profileScale, thiagoPerfil } from '../test/perfilFixtures';
import { renderSdui } from '../test/sdui';
import { PreferenceList } from './components/PreferenceList';
import { ProfileFieldList } from './components/ProfileFieldList';
import { ProfileHeader } from './components/ProfileHeader';
import { ProfileScale } from './components/ProfileScale';
import { lookup } from './registry';
import { SduiScreen } from './SduiScreen';
import type { Component, Preferences } from './types';

const SAVE_FAILED = 'Não foi possível salvar. Tente de novo.';

afterEach(() => {
  vi.restoreAllMocks();
});

function prefsProps(prefs: Preferences) {
  return preferenceList(prefs).props;
}

/** A promise the test settles by hand. */
function deferred() {
  let resolve: () => void = () => undefined;
  let reject: (err: Error) => void = () => undefined;
  const promise = new Promise<void>((ok, fail) => {
    resolve = ok;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function onBars(item: HTMLElement) {
  return Array.from(item.querySelectorAll('.sdui-risk i')).filter((bar) => bar.getAttribute('data-on') === 'true').length;
}

describe('registry', () => {
  it.each(['profile_header', 'profile_scale', 'profile_field_list', 'preference_list'])('registers %s', (type) => {
    expect(lookup(type)).toBeDefined();
  });
});

describe('ProfileHeader', () => {
  it('shows the name, subtitle, and account', () => {
    const c = profileHeader('Fernanda Lima', 'FL', 'Cliente Essencial desde 2024', 'Conta 3301-7 · Orla Invest');
    renderSdui(<ProfileHeader variant={c.variant} props={c.props} />);
    const card = screen.getByRole('region', { name: 'Fernanda Lima' });
    expect(card).toHaveTextContent('FL');
    expect(card).toHaveTextContent('Cliente Essencial desde 2024');
    expect(within(card).getByText('Conta 3301-7 · Orla Invest')).toHaveClass('sdui-profile__account');
  });

  it('leaves the account out when registration failed', () => {
    const c = profileHeader('Fernanda Lima', 'FL', 'Cliente Essencial desde 2024');
    const { container } = renderSdui(<ProfileHeader variant={c.variant} props={c.props} />);
    expect(container.querySelector('.sdui-profile__account')).toBeNull();
  });
});

describe('ProfileScale', () => {
  it.each([
    ['conservador', 'Conservador'],
    ['moderado', 'Moderado'],
    ['arrojado', 'Arrojado'],
  ] as const)('marks %s as the client level', (profile, label) => {
    const c = profileScale(profile, '12/03/2026');
    renderSdui(<ProfileScale variant={c.variant} props={c.props} />);
    const card = screen.getByRole('region', { name: 'Seu perfil de investidor' });
    expect(card).toHaveTextContent('Define os destaques de Investir e quando uma compra recebe aviso.');
    const levels = within(card).getAllByRole('listitem');
    expect(levels.map((item) => item.querySelector('strong')?.textContent)).toEqual(['Conservador', 'Moderado', 'Arrojado']);
    const current = levels.filter((item) => item.getAttribute('aria-current') === 'true');
    expect(current).toHaveLength(1);
    expect(current[0]).toHaveTextContent(label);
    expect(current[0]).toHaveTextContent('Seu perfil');
    expect(within(card).getAllByText('Seu perfil')).toHaveLength(1);
    expect(card).toHaveTextContent('Última avaliação em 12/03/2026. Para refazer o questionário, fale com a sua assessora.');
  });

  it('fills the bars with the max risk the BFF sent', () => {
    const c = profileScale('moderado', '20/01/2026');
    renderSdui(<ProfileScale variant={c.variant} props={c.props} />);
    const levels = screen.getAllByRole('listitem');
    expect(levels.map(onBars)).toEqual([2, 3, 5]);
    expect(levels.map((item) => item.getAttribute('data-current'))).toEqual(['false', 'true', 'false']);
    expect(levels[2]).toHaveTextContent('Produtos até risco 5');
  });

  it('lays the levels in one column on the phone and three on desktop', () => {
    const c = profileScale('conservador', '12/03/2026');
    const { rerenderWith } = renderSdui(<ProfileScale variant={c.variant} props={c.props} />);
    expect(screen.getByRole('list')).toHaveAttribute('data-columns', '1');
    rerenderWith(<ProfileScale variant={c.variant} props={c.props} />, { wide: true });
    expect(screen.getByRole('list')).toHaveAttribute('data-columns', '3');
  });
});

describe('ProfileFieldList', () => {
  it('lists every field with the footnote', () => {
    const c = profileFields([
      ['Nome', 'Fernanda Lima'],
      ['E-mail', 'fernanda.lima@example.com'],
    ]);
    renderSdui(<ProfileFieldList variant={c.variant} props={c.props} />);
    const card = screen.getByRole('region', { name: 'Dados cadastrais' });
    expect(within(card).getByText('E-mail').nextElementSibling).toHaveTextContent('fernanda.lima@example.com');
    expect(card).toHaveTextContent('Dados fictícios. Alterar cadastro fica fora da simulação.');
  });

  it('draws no footnote when the BFF sent none', () => {
    const c = profileFields([['Nome', 'Fernanda Lima']]);
    const { container } = renderSdui(<ProfileFieldList variant={c.variant} props={{ ...c.props, footnote: '' }} />);
    expect(container.querySelector('.sdui-fields__note')).toBeNull();
  });
});

describe('PreferenceList', () => {
  it('shows the stored channel, the beta switch, and the local theme', () => {
    renderSdui(<PreferenceList variant="default" props={prefsProps({ channel: 'chat', beta: false })} />);
    const card = screen.getByRole('region', { name: 'Preferências' });
    const group = within(card).getByRole('radiogroup', { name: 'Canal preferido' });
    expect(within(group).getByRole('radio', { name: 'Chat' })).toBeChecked();
    expect(within(group).getByRole('radio', { name: 'E-mail' })).not.toBeChecked();
    const beta = within(card).getByRole('switch', { name: 'Programa beta' });
    expect(beta).toHaveAttribute('aria-checked', 'false');
    expect(beta).toHaveAccessibleDescription(BETA_OFF);
    const theme = within(card).getByRole('button', { name: 'Tema escuro' });
    expect(theme).toHaveTextContent('Escuro');
    expect(theme).toHaveAttribute('aria-pressed', 'true');
    expect(card).toHaveTextContent('Fica salvo só neste navegador');
    expect(card).toHaveAttribute('aria-busy', 'false');
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('toggles the local theme without saving', async () => {
    const user = userEvent.setup();
    const { value, rerenderWith } = renderSdui(<PreferenceList variant="default" props={prefsProps({ channel: 'chat', beta: false })} />);
    await user.click(screen.getByRole('button', { name: 'Tema escuro', pressed: true }));
    expect(value.onToggleTheme).toHaveBeenCalledOnce();
    expect(value.onPreferences).not.toHaveBeenCalled();
    rerenderWith(<PreferenceList variant="default" props={prefsProps({ channel: 'chat', beta: false })} />, { light: true });
    const theme = screen.getByRole('button', { name: 'Tema escuro', pressed: false });
    expect(theme).toHaveTextContent('Claro');
  });

  it('saves a channel, shows it while saving, and follows the reloaded screen', async () => {
    const user = userEvent.setup();
    const save = deferred();
    const onPreferences = vi.fn(() => save.promise);
    const { rerenderWith } = renderSdui(<PreferenceList variant="default" props={prefsProps({ channel: 'chat', beta: false })} />, { onPreferences });
    await user.click(screen.getByRole('radio', { name: 'E-mail' }));
    expect(onPreferences).toHaveBeenCalledWith({ channel: 'email', beta: false });
    expect(screen.getByRole('radio', { name: 'E-mail' })).toBeChecked();
    await act(async () => {
      save.resolve();
      await save.promise;
    });
    expect(screen.getByRole('radio', { name: 'E-mail' })).toBeChecked();
    expect(screen.queryByRole('alert')).toBeNull();

    rerenderWith(<PreferenceList variant="default" props={prefsProps({ channel: 'email', beta: false })} />, { onPreferences });
    expect(screen.getByRole('radio', { name: 'E-mail' })).toBeChecked();
    // A later reload (a reseed) brings chat back: the stored value wins.
    rerenderWith(<PreferenceList variant="default" props={prefsProps({ channel: 'chat', beta: false })} />, { onPreferences });
    expect(screen.getByRole('radio', { name: 'Chat' })).toBeChecked();
  });

  it('turns beta on from the keyboard and keeps the channel', async () => {
    const user = userEvent.setup();
    const { value, rerenderWith } = renderSdui(<PreferenceList variant="default" props={prefsProps({ channel: 'email', beta: false })} />);
    const beta = screen.getByRole('switch', { name: 'Programa beta' });
    beta.focus();
    await user.keyboard(' ');
    expect(value.onPreferences).toHaveBeenCalledWith({ channel: 'email', beta: true });
    expect(beta).toHaveAttribute('aria-checked', 'true');
    rerenderWith(<PreferenceList variant="default" props={prefsProps({ channel: 'email', beta: true })} />);
    expect(screen.getByRole('switch', { name: 'Programa beta' })).toHaveAccessibleDescription(BETA_ON);
  });

  it('is busy with both controls disabled while a change saves', async () => {
    const user = userEvent.setup();
    const save = deferred();
    const onPreferences = vi.fn(() => save.promise);
    renderSdui(<PreferenceList variant="default" props={prefsProps({ channel: 'chat', beta: false })} />, { onPreferences });
    await user.click(screen.getByRole('switch', { name: 'Programa beta' }));
    const card = screen.getByRole('region', { name: 'Preferências' });
    expect(card).toHaveAttribute('aria-busy', 'true');
    expect(screen.getByRole('switch', { name: 'Programa beta' })).toBeDisabled();
    expect(screen.getByRole('radio', { name: 'Chat' })).toBeDisabled();
    expect(screen.getByRole('radio', { name: 'E-mail' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Tema escuro' })).toBeEnabled();
    await user.click(screen.getByRole('radio', { name: 'E-mail' }));
    expect(onPreferences).toHaveBeenCalledOnce();
    await act(async () => {
      save.resolve();
      await save.promise;
    });
    expect(card).toHaveAttribute('aria-busy', 'false');
    expect(screen.getByRole('switch', { name: 'Programa beta' })).toBeEnabled();
    expect(screen.getByRole('radio', { name: 'E-mail' })).toBeEnabled();
  });

  it('drops the pending choice when a new envelope arrives with the same stored values', async () => {
    const user = userEvent.setup();
    const save = deferred();
    const onPreferences = vi.fn(() => save.promise);
    const { rerenderWith } = renderSdui(<PreferenceList variant="default" props={prefsProps({ channel: 'chat', beta: false })} />, { onPreferences });
    await user.click(screen.getByRole('radio', { name: 'E-mail' }));
    expect(screen.getByRole('radio', { name: 'E-mail' })).toBeChecked();
    // A re-read that still stores chat (another write won): the envelope wins.
    rerenderWith(<PreferenceList variant="default" props={prefsProps({ channel: 'chat', beta: false })} />, { onPreferences });
    expect(screen.getByRole('radio', { name: 'Chat' })).toBeChecked();
    await act(async () => {
      save.resolve();
      await save.promise;
    });
    expect(screen.getByRole('radio', { name: 'Chat' })).toBeChecked();
  });

  it('keeps the previous value and alerts when the save fails, then clears the alert on a retry', async () => {
    const user = userEvent.setup();
    const onPreferences = vi.fn().mockRejectedValueOnce(new Error('502')).mockResolvedValueOnce(undefined);
    renderSdui(<PreferenceList variant="default" props={prefsProps({ channel: 'chat', beta: false })} />, { onPreferences });
    await user.click(screen.getByRole('switch', { name: 'Programa beta' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(SAVE_FAILED);
    expect(screen.getByRole('switch', { name: 'Programa beta' })).toHaveAttribute('aria-checked', 'false');
    expect(screen.getByRole('radio', { name: 'Chat' })).toBeChecked();

    await user.click(screen.getByRole('switch', { name: 'Programa beta' }));
    expect(onPreferences).toHaveBeenLastCalledWith({ channel: 'chat', beta: true });
    expect(screen.queryByRole('alert')).toBeNull();
  });
});

describe('Perfil screen', () => {
  it.each([
    ['Fernanda', fernandaPerfil(), 'Conservador', 'fernanda.lima@example.com'],
    ['Thiago', thiagoPerfil(), 'Arrojado', 'thiago.azevedo@example.com'],
  ])('renders %s in catalog order', (_, envelope, level, email) => {
    const { container } = renderSdui(<SduiScreen screen={envelope} />);
    expect(screen.getByRole('heading', { level: 1, name: 'Perfil' })).toBeInTheDocument();
    expect(Array.from(container.querySelectorAll('.sdui-section')).map((node) => [node.getAttribute('data-section'), node.getAttribute('data-span')])).toEqual([
      ['header', '2'],
      ['suitability', '2'],
      ['registration', '1'],
      ['preferences', '1'],
      ['advisor', '2'],
    ]);
    expect(screen.getByRole('listitem', { current: true })).toHaveTextContent(level);
    expect(screen.getByText(email)).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Sua assessora' })).toBeInTheDocument();
  });

  it('renders the sections the BFF kept when some were omitted', () => {
    const envelope = fernandaPerfil();
    const kept = envelope.sections.filter((section) => section.id === 'preferences');
    const withoutAdvisory = { ...envelope, sections: kept, omitted: [{ id: 'header', type: 'profile_header', reason: 'advisory' }] };
    renderSdui(<SduiScreen screen={withoutAdvisory} />);
    expect(screen.getByRole('region', { name: 'Preferências' })).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Seu perfil de investidor' })).toBeNull();
  });

  it('draws an unknown level list without crashing', () => {
    const c: Component = { type: 'profile_scale', variant: 'x', props: { ...profileScale('moderado', '20/01/2026').props, levels: [] } };
    renderSdui(<ProfileScale variant={c.variant} props={c.props} />);
    expect(screen.queryAllByRole('listitem')).toHaveLength(0);
  });
});
