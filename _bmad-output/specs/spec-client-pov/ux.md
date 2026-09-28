# UX

Replicate the adopted artboards. `Main.dc.html` and `Selecao-mobile.dc.html` are selection. `ClientPov.dc.html` and `ClientPov-Desktop.dc.html` are the client list. `ClientApp.dc.html` is the client app; the other `ClientApp-*` files are that component with fixed props. Selection and the list use `tokens.css` (Albert Sans, Literata, JetBrains Mono) and stay on the Advisor Radar light theme. They have no dark theme.

Where this file names a route, it wins over the artboard. The walkthrough tag `POST /api/v1/client-pov/{id}/complaints` is a placeholder. Ship `POST /v1/client-pov/customers/{id}/complaints`.

Shipped copy is Portuguese. Code and this contract stay in English. Orla Invest is fictional: Manrope, gold accent `#f2b544`, no real brokerage name, mark, or color. Palettes are `palette()` in `ClientApp.dc.html`. Dark is the client-app default. The theme control reads "Tema claro" or "Tema escuro". Honor `prefers-reduced-motion`. Minimum touch target is 44px.

The team card opens the queue at `/advisor-radar/fila` (`https://leohts.tech/advisor-radar/fila`). Review, the manager panel, and the 360 view stay at `/advisor-radar/revisao`, `/advisor-radar/painel`, and `/advisor-radar/clientes/{id}`. Do not change how those screens behave.

## Selection

`/advisor-radar`. Headline: "O mesmo sinal, visto dos dois lados da conversa."

Two cards:

| Card | Eyebrow | What it opens |
|---|---|---|
| Visão do time | Assessor · Analista · Gestor | `/advisor-radar/fila`. The team product still includes the queue, triage review, the manager panel, and the 360 view. |
| Visão do cliente | Um cliente fictício por segmento | `/advisor-radar/client-pov`. Withdrawals, deposits, messages, and complaints become events on the team queue. |

Exact sentences are the adopted artboard for that width (390 and 1440). Desktop cards in `Main.dc.html` also show chips: Fila do assessor, Revisão de triagem, Painel da assessoria, Visão 360 do cliente; and Saque e wire-out, Depósito, Mensagem livre, Reclamações prontas. The team button reads "Entrar como time".

Below the cards, three pillars: alertas proativos (the five phase-1 rules), triagem de mensagens (intent, frustration, churn, human request, review under 0.85), casos com SLA (segment SLA, TTL escalation, no cron).

Then "Como uma reclamação chega à fila": eight steps, auto-advance every 2.6 seconds, pause control, and `prefers-reduced-motion` stops the motion. Desktop draws the graph in `Main.dc.html`. Mobile uses the ordered list in `Selecao-mobile.dc.html`. Same steps:

| Step | Title | Tag to show |
|---|---|---|
| 1 | O cliente abre uma reclamação no app | `POST /v1/client-pov/customers/{id}/complaints` |
| 2 | O BFF entrega o comando ao account-sim | gRPC · deadline propagado |
| 3 | Mensagem e evento gravados na mesma transação | PostgreSQL · transactional outbox |
| 4 | O relay publica no RabbitMQ | `message.received` |
| 5 | A triagem classifica a mensagem | Jev · fallback heurístico |
| 6 | Uma regra gera o alerta e o caso ganha SLA | `message.triaged` → `alert.raised` |
| 7 | A visão 360 é atualizada | Elasticsearch |
| 8 | O alerta aparece na fila do assessor | SSE · `alert.raised` |

The footer lists the stack printed on that artboard and states that the data is fictional and that no message is sent to a real customer.

## Client list

`/advisor-radar/client-pov`. One card per seeded client. Each card shows initials, name, client since, advisor, segment, assets, base SLA, and the hint. SLA labels match phase 1: Essencial 24 h, Advance 4 h, Singular 1 h. Assets on the card are the seed, not the artboard, when those differ.

| Client | Hint |
|---|---|
| Fernanda Lima | Perto do teto da faixa. Um depósito pode subir o segmento; uma reclamação mostra o SLA mais longo. |
| Thiago Azevedo | Acabou de subir de Essencial para Advance com um depósito grande. |
| Mariana Costa | Já reclamou de uma transferência atrasada. Um saque grande ou uma ameaça de saída sobem a prioridade na hora. |

Footer: "Nenhuma mensagem é enviada a pessoas reais. Os eventos entram na mesma fila que o time vê."

## Client app

`/advisor-radar/client-pov/{id}`. Brand lockup "orla. invest". A fixed Advisor Radar strip stays on the light tokens above the brokerage UI: "Simulação · vendo como {name}" and "Trocar cliente", which returns to the list.

Phone width is 390. Desktop is 1440 with a 240px sidebar. Actions on desktop open a 440px panel over the home, with a scrim that closes it; the header control says "Fechar". On the phone the action replaces the home and the control says "Voltar". If the desktop layout is cut, center the phone layout.

