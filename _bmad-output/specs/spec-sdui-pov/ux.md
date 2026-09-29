# UX

Replicate the adopted artboards in `docs/design/sdui-full-pov/project/`. `OrlaApp.dc.html` is the client app, and every `Home-*`, `Investir-*`, `Compra-*`, `Carteira-*`, and `Perfil-*` artboard is that component with fixed props. `Catalogo.dc.html` shows every type. `Main.dc.html` and `Selecao-SDUI-mobile.dc.html` are the selection screen. Anything this file does not change stays as the adopted phase-2 `ux.md` defines it: Orla Invest identity, Manrope, gold `#f2b544`, dark by default, light theme, 44 px minimum targets, `prefers-reduced-motion`, and the team routes.

Values in the artboards are illustrative. Every number, percentage, warning, badge, and variant choice comes from the BFF. Web never computes one. Shipped copy is Portuguese. The copy below is the catalog's starting text, with `{{…}}` marking fields that Go fills.

## Selection screen (`/advisor-radar`)

- The header gains anchor links "Arquitetura" and "Server-Driven UI", with a "Novo" chip on the latter.
- Below the two view cards is a band: "Novo nesta versão · Server-Driven UI". Its text is "Três clientes, três inícios diferentes, nenhuma linha de front escrita para cada um." and "O bff devolve a tela como página, seções e componentes, já preenchida para o momento do cliente. O app só renderiza." Its link "Ver como funciona" goes to `#sdui`.
- A fifth pillar reads "Telas pelo servidor": "O app do cliente é server-driven. O backend escolhe a variante de cada seção pelo momento do cliente, e o front só desenha um catálogo fixo de componentes." The walkthrough's alert pillar adds "compra acima do perfil".
- `#sdui` follows `#arquitetura` and has the heading "O backend monta a tela de cada cliente". It has client tabs (Fernanda · Essencial, Thiago · Advance, Mariana · Singular). Side by side it shows the BFF response for `GET …/screens/home` ("200 · 1 requisição"), the moment ("Momento (advisory, gRPC)"), the rule that chose the variant, and the rendered home.
- Five steps follow:
  1. Uma requisição por tela.
  2. Retrato do cliente em paralelo.
  3. Uma variante por seção.
  4. Texto do catálogo.
  5. O app renderiza por type.
- Four principles follow: Regra no backend, Falha isolada, Versão e beta, and Observável.
- Step and principle copy is in `Main.dc.html`. The mobile layout is `Selecao-SDUI-mobile.dc.html` at 390 px.

## Client app shell

- Tabs: Início, Investir, Carteira, Perfil. They are a bottom bar on the phone and a sidebar on desktop. Routes are `/advisor-radar/client-pov/{id}` for home and `…/{id}/investir`, `…/{id}/carteira`, and `…/{id}/perfil`.
- The simulation strip shows "Simulação · vendo como {{name}}", then "Dia simulado {{day}}". Its buttons are "Avançar um dia" (phone: "+1 dia"), "Raio-X SDUI" (phone: "Raio-X", `aria-pressed`), and "Trocar cliente".
- Raio-X outlines each section and labels it `id · type · variant`. A banner shows `GET /v1/client-pov/customers/{id}/screens/{slug}` and "slug {{slug}} · revision {{revision}} · schema {{n}} · {{count}} seções". An omitted section shows only in Raio-X, as a dashed placeholder "`{{id}} · {{type}} · omitido`". Raio-X is a demo tool and not part of a real client app.
- Screen titles come from the BFF:
  - "Olá, {{first}}" / "Cliente {{segment}} desde {{since}}"
  - "Investir" / "Produtos fictícios · preço fixo da simulação"
  - "Carteira" / "Valores de mercado no dia simulado {{day}}"
  - "Perfil" / "Seus dados, seu perfil de investidor e suas preferências"
- Section order comes from the BFF. Web decides the breakpoint grid: one column on the phone and two on desktop, with the span per section as drawn.

## Team queue

The only team-side change is a new alert card kind, `perfil`, with the rule text "Compra acima do perfil de investidor". It uses the same card as the other kinds and needs a label and icon in the queue. There is no automatic case: the advisor opens one from the queue as usual.

## Component catalog (15 types)

