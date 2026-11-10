# Content Factory — Design System (Master)

Global source of truth for the web UI, following the `ui-ux-pro-max` skill's
Master + page-override pattern. Page files in `pages/` override this file for
that page only.

**Provenance.** Python was not available, so the skill's `--design-system`
script was not run. Every decision below comes from rows read directly from the
skill's data files (`.claude/skills/ui-ux-pro-max/data/`), cited inline.
Deliberate deviations are labeled.

## Product analysis

| Aspect | Decision | Source |
| --- | --- | --- |
| Product type | Internal SaaS operations tool: channel portfolio, AI generation jobs, measured trend data | `products.csv` #1 SaaS (General), #188 RPA / Automation Dashboard, #7 Analytics Dashboard |
| Audience / context | One operator at a desktop, long working sessions, frequent status checks | — |
| Stack | React 18 + TypeScript + Vite, plain CSS tokens (no Tailwind) | `web/package.json` |

## Style

- **Primary style: Minimalism & Swiss Style** (`styles.csv` #1). `ui-reasoning.csv` #1 (SaaS) routes UX-focused products to it. It's best for "enterprise apps, dashboards, SaaS platforms, professional tools".
- **Data surfaces: Data-Dense Dashboard** (`styles.csv` #28) for tables and metric grids: compact rows, sticky headers, sortable/filterable data.
- **Interaction feel: Flat + micro-interactions** (`products.csv` #16): quick feedback in 150–250 ms.
- **Avoid** (`ui-reasoning.csv` #1, #188): excessive animation, dark mode *by default*, decorative charts, hidden error states, slow dashboards.
- **Deviation:** Swiss style specifies `--border-radius: 0`. We use a 6 px radius on controls and 8 px on cards, so hit areas read clearly as interactive.

## Color tokens

Light theme from `colors.csv` #1 (SaaS General). Dark theme from `colors.csv` #188, with tonal (lighter, desaturated) variants per `quick-reference.md` §6 `color-dark-mode`. Status colors follow #188: running/success green, failed red, queued amber, completed/info blue.

| Token | Light | Dark |
| --- | --- | --- |
| `--color-bg` | `#F8FAFC` | `#020617` |
| `--color-surface` | `#FFFFFF` | `#0E1223` |
| `--color-surface-2` | `#F1F5F9` | `#1A1E2F` |
| `--color-text` | `#1E293B` | `#F8FAFC` |
| `--color-text-muted` | `#475569` | `#94A3B8` |
| `--color-border` | `#E2E8F0` | `#334155` |
| `--color-control-border` | `#64748B` | `#64748B` |
| `--color-primary` / `--color-on-primary` | `#2563EB` / `#FFFFFF` | `#60A5FA` / `#020617` |
| `--color-primary-text` (links) | `#1D4ED8` | `#93C5FD` |
| `--color-danger` / `--color-on-danger` | `#DC2626` / `#FFFFFF` | `#F87171` / `#020617` |
| `--color-focus` | `#2563EB` | `#93C5FD` |
| success fg / bg | `#166534` / `#DCFCE7` | `#86EFAC` / `#052E16` |
| warning fg / bg | `#92400E` / `#FEF3C7` | `#FCD34D` / `#3A2A05` |
| danger fg / bg | `#991B1B` / `#FEE2E2` | `#FCA5A5` / `#3B0D0D` |
| info fg / bg | `#1E40AF` / `#DBEAFE` | `#BFDBFE` / `#172554` |
| neutral fg / bg | `#334155` / `#F1F5F9` | `#CBD5E1` / `#1E293B` |

All text pairs were measured at ≥ 4.5:1, and control borders and focus rings at ≥ 3:1, in both themes (lowest: dark control border 3.91:1). Single accent only: the SaaS palette's orange CTA accent is intentionally unused, per Swiss style (`--accent-color: single primary only`).

## Typography

- **Pairing: "Minimal Swiss" — Inter / Inter** (`typography.csv` #5). Self-hosted via `@fontsource-variable/inter` (SIL OFL 1.1), so no third-party font request.
- Scale: 12 / 14 / 16 / 18 / 22 / 28 px. Body 16 px, line height 1.5 (`font-scale`, `line-height`).
- Weights: 400 body, 500 labels, 600 headings/buttons, 700 page titles (`weight-hierarchy`).
- `font-variant-numeric: tabular-nums` for metrics, scores, dates, and tables (`number-tabular`).
- Nothing smaller than 12 px; prose limited to about 70 characters per line (`line-length`).

## Spacing, layout, elevation

- 4 px base scale: 4, 8, 12, 16, 24, 32, 48 (`spacing-scale`). Dashboard density sits at the dense end (8–32 px).
- App shell (`adaptive-navigation`): a persistent left sidebar at ≥ 1024 px, a top bar below that. Content max width 1200 px.
- Breakpoints: 640 / 1024 / 1440 px. No horizontal page scroll; wide tables scroll inside their own container.
- Elevation: borders instead of shadows (Swiss). One overlay shadow is used only for dialogs and toasts.
- z-index scale: base 0, sticky 10, sidebar 20, toast 40, dialog 50.

## Iconography

- **Lucide** (`lucide-react`, ISC license), outline only, 2 px stroke, 16 px inline / 20 px navigation (`icon-style-consistent`, `no-emoji-icons`).
- Icons beside visible text are `aria-hidden`. Icon-only controls require an `aria-label` (`icon-context`, `aria-labels`).
- Never use text glyphs or emoji (✓ ○ ⚙) as structural icons.

## Motion

- Tokens: `--duration-fast: 150ms`, `--duration-base: 200ms`, `--ease-out: cubic-bezier(0.2, 0, 0, 1)`. Exits are about 30% faster than entries (`exit-faster-than-enter`).
- Animate only `opacity` and `transform` (`transform-performance`).
- `prefers-reduced-motion: reduce` disables transitions and spinners rotate slower (`reduced-motion`).

## Components and patterns

| Pattern | Rule | Source |
| --- | --- | --- |
| Buttons | One primary action per view; secondary/ghost subordinate; danger visually separated; disabled during async with an inline spinner; min height 40 px (44 px on touch) | `primary-action`, `loading-buttons`, `destructive-emphasis`, `touch-target-size` |
| Confirmation | Accessible dialog (focus trap, Esc, focus return) for destructive or external actions; never `window.confirm` | `confirmation-dialogs`, `modal-escape` |
| Feedback | Polite live-region toasts for success (auto-dismiss 4 s, never steal focus); errors inline with a recovery action | `toast-accessibility`, `success-feedback`, `error-recovery` |
| Loading | Skeletons for content that takes > 300 ms; spinners only inside buttons/progress; reserve space to avoid layout shift | `progressive-loading`, `content-jumping` |
| Empty states | Explain what is missing and offer the next action | `empty-states` |
| Status | Badge = icon + text + color, never color alone | `color-not-only` |
| Multi-step | Stepper with "Step n of 6", completed/current states, free back navigation | `multi-step-progress` |
| Navigation | Icon + label nav items, clear active state, skip link, focus moves to main content on route change, breadcrumb inside channel setup | `nav-label-icon`, `nav-state-active`, `skip-links`, `focus-on-route-change`, `breadcrumb-web` |
| AI content | Every generated block is labeled "AI-generated" with model and time; measured data is labeled separately from estimates | `ux-guidelines.csv` AI Interaction / Disclaimer |
| Forms | Visible labels, helper text for non-obvious fields, errors below the field via `aria-describedby` | `input-labels`, `input-helper-text`, `error-placement` |

## Pre-delivery checks (web)

From `quick-reference.md` §1–§3 and `pro-rules.md` (web-applicable items): contrast in both themes, visible focus on every control, keyboard-only pass, 375 px width without horizontal scroll, reduced motion, no emoji icons, consistent icon family, semantic tokens only (no raw hex in components).
