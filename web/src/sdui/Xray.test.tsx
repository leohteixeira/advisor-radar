import { screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderSdui } from '../test/sdui';
import { marianaHome, SEED, thiagoHome } from '../test/sduiFixtures';
import { SduiScreen } from './SduiScreen';
import type { Screen } from './types';
import { failureNotes, omittedLabel, sectionLabel, xrayMeta } from './Xray';

afterEach(() => {
  vi.restoreAllMocks();
});

function timelineDown(): Screen {
  const home = thiagoHome();
  return {
    ...home,
    sections: home.sections.filter((section) => section.id !== 'activity'),
    omitted: [{ id: 'activity', type: 'activity_list', reason: 'timeline' }],
  };
}

function labels(container: HTMLElement): (string | null)[] {
  return Array.from(container.querySelectorAll('.sdui-xray-label')).map((node) => node.textContent);
}

describe('Raio-X off', () => {
  it('puts nothing of Raio-X in the DOM', () => {
    const { container } = renderSdui(<SduiScreen screen={timelineDown()} />);
    expect(container.querySelector('.sdui-screen')).not.toHaveAttribute('data-xray');
    expect(container.querySelector('.sdui-xray-banner')).toBeNull();
    expect(container.querySelector('.sdui-xray-label')).toBeNull();
    expect(container.querySelector('.sdui-section--omitted')).toBeNull();
    expect(screen.queryByText(/omitido/)).not.toBeInTheDocument();
    expect(screen.queryByText(/fora do ar/)).not.toBeInTheDocument();
  });
});

