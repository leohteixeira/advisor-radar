import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { ACTIONS, EMPTY_ACTIVITY, RECENT_ACTIVITY } from '../test/sduiFixtures';
import { renderSdui } from '../test/sdui';
import { ActionGrid } from './components/ActionGrid';
import { ActivityList } from './components/ActivityList';
import { AdvisorCard } from './components/AdvisorCard';
import { MomentCard } from './components/MomentCard';
import { WealthSummary } from './components/WealthSummary';

const MASK = 'US$ ••••••';

describe('moment_card', () => {
  const variants = [
    { variant: 'portfolio_drop', tone: 'neg', icon: 'drop', meta: undefined, action: { type: 'navigate', label: 'Ver carteira', target: 'carteira' } },
    { variant: 'case_open', tone: 'info', icon: 'inbox', meta: 'Protocolo 01A0E3A5-2F4C · aberto há 3 min', action: { type: 'panel', label: 'Ver conversa', target: 'message' } },
    { variant: 'segment_upgraded', tone: 'gold', icon: 'segment', meta: undefined, action: { type: 'navigate', label: 'Ver produtos', target: 'investir' } },
    { variant: 'segment_upgrade_near', tone: 'gold', icon: 'segment', meta: undefined, action: { type: 'panel', label: 'Depositar', target: 'deposit' } },
    { variant: 'idle_cash', tone: 'info', icon: 'cash', meta: undefined, action: { type: 'navigate', label: 'Ver produtos', target: 'investir' } },
    { variant: 'portfolio_review', tone: 'neutral', icon: 'calendar', meta: undefined, action: { type: 'panel', label: 'Conversar', target: 'message' } },
    { variant: 'welcome', tone: 'neutral', icon: 'spark', meta: undefined, action: undefined },
  ];

  it.each(variants)('renders $variant from props', ({ variant, tone, icon, meta, action }) => {
    renderSdui(
      <MomentCard
        variant={variant}
        props={{ kicker: `Kicker ${variant}`, title: `Title ${variant}`, body: `Body ${variant}`, meta, tone, icon, action }}
      />,
    );
    const card = screen.getByRole('region', { name: `Kicker ${variant}` });
    expect(card).toHaveClass(`sdui-tone--${tone}`);
    expect(within(card).getByRole('heading', { name: `Title ${variant}` })).toBeInTheDocument();
    expect(within(card).getByText(`Body ${variant}`)).toBeInTheDocument();
    expect(card.querySelector('.sdui-moment__icon svg')).not.toBeNull();
    if (meta) {
      expect(within(card).getByText(meta)).toHaveClass('sdui-moment__meta');
    } else {
      expect(card.querySelector('.sdui-moment__meta')).toBeNull();
    }
    if (action) {
      expect(within(card).getByRole('button', { name: action.label })).toHaveClass('sdui-btn--primary');
    } else {
      expect(within(card).queryByRole('button')).not.toBeInTheDocument();
    }
  });

  it('maps an unknown tone to neutral and draws no unknown icon', () => {
    renderSdui(<MomentCard variant="future" props={{ kicker: 'K', title: 'T', body: 'B', tone: 'purple', icon: 'rocket' }} />);
    const card = screen.getByRole('region', { name: 'K' });
    expect(card).toHaveClass('sdui-tone--neutral');
    expect(card.querySelector('.sdui-moment__icon')).toBeNull();
  });

  it('navigates from its action', async () => {
    const user = userEvent.setup();
    const { value } = renderSdui(
      <MomentCard
        variant="idle_cash"
        props={{ kicker: 'Caixa parado', title: 'T', body: 'B', tone: 'info', action: { type: 'navigate', label: 'Ver produtos', target: 'investir' } }}
      />,
    );
    await user.click(screen.getByRole('button', { name: 'Ver produtos' }));
    expect(value.onNavigate).toHaveBeenCalledWith('investir');
  });
});

