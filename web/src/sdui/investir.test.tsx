import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { fernandaInvestir, product, thiagoInvestir } from '../test/sduiFixtures';
import { renderSdui } from '../test/sdui';
import { InvestSummary } from './components/InvestSummary';
import { ProductList } from './components/ProductList';
import { ProductRail } from './components/ProductRail';
import { RiskBars } from './components/RiskBars';
import { findPurchase } from './purchase';
import { lookup } from './registry';
import { SduiError, SduiScreen } from './SduiScreen';
import type { Screen } from './types';

const MASK = 'US$ ••••••';
const CONSERVADOR = { name: 'conservador', max: 2 } as const;
const ARROJADO = { name: 'arrojado', max: 5 } as const;

afterEach(() => {
  vi.restoreAllMocks();
});

function bars(root: ParentNode, index = 0) {
  const risk = root.querySelectorAll('.sdui-risk')[index] as HTMLElement;
  return {
    above: risk.getAttribute('data-above'),
    on: Array.from(risk.querySelectorAll('i')).map((bar) => bar.getAttribute('data-on')),
  };
}

describe('registry', () => {
  it.each(['invest_summary', 'product_rail', 'product_list'])('registers %s', (type) => {
    expect(lookup(type)).toBeDefined();
  });
});

describe('RiskBars', () => {
  it.each([
    [3, false, ['true', 'true', 'true', 'false', 'false'], 'false'],
    [5, true, ['true', 'true', 'true', 'true', 'true'], 'true'],
    [9, false, ['true', 'true', 'true', 'true', 'true'], 'false'],
    [-2, 'yes', ['false', 'false', 'false', 'false', 'false'], 'false'],
    ['4', true, ['false', 'false', 'false', 'false', 'false'], 'true'],
  ])('draws risk %s (above %s)', (risk, above, on, flag) => {
    const { container } = renderSdui(<RiskBars risk={risk} above={above} />);
    expect(bars(container)).toEqual({ above: flag, on });
    expect(container.querySelector('.sdui-risk')).toHaveAttribute('aria-hidden', 'true');
  });
});

describe('invest_summary', () => {
  const props = { cash_label: 'Disponível para investir', cash: 'US$ 60.520,00', cash_cents: 6052000, profile_chip: 'Perfil arrojado' };

  it('renders the default variant with the profile chip', () => {
    renderSdui(<InvestSummary variant="default" props={props} />);
    const card = screen.getByRole('region', { name: 'Disponível para investir' });
    expect(within(card).getByText('US$ 60.520,00')).toBeInTheDocument();
    expect(within(card).getByText('Perfil arrojado')).toHaveClass('sdui-invest__chip');
  });

  it('leaves the chip out without a profile and masks the cash', () => {
    const { container } = renderSdui(<InvestSummary variant="default" props={{ ...props, profile_chip: undefined }} />, { masked: true });
    expect(container.querySelector('.sdui-invest__chip')).toBeNull();
    expect(screen.getByText(MASK)).toBeInTheDocument();
    expect(screen.queryByText('US$ 60.520,00')).not.toBeInTheDocument();
  });
});

