/**
 * SDUI home envelopes shaped like the BFF output of story 5 for the three seed
 * clients (internal/screen/catalog.json, captured from a local BFF).
 */
import type { Component, Screen } from '../sdui/types';

export const SEED = {
  fernanda: '01a0e3a4-9a44-757a-ac8f-dab7db5eb068',
  thiago: '01a0e3a4-9a44-75dd-b3a0-403a7a87836e',
  mariana: '01a0e3a4-9a44-7566-b5de-eb2e365799f8',
} as const;

function welcome(first: string): Component {
  return {
    type: 'moment_card',
    variant: 'welcome',
    props: {
      kicker: 'Tudo em dia',
      title: `Olá, ${first}. Sua conta está em dia.`,
      body: 'Quando algo mudar na sua carteira, você vê aqui primeiro.',
      tone: 'neutral',
      icon: 'spark',
    },
  };
}

function wealth(total: string, cash: string, cashCents: number, allocation: [string, string, number][]): Component {
  const labels: Record<string, string> = { stocks: 'Ações', etfs: 'ETFs', fixed_income: 'Renda fixa', cash: 'Caixa' };
  return {
    type: 'wealth_summary',
    variant: 'default',
    props: {
      total_label: 'Patrimônio total',
      total,
      cash_label: 'Disponível para saque',
      cash,
      cash_cents: cashCents,
      allocation: allocation.map(([cls, share, width]) => ({ class: cls, label: labels[cls], share, bar_width: width })),
    },
  };
}

export const ACTIONS: Component = {
  type: 'action_grid',
  variant: 'default',
  props: {
    items: [
      { label: 'Depositar', icon: 'deposit', action: { type: 'panel', label: 'Depositar', target: 'deposit' } },
      { label: 'Sacar', icon: 'withdraw', action: { type: 'panel', label: 'Sacar', target: 'withdraw' } },
      { label: 'Mensagem', icon: 'msg', action: { type: 'panel', label: 'Mensagem', target: 'message' } },
      { label: 'Reclamar', icon: 'alert', action: { type: 'panel', label: 'Reclamar', target: 'complaint' } },
    ],
  },
};

function advisor(variant: 'default' | 'dedicated', sla: string, segment: string): Component {
  return {
    type: 'advisor_card',
    variant,
    props: {
      kicker: variant === 'dedicated' ? 'Sua assessora dedicada' : 'Sua assessora',
      name: 'Ana Paula Ribeiro',
      initials: 'AP',
      meta: `Resposta em até ${sla} · cliente ${segment}`,
      action: { type: 'panel', label: 'Conversar', target: 'message' },
    },
  };
}

export const EMPTY_ACTIVITY: Component = {
  type: 'activity_list',
  variant: 'empty',
  props: {
    title: 'Atividade recente',
    items: [],
    empty_text: 'Suas movimentações aparecem aqui assim que acontecerem.',
  },
};

export const RECENT_ACTIVITY: Component = {
  type: 'activity_list',
  variant: 'recent',
  props: {
    title: 'Atividade recente',
    items: [
      { icon: 'in', title: 'Depósito via câmbio', meta: 'há 4 dias' },
      { icon: 'msg', title: 'Mensagem para a assessoria', meta: 'ontem' },
    ],
  },
};

function home(first: string, subtitle: string, sections: [string, Component][]): Screen {
  return {
    schema_version: 1,
    slug: 'home',
    revision: 'v1',
    title: `Olá, ${first}`,
    subtitle,
    sections: sections.map(([id, component]) => ({ id, components: [component] })),
    omitted: [],
  };
}

export function fernandaHome(): Screen {
  return home('Fernanda', 'Cliente Essencial desde 2024', [
    ['moment', welcome('Fernanda')],
    ['wealth', wealth('US$ 8.200,00', 'US$ 1.148,00', 114800, [['stocks', '20%', 20], ['etfs', '45%', 45], ['fixed_income', '21%', 21], ['cash', '14%', 14]])],
    ['actions', ACTIONS],
    ['advisor', advisor('default', '24 h', 'Essencial')],
    ['activity', EMPTY_ACTIVITY],
  ]);
}

