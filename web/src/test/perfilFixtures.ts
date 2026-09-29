/**
 * SDUI Perfil envelopes shaped like the BFF output of story 12 for the seed
 * clients (internal/screen/catalog.json and the advisory max-risk table).
 */
import type { Component, Preferences, Screen } from '../sdui/types';

const LEVELS = [
  { key: 'conservador', label: 'Conservador', description: 'Prioriza preservar o patrimônio. Aceita pouca oscilação.', max: 2 },
  { key: 'moderado', label: 'Moderado', description: 'Aceita oscilação moderada em busca de mais retorno.', max: 3 },
  { key: 'arrojado', label: 'Arrojado', description: 'Aceita oscilação alta no curto prazo para buscar retorno maior.', max: 5 },
] as const;

export const BETA_OFF = 'Veja antes as novas versões das telas.';
export const BETA_ON = 'Ligado. Você recebe a revision v2 do início antes dos outros clientes.';

export function profileHeader(name: string, initials: string, subtitle: string, account?: string): Component {
  return { type: 'profile_header', variant: 'default', props: { initials, name, subtitle, ...(account ? { account } : {}) } };
}

export function profileScale(profile: (typeof LEVELS)[number]['key'], assessed: string): Component {
  return {
    type: 'profile_scale',
    variant: profile,
    props: {
      title: 'Seu perfil de investidor',
      subtitle: 'Define os destaques de Investir e quando uma compra recebe aviso.',
      current_label: 'Seu perfil',
      levels: LEVELS.map((level) => ({
        key: level.key,
        label: level.label,
        description: level.description,
        limit: `Produtos até risco ${level.max}`,
        max_risk: level.max,
        current: level.key === profile,
      })),
      footer: `Última avaliação em ${assessed}. Para refazer o questionário, fale com a sua assessora.`,
    },
  };
}

export function profileFields(values: [string, string][]): Component {
  return {
    type: 'profile_field_list',
    variant: 'default',
    props: {
      title: 'Dados cadastrais',
      fields: values.map(([label, value]) => ({ label, value })),
      footnote: 'Dados fictícios. Alterar cadastro fica fora da simulação.',
    },
  };
}

export function preferenceList(prefs: Preferences): Component {
  return {
    type: 'preference_list',
    variant: 'default',
    props: {
      title: 'Preferências',
      theme: { label: 'Tema', hint: 'Fica salvo só neste navegador' },
      channel: {
        label: 'Canal preferido',
        hint: 'Por onde a assessoria fala com você',
        value: prefs.channel,
        options: [
          { value: 'chat', label: 'Chat' },
          { value: 'email', label: 'E-mail' },
        ],
      },
      beta: { label: 'Programa beta', hint: prefs.beta ? BETA_ON : BETA_OFF, enabled: prefs.beta },
    },
  };
}

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

function perfil(sections: [string, Component][]): Screen {
  return {
    schema_version: 1,
    slug: 'perfil',
    revision: 'v1',
    title: 'Perfil',
    subtitle: 'Seus dados, seu perfil de investidor e suas preferências',
    sections: sections.map(([id, component]) => ({ id, components: [component] })),
    omitted: [],
  };
}

/** Fernanda's Perfil: conservador, assessed 12/03/2026, with the given preferences. */
export function fernandaPerfil(prefs: Preferences = { channel: 'chat', beta: false }): Screen {
  return perfil([
    ['header', profileHeader('Fernanda Lima', 'FL', 'Cliente Essencial desde 2024', 'Conta 3301-7 · Orla Invest')],
    ['suitability', profileScale('conservador', '12/03/2026')],
    [
      'registration',
      profileFields([
        ['Nome', 'Fernanda Lima'],
        ['E-mail', 'fernanda.lima@example.com'],
        ['Telefone', '+55 (19) •••••-4471'],
        ['Cidade', 'Campinas, SP · Brasil'],
        ['Segmento', 'Essencial'],
        ['Cliente desde', '2024'],
      ]),
    ],
    ['preferences', preferenceList(prefs)],
    ['advisor', advisor('default', '24 h', 'Essencial')],
  ]);
}

/** Thiago's Perfil: arrojado, assessed 04/08/2026, chat with beta off. */
export function thiagoPerfil(): Screen {
  return perfil([
    ['header', profileHeader('Thiago Azevedo', 'TA', 'Cliente Advance desde 2024', 'Conta 2847-1 · Orla Invest')],
    ['suitability', profileScale('arrojado', '04/08/2026')],
    [
      'registration',
      profileFields([
        ['Nome', 'Thiago Azevedo'],
        ['E-mail', 'thiago.azevedo@example.com'],
        ['Telefone', '+55 (48) •••••-2093'],
        ['Cidade', 'Florianópolis, SC · Brasil'],
        ['Segmento', 'Advance'],
        ['Cliente desde', '2024'],
      ]),
    ],
    ['preferences', preferenceList({ channel: 'chat', beta: false })],
    ['advisor', advisor('default', '4 h', 'Advance')],
  ]);
}
