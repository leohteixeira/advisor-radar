import type { ClientInfo } from './types';

export const CLIENTS: Record<string, ClientInfo> = {
  c01: { id: 'c01', name: 'Mariana Costa', segment: 'Singular', aum: 248300, advisor: 'Ana Paula Ribeiro', since: '2021' },
  c02: { id: 'c02', name: 'Paulo Henrique Souza', segment: 'Advance', aum: 96400, advisor: 'Ana Paula Ribeiro', since: '2022' },
  c03: { id: 'c03', name: 'Fernanda Lima', segment: 'Essencial', aum: 8200, advisor: 'Ana Paula Ribeiro', since: '2024' },
  c04: { id: 'c04', name: 'Carlos Eduardo Ramos', segment: 'Advance', aum: 142000, advisor: 'Ana Paula Ribeiro', since: '2020' },
  c05: { id: 'c05', name: 'Juliana Martins', segment: 'Singular', aum: 512900, advisor: 'Ana Paula Ribeiro', since: '2019' },
  c06: { id: 'c06', name: 'Roberto Nascimento', segment: 'Essencial', aum: 7800, advisor: 'Ana Paula Ribeiro', since: '2025' },
  c07: { id: 'c07', name: 'Ana Beatriz Oliveira', segment: 'Advance', aum: 190500, advisor: 'Ana Paula Ribeiro', since: '2021' },
  c08: { id: 'c08', name: 'Lucas Pereira', segment: 'Essencial', aum: 6100, advisor: 'Ana Paula Ribeiro', since: '2023' },
  c09: { id: 'c09', name: 'Patrícia Gomes', segment: 'Singular', aum: 780000, advisor: 'Ana Paula Ribeiro', since: '2018' },
  c10: { id: 'c10', name: 'Marcelo Ferreira', segment: 'Advance', aum: 88000, advisor: 'Ana Paula Ribeiro', since: '2022' },
  c11: { id: 'c11', name: 'Sérgio Cardoso', segment: 'Singular', aum: 450000, advisor: 'Ana Paula Ribeiro', since: '2019' },
  c12: { id: 'c12', name: 'Helena Barbosa', segment: 'Advance', aum: 108000, advisor: 'Ana Paula Ribeiro', since: '2023' },
  c13: { id: 'c13', name: 'Thiago Azevedo', segment: 'Advance', aum: 68000, advisor: 'Ana Paula Ribeiro', since: '2024' },
  c14: { id: 'c14', name: 'Camila Rodrigues', segment: 'Advance', aum: 175000, advisor: 'Ana Paula Ribeiro', since: '2020' },
  c15: { id: 'c15', name: 'Rafael Monteiro', segment: 'Singular', aum: 950000, advisor: 'Ana Paula Ribeiro', since: '2017' },
  c16: { id: 'c16', name: 'Beatriz Santana', segment: 'Essencial', aum: 7500, advisor: 'Ana Paula Ribeiro', since: '2025' },
  c17: { id: 'c17', name: 'Diego Carvalho', segment: 'Essencial', aum: 8900, advisor: 'Ana Paula Ribeiro', since: '2024' },
  c18: { id: 'c18', name: 'Vanessa Moreira', segment: 'Advance', aum: 119000, advisor: 'Ana Paula Ribeiro', since: '2021' },
  c19: { id: 'c19', name: 'Gustavo Teixeira', segment: 'Advance', aum: 141000, advisor: 'Ana Paula Ribeiro', since: '2022' },
  c20: { id: 'c20', name: 'Isabela Nunes', segment: 'Essencial', aum: 9400, advisor: 'Ana Paula Ribeiro', since: '2025' },
  c21: { id: 'c21', name: 'Otávio Freitas', segment: 'Essencial', aum: 5800, advisor: 'Bruno Dias', since: '2024' },
  c22: { id: 'c22', name: 'Renata Albuquerque', segment: 'Advance', aum: 67000, advisor: 'Carla Menezes', since: '2023' },
};

export function clientName(id: string): string {
  return CLIENTS[id]?.name ?? id;
}

export function clientSegment(id: string): string {
  return CLIENTS[id]?.segment ?? 'Essencial';
}
