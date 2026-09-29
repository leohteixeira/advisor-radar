# Advisor Radar — mapa de componentes (React + TypeScript)

Nomes de arquivo espelham o protótipo `Advisor Radar.dc.html`. Todos usam só CSS vars de `tokens.css`; nenhum depende de biblioteca além de React.

## Domínio (`src/domain/types.ts`)
```ts
type Segment = 'Essencial' | 'Advance' | 'Singular';
type AlertType = 'saque' | 'queda' | 'aporte' | 'segmento' | 'contato' | 'risco';
type Intent = 'Operacional' | 'Câmbio' | 'Tributação' | 'Investimento' | 'Resgate' | 'Reclamação' | 'Encerramento' | 'Contato';
type Frustration = 0 | 1 | 2 | 3;               // Calmo, Incomodado, Frustrado, Muito frustrado
type Confidence = 'alta' | 'média' | 'baixa';   // ≥0.75, ≥0.5, <0.5 — faixas só de exibição; revisão quando a intenção < 0.85
type CaseState = 'Aberto' | 'Em atendimento' | 'Aguardando cliente' | 'Resolvido';
type SlaState = 'No prazo' | 'Vencendo' | 'Vencido'; // Vencendo = restante < max(33% do total, 20 min)
```
Faixas fictícias: Essencial até US$ 10.000 inclusive · Advance acima de US$ 10.000 até US$ 200.000 inclusive · Singular acima de US$ 200.000. SLA base por segmento (min): Essencial 1440 · Advance 240 · Singular 60. Reduz pela metade se risco de saída, frustração ≥ Frustrado, pedido humano ou saque relevante.

## Componentes (`src/components/`)
| Arquivo | Props principais | Onde aparece |
|---|---|---|
| `SignalCard.tsx` | `signal`, `onOpen`, `onOpenCase`, `onContacted`, `onSnooze`, `justArrived` | Fila. Borda esquerda: crit / warn / neutra |
| `SegmentBadge.tsx` | `segment` | Card, detalhes, 360 |
| `IntentBadge.tsx` | `intent`, `confidence` | 3 barrinhas = certeza; nunca mostra número |
| `ConfidenceBadge.tsx` | `confidence` | Painel "Por que isso está aqui", revisão |
| `ReviewBadge.tsx` | — | "Precisa de revisão" (tracejado, warn) |
| `FallbackBadge.tsx` | — | "Classificação simplificada" (contorno neutro) |
| `FrustrationIndicator.tsx` | `level: Frustration` | 4 barras crescentes + rótulo (não depende só de cor) |
| `SlaCounter.tsx` | `deadline`, `total`, `variant: 'inline' \| 'block'` | Card (inline) e Caso (bloco com mm:ss e barra) |
| `CaseRail.tsx` | `state: CaseState` | Trilho de 4 etapas |
| `TimelineItem.tsx` | `kind`, `title`, `text`, `meta`, `at` | Visão 360; marcador por tipo |
| `DistributionBars.tsx` | `dist: Record<Intent, number>`, `showNumbers?` | Detalhe da mensagem (expansível) |
| `FilterBar.tsx` | `query`, `filters`, `onChange` | Fila: busca por cliente + grupos multi-seleção (Andamento, SLA, Sinal, Motivo, Segmento) em popover com checkboxes; filtros ativos viram tags removíveis + "Limpar filtros". No desktop, Andamento controla quais colunas do quadro aparecem |
| `FilterChips.tsx` | `options`, `value`, `onChange` | Timeline da visão 360 (seleção única) |
| `SearchField.tsx` | `value`, `onChange` | Timeline |
| `NewItemsPill.tsx` | `count`, `onMerge` | Fila; itens ficam em buffer até o toque |
| `Toast.tsx` | `text`, `onUndo?` | Global |
| `SkeletonCard.tsx` / `EmptyState.tsx` / `ErrorState.tsx` | — | Estados da fila |
| `ReconnectBanner.tsx` | `attempt` | Estado SSE "reconectando" |
| `MetricCard.tsx` / `StackedBar.tsx` / `BarList.tsx` | — | Painel do gestor |
| `Icon.tsx` | `name` | Traço 1.8, 24×24; um por tipo de alerta e por ação |

## Telas (`src/screens/`)
`QueueScreen` · `MessageDetail` · `AlertDetail` · `CaseScreen` · `Client360` · `ReviewQueue` · `ManagerDashboard` · `DemoPanel` (fora do produto, `/demo`).
Layout: `AppShell` com `PersonaSwitcher` no topo. Mobile: o mesmo `QueueBoard`, com colunas de `min(84vw,320px)` em rolagem horizontal com snap; detalhe substitui o quadro. ≥900 px: `QueueBoard` em kanban com 4 colunas (`Novos sinais` · `Aberto` · `Em atendimento` · `Aguardando cliente`, `repeat(4,minmax(0,1fr))`), `CaseCard.tsx` nas colunas de caso (`onAdvance` move para a próxima etapa), e o detalhe abre em `DetailModal.tsx` (max-width 760, fecha com Esc, ✕ ou clique fora). "Abrir caso" no desktop move o card para `Aberto` sem abrir o modal.

## Tempo real (SSE)
Evento `signal` com o payload de `mock-data.js` (`SIGNALS`/`STREAM`). Cliente mantém `incoming[]` separado de `items[]`; `NewItemsPill` faz o merge. Em `onerror`, estado `reconnecting` com contador de tentativas; eventos perdidos vêm via `Last-Event-ID`.