| Type | Used by (section id) | Variants |
|---|---|---|
| `moment_card` | home `moment` | `portfolio_drop`, `case_open`, `segment_upgraded`, `segment_upgrade_near`, `idle_cash`, `portfolio_review`, `welcome` (default) |
| `wealth_summary` | home `wealth` | `default`, `with_day_change` |
| `action_grid` | home `actions` | `default` (Depositar, Sacar, Mensagem, Reclamar → `panel` actions) |
| `advisor_card` | home and perfil `advisor` | `default`, `dedicated` (Singular) |
| `activity_list` | home `activity`, carteira `history` | `recent`, `empty`, `history` |
| `invest_summary` | investir `cash` | `default` (cash and profile chip) |
| `product_rail` | investir `highlights`; home `highlights` in beta revision `v2` | `profile_conservador`, `profile_moderado`, `profile_arrojado` |
| `product_list` | investir `fixed_income`, `etfs`, `stocks` | `fixed_income`, `etf`, `stocks` |
| `portfolio_summary` | carteira `summary` | `default`, `with_day_change` |
| `allocation_breakdown` | carteira `allocation` | `default` |
| `position_list` | carteira `positions_stocks`, `positions_etf`, `positions_fixed_income` | `stocks`, `etf`, `fixed_income`. A class with no position has no section. |
| `profile_header` | perfil `header` | `default` |
| `profile_scale` | perfil `suitability` | `conservador`, `moderado`, `arrojado` |
| `profile_field_list` | perfil `registration` | `default` |
| `preference_list` | perfil `preferences` | `default` |

## Home moments (starting copy)

| Variant | Tone | Kicker | Title | Body | Action |
|---|---|---|---|---|---|
| `portfolio_drop` | neg | Sua carteira hoje | {{first}}, sua carteira caiu {{drop_pct}} hoje | {{product}} recuou {{product_pct}} no dia simulado {{day}}. A {{advisor}} já foi avisada e vai falar com você. | Ver carteira → `navigate carteira` |
| `case_open` | info | Atendimento em andamento | Sua reclamação está com a {{advisor}} | Como cliente {{segment}}, você recebe resposta em até {{sla}}. Meta line: "Protocolo {{protocol}} · aberto há {{age}}" | Ver conversa → `panel message` |
| `segment_upgraded` | gold | Novo segmento | {{first}}, você agora é cliente {{segment}} | Sua assessoria passa a responder em até {{sla}}. | Ver produtos → `navigate investir` |
| `segment_upgrade_near` | gold | Perto do Advance | {{first}}, faltam {{gap}} para o Advance | A partir de US$ 10 mil você vira cliente Advance, com resposta da assessoria em até 4 h. | Depositar → `panel deposit` |
| `idle_cash` | info | Caixa parado | {{first}}, {{cash_share}} do seu patrimônio está em caixa | {{cash}} parados há {{idle_days}} dias. Veja produtos para o seu perfil {{profile}}. | Ver produtos → `navigate investir` |
| `portfolio_review` | neutral | Sua assessora | {{first}}, sua revisão de carteira está disponível | A {{advisor}} separou 30 minutos nesta semana para revisar a carteira com você. | Conversar → `panel message` |
| `welcome` | neutral | Tudo em dia | Olá, {{first}}. Sua conta está em dia. | Quando algo mudar na sua carteira, você vê aqui primeiro. | none |

- As served, `idle_cash` fills `{{idle_days}}` with the unit, "1 dia" or "4 dias", so the body template reads "{{cash}} parados há {{idle_days}}.". When the timeline cannot date the idle cash, the body is "{{cash}} parados em caixa. Veja produtos para o seu perfil {{profile}}."
- As served, the `segment_upgrade_near` body reads the threshold and the SLA from the backend: "A partir de US$ 10.000,00 você vira cliente Advance, com resposta da assessoria em até 4 h."
- As served, the `case_open` meta template is "Protocolo {{protocol}} · aberto {{age}}", where `{{age}}` is the relative time with its own "há" ("há 3 min"), so a case opened under a minute ago reads "aberto agora".

- `wealth_summary` shows total patrimony, cash, and the allocation bar with percentages. `with_day_change` adds the pill "{{signed_delta}} ({{signed_pct}}) no dia {{day}}".
- The eye toggle masks values as "US$ ••••••" in web. This is presentation only.
- `activity_list` `empty` reads "Suas movimentações aparecem aqui assim que acontecerem."

## Investir (hybrid)

- **Server-driven sections:**
  - `cash`, with available cash and the chip "Perfil {{profile}}";
  - `highlights`, "Para o seu perfil {{profile}}", "Escolhidos pelo backend a partir do seu perfil de investidor.";
  - the lists "Renda fixa", "ETFs", and "Ações".
