import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { fernandaCarteira, marianaCarteira, thiagoCarteira } from '../test/sduiFixtures';
import { renderSdui } from '../test/sdui';
import { AllocationBreakdown } from './components/AllocationBreakdown';
import { PortfolioSummary } from './components/PortfolioSummary';
import { PositionList } from './components/PositionList';
import { lookup } from './registry';
import { SduiScreen } from './SduiScreen';
import type { Screen } from './types';

const MASK = 'US$ ••••••';

/** The props of one section of an envelope. */
function propsOf(envelope: Screen, id: string): Record<string, unknown> {
  const section = envelope.sections.find((item) => item.id === id);
  if (!section?.components[0]) {
    throw new Error(`fixture: no section ${id}`);
  }
  return section.components[0].props;
}

describe('registry', () => {
  it.each(['portfolio_summary', 'allocation_breakdown', 'position_list'])('registers %s', (type) => {
    expect(lookup(type)).toBeDefined();
  });
});

describe('portfolio_summary', () => {
  const props = propsOf(marianaCarteira(), 'summary');

  it('renders the default variant from props', () => {
    renderSdui(<PortfolioSummary variant="default" props={props} />);
    const card = screen.getByRole('region', { name: 'Patrimônio total' });
    expect(within(card).getByText('US$ 248.300,00')).toHaveClass('sdui-wealth__total');
    const stats = Array.from(card.querySelectorAll('.sdui-stats > div')).map((row) => [row.querySelector('dt')?.textContent, row.querySelector('dd')?.textContent]);
    expect(stats).toEqual([
      ['Valor aplicado', 'US$ 168.900,00'],
      ['Rentabilidade', '+US$ 19.400,00 (+11,5%)'],
      ['Caixa', 'US$ 60.000,00'],
      ['Dia simulado', '0'],
    ]);
    expect(within(card).getByText('+US$ 19.400,00 (+11,5%)')).toHaveClass('sdui-tone--pos');
    expect(within(card).getByText('US$ 168.900,00')).not.toHaveAttribute('class');
  });

  it('maps the return tone and renders an unknown tone as neutral', () => {
    const stats = [
      { label: 'Rentabilidade', value: '−US$ 500,00 (−50,0%)', tone: 'neg', money: true },
      { label: 'Outro', value: 'x', tone: 'gold' },
    ];
    renderSdui(<PortfolioSummary variant="default" props={{ ...props, stats }} />);
    expect(screen.getByText('−US$ 500,00 (−50,0%)')).toHaveClass('sdui-tone--neg');
    expect(screen.getByText('x')).toHaveClass('sdui-tone--neutral');
  });

  it('masks money stats and the total, but not the day, and toggles the eye', async () => {
    const user = userEvent.setup();
    const { value } = renderSdui(<PortfolioSummary variant="default" props={props} />, { masked: true });
    const card = screen.getByRole('region', { name: 'Patrimônio total' });
    expect(within(card).getAllByText(MASK)).toHaveLength(4);
    expect(within(card).getByText('0')).toBeInTheDocument();
    expect(within(card).queryByText(/US\$ \d/)).not.toBeInTheDocument();
    const eye = within(card).getByRole('button', { name: 'Mostrar valores' });
    expect(eye).toHaveAttribute('aria-pressed', 'true');
    await user.click(eye);
    expect(value.onToggleMask).toHaveBeenCalledTimes(1);
  });

  it('offers to hide values when they show', () => {
    renderSdui(<PortfolioSummary variant="default" props={props} />);
    expect(screen.getByRole('button', { name: 'Esconder valores' })).toHaveAttribute('aria-pressed', 'false');
  });
});

describe('allocation_breakdown', () => {
  it('renders the four classes with value, share, and bar from props', () => {
    const { container } = renderSdui(<AllocationBreakdown variant="default" props={propsOf(thiagoCarteira(), 'allocation')} />);
    const card = screen.getByRole('region', { name: 'Alocação' });
    expect(within(card).getByRole('heading', { name: 'Alocação' })).toBeInTheDocument();
    const rows = within(card).getAllByRole('listitem').map((row) => row.querySelector('.sdui-breakdown__row')?.textContent);
    expect(rows).toEqual(['AçõesUS$ 2.040,003%', 'ETFsUS$ 5.440,008%', 'Renda fixaUS$ 0,000%', 'CaixaUS$ 60.520,0089%']);
    const fills = Array.from(container.querySelectorAll<HTMLElement>('.sdui-breakdown__track span'));
    expect(fills.map((fill) => fill.style.width)).toEqual(['3%', '8%', '0%', '89%']);
    expect(fills.map((fill) => fill.className)).toEqual(['sdui-alloc--stocks', 'sdui-alloc--etfs', 'sdui-alloc--fixed_income', 'sdui-alloc--cash']);
  });

  it('bounds bar_width, colors an unknown class neutral, and masks values', () => {
    const rows = [{ class: 'crypto', label: 'Outro', value: 'US$ 1,00', share: '100%', bar_width: 140 }];
    const { container } = renderSdui(<AllocationBreakdown variant="default" props={{ title: 'Alocação', rows }} />, { masked: true });
    const fill = container.querySelector<HTMLElement>('.sdui-breakdown__track span');
    expect(fill?.style.width).toBe('100%');
    expect(fill).toHaveClass('sdui-alloc--other');
    expect(screen.getByText(MASK)).toBeInTheDocument();
    expect(screen.queryByText('US$ 1,00')).not.toBeInTheDocument();
    expect(screen.getByText('100%')).toBeInTheDocument();
  });
});

