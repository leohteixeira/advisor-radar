import { renderHook, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { EMPTY_ACTIVITY, fernandaHome, marianaHome, thiagoHome } from '../test/sduiFixtures';
import { renderSdui } from '../test/sdui';
import { useSdui } from './context';
import { lookup } from './registry';
import { SduiLoading, SduiScreen } from './SduiScreen';
import type { Screen } from './types';

afterEach(() => {
  vi.restoreAllMocks();
});

function sectionIDs(container: HTMLElement): (string | null)[] {
  return Array.from(container.querySelectorAll('.sdui-section')).map((node) => node.getAttribute('data-section'));
}

describe('SduiScreen seed homes', () => {
  it('renders Fernanda', () => {
    const { container } = renderSdui(<SduiScreen screen={fernandaHome()} />);
    expect(screen.getByRole('heading', { level: 1, name: 'Olá, Fernanda' })).toBeInTheDocument();
    expect(screen.getByText('Cliente Essencial desde 2024')).toBeInTheDocument();
    expect(sectionIDs(container)).toEqual(['moment', 'wealth', 'actions', 'advisor', 'activity']);
    expect(screen.getByText('US$ 8.200,00')).toBeInTheDocument();
    expect(screen.getByText('Resposta em até 24 h · cliente Essencial')).toBeInTheDocument();
    expect(screen.getByText('Suas movimentações aparecem aqui assim que acontecerem.')).toBeInTheDocument();
  });

  it('renders Thiago with the wealth values from props', () => {
    const { container } = renderSdui(<SduiScreen screen={thiagoHome()} />);
    expect(screen.getByRole('heading', { level: 1, name: 'Olá, Thiago' })).toBeInTheDocument();
    expect(sectionIDs(container)).toEqual(['moment', 'wealth', 'actions', 'advisor', 'activity']);
    const wealth = screen.getByRole('region', { name: 'Patrimônio total' });
    expect(within(wealth).getByText('US$ 68.000,00')).toBeInTheDocument();
    expect(within(wealth).getAllByRole('listitem').map((li) => li.textContent)).toEqual(['Ações3%', 'ETFs8%', 'Caixa89%']);
    expect(screen.getByText('Depósito via câmbio')).toBeInTheDocument();
    expect(container.querySelector('[data-section="moment"]')).toHaveAttribute('data-span', '2');
    expect(container.querySelector('[data-section="wealth"]')).toHaveAttribute('data-span', '1');
  });

  it('renders Mariana with the dedicated advisor', () => {
    renderSdui(<SduiScreen screen={marianaHome()} />);
    expect(screen.getByRole('heading', { level: 1, name: 'Olá, Mariana' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Sua assessora dedicada' })).toHaveTextContent('Resposta em até 1 h · cliente Singular');
    expect(screen.getByText('US$ 248.300,00')).toBeInTheDocument();
  });
});

describe('SduiScreen rendering rules', () => {
  it('renders sections in array order', () => {
    const home = thiagoHome();
    home.sections.reverse();
    const { container } = renderSdui(<SduiScreen screen={home} />);
    expect(sectionIDs(container)).toEqual(['activity', 'advisor', 'actions', 'wealth', 'moment']);
  });

  it('renders the heading without a subtitle', () => {
    const { container } = renderSdui(<SduiScreen screen={{ ...thiagoHome(), title: 'Olá', subtitle: undefined }} />);
    expect(screen.getByRole('heading', { level: 1, name: 'Olá' })).toBeInTheDocument();
    expect(container.querySelector('.sdui-screen__head span')).toBeNull();
  });

  it('renders nothing for an omitted section', () => {
    const home: Screen = {
      ...thiagoHome(),
      sections: thiagoHome().sections.filter((section) => section.id !== 'activity'),
      omitted: [{ id: 'activity', type: 'activity_list', reason: 'timeline' }],
    };
    const { container } = renderSdui(<SduiScreen screen={home} />);
    expect(sectionIDs(container)).toEqual(['moment', 'wealth', 'actions', 'advisor']);
    expect(screen.queryByText('Atividade recente')).not.toBeInTheDocument();
  });

  it('renders nothing for an unknown type, reports it once, and keeps the rest', () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const home = thiagoHome();
    home.sections[0]?.components.unshift({ type: 'hero_banner', variant: 'default', props: { title: 'Banner' } });
    const { container, rerenderWith } = renderSdui(<SduiScreen screen={home} />);
    rerenderWith(<SduiScreen screen={home} />, { masked: true });
    expect(screen.queryByText('Banner')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Olá, Thiago. Sua conta está em dia.' })).toBeInTheDocument();
    expect(sectionIDs(container)).toHaveLength(5);
    expect(error).toHaveBeenCalledTimes(1);
    expect(error).toHaveBeenCalledWith('sdui: unknown component type hero_banner');
  });

  it('isolates a component that throws and keeps its siblings', () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const home = thiagoHome();
    const broken = { type: 'wealth_summary', variant: 'default', props: { total_label: 'Patrimônio total' } };
    home.sections[1] = { id: 'wealth', components: [broken] };
    home.sections[0]?.components.push({ type: 'moment_card', variant: 'welcome', props: { kicker: 'K', title: { bad: true }, body: 'B', tone: 'neutral' } });
    const { container } = renderSdui(<SduiScreen screen={home} />);
    expect(container.querySelector('[data-section="wealth"]')).toBeEmptyDOMElement();
    expect(screen.getByRole('heading', { name: 'Olá, Thiago. Sua conta está em dia.' })).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'K' })).not.toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: 'Ações' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Atividade recente' })).toBeInTheDocument();
    expect(error).toHaveBeenCalledWith('sdui: component wealth_summary failed', expect.any(TypeError));
    expect(error).toHaveBeenCalledWith('sdui: component moment_card failed', expect.any(Error));
  });

  it('renders a component again when a new envelope fixes it', () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const broken: Screen = { ...thiagoHome(), sections: [{ id: 'activity', components: [{ type: 'activity_list', variant: 'recent', props: { title: 'Atividade recente' } }] }] };
    const { rerenderWith } = renderSdui(<SduiScreen screen={broken} />);
    expect(screen.queryByRole('region', { name: 'Atividade recente' })).not.toBeInTheDocument();
    rerenderWith(<SduiScreen screen={broken} />);
    expect(screen.queryByRole('region', { name: 'Atividade recente' })).not.toBeInTheDocument();
    const fixed: Screen = { ...broken, sections: [{ id: 'activity', components: [EMPTY_ACTIVITY] }] };
    rerenderWith(<SduiScreen screen={fixed} />);
    expect(screen.getByRole('region', { name: 'Atividade recente' })).toBeInTheDocument();
  });

  it.each(['extrato', 'constructor', 'toString'])('spans one column for the unknown slug %s', (slug) => {
    const { container } = renderSdui(<SduiScreen screen={{ ...thiagoHome(), slug: slug as Screen['slug'] }} />);
    expect(container.querySelector('[data-section="moment"]')).toHaveAttribute('data-span', '1');
  });
});

describe('registry', () => {
  it('knows the five home types and nothing else', () => {
    for (const type of ['moment_card', 'wealth_summary', 'action_grid', 'advisor_card', 'activity_list']) {
      expect(lookup(type)).toBeTypeOf('function');
    }
    expect(lookup('hero_banner')).toBeUndefined();
    expect(lookup('toString')).toBeUndefined();
  });
});

describe('SduiLoading', () => {
  it('shows the skeleton status and the note', () => {
    renderSdui(<SduiLoading />);
    expect(screen.getByRole('status')).toHaveTextContent('Montando sua tela…');
    expect(screen.getByText(/Uma requisição só\./)).toBeInTheDocument();
  });
});

describe('useSdui', () => {
  it('fails outside an SduiContext', () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    expect(() => renderHook(() => useSdui())).toThrow('sdui: component rendered outside SduiContext');
  });
});