export function thiagoHome(): Screen {
  return home('Thiago', 'Cliente Advance desde 2024', [
    ['moment', welcome('Thiago')],
    ['wealth', wealth('US$ 68.000,00', 'US$ 60.520,00', 6052000, [['stocks', '3%', 3], ['etfs', '8%', 8], ['cash', '89%', 89]])],
    ['actions', ACTIONS],
    ['advisor', advisor('default', '4 h', 'Advance')],
    ['activity', RECENT_ACTIVITY],
  ]);
}

export function marianaHome(): Screen {
  return home('Mariana', 'Cliente Singular desde 2021', [
    ['moment', welcome('Mariana')],
    ['wealth', wealth('US$ 248.300,00', 'US$ 60.000,00', 6000000, [['stocks', '37%', 37], ['etfs', '24%', 24], ['fixed_income', '15%', 15], ['cash', '24%', 24]])],
    ['actions', ACTIONS],
    ['advisor', advisor('dedicated', '1 h', 'Singular')],
    ['activity', EMPTY_ACTIVITY],
  ]);
}

/** The phase-2 home JSON for Thiago (GET /v1/client-pov/customers/{id}). */
export function thiagoPhase2() {
  return {
    customer_id: SEED.thiago,
    name: 'Thiago Azevedo',
    segment: 'Advance',
    advisor: 'Ana Paula Ribeiro',
    sla: '4 h',
    since: '2024',
    assets: 6800000,
    caixa: 6052000,
    allocation: { acoes: 204000, etfs: 544000, renda_fixa: 0, caixa: 6052000 },
    activity: [],
    messages: [],
  };
}

/** The phase-2 home JSON for Fernanda (GET /v1/client-pov/customers/{id}). */
export function fernandaPhase2() {
  return {
    customer_id: SEED.fernanda,
    name: 'Fernanda Lima',
    segment: 'Essencial',
    advisor: 'Ana Paula Ribeiro',
    sla: '24 h',
    since: '2024',
    assets: 820000,
    caixa: 114800,
    allocation: { acoes: 164000, etfs: 369000, renda_fixa: 172200, caixa: 114800 },
    activity: [],
    messages: [],
  };
}

/** The phase-2 home JSON for Mariana (GET /v1/client-pov/customers/{id}), matching marianaCarteira. */
export function marianaPhase2() {
  return {
    customer_id: SEED.mariana,
    name: 'Mariana Costa',
    segment: 'Singular',
    advisor: 'Ana Paula Ribeiro',
    sla: '1 h',
    since: '2021',
    assets: 24830000,
    caixa: 6000000,
    allocation: { acoes: 9090000, etfs: 6060000, renda_fixa: 3680000, caixa: 6000000 },
    activity: [],
    messages: [],
  };
}

/** Thiago's home while his cash is idle: the moment points to Investir. */
export function thiagoIdleCashHome(): Screen {
  const screen = thiagoHome();
  screen.sections[0] = {
    id: 'moment',
    components: [
      {
        type: 'moment_card',
        variant: 'idle_cash',
        props: {
          kicker: 'Caixa parado',
          title: 'Thiago, 89% do seu patrimônio está em caixa',
          body: 'US$ 60.520,00 parados há 4 dias. Veja produtos para o seu perfil arrojado.',
          tone: 'info',
          icon: 'cash',
          action: { type: 'navigate', label: 'Ver produtos', target: 'investir' },
        },
      },
    ],
  };
  return screen;
}

/** The account-sim catalog as story 4 seeds it, with the BFF display strings. */
const CATALOG = [
  { id: 'tbill', name: 'Orla T-Bill 6 meses', cls: 'Renda fixa', risk: 1, ret: '4,9% a.a.', min: 'Mínimo US$ 100', minCents: 10000 },
  { id: 'corp', name: 'Orla Corporate IG 2029', cls: 'Renda fixa', risk: 2, ret: '5,6% a.a.', min: 'Mínimo US$ 1.000', minCents: 100000 },
  { id: 'renda', name: 'Maré Renda Global ETF', cls: 'ETF', risk: 2, ret: '+3,8% em 12 meses', min: 'Mínimo US$ 50', minCents: 5000 },
  { id: 'acoesg', name: 'Maré Ações Globais ETF', cls: 'ETF', risk: 3, ret: '+11,2% em 12 meses', min: 'Mínimo US$ 50', minCents: 5000 },
  { id: 'farol', name: 'Farol Saúde', cls: 'Ação', risk: 4, ret: '+9,4% em 12 meses', min: 'Mínimo US$ 10', minCents: 1000 },
  { id: 'cobalto', name: 'Cobalto Semicondutores', cls: 'Ação', risk: 5, ret: '+27,1% em 12 meses', min: 'Mínimo US$ 10', minCents: 1000 },
] as const;