describe('Raio-X on', () => {
  it('labels every rendered section with id, type, and variant, and shows the banner', () => {
    const { container } = renderSdui(<SduiScreen screen={thiagoHome()} xray={{ customerID: SEED.thiago }} />);
    expect(container.querySelector('.sdui-screen')).toHaveAttribute('data-xray', 'on');
    expect(labels(container)).toEqual([
      'moment · moment_card · welcome',
      'wealth · wealth_summary · default',
      'actions · action_grid · default',
      'advisor · advisor_card · default',
      'activity · activity_list · recent',
    ]);
    const banner = screen.getByRole('region', { name: 'Raio-X SDUI' });
    expect(within(banner).getByText(`GET /v1/client-pov/customers/${SEED.thiago}/screens/home`)).toBeInTheDocument();
    expect(within(banner).getByText('slug home · revision v1 · schema 1 · 5 seções')).toBeInTheDocument();
    expect(container.querySelector('.sdui-section--omitted')).toBeNull();
  });

  it('keeps the components rendering under the labels', () => {
    renderSdui(<SduiScreen screen={marianaHome()} xray={{ customerID: SEED.mariana }} />);
    expect(screen.getByText('advisor · advisor_card · dedicated')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Sua assessora dedicada' })).toBeInTheDocument();
    expect(screen.getByText('US$ 248.300,00')).toBeInTheDocument();
  });

  it('draws an omitted section as a dashed placeholder after the rendered ones and names the source in the banner', () => {
    const { container } = renderSdui(<SduiScreen screen={timelineDown()} xray={{ customerID: SEED.thiago }} />);
    const sections = Array.from(container.querySelectorAll('.sdui-section'));
    expect(sections.map((node) => node.getAttribute('data-section'))).toEqual(['moment', 'wealth', 'actions', 'advisor', 'activity']);
    const placeholder = sections.at(-1) as HTMLElement;
    expect(placeholder).toHaveClass('sdui-section--omitted');
    expect(placeholder).toHaveAttribute('data-span', '1');
    expect(within(placeholder).getAllByText('activity · activity_list · omitido')).toHaveLength(2);
    expect(within(placeholder).getByText(/O timeline-indexer não respondeu a tempo/)).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Atividade recente' })).not.toBeInTheDocument();
    expect(screen.getByText('slug home · revision v1 · schema 1 · 4 seções · timeline fora do ar')).toBeInTheDocument();
  });

  it('spans an omitted wide section over both columns', () => {
    const home = thiagoHome();
    const screenWithout: Screen = {
      ...home,
      sections: home.sections.slice(1),
      omitted: [{ id: 'moment', type: 'moment_card', reason: 'build_error' }],
    };
    const { container } = renderSdui(<SduiScreen screen={screenWithout} xray={{ customerID: SEED.thiago }} />);
    expect(container.querySelector('.sdui-section--omitted')).toHaveAttribute('data-span', '2');
    expect(screen.getByText(/O bff não conseguiu montar a seção/)).toBeInTheDocument();
    expect(screen.getByText('slug home · revision v1 · schema 1 · 4 seções · erro ao montar moment')).toBeInTheDocument();
  });

  it('keeps placeholders in omitted order and reports each source once', () => {
    const screenWithout: Screen = {
      ...thiagoHome(),
      sections: thiagoHome().sections.slice(0, 1),
      omitted: [
        { id: 'wealth', type: 'wealth_summary', reason: 'account-sim' },
        { id: 'advisor', type: 'advisor_card', reason: 'advisory' },
        { id: 'activity', type: 'activity_list', reason: 'timeline' },
        { id: 'actions', type: 'action_grid', reason: 'build_error' },
        { id: 'highlights', type: 'product_rail', reason: 'advisory' },
        { id: 'extra', type: 'hero_banner', reason: '' },
        { id: 'late', type: 'late_list', reason: 'ledger' },
      ],
    };
    const { container } = renderSdui(<SduiScreen screen={screenWithout} xray={{ customerID: SEED.thiago }} />);
    const omitted = Array.from(container.querySelectorAll('.sdui-section--omitted')).map((node) => node.getAttribute('data-section'));
    expect(omitted).toEqual(['wealth', 'advisor', 'activity', 'actions', 'highlights', 'extra', 'late']);
    expect(
      screen.getByText(
        'slug home · revision v1 · schema 1 · 1 seção · account-sim fora do ar · advisory fora do ar · timeline fora do ar · erro ao montar actions · ledger fora do ar',
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(/O account-sim não respondeu a tempo/)).toBeInTheDocument();
    expect(screen.getAllByText(/O advisory não respondeu a tempo/)).toHaveLength(2);
    expect(screen.getByText(/O ledger não respondeu a tempo/)).toBeInTheDocument();
    expect(screen.getByText(/^O bff tirou a seção da resposta\./)).toBeInTheDocument();
  });

  it('labels a section without components by its id and counts it', () => {
    const empty: Screen = { ...thiagoHome(), sections: [{ id: 'moment', components: [] }], omitted: [] };
    const { container } = renderSdui(<SduiScreen screen={empty} xray={{ customerID: 'a b' }} />);
    expect(labels(container)).toEqual(['moment']);
    expect(screen.getByText('GET /v1/client-pov/customers/a%20b/screens/home')).toBeInTheDocument();
    expect(screen.getByText('slug home · revision v1 · schema 1 · 1 seção')).toBeInTheDocument();
  });

  it('names advisory behind the moments and profile reasons', () => {
    const screenWithout: Screen = {
      ...thiagoHome(),
      sections: thiagoHome().sections.slice(1),
      omitted: [
        { id: 'moment', type: 'moment_card', reason: 'moments' },
        { id: 'highlights', type: 'product_rail', reason: 'profile' },
      ],
    };
    renderSdui(<SduiScreen screen={screenWithout} xray={{ customerID: SEED.thiago }} />);
    expect(screen.getAllByText(/^O advisory não respondeu a tempo/)).toHaveLength(2);
    expect(screen.queryByText(/O moments|O profile/)).not.toBeInTheDocument();
    expect(screen.getByText('slug home · revision v1 · schema 1 · 4 seções · moments fora do ar · profile fora do ar')).toBeInTheDocument();
  });

  it('shows an empty screen with every section omitted', () => {
    const none: Screen = { ...thiagoHome(), revision: 'v2', schema_version: 2, sections: [], omitted: [{ id: 'moment', type: 'moment_card', reason: 'build_error' }] };
    renderSdui(<SduiScreen screen={none} xray={{ customerID: SEED.thiago }} />);
    expect(screen.getByText('slug home · revision v2 · schema 2 · 0 seções · erro ao montar moment')).toBeInTheDocument();
  });
});

describe('Raio-X helpers', () => {
  it('builds labels from the envelope', () => {
    expect(sectionLabel({ id: 'wealth', components: [{ type: 'wealth_summary', variant: 'with_day_change', props: {} }] })).toBe(
      'wealth · wealth_summary · with_day_change',
    );
    expect(sectionLabel({ id: 'moment', components: [{ type: 'moment_card', variant: '', props: {} }] })).toBe('moment · moment_card');
    expect(omittedLabel({ id: 'activity', type: 'activity_list', reason: 'timeline' })).toBe('activity · activity_list · omitido');
  });

  it('lists distinct source failures and every build error', () => {
    expect(
      failureNotes([
        { id: 'a', type: 't', reason: 'timeline' },
        { id: 'b', type: 't', reason: 'timeline' },
        { id: 'c', type: 't', reason: 'build_error' },
        { id: 'd', type: 't', reason: 'build_error' },
        { id: 'e', type: 't', reason: '' },
      ]),
    ).toEqual(['timeline fora do ar', 'erro ao montar c', 'erro ao montar d']);
  });

  it('describes the envelope', () => {
    expect(xrayMeta(thiagoHome())).toBe('slug home · revision v1 · schema 1 · 5 seções');
  });
});