describe('product_rail', () => {
  it.each([
    ['profile_conservador', CONSERVADOR, ['tbill', 'corp']],
    ['profile_moderado', { name: 'moderado', max: 3 } as const, ['acoesg', 'corp']],
    ['profile_arrojado', ARROJADO, ['cobalto', 'acoesg']],
  ] as const)('renders %s cards from props', (variant, profile, picks) => {
    renderSdui(
      <ProductRail
        variant={variant}
        props={{ title: `Para o seu perfil ${profile.name}`, subtitle: 'Escolhidos pelo backend a partir do seu perfil de investidor.', products: picks.map((id) => product(id, profile)) }}
      />,
    );
    const rail = screen.getByRole('region', { name: `Para o seu perfil ${profile.name}` });
    expect(within(rail).getByRole('heading', { level: 2 })).toHaveTextContent(`Para o seu perfil ${profile.name}`);
    expect(within(rail).getByText('Escolhidos pelo backend a partir do seu perfil de investidor.')).toBeInTheDocument();
    expect(within(rail).getAllByRole('article').map((card) => card.getAttribute('aria-label'))).toEqual(picks.map((id) => product(id, profile).name));
    expect(within(rail).getAllByRole('button', { name: 'Investir' })).toHaveLength(2);
  });

  it('renders a product within the profile with its values and no badge', () => {
    renderSdui(<ProductRail variant="profile_arrojado" props={{ title: 'T', subtitle: 'S', products: [product('cobalto', ARROJADO)] }} />);
    const card = screen.getByRole('article', { name: 'Cobalto Semicondutores' });
    expect(within(card).getByText('Ação')).toHaveClass('sdui-kicker');
    expect(within(card).getByText('Risco 5 de 5')).toBeInTheDocument();
    expect(within(card).getByText('+27,1% em 12 meses')).toHaveClass('sdui-product__return');
    expect(within(card).getByText('Mínimo US$ 10')).toBeInTheDocument();
    expect(card.querySelector('.sdui-badge')).toBeNull();
    expect(bars(card)).toEqual({ above: 'false', on: ['true', 'true', 'true', 'true', 'true'] });
  });

  it('renders a product above the profile with the badge and warning bars', () => {
    renderSdui(<ProductRail variant="profile_conservador" props={{ title: 'T', subtitle: '', products: [product('acoesg', CONSERVADOR)] }} />);
    const card = screen.getByRole('article', { name: 'Maré Ações Globais ETF' });
    expect(within(card).getByText('Acima do seu perfil')).toHaveClass('sdui-badge');
    expect(bars(card)).toEqual({ above: 'true', on: ['true', 'true', 'true', 'false', 'false'] });
    expect(screen.queryByText('S')).not.toBeInTheDocument();
    expect(document.querySelector('.sdui-rail__head p')).toBeNull();
  });

  it('opens the purchase form for the card product', async () => {
    const user = userEvent.setup();
    const { value } = renderSdui(<ProductRail variant="profile_arrojado" props={{ title: 'T', subtitle: 'S', products: [product('cobalto', ARROJADO), product('acoesg', ARROJADO)] }} />);
    await user.click(within(screen.getByRole('article', { name: 'Maré Ações Globais ETF' })).getByRole('button', { name: 'Investir' }));
    expect(value.onPurchase).toHaveBeenCalledWith('acoesg');
  });

  it('lays the cards in one column on the phone and two on desktop', () => {
    const props = { title: 'T', subtitle: 'S', products: [product('cobalto', ARROJADO), product('acoesg', ARROJADO)] };
    const { container, rerenderWith } = renderSdui(<ProductRail variant="profile_arrojado" props={props} />);
    expect(container.querySelector('.sdui-rail__grid')).toHaveAttribute('data-columns', '1');
    rerenderWith(<ProductRail variant="profile_arrojado" props={props} />, { wide: true });
    expect(container.querySelector('.sdui-rail__grid')).toHaveAttribute('data-columns', '2');
  });
});

describe('product_list', () => {
  it.each([
    ['fixed_income', 'Renda fixa', ['tbill', 'corp']],
    ['etf', 'ETFs', ['renda', 'acoesg']],
    ['stocks', 'Ações', ['farol', 'cobalto']],
  ] as const)('renders the %s variant from props', (variant, title, ids) => {
    renderSdui(<ProductList variant={variant} props={{ title, products: ids.map((id) => product(id, ARROJADO)) }} />);
    const card = screen.getByRole('region', { name: title });
    expect(within(card).getByRole('heading', { level: 2, name: title })).toBeInTheDocument();
    const rows = within(card).getAllByRole('button');
    expect(rows).toHaveLength(2);
    ids.forEach((id, index) => {
      const p = product(id, ARROJADO);
      expect(rows[index]).toHaveTextContent(p.name);
      expect(rows[index]).toHaveTextContent(`${p.risk_label} · ${p.return_label}`);
      expect(rows[index]).toHaveTextContent('Investir');
    });
  });

  it('marks the rows above the profile and keeps the others plain', () => {
    renderSdui(<ProductList variant="stocks" props={{ title: 'Ações', products: [product('farol', CONSERVADOR), product('tbill', CONSERVADOR)] }} />);
    const [above, within2] = screen.getAllByRole('button');
    expect(above).toHaveTextContent('Acima do seu perfil');
    expect(bars(above as HTMLElement)).toEqual({ above: 'true', on: ['true', 'true', 'true', 'true', 'false'] });
    expect(within2).not.toHaveTextContent('Acima do seu perfil');
    expect(bars(within2 as HTMLElement)).toEqual({ above: 'false', on: ['true', 'false', 'false', 'false', 'false'] });
  });

  it('opens the purchase form from a row', async () => {
    const user = userEvent.setup();
    const { value } = renderSdui(<ProductList variant="etf" props={{ title: 'ETFs', products: [product('renda', ARROJADO), product('acoesg', ARROJADO)] }} />);
    await user.click(screen.getByRole('button', { name: /Maré Renda Global ETF/ }));
    expect(value.onPurchase).toHaveBeenCalledWith('renda');
  });

  it('hides a row whose purchase action has no product_id and reports it', () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const broken = { ...product('tbill', null), action: { type: 'panel', label: 'Investir', target: 'purchase' } };
    renderSdui(<ProductList variant="fixed_income" props={{ title: 'Renda fixa', products: [broken, product('corp', null)] }} />);
    expect(screen.getAllByRole('button')).toHaveLength(1);
    expect(screen.queryByText('Orla T-Bill 6 meses')).not.toBeInTheDocument();
    expect(error).toHaveBeenCalledWith('sdui: purchase panel has no product_id');
  });
});