export type ProductID = (typeof CATALOG)[number]['id'];

export interface InvestorProfile {
  name: 'conservador' | 'moderado' | 'arrojado';
  max: number;
}

/** One product as the BFF shapes it for `profile` (internal/screen/investir.go); null is an unknown profile. */
export function product(id: ProductID, profile: InvestorProfile | null) {
  const p = CATALOG.find((item) => item.id === id);
  if (!p) {
    throw new Error(`fixture: unknown product ${id}`);
  }
  const above = profile !== null && p.risk > profile.max;
  return {
    product_id: p.id,
    name: p.name,
    class_label: p.cls,
    risk: p.risk,
    risk_label: `Risco ${p.risk} de 5`,
    return_label: p.ret,
    minimum: p.min,
    minimum_cents: p.minCents,
    above_profile: above,
    ...(above && profile
      ? {
          badge: 'Acima do seu perfil',
          warning: `Este produto tem risco ${p.risk}. Seu perfil é ${profile.name}, que vai até risco ${profile.max}. Você pode investir mesmo assim, e a sua assessora será avisada.`,
        }
      : {}),
    action: { type: 'panel', label: 'Investir', target: 'purchase', product_id: p.id },
  };
}

const PICKS: Record<InvestorProfile['name'], ProductID[]> = {
  conservador: ['tbill', 'corp'],
  moderado: ['acoesg', 'corp'],
  arrojado: ['cobalto', 'acoesg'],
};

function investir(cash: string, cashCents: number, profile: InvestorProfile): Screen {
  const list = (ids: ProductID[]) => ids.map((id) => product(id, profile));
  return {
    schema_version: 1,
    slug: 'investir',
    revision: 'v1',
    title: 'Investir',
    subtitle: 'Produtos fictícios · preço fixo da simulação',
    sections: [
      {
        id: 'cash',
        components: [
          {
            type: 'invest_summary',
            variant: 'default',
            props: { cash_label: 'Disponível para investir', cash, cash_cents: cashCents, profile_chip: `Perfil ${profile.name}` },
          },
        ],
      },
      {
        id: 'highlights',
        components: [
          {
            type: 'product_rail',
            variant: `profile_${profile.name}`,
            props: {
              title: `Para o seu perfil ${profile.name}`,
              subtitle: 'Escolhidos pelo backend a partir do seu perfil de investidor.',
              products: list(PICKS[profile.name]),
            },
          },
        ],
      },
      { id: 'fixed_income', components: [{ type: 'product_list', variant: 'fixed_income', props: { title: 'Renda fixa', products: list(['tbill', 'corp']) } }] },
      { id: 'etfs', components: [{ type: 'product_list', variant: 'etf', props: { title: 'ETFs', products: list(['renda', 'acoesg']) } }] },
      { id: 'stocks', components: [{ type: 'product_list', variant: 'stocks', props: { title: 'Ações', products: list(['farol', 'cobalto']) } }] },
    ],
    omitted: [],
  };
}

/** Thiago's Investir envelope: arrojado, nothing above his profile. */
export function thiagoInvestir(): Screen {
  return investir('US$ 60.520,00', 6052000, { name: 'arrojado', max: 5 });
}

/** Fernanda's Investir envelope: conservador (max 2), so acoesg, farol, and cobalto are above it. */
export function fernandaInvestir(): Screen {
  return investir('US$ 1.148,00', 114800, { name: 'conservador', max: 2 });
}