describe('wealth_summary', () => {
  const base = {
    total_label: 'Patrimônio total',
    total: 'US$ 68.000,00',
    cash_label: 'Disponível para saque',
    cash: 'US$ 60.520,00',
    cash_cents: 6052000,
    allocation: [
      { class: 'stocks', label: 'Ações', share: '3%', bar_width: 3 },
      { class: 'etfs', label: 'ETFs', share: '8%', bar_width: 8 },
      { class: 'cash', label: 'Caixa', share: '89%', bar_width: 89 },
    ],
  };

  it('renders the default variant from props', () => {
    const { container } = renderSdui(<WealthSummary variant="default" props={base} />);
    const card = screen.getByRole('region', { name: 'Patrimônio total' });
    expect(within(card).getByText('US$ 68.000,00')).toBeInTheDocument();
    expect(within(card).getByText('US$ 60.520,00')).toBeInTheDocument();
    expect(within(card).getByText('Disponível para saque')).toBeInTheDocument();
    expect(within(card).getAllByRole('listitem').map((li) => li.textContent)).toEqual(['Ações3%', 'ETFs8%', 'Caixa89%']);
    const bars = Array.from(container.querySelectorAll<HTMLElement>('.sdui-bar span'));
    expect(bars.map((bar) => bar.style.width)).toEqual(['3%', '8%', '89%']);
    expect(bars.map((bar) => bar.className)).toEqual(['sdui-alloc--stocks', 'sdui-alloc--etfs', 'sdui-alloc--cash']);
    expect(container.querySelector('.sdui-pill')).toBeNull();
  });

  it.each([
    ['neg', '−US$ 38.520,00 (−15,5%) no dia 3', 'sdui-tone--neg'],
    ['pos', '+US$ 1.200,00 (+1,8%) no dia 4', 'sdui-tone--pos'],
    ['neutral', 'US$ 0,00 (0,0%) no dia 5', 'sdui-tone--neutral'],
    ['info', 'US$ 0,00 (0,0%) no dia 6', 'sdui-tone--neutral'],
  ])('renders with_day_change in tone %s', (tone, change, cls) => {
    renderSdui(<WealthSummary variant="with_day_change" props={{ ...base, day_change: change, day_change_tone: tone }} />);
    expect(screen.getByText(change)).toHaveClass('sdui-pill', cls);
  });

  it('uses bar_width as is, bounded to 0–100, and colors unknown classes neutral', () => {
    const { container } = renderSdui(
      <WealthSummary
        variant="default"
        props={{
          ...base,
          allocation: [
            { class: 'crypto', label: 'Outros', share: '120%', bar_width: 120 },
            { class: 'fixed_income', label: 'Renda fixa', share: '−', bar_width: -4 },
            { class: 'cash', label: 'Caixa', share: '?', bar_width: '50' },
          ],
        }}
      />,
    );
    const bars = Array.from(container.querySelectorAll<HTMLElement>('.sdui-bar span'));
    expect(bars.map((bar) => bar.style.width)).toEqual(['100%', '0%', '0%']);
    expect(bars[0]).toHaveClass('sdui-alloc--other');
    expect(bars[1]).toHaveClass('sdui-alloc--fixed_income');
  });

  it('masks money when the eye toggle is on', async () => {
    const user = userEvent.setup();
    const { value, rerenderWith } = renderSdui(<WealthSummary variant="default" props={base} />);
    await user.click(screen.getByRole('button', { name: 'Esconder valores' }));
    expect(value.onToggleMask).toHaveBeenCalledTimes(1);

    rerenderWith(<WealthSummary variant="with_day_change" props={{ ...base, day_change: '+US$ 1,00 (+0,1%) no dia 2', day_change_tone: 'pos' }} />, {
      masked: true,
    });
    expect(screen.getByRole('button', { name: 'Mostrar valores' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getAllByText(MASK)).toHaveLength(3);
    expect(screen.queryByText('US$ 68.000,00')).not.toBeInTheDocument();
    expect(screen.queryByText('US$ 60.520,00')).not.toBeInTheDocument();
    expect(screen.getByText('89%')).toBeInTheDocument();
  });
});

describe('action_grid', () => {
  it('renders the default variant and opens each panel', async () => {
    const user = userEvent.setup();
    const { value } = renderSdui(<ActionGrid variant="default" props={ACTIONS.props} />);
    const nav = screen.getByRole('navigation', { name: 'Ações' });
    const buttons = within(nav).getAllByRole('button');
    expect(buttons.map((button) => button.textContent)).toEqual(['Depositar', 'Sacar', 'Mensagem', 'Reclamar']);
    for (const button of buttons) {
      expect(button.querySelector('svg')).not.toBeNull();
      await user.click(button);
    }
    expect(value.onPanel).toHaveBeenNthCalledWith(1, 'deposit');
    expect(value.onPanel).toHaveBeenNthCalledWith(2, 'withdraw');
    expect(value.onPanel).toHaveBeenNthCalledWith(3, 'message');
    expect(value.onPanel).toHaveBeenNthCalledWith(4, 'complaint');
  });

  it('draws no unknown icon', () => {
    renderSdui(<ActionGrid variant="default" props={{ items: [{ label: 'Extrato', icon: 'receipt', action: { type: 'note', label: 'Extrato', text: 'Em breve.' } }] }} />);
    expect(screen.getByRole('button', { name: 'Extrato' }).querySelector('svg')).toBeNull();
  });
});

describe('advisor_card', () => {
  it.each([
    ['default', 'Sua assessora', 'Resposta em até 4 h · cliente Advance'],
    ['dedicated', 'Sua assessora dedicada', 'Resposta em até 1 h · cliente Singular'],
  ])('renders the %s variant from props', async (variant, kicker, meta) => {
    const user = userEvent.setup();
    const { value } = renderSdui(
      <AdvisorCard
        variant={variant}
        props={{ kicker, name: 'Ana Paula Ribeiro', initials: 'AP', meta, action: { type: 'panel', label: 'Conversar', target: 'message' } }}
      />,
    );
    const card = screen.getByRole('region', { name: kicker });
    expect(within(card).getByText('Ana Paula Ribeiro')).toBeInTheDocument();
    expect(within(card).getByText('AP')).toBeInTheDocument();
    expect(within(card).getByText(meta)).toBeInTheDocument();
    await user.click(within(card).getByRole('button', { name: 'Conversar' }));
    expect(value.onPanel).toHaveBeenCalledWith('message');
  });

  it('renders no control without an action', () => {
    renderSdui(<AdvisorCard variant="default" props={{ kicker: 'Sua assessora', name: 'Ana', initials: 'A', meta: 'm' }} />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});

describe('activity_list', () => {
  it('renders the recent variant from props', () => {
    renderSdui(<ActivityList variant="recent" props={RECENT_ACTIVITY.props} />);
    const card = screen.getByRole('region', { name: 'Atividade recente' });
    expect(within(card).getByRole('heading', { name: 'Atividade recente' })).toBeInTheDocument();
    const rows = within(card).getAllByRole('listitem');
    expect(rows.map((row) => row.textContent)).toEqual(['Depósito via câmbiohá 4 dias', 'Mensagem para a assessoriaontem']);
    expect(rows[0]?.querySelector('svg')).not.toBeNull();
    expect(card.querySelector('.sdui-activity__empty')).toBeNull();
  });

  it('renders the empty variant from props', () => {
    renderSdui(<ActivityList variant="empty" props={EMPTY_ACTIVITY.props} />);
    expect(screen.getByText('Suas movimentações aparecem aqui assim que acontecerem.')).toBeInTheDocument();
    expect(screen.queryByRole('list')).not.toBeInTheDocument();
  });

  it('renders the history variant with signed values and tones', () => {
    renderSdui(
      <ActivityList
        variant="history"
        props={{
          title: 'Movimentações',
          items: [
            { icon: 'drop', title: 'Reavaliação diária', meta: 'dia simulado 3 · Cobalto −53,5%', value: '− US$ 38.502,00', tone: 'neg' },
            { icon: 'in', title: 'Depósito', meta: 'há 4 dias', value: '+ US$ 60.000,00', tone: 'pos' },
            { icon: 'mystery', title: 'Outro', meta: 'ontem', value: 'US$ 1,00', tone: 'gold' },
          ],
        }}
      />,
    );
    expect(screen.getByText('− US$ 38.502,00')).toHaveClass('sdui-tone--neg');
    expect(screen.getByText('+ US$ 60.000,00')).toHaveClass('sdui-tone--pos');
    expect(screen.getByText('US$ 1,00')).toHaveClass('sdui-tone--neutral');
    expect(screen.getAllByRole('listitem')[2]?.querySelector('svg')).toBeNull();
  });

  it('renders the empty history text and masks values', () => {
    const { rerenderWith } = renderSdui(<ActivityList variant="history" props={{ title: 'Movimentações', items: [], empty_text: 'Nenhuma movimentação ainda.' }} />);
    expect(screen.getByText('Nenhuma movimentação ainda.')).toBeInTheDocument();
    rerenderWith(<ActivityList variant="history" props={{ title: 'Movimentações', items: [{ title: 'Saque', meta: 'ontem', value: '− US$ 20.000,00', tone: 'neg' }] }} />, {
      masked: true,
    });
    expect(screen.getByText(MASK)).toBeInTheDocument();
    expect(screen.queryByText('− US$ 20.000,00')).not.toBeInTheDocument();
  });

  it('renders nothing for an empty list without empty text', () => {
    const { container } = renderSdui(<ActivityList variant="recent" props={{ title: 'Atividade recente', items: [] }} />);
    expect(container.querySelector('.sdui-activity p')).toBeNull();
  });
});