describe('Investir envelopes', () => {
  it('renders Thiago with nothing above his profile', () => {
    const { container } = renderSdui(<SduiScreen screen={thiagoInvestir()} />);
    expect(screen.getByRole('heading', { level: 1, name: 'Investir' })).toBeInTheDocument();
    expect(screen.getByText('Produtos fictícios · preço fixo da simulação')).toBeInTheDocument();
    expect(Array.from(container.querySelectorAll('.sdui-section')).map((node) => [node.getAttribute('data-section'), node.getAttribute('data-span')])).toEqual([
      ['cash', '2'],
      ['highlights', '2'],
      ['fixed_income', '1'],
      ['etfs', '1'],
      ['stocks', '1'],
    ]);
    expect(screen.getByText('Perfil arrojado')).toBeInTheDocument();
    expect(container.querySelector('.sdui-badge')).toBeNull();
    expect(container.querySelector('.sdui-risk[data-above="true"]')).toBeNull();
  });

  it('renders Fernanda with the badge on acoesg, farol, and cobalto', () => {
    const { container } = renderSdui(<SduiScreen screen={fernandaInvestir()} />);
    const marked = Array.from(container.querySelectorAll('.sdui-products__row'))
      .filter((row) => row.querySelector('.sdui-badge'))
      .map((row) => row.querySelector('strong')?.textContent);
    expect(marked).toEqual(['Maré Ações Globais ETF', 'Farol Saúde', 'Cobalto Semicondutores']);
    expect(within(screen.getByRole('region', { name: 'Para o seu perfil conservador' })).queryByText('Acima do seu perfil')).not.toBeInTheDocument();
  });
});

describe('findPurchase', () => {
  it('reads the product and the cash from the envelope', () => {
    expect(findPurchase(fernandaInvestir(), 'cobalto')).toEqual({
      product: product('cobalto', CONSERVADOR),
      cash: { display: 'US$ 1.148,00', cents: 114800 },
    });
  });

  it('takes the first match, from the rail or a list', () => {
    expect(findPurchase(thiagoInvestir(), 'acoesg')?.product).toEqual(product('acoesg', ARROJADO));
    expect(findPurchase(thiagoInvestir(), 'farol')?.product).toEqual(product('farol', ARROJADO));
  });

  it('has no cash when invest_summary was omitted or malformed', () => {
    const screen: Screen = thiagoInvestir();
    const noCash = { ...screen, sections: screen.sections.filter((section) => section.id !== 'cash') };
    expect(findPurchase(noCash, 'tbill')).toEqual({ product: product('tbill', ARROJADO), cash: undefined });
    const malformed = thiagoInvestir();
    (malformed.sections[0]?.components[0]?.props as Record<string, unknown>).cash_cents = '6052000';
    expect(findPurchase(malformed, 'tbill')?.cash).toBeUndefined();
  });

  it('returns null for a product that is not on the screen', () => {
    expect(findPurchase(thiagoInvestir(), 'nope')).toBeNull();
    const screen = thiagoInvestir();
    (screen.sections[4]?.components[0]?.props as { products: unknown[] }).products = [null, 'farol', { product_id: 'farol' }];
    expect(findPurchase(screen, 'farol')).toBeNull();
  });
});

describe('SduiError', () => {
  it('says the screen could not be built and retries', async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    renderSdui(<SduiError onRetry={onRetry} />);
    expect(screen.getByRole('alert')).toHaveTextContent('Não foi possível montar sua tela.');
    await user.click(screen.getByRole('button', { name: 'Tentar de novo' }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