Sidebar and the bottom bar show Início, Investir, Carteira, and Perfil. Only Início works. Investir, Carteira, and Perfil are visual. The notifications control is visual only.

Home ("Olá, {first}"): total assets with a hide control ("Esconder valores" / "Mostrar valores", local only), segment pill, four-class allocation bar, cash available to withdraw, the advisor Ana Paula Ribeiro with "Conversar", and recent activity. Empty activity: "Nenhuma movimentação nos últimos 30 dias." Footer: "Orla Invest é uma corretora fictícia criada para a demo do Advisor Radar." Percentages, alerts, and SLA are not computed in the app. Balances come from the seed in `architecture.md`. Mariana's available cash is US$ 60.000,00, not the artboard's US$ 31.450,00. Fernanda and Thiago match the artboard cash. A class at zero is hidden. The visitor types dollars; the client sends integer cents.

Actions, in order: Depositar (plus in a circle), Sacar (bank), Mensagem, Reclamar. Up and down arrows are not used.

### Deposit

Amount in USD, chips US$ 1.000, US$ 10.000, US$ 50.000. Origin: "Câmbio a partir do Brasil" (default) or "Wire de outro banco". Hint: the advisory may raise a large-deposit alert and, if assets cross a band, a segment-change alert. Confirm is enabled only when the amount is greater than zero. Title after send: "Depósito solicitado".

### Withdrawal

Amount in USD, chips US$ 1.000, US$ 5.000, US$ 20.000, and "Tudo" (the available cash). Destination: "Conta nos EUA" (default, fictional bank, final 4821) or "Conta no Brasil" (final 0937). When the typed amount exceeds available cash, show "Valor acima do disponível para saque" and keep Confirm disabled. That check is presentational. The refusal the visitor can trust is the backend `422`. The withdrawal does not sell positions. On Mariana, the US$ 20.000 chip stays under the relevant-withdrawal line; "Tudo" crosses it. Hint: the app does not decide whether the withdrawal is a relevant withdrawal or an asset drop. Title after send: "Saque solicitado".

A `429` shows "Muitas ações em pouco tempo. Espere um minuto e tente de novo." for the minute cap, and "Limite de ações desta demo atingido por hoje." for the day cap. Nothing is published.

### Free message

Thread shows only what this client sent. No advisor bubble. Empty: "Ainda não há mensagens. Escreva a primeira." Channels: Chat (default) and E-mail. Send stays disabled while the text is empty. After send, return target is the thread. Title: "Mensagem enviada". This screen is the fourth cut.

### Complaint

Four presets, or free text. Choosing a preset clears the free text, and typing clears the preset. Send stays disabled until one of them is non-empty. Channel is chat. Title: "Reclamação registrada". Back goes home.

| Title | Text sent |
|---|---|
| Minha transferência está atrasada | Pedi uma transferência há dias e até agora não caiu. Preciso de uma posição hoje. |
| Uma cobrança que não reconheço | Apareceu uma cobrança no meu extrato que eu não reconheço. Quero entender o que é. |
| Quero falar com uma pessoa | Não quero mais resposta automática. Preciso que alguém da assessoria me ligue. |
| Estou pensando em sair | Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora. |

The demo uses the fourth preset.

## Confirmation

"Bastidores · simulação" uses the Advisor Radar light surface even inside the dark brokerage theme. It shows the routing key, a one-line payload summary, `event_id`, and `trace_id` when a trace exists. The sample trace on the artboard is a placeholder. Protocolo is the first two groups of `event_id`, uppercased (`01a0e3a5-2f4c-…` shows as `01A0E3A5-2F4C`). The UI formats it. The `202` does not send a protocol field.

Account steps: "Gravado na outbox do account-sim", "Publicado no RabbitMQ", "Avaliado pelas regras do advisory", "Na fila da assessoria, se uma regra disparar". Message steps use "Classificado pela triagem" and "Na fila da assessoria" as the last two. Each step reads feito, agora, or aguardando. Caption: "Meta do produto: do evento à fila em menos de 2 s (p95)."

Primary button returns home, or back to the thread after a free message. Secondary: "Ver na fila do time", linking to `/advisor-radar/fila`.

Steps advance on `GET /v1/client-pov/customers/{id}/stream`, as mapped in `architecture.md`. If live Bastidores is cut, keep `event_id` and the queue link.

## Demo

Phone script:

1. Open `/advisor-radar` and explain the walkthrough in about 30 seconds.
2. Enter as Mariana Costa and send "Estou pensando em sair".
3. Show Bastidores moving through the outbox, RabbitMQ, and triage.
4. Open `/advisor-radar/fila`. The churn alert is at the top of the queue with the Singular SLA.
5. Return as Fernanda Lima, deposit US$ 10.000, and show the large-deposit and segment-change alerts.
6. Submit the same withdrawal twice with the same idempotency key and show a single event.
