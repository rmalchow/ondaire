# ondaire branding

The ondaire mark is a mint rounded-square badge containing a bespoke lowercase
**"o" glyph**, paired with an "ndaire" wordmark set in
[Fraunces](https://fonts.google.com/specimen/Fraunces) — the same display serif
the marketing site and the web UI header use. The badge + glyph + wordmark path
data is hand-tuned and lives directly in the SVGs below; there is no font
extraction step to rerun if you touch the mark.

## Palette

| Role | Hex |
|---|---|
| Ink (near-black) | `#11151a` |
| Mint (accent) | `#35e3b3` |
| White | `#ffffff` |

## Variants

| File | What | Use |
|---|---|---|
| `ondaire-mark-mint-dark.svg` | ink "o" on a mint rounded square | **primary** — favicon, HA icon, app icon. Best contrast at small sizes. |
| `ondaire-mark-mint-white.svg` | white "o" on a mint rounded square | soft alternate |
| `ondaire-mark-mint-square.svg` | ink "o" on a full-bleed mint square | apple-touch / PWA maskable (the OS applies its own rounding) |
| `ondaire-lockup-light.svg` | mark + "ndaire" wordmark, ink text | horizontal lockup for **light** backgrounds; HA `logo.png` |
| `ondaire-lockup-dark.svg` | same lockup, white/light text | **dark** backgrounds; HA `dark_logo.png`; the web UI header and site nav use this geometry inline |

`png/` holds the marks at 128 / 256 / 512 / 1024 and the lockups at 160 / 320 / 640px tall.

## Regenerating

```sh
./generate.sh
```

Requires `inkscape` and `imagemagick`. It:

1. rasterizes `png/` from the mark SVGs;
2. distributes the favicon set into `site/src/assets/` (`favicon.svg`,
   `favicon.ico`, `apple-touch-icon.png`, `icon-192/512.png`) — wired into every
   page's `<head>` from `site/build.mjs`;
3. writes the Home Assistant brand icons into
   `integrations/homeassistant/brands/custom_integrations/ondaire/` — see that
   folder's README for how they reach the HA UI.