/** One position row: product, applied, value, and the signed return with its tone. */
type Holding = [ProductID, string, string, string, 'pos' | 'neg' | 'neutral'];

interface Wallet {
  total: string;
  applied: string;
  gain: string;
  cash: string;
  /** Ações, ETFs, Renda fixa, and Caixa: value, share, and bar width. */
  allocation: [string, string, number][];
  stocks: [string, Holding[]] | null;
  etf: [string, Holding[]] | null;
  fixedIncome: [string, Holding[]] | null;
  history: { icon?: string; title: string; meta: string; value?: string }[];
}

function positionList(variant: 'stocks' | 'etf' | 'fixed_income', title: string, [subtotal, rows]: [string, Holding[]]): Component {
  return {
    type: 'position_list',
    variant,
    props: {
      title,
      subtotal,
      applied_label: 'Aplicado',
      items: rows.map(([id, applied, value, ret, tone]) => ({
        product_id: id,
        name: CATALOG.find((item) => item.id === id)?.name,
        applied,
        value,
        return: ret,
        return_tone: tone,
      })),
    },
  };
}

/** A Carteira envelope as internal/screen/carteira.go builds it on day 0. */
function carteira(w: Wallet): Screen {
  const classes = [
    ['stocks', 'Ações'],
    ['etfs', 'ETFs'],
    ['fixed_income', 'Renda fixa'],
    ['cash', 'Caixa'],
  ] as const;
  const sections: Screen['sections'] = [
    {
      id: 'summary',
      components: [
        {
          type: 'portfolio_summary',
          variant: 'default',
          props: {
            total_label: 'Patrimônio total',
            total: w.total,
            stats: [
              { label: 'Valor aplicado', value: w.applied, money: true },
              { label: 'Rentabilidade', value: w.gain, tone: 'pos', money: true },
              { label: 'Caixa', value: w.cash, money: true },
              { label: 'Dia simulado', value: '0' },
            ],
          },
        },
      ],
    },
    {
      id: 'allocation',
      components: [
        {
          type: 'allocation_breakdown',
          variant: 'default',
          props: {
            title: 'Alocação',
            rows: classes.map(([cls, label], index) => {
              const [value, share, width] = w.allocation[index] ?? ['US$ 0,00', '0%', 0];
              return { class: cls, label, value, share, bar_width: width };
            }),
          },
        },
      ],
    },
  ];
  if (w.stocks) {
    sections.push({ id: 'positions_stocks', components: [positionList('stocks', 'Ações', w.stocks)] });
  }
  if (w.etf) {
    sections.push({ id: 'positions_etf', components: [positionList('etf', 'ETFs', w.etf)] });
  }
  if (w.fixedIncome) {
    sections.push({ id: 'positions_fixed_income', components: [positionList('fixed_income', 'Renda fixa', w.fixedIncome)] });
  }
  sections.push({
    id: 'history',
    components: [
      {
        type: 'activity_list',
        variant: 'history',
        props: {
          title: 'Movimentações',
          items: w.history,
          ...(w.history.length === 0 ? { empty_text: 'Nenhuma movimentação ainda.' } : {}),
        },
      },
    ],
  });
  return {
    schema_version: 1,
    slug: 'carteira',
    revision: 'v1',
    title: 'Carteira',
    subtitle: 'Valores de mercado no dia simulado 0',
    sections,
    omitted: [],
  };
}

/** Mariana's Carteira: five positions in three classes (Carteira-Mariana). */
export function marianaCarteira(): Screen {
  return carteira({
    total: 'US$ 248.300,00',
    applied: 'US$ 168.900,00',
    gain: '+US$ 19.400,00 (+11,5%)',
    cash: 'US$ 60.000,00',
    allocation: [
      ['US$ 90.900,00', '37%', 37],
      ['US$ 60.600,00', '24%', 24],
      ['US$ 36.800,00', '15%', 15],
      ['US$ 60.000,00', '24%', 24],
    ],
    stocks: [
      'US$ 90.900,00',
      [
        ['cobalto', 'US$ 60.000,00', 'US$ 72.000,00', '+20,0%', 'pos'],
        ['farol', 'US$ 17.500,00', 'US$ 18.900,00', '+8,0%', 'pos'],
      ],
    ],
    etf: [
      'US$ 60.600,00',
      [
        ['acoesg', 'US$ 36.000,00', 'US$ 40.600,00', '+12,8%', 'pos'],
        ['renda', 'US$ 19.400,00', 'US$ 20.000,00', '+3,1%', 'pos'],
      ],
    ],
    fixedIncome: ['US$ 36.800,00', [['corp', 'US$ 36.000,00', 'US$ 36.800,00', '+2,2%', 'pos']]],
    history: [
      { icon: 'msg', title: 'Mensagem · e-mail', meta: 'ontem' },
      { icon: 'out', title: 'Saque', meta: 'há 3 dias' },
    ],
  });
}

