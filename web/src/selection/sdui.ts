/**
 * Copy of the selection screen's Server-Driven UI section (#sdui), from
 * docs/design/sdui-full-pov/project/Main.dc.html. This is documentation copy:
 * nothing here decides what the BFF serves.
 */

/**
 * The three seed clients, in tab order, by their fixed seed ids (ADR 0007).
 * `segment` is the design copy used when the POV list is unavailable.
 */
export const SDUI_CLIENTS = [
  { id: '01a0e3a4-9a44-757a-ac8f-dab7db5eb068', first: 'Fernanda', segment: 'Essencial' },
  { id: '01a0e3a4-9a44-75dd-b3a0-403a7a87836e', first: 'Thiago', segment: 'Advance' },
  { id: '01a0e3a4-9a44-7566-b5de-eb2e365799f8', first: 'Mariana', segment: 'Singular' },
] as const;

export type SduiClientName = (typeof SDUI_CLIENTS)[number]['first'];

/** The client selected when the section opens, as in the artboard. */
export const SDUI_DEFAULT_CLIENT: SduiClientName = 'Thiago';

export interface MomentRule {
  /** Position in the home moment priority, top to bottom. */
  rank: number;
  /** The fact the row documents. */
  fact: string;
  /** The service that evaluates the fact. */
  source: string;
}

/**
 * The home moment priority table of specs/http/bff.md ("Home moment
 * priority"), keyed by variant, in Portuguese. Web shows the row of the
 * variant the BFF sent; it never evaluates a row.
 */
export const MOMENT_RULES: Readonly<Record<string, MomentRule>> = {
  portfolio_drop: {
    rank: 1,
    fact: 'A perda da última reavaliação passa de 15% do patrimônio de antes dela.',
    source: 'advisory',
  },
  case_open: {
    rank: 2,
    fact: 'O cliente tem um caso aberto (cases, filtrado pelo cliente).',
    source: 'cases',
  },
  segment_upgraded: {
    rank: 3,
    fact: 'Um evento de conta ao vivo (schema_version 2 ou mais) subiu o segmento do cliente nas últimas 24 h.',
    source: 'advisory',
  },
  segment_upgrade_near: {
    rank: 4,
    fact: 'Patrimônio de US$ 7.500 até abaixo de US$ 10.000 (750000 ≤ patrimony_cents < 1000000).',
    source: 'advisory',
  },
  idle_cash: {
    rank: 5,
    fact: 'Patrimônio acima de zero e caixa igual ou acima de 50% do patrimônio.',
    source: 'advisory',
  },
  portfolio_review: {
    rank: 6,
    fact: 'O cliente é Singular.',
    source: 'advisory',
  },
  welcome: {
    rank: 7,
    fact: 'Padrão, inclusive quando os fatos de momento falham.',
    source: 'nenhuma fonte',
  },
};

/** Rows in the moment priority table. */
export const MOMENT_RULE_COUNT = Object.keys(MOMENT_RULES).length;

/** The documented row for a variant, or undefined when the table has none. */
export function momentRule(variant: string): MomentRule | undefined {
  return Object.hasOwn(MOMENT_RULES, variant) ? MOMENT_RULES[variant] : undefined;
}

export interface SduiStep {
  title: string;
  text: string;
  tag: string;
}

/** How the BFF composes one screen, in order. */
export const SDUI_STEPS: readonly SduiStep[] = [
  {
    title: 'Uma requisição por tela',
    text: 'O app pede a tela pelo slug. Nada de várias chamadas pela rede do celular.',
    tag: 'screens/{slug}',
  },
  {
    title: 'Retrato do cliente em paralelo',
    text: 'Conta e posições, momentos, casos e timeline por gRPC, com um deadline para a tela toda.',
    tag: 'errgroup · deadline',
  },
  {
    title: 'Uma variante por seção',
    text: 'Cada variante é uma interface pequena. Vence a primeira que casa; a padrão casa sempre.',
    tag: 'Variant.Matches',
  },
  {
    title: 'Texto do catálogo',
    text: 'A copy vive num catálogo versionado no Git, embutido no bff, com variáveis como nome e assessora.',
    tag: 'go:embed · text/template',
  },
  {
    title: 'O app renderiza por type',
    text: 'Um registro de componentes React. Um type desconhecido ou quebrado some; o resto da tela fica.',
    tag: 'type → componente',
  },
];

export interface SduiPrinciple {
  title: string;
  text: string;
}

export const SDUI_PRINCIPLES: readonly SduiPrinciple[] = [
  {
    title: 'Regra no backend',
    text: 'Percentuais, valores formatados, avisos de perfil e SLA chegam prontos. O front não calcula nada.',
  },
  {
    title: 'Falha isolada',
    text: 'Se uma fonte cai, a seção dela usa a variante padrão ou some. A tela nunca cai por causa de um componente.',
  },
  {
    title: 'Versão e beta',
    text: 'Cada tela tem slug e revision. Quem entra no programa beta pelo Perfil recebe a revision nova antes dos outros.',
  },
  {
    title: 'Observável',
    text: 'Um span por seção montada e uma métrica por variante servida. Dá para ver quem viu o quê, e quanto cada fonte custou.',
  },
];
