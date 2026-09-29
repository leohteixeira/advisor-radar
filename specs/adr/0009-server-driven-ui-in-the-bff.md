# ADR 0009 — Server-Driven UI composed in the BFF

- **Status:** accepted
- **Date:** 2026-09-28

## Context

The phase-2 client app hardcodes copy, allocation percentages, currency
formatting, the over-cash withdrawal check, and Bastidores labels in web, which
contradicts the rule that business logic stays in the backend. Every client sees
the same home, so account and message events change nothing on the client side.
Phase 3 needs each client's screen to follow that client's moment, and needs
copy, section order, and variants to change without a web release. The BFF
already aggregates gRPC reads for the frontend and stores nothing.

## Decision

- **The BFF composes screens.** `GET /v1/client-pov/customers/{id}/screens/{slug}`
  returns a page of sections and components, already filled for that client. The
  BFF gathers its inputs over gRPC under one screen deadline and still stores
  nothing.
- **The catalog is embedded, read-only config.** Sections, variants, and copy
  templates live in one versioned file embedded in the BFF with `go:embed`, keyed
  in English. There is no CMS and no runtime write. The `add-sdui-variant` skill
  is the editing tool; a change ships by rebuilding and restarting the BFF.
- **Advisory owns the moment.** Segmentation, investor profile, and client moment
  facts come from advisory over gRPC. Advisory evaluates every moment condition
  (drop, segment upgrade, near upgrade, idle cash, Singular) and returns each one
  as a fact; cases supplies the open-case fact. The BFF never decides a moment,
  compares a threshold, or computes segmentation; it only applies the fixed
  priority order in `specs/http/bff.md` to the facts it receives.
- **Components are semantic domain types.** A component is a type such as
  `moment_card` or `position_list`. There are no `row`, `column`, or `text`
  primitives.
- **Split of control.** The server controls section order (the array order is
  the render order) and point props such as tone and emphasis. Web owns style,
  the breakpoint grid, and layout.
- **Variant selection.** Every section has an ordered list of variants. The first
  variant that matches wins. The last is the default and always matches. A test
  parses and executes every catalog template and checks that every section ends
  in an always-matching default.
- **Failure policy.** A section whose source failed or ran past the screen
  deadline falls back to its default variant when the default does not need that
  source. A section that depends only on the failed source is omitted. A `Build`
  error drops only that component. The response stays `200`.
- **Web renders by `type`.** Web maps each `type` to a component. An unknown
  `type` renders nothing and is reported. A new `type` needs web code and a
  contract entry in `specs/http/bff.md`; a new variant of an existing type needs
  neither.
- **Values arrive ready to display.** Go formats numbers, money
  (`"US$ 48.210,00"`), and dates before they enter a template. A value that a form
  needs as a number is also sent as integer USD cents.
- **Versioning.** Every response carries `schema_version`, `slug`, and
  `revision`. The screen contract, including the `X-SDUI-Schema` header, is in
  `specs/http/bff.md`.

## Consequences

Copy, section order, and variants of existing types change in the BFF alone.
Web stops computing percentages, currency, dates, validation, and SLA. A screen
costs one HTTP request, and one slow source degrades a section instead of the
screen. The catalog only changes through a build, so a content change needs a
BFF restart. Web and BFF must agree on the component table in
`specs/http/bff.md`, and a new component type still needs a coordinated web
release.

## Reassessment trigger

Revisit if content must change without a BFF restart, if a second client
(native app) needs the same screens with a different component set, or if
screens need generic layout primitives.