/** Thiago's Carteira: stocks and ETFs only, so no fixed income section. */
export function thiagoCarteira(): Screen {
  return carteira({
    total: 'US$ 68.000,00',
    applied: 'US$ 7.100,00',
    gain: '+US$ 380,00 (+5,4%)',
    cash: 'US$ 60.520,00',
    allocation: [
      ['US$ 2.040,00', '3%', 3],
      ['US$ 5.440,00', '8%', 8],
      ['US$ 0,00', '0%', 0],
      ['US$ 60.520,00', '89%', 89],
    ],
    stocks: ['US$ 2.040,00', [['cobalto', 'US$ 1.900,00', 'US$ 2.040,00', '+7,4%', 'pos']]],
    etf: ['US$ 5.440,00', [['acoesg', 'US$ 5.200,00', 'US$ 5.440,00', '+4,6%', 'pos']]],
    fixedIncome: null,
    history: [{ icon: 'in', title: 'Aporte', meta: 'há 4 dias' }],
  });
}

/** Thiago after buying all his cash in acoesg ("Tudo"): the history row carries the purchase amount. */
export function thiagoCarteiraAfterTudo(): Screen {
  return carteira({
    total: 'US$ 68.000,00',
    applied: 'US$ 67.620,00',
    gain: '+US$ 380,00 (+0,6%)',
    cash: 'US$ 0,00',
    allocation: [
      ['US$ 2.040,00', '3%', 3],
      ['US$ 65.960,00', '97%', 97],
      ['US$ 0,00', '0%', 0],
      ['US$ 0,00', '0%', 0],
    ],
    stocks: ['US$ 2.040,00', [['cobalto', 'US$ 1.900,00', 'US$ 2.040,00', '+7,4%', 'pos']]],
    etf: ['US$ 65.960,00', [['acoesg', 'US$ 65.720,00', 'US$ 65.960,00', '+0,4%', 'pos']]],
    fixedIncome: null,
    history: [
      { icon: 'out', title: 'Compra · Maré Ações Globais ETF', meta: 'há 2 min', value: '−US$ 60.520,00' },
      { icon: 'in', title: 'Aporte', meta: 'há 4 dias' },
    ],
  });
}

/** Fernanda's Carteira: all three classes and no movement yet. */
export function fernandaCarteira(): Screen {
  return carteira({
    total: 'US$ 8.200,00',
    applied: 'US$ 6.840,00',
    gain: '+US$ 212,00 (+3,1%)',
    cash: 'US$ 1.148,00',
    allocation: [
      ['US$ 1.640,00', '20%', 20],
      ['US$ 3.690,00', '45%', 45],
      ['US$ 1.722,00', '21%', 21],
      ['US$ 1.148,00', '14%', 14],
    ],
    stocks: ['US$ 1.640,00', [['farol', 'US$ 1.590,00', 'US$ 1.640,00', '+3,1%', 'pos']]],
    etf: [
      'US$ 3.690,00',
      [
        ['acoesg', 'US$ 1.900,00', 'US$ 2.000,00', '+5,3%', 'pos'],
        ['renda', 'US$ 1.660,00', 'US$ 1.690,00', '+1,8%', 'pos'],
      ],
    ],
    fixedIncome: ['US$ 1.722,00', [['tbill', 'US$ 1.690,00', 'US$ 1.722,00', '+1,9%', 'pos']]],
    history: [],
  });
}
