---
title: 'Add desktop layout, light theme, and free message'
type: 'feature'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: '897acd3e145febe1b8a1a512c4375cfd383fea96'
followup_review_recommended: false
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:** The phone app replaces the home for every action, stays dark, and cannot send a free message.

**Approach:** From 900px up, actions open a 440px panel with Fechar and a scrim. The theme control switches between Tema claro and Tema escuro, dark by default. A free message can go by chat or e-mail.

## Boundaries & Constraints

**Always:** Send stays disabled while the text is empty. The empty thread reads "Ainda não há mensagens. Escreva a primeira." The panel is 440px. Light theme uses the light surface.

**Never:** Live tracing, an import from docs, or a change to the team queue.

</intent-contract>

## Auto Run Result

Status: done

Summary: Desktop actions use the 440px panel and Fechar. The theme button toggles Tema claro and Tema escuro. A free e-mail message posts to `/messages` and confirms with "Mensagem enviada".

Verification: `pnpm test` passed (41 tests) and `tsc --noEmit` passed. In the browser the client route still shows the load error because the BFF is down, so the panel and the theme were exercised in the test, not on the live page.