describe('position_list', () => {
  const mariana = marianaCarteira();

  it.each([
    ['stocks', 'positions_stocks', 'Ações', 'US$ 90.900,00', ['Cobalto Semicondutores', 'Farol Saúde']],
    ['etf', 'positions_etf', 'ETFs', 'US$ 60.600,00', ['Maré Ações Globais ETF', 'Maré Renda Global ETF']],
    ['fixed_income', 'positions_fixed_income', 'Renda fixa', 'US$ 36.800,00', ['Orla Corporate IG 2029']],
  ])('renders the %s variant from props', (variant, id, title, subtotal, names) => {
    renderSdui(<PositionList variant={variant} props={propsOf(mariana, id)} />);
    const card = screen.getByRole('region', { name: title });
    expect(within(card).getByRole('heading', { name: title })).toBeInTheDocument();
    expect(card.querySelector('.sdui-positions__head b')).toHaveTextContent(subtotal);
    const rows = within(card).getAllByRole('listitem');
    expect(rows.map((row) => row.querySelector('strong')?.textContent)).toEqual(names);
  });

  it('shows applied, value, and the signed return with its tone', () => {
    renderSdui(<PositionList variant="stocks" props={propsOf(mariana, 'positions_stocks')} />);
    const cobalto = screen.getAllByRole('listitem')[0] as HTMLElement;
    expect(within(cobalto).getByText('Aplicado US$ 60.000,00')).toBeInTheDocument();
    expect(within(cobalto).getByText('US$ 72.000,00')).toBeInTheDocument();
    expect(within(cobalto).getByText('+20,0%')).toHaveClass('sdui-tone--pos');
  });

  it('maps negative, zero, and unknown tones', () => {
    const items = [
      { product_id: 'cobalto', name: 'Cobalto', applied: 'US$ 2,00', value: 'US$ 1,00', return: '−50,0%', return_tone: 'neg' },
      { product_id: 'farol', name: 'Farol', applied: 'US$ 1,00', value: 'US$ 1,00', return: '0,0%', return_tone: 'neutral' },
      { product_id: 'tbill', name: 'T-Bill', applied: 'US$ 1,00', value: 'US$ 1,00', return: '?', return_tone: 'gold' },
    ];
    renderSdui(<PositionList variant="stocks" props={{ title: 'Ações', subtotal: 'US$ 3,00', applied_label: 'Aplicado', items }} />);
    expect(screen.getByText('−50,0%')).toHaveClass('sdui-tone--neg');
    expect(screen.getByText('0,0%')).toHaveClass('sdui-tone--neutral');
    expect(screen.getByText('?')).toHaveClass('sdui-tone--neutral');
  });

  it('masks the subtotal, applied, and value but keeps the return', () => {
    renderSdui(<PositionList variant="fixed_income" props={propsOf(mariana, 'positions_fixed_income')} />, { masked: true });
    const card = screen.getByRole('region', { name: 'Renda fixa' });
    expect(within(card).getAllByText(MASK)).toHaveLength(2);
    expect(within(card).getByText(`Aplicado ${MASK}`)).toBeInTheDocument();
    expect(within(card).queryByText(/US\$ \d/)).not.toBeInTheDocument();
    expect(within(card).getByText('+2,2%')).toBeInTheDocument();
  });
});

describe('Carteira envelope', () => {
  it('renders Mariana: summary, allocation, three position lists, and history, one column each', () => {
    const { container } = renderSdui(<SduiScreen screen={marianaCarteira()} />);
    expect(screen.getByRole('heading', { level: 1, name: 'Carteira' })).toBeInTheDocument();
    expect(screen.getByText('Valores de mercado no dia simulado 0')).toBeInTheDocument();
    const sections = Array.from(container.querySelectorAll<HTMLElement>('.sdui-section'));
    expect(sections.map((section) => section.dataset.section)).toEqual([
      'summary',
      'allocation',
      'positions_stocks',
      'positions_etf',
      'positions_fixed_income',
      'history',
    ]);
    expect(sections.every((section) => section.dataset.span === '1')).toBe(true);
    const history = screen.getByRole('region', { name: 'Movimentações' });
    expect(within(history).getAllByRole('listitem')).toHaveLength(2);
  });

  it('renders Thiago without a fixed income list', () => {
    const { container } = renderSdui(<SduiScreen screen={thiagoCarteira()} />);
    expect(container.querySelector('[data-section="positions_fixed_income"]')).toBeNull();
    expect(screen.queryByRole('region', { name: 'Renda fixa' })).not.toBeInTheDocument();
    expect(screen.getByText('Renda fixa')).toBeInTheDocument();
  });

  it('renders the empty history of Fernanda', () => {
    renderSdui(<SduiScreen screen={fernandaCarteira()} />);
    const history = screen.getByRole('region', { name: 'Movimentações' });
    expect(within(history).getByText('Nenhuma movimentação ainda.')).toBeInTheDocument();
    expect(within(history).queryByRole('list')).not.toBeInTheDocument();
  });
});
