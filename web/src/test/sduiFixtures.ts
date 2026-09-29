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
