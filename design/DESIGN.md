# Ensemble — Design System Brief

> Distilled from the Ensemble Svelte codebase (`web/` control app + `site/` marketing site).
> This is the design contract for the `ensemble-design` Open Design project. It captures the
> existing visual language so generated artifacts match the product, not a generic theme.

## Product

**Ensemble** is a self-hosted, multi-room / whole-home audio controller (Sonos-like): you
group rooms, see live nodes/devices, browse media, control playback and per-member volume,
and wire up Spotify endpoints. Two surfaces share one identity:

- **Control app** (`web/`, Svelte) — dense, operational, dark. The thing you actually drive.
- **Marketing site** (`site/`) — editorial, serif-led, also dark.

Brand mark: the **"ensemble" wordmark set in Fraunces**, followed by a single **accent dot with a mint glow**.

---

## Color

Shared accent across both surfaces: **`#35e3b3`** (mint/teal). Both surfaces are dark; the site
runs a touch darker and adds a second cyan accent.

### Control app (`web/src/app.css`)
| Token | Value | Role |
|------|-------|------|
| `--bg` | `#11151a` | app background |
| `--panel` | `#1a212b` | card / pill surface |
| `--panel-2` | `#222b37` | inputs, buttons, raised surface |
| `--fg` | `#e6edf3` | primary text |
| `--muted` | `#8b97a7` | secondary text, labels |
| `--accent` | `#35e3b3` | primary accent, focus, links |
| `--danger` | `#ff6b6b` | destructive, offline |
| `--ok` | `#4ade80` | healthy / connected |
| `--badge` | `#2d3a4d` | badge fill |
| `--border` | `#2a3340` | hairlines, outlines |
| warn | `#f59e0b` | reconnecting / degraded |

### Marketing site (`site/src/assets/styles.css`)
| Token | Value | Role |
|------|-------|------|
| `--bg` / `--bg-2` | `#0a0c10` / `#0e131a` | page / alt section |
| `--panel` | `#11161f` | cards |
| `--ink` | `#e9eef4` | primary text |
| `--muted` / `--faint` | `#8b95a6` / `#5d6675` | secondary / tertiary |
| `--line` | `rgba(255,255,255,.08)` | hairlines |
| `--accent` | `#35e3b3` | primary accent |
| `--accent-2` | `#5ad1ff` | secondary cyan accent |
| `--accent-ink` | `#03130d` | text on accent fills |

---

## Typography

- **Control app:** native `system-ui` stack (`-apple-system, Segoe UI, Roboto, Helvetica, Arial`),
  **14px** base, line-height **1.45**, headings weight **600**. Monospace (`ui-monospace, SFMono-Regular, Menlo`)
  for node IDs and numeric stats; `font-variant-numeric: tabular-nums` for percentages.
- **Marketing site:** self-hosted trio —
  - **Fraunces** (`--serif`) — display, headings, wordmark; soft weights (~420).
  - **IBM Plex Sans** (`--sans`) — body copy.
  - **IBM Plex Mono** (`--mono`) — code, flags, params, labels, eyebrows.

Section headers in the app are **uppercase, letter-spacing `0.05em`, muted color, 15px**.

---

## Layout, spacing & radius

- Containers: app `max-width: 920px`; site `max-width: 1120px`; both centered, dark.
- Radius ladder: **4** (badge) · **6** (button, input, toast) · **8** (card) · **10** (chip) · **999** (pill).
- Spacing rhythm: gaps of **6 / 8 / 10 / 12px**; card padding **14×16px**; section spacing **22px**.
- Hairline-driven structure: 1px `--border` separators rather than heavy shadows.

---

## Components (inventory from the live app)

- **Buttons** `.btn` — `panel-2` fill, border, 6px radius, 13px. Variants: `.btn-accent`, `.btn-danger`,
  `:disabled` (40% opacity). Hover lifts the border to accent.
- **Badge** `.badge` — solid `--badge` chip, 11px, 600 weight.
- **Chip** `.chip` — outlined, muted text, 10px radius (metadata tags).
- **Status dot** `.dot` — 8px circle; `.alive/.ok` green, `.dead/.closed` red, `.reconnecting/.connecting` amber.
- **Status pill** `.status-pill` — node-name pill with a leading state dot and **colored glow border**
  (`.good` / `.warn` / `.bad`) via `color-mix`.
- **Card** `.card` — `--panel` surface, 1px border, 8px radius. The primary container.
- **GroupCard** — selectable room card; selected/hover state uses the **signature accent glow**.
- **MemberRow / NodeRow / NodeSwitcher** — device rows with mono IDs and inline stats.
- **PlaybackBar** — transport + now-playing.
- **VolumeSlider** — native `range`, `accent-color: var(--accent)`, with a tabular-nums `%` readout.
- **MediaBrowser** — file/folder rows + clickable breadcrumb (`.crumbs`), folder rows reuse `.media-file`.
- **EditableText** — inline edit affordance: dashed underline on hover → accent-bordered input.
- **Toast** — bottom-right stack, left border colored by severity (`--danger` default, `--ok` success), soft shadow.
- **Inputs** — text/number/select on `panel-2`, accent focus; SpotifyEndpoints form.
- **Banners** — `UnreachableBanner` for offline/degraded state.

---

## Signature treatments (keep these — they *are* the brand)

1. **Accent glow.** Selected rooms and hovered icon-links get a layered mint glow:
   `box-shadow: 0 0 0 3px color-mix(accent 16%), 0 0 22px -6px color-mix(accent 45%)`.
2. **`color-mix(in srgb, …)`** for all translucent borders and state tints — never flat opacity hacks.
3. **Status as color.** Connection health is always a green / amber / red dot or pill, never text alone.
4. **Tabular numerics** for volumes, stats, percentages.
5. **Hairlines over shadows** in the app; **serif editorial restraint** on the site.

---

## When generating new UI

- Default to the **dark control-app palette**; reach for the site palette/fonts only for marketing/editorial.
- Use existing tokens by name; do not introduce new hues. The only accents are mint `#35e3b3` and (site) cyan `#5ad1ff`.
- Match density: compact 14px, generous hairlines, 6–8px radii. Avoid rounded-2xl card soup.
- Any connection/health state must use the dot/pill semantics above.
