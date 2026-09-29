# Advisor Radar — Server-Driven UI in the client app

Design canvas for phase 3: the client app as Server-Driven UI (SDUI), the functional Investir,
Carteira and Perfil screens, and the SDUI highlight on the selection screen. Source canvas:
<https://claude.ai/artifact/4nqsAgu5BEhLGWCorF7qt4>. Brief:
`/workspace/docs/advisor-radar/advisor_radar_sdui_brief.md`.

Each `project/*.dc.html` file is one artboard; `project/canvas.json` holds the layout. UI copy is
Portuguese; data, names, products and branding are fictional. Values are illustrative; the
backend owns every number, percentage, warning and variant choice.

## Artboards

| Artboard | What it shows |
|---|---|
| `Main.dc.html` | Selection screen (desktop): "Novo" band, anchor links, a fifth pillar, and a new interactive `#sdui` section after `#arquitetura` (client tabs, the bff JSON, the rendered home, the five composition steps and four principles) |
| `Selecao-SDUI-mobile.dc.html` | The same band and SDUI section at 390 px |
| `OrlaApp.dc.html` | The single interactive client app. It renders every screen from a list of sections (`id`, `type`, `variant`), the way the real renderer will. Every other `ClientApp` artboard imports it with fixed props |
| `Home-*.dc.html` | The three homes (`segment_upgrade_near`, `idle_cash`, `portfolio_review`), live changes (`case_open`, `portfolio_drop` drawn on day 12; the shipped shock is day 3), X-ray views, loading, and isolated failure |
| `Investir-*.dc.html` | Hybrid screen: server-driven catalog and highlights per investor profile |
| `Compra-*.dc.html` | Coded purchase form with the backend suitability warning, then the confirmation with "Bastidores" |
| `Carteira-*.dc.html` | Individual positions, allocation, day change and history on simulated day 12 |
| `Perfil-*.dc.html` | Investor profile scale (highlight), registration data, advisor, preferences, beta program |
| `Catalogo.dc.html` | Every SDUI component type with its section, type and variant label |

## Component types

`moment_card`, `wealth_summary`, `action_grid`, `advisor_card`, `activity_list`,
`invest_summary`, `product_rail`, `product_list`, `portfolio_summary`, `allocation_breakdown`,
`position_list`, `profile_header`, `profile_scale`, `profile_field_list`, `preference_list`.

A new type needs frontend code. A new variant of an existing type does not.

## Moment priority (home `moment` section)

1. `portfolio_drop`: day loss above 15% of patrimony.
2. `case_open`: an open case for the client.
3. `segment_upgraded`: advisory recorded a segment upgrade in the last 24 h.
4. `segment_upgrade_near`: patrimony between US$ 7,500 and US$ 10,000.
5. `idle_cash`: cash at or above 50% of patrimony.
6. `portfolio_review`: Singular client.
7. `welcome`: default; also used when the moments source fails.

## Simulation strip

The strip adds "Dia simulado N" with "Avançar um dia" (global market day, brief decision 19) and
a "Raio-X SDUI" toggle that outlines each section with its `id · type · variant`. The X-ray
toggle is a demo tool (brief decision 20).

## Open points for implementation

- Mariana's scripted shock is drawn on day 12. The shipped calibration (brief decision 23) is
  Cobalto −53.5% on simulated day 3, with no variation on other days: Mariana −15.5%, Thiago
  about −1.6%.
- After Fernanda's USD 10,000 deposit, the home shows `segment_upgraded` (brief decision 22),
  which has no artboard. Its starting copy is in the spec's `ux.md`.
- The beta revision is home `v2`, which adds the profile highlights (`product_rail`) after
  `moment` (brief decision 24). The Perfil beta text reads "revision v2 do início".
- Loading and isolated-failure copy is a proposal; the failure artboard shows the omitted section
  only in X-ray mode, because the real app renders nothing there.
