# UX

Replicate the adopted companions. `Advisor Radar.dc.html` is the product screens. `Advisor Radar - Design System.dc.html` is the tokens, type, and components. `tokens.css` and `tokens.json` are the values. `mock-data.js` is the deterministic seed: clients, signals, the market-day stream, the review rows, cases, one timeline, and the manager snapshot. Do not invent a second visual system.

`handoff/components.md` was absorbed here with two corrections. Do not render any real institution name. Review is intent probability below 0.85, not "everything below alta". The bands below are display only, so a card can show alta and still say "Precisa de revisão".

## Shell

One responsive app. `AppShell` with `PersonaSwitcher` and a theme toggle. Personas and their landing views:

| Persona | Label | Lands on |
|---|---|---|
| Advisor | Fila | Queue |
| Analyst | Revisão de triagem | Review queue |
| Manager | Painel da assessoria | Manager dashboard |

No real login. Switching persona is the fictional sign-in. Light is the default (`data-theme="light"`). Dark is the other theme. The toggle label is "Mudar para tema escuro" or "Mudar para tema claro". Honor `prefers-reduced-motion`. Minimum touch target is 44px. Breakpoints: 720px tablet, 900px desktop. Fonts: Albert Sans for UI, Literata for names, message quotes, and screen titles, JetBrains Mono for SLA, ids, and the demo. Color, type, space, and radius come from the token files. Components use those CSS variables and React only. No component library.

Shipped copy is Portuguese. Code and this contract stay in English.

React file names match the prototype map: `SignalCard`, `SegmentBadge`, `IntentBadge`, `ConfidenceBadge`, `ReviewBadge`, `FallbackBadge`, `FrustrationIndicator`, `SlaCounter`, `CaseRail`, `TimelineItem`, `DistributionBars`, `FilterBar`, `FilterChips`, `SearchField`, `NewItemsPill`, `Toast`, `SkeletonCard`, `EmptyState`, `ErrorState`, `ReconnectBanner`, `MetricCard`, `StackedBar`, `BarList`, `Icon`, `CaseCard`, `DetailModal`, `QueueBoard`, `AppShell`, `PersonaSwitcher`.

## Screens

`QueueScreen`, `MessageDetail`, `AlertDetail`, `CaseScreen`, `Client360`, `ReviewQueue`, `ManagerDashboard`. `DemoPanel` is outside the product, at `/demo`.

Below 900px the queue is one horizontal board. Columns are `min(84vw, 320px)` with snap. Opening a card replaces the board. At 900px and above the board is four columns of `minmax(0, 1fr)`: Novos sinais, Aberto, Em atendimento, Aguardando cliente. Case cards in the last three columns advance in place. Detail opens in a modal, max width 760, closed by Esc, the close control, or a click outside. "Abrir caso" on desktop moves the signal into Aberto and does not open the modal.

## Queue and live updates

A signal card shows who, the segment, why it is here, and the SLA. The left border is critical, warning, or neutral, as in the prototype. Actions on the card: open, open case, Contatado, Adiar 1 h. Those two actions persist in advisory's PostgreSQL and survive a reload. Snooze hides the card for one hour. Undo removes the persisted action. A toast confirms the action, with undo when the prototype shows undo.

SSE event name is `signal`. The payload shape is the `SIGNALS` / `STREAM` objects in `mock-data.js`. The client keeps `incoming` apart from `items`. A pill shows the buffered count and merges on tap. On socket error, show a reconnecting banner with the attempt count. Catch-up uses `Last-Event-ID`. Empty, loading, and error states use the skeleton, empty, and error components.

Filters: text search on the customer, plus multi-select groups Andamento, SLA, Sinal, Motivo, and Segmento. Active filters become removable tags, plus "Limpar filtros". On desktop, Andamento chooses which kanban columns are visible.

## Triage presentation

| Wire intent | Label |
|---|---|
| operacional | Operacional |
| cambio | Câmbio |
| tributacao | Tributação |
| investimento | Investimento |
| resgate | Resgate |
| reclamacao | Reclamação |
| encerramento | Encerramento |
| contato | Contato |

Frustration labels, in order: Calmo, Incomodado, Frustrado, Muito frustrado. Show four rising bars plus the label. Do not rely on color alone.

Confidence bands, display only: alta at or above 0.75, média at or above 0.5, baixa below 0.5. The intent badge is three bars and no numeral. The "Por que isso está aqui" panel may show the confidence label. The full distribution can expand and then show numbers. A message with intent probability below 0.85 shows "Precisa de revisão". A degraded result shows "Classificação simplificada". A message with churn risk uses the risco treatment, label "Mensagem com risco".

Alert labels: Saque relevante, Queda de patrimônio, Aporte grande, Mudança de segmento, Sem contato há muito tempo.

Segments: Essencial, Advance, Singular. Bounds are in the SPEC assumptions.

## Case, 360, review, manager

The case screen shows the four-step rail, a block SLA counter in `mm:ss` with a bar, and notes. The card uses the inline SLA counter. SLA colors follow No prazo, Vencendo, and Vencido in `state-machines.md`.

The 360 timeline mixes messages, alerts, cases, notes, and calls. Chips are single-select: Todos, Conta, Mensagens, Casos, Notas. A search field filters the timeline text. The advisor can add a note on the case; it shows on that case and on the timeline.

The review queue is the analyst surface. Each row shows the message and the intent distribution. The analyst can set a corrected intent. Membership is intent probability below 0.85, including heuristic results at 0.3 or 0.6. The seed `REVIEW` list is that queue: every row is under 0.85, including alta scores such as 0.82 and 0.79.

The manager panel shows backlog per advisor (open, at risk, overdue), cases with SLA at risk, average minutes to first contact, the share of messages that needed review, and the fallback share. The seed snapshot is `MANAGER` in `mock-data.js`.

## Demo

The five-minute phone script:

1. Open the advisor queue.
2. Simulate a market day. Alerts and triaged messages arrive live.
3. Open a churn message and show the triage signals.
4. Open a case, let the SLA expire, show the escalation.
5. Show the full trace in Grafana.
6. Stop a consumer, start it, show the event was not applied twice.
7. Cut the model, show heuristic classification marked as simplified.

The `/demo` panel may expose the simulator and the failure switches. It is not part of the advisor, analyst, or manager product nav.

Icons are 24×24 with stroke 1.8, one per alert type and per action, paths as in `mock-data.js`.