- Each product card shows name, class, five risk bars with "Risco {{n}} de 5", indicative return, minimum, and an "Investir" `panel purchase` action. When the product is above the client's profile, the BFF sets the "Acima do seu perfil" badge. Its bars use the warning color.
- **Coded purchase form** (`Compra-*`):
  - Title: "Investir em {{name}}". It shows class, risk bars, and indicative return, plus "Preço fixo da simulação, sem cotação. Rentabilidade fictícia."
  - The "Quanto investir" field has the chips "US$ 250", "US$ 1.000", and "Tudo", and the line "Disponível no caixa: {{cash}}". The amount is capped at cash, which the BFF sends in cents.
  - Above profile, the form shows the BFF warning: "Este produto tem risco {{risk}}. Seu perfil é {{profile}}, que vai até risco {{max}}. Você pode investir mesmo assim, e a sua assessora será avisada." It does not block.
  - Buttons: "Confirmar compra de {{amount}}" and "Cancelar".
- **Confirmation** ("Compra enviada"):
  - Lead: "{{amount}} em {{name}}. O valor saiu do caixa e já aparece na sua carteira."
  - It shows the protocol (first two groups of `event_id`, uppercased, as in phase 2).
  - Bastidores shows `account.event.recorded · kind aplicacao · schema 3` and the `event_id`. The steps are "Gravado na outbox do account-sim", "Publicado no RabbitMQ", "Regra: compra acima do perfil" (or "Avaliado pelas regras"), and "Na fila da assessoria".
  - Buttons: "Ver na fila do time" goes to `/advisor-radar/fila`, and "Voltar ao início".

## Carteira (100% SDUI)

- `summary`: total patrimony, with the day-change pill when the day moved. It shows the stats "Valor aplicado", "Rentabilidade" (signed, with percentage), "Caixa", and "Dia simulado".
- `allocation`: Ações, ETFs, Renda fixa, and Caixa, each with value, percentage, and bar.
- `positions_*`: one list per class with a subtotal. Each row has product, applied value, current value, and a signed return.
- `history`: "Movimentações", from the timeline. Empty state: "Nenhuma movimentação ainda." A revaluation row reads "Reavaliação diária" with "dia simulado {{day}} · {{product}} {{pct}}".

## Perfil (100% SDUI)

- `header`: initials, name, "Cliente {{segment}} desde {{since}}", and the fictional account number ("Conta 3301-7 · Orla Invest").
- `suitability` is the highlight:
  - three levels, each with label, description, "Produtos até risco {{max}}", and bars;
  - the client's own level is marked;
  - subtitle: "Define os destaques de Investir e quando uma compra recebe aviso.";
  - foot: "Última avaliação em {{date}}. Para refazer o questionário, fale com a sua assessora."
- Level descriptions:
  - Conservador: "Prioriza preservar o patrimônio. Aceita pouca oscilação."
  - Moderado: "Aceita oscilação moderada em busca de mais retorno."
  - Arrojado: "Aceita oscilação alta no curto prazo para buscar retorno maior."
- `registration`: Nome, E-mail (`@example.com`), Telefone (masked), Cidade (city and country), Segmento, Cliente desde.
- `preferences`:
  - "Tema": "Fica salvo só neste navegador". It is local only.
  - "Canal preferido": "Por onde a assessoria fala com você". Chat or E-mail, persisted in account-sim.
  - "Programa beta": off, "Veja antes as novas versões das telas."; on, "Ligado. Você recebe a revision v2 do início antes dos outros clientes." The design said "das telas"; only the home has a `v2`.
- `advisor`: "Sua assessora" or "Sua assessora dedicada", with "Resposta em até {{sla}} · cliente {{segment}}".

## States

- **Loading** (`Home-Carregando`): a skeleton of the section shapes with `role="status"` "Montando sua tela…". The note under it reads "Uma requisição só. Se passar do tempo, a tela mostra o que já chegou e cada seção atrasada cai na variante padrão."
- **Isolated failure** (`Home-Falha-Isolada`): `moment` shows `welcome`, and `activity` is omitted. The real app renders nothing where `activity` was. Raio-X shows it as omitted, and the banner adds "timeline fora do ar".
- **Unsupported schema** (`406`): the screen area shows "Atualize o app para ver esta tela." with a reload button. The simulation strip and tabs stay.
- **Unknown type or component error**: nothing renders in that slot. The error is reported, and the rest of the screen stays.
