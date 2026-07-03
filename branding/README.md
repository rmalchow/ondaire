# ensemble branding

The ensemble mark is the lowercase **"e" from [Fraunces](https://fonts.google.com/specimen/Fraunces)**
(weight 900 / Black) — the same display serif the marketing site uses. The glyph
is extracted straight from the site's own subsetted font so the logo and the site
are pixel-identical.

## Palette

| Role | Hex |
|---|---|
| Ink (near-black) | `#11151a` |
| Mint (accent) | `#35e3b3` |
| White | `#ffffff` |

## Variants

| File | What | Use |
|---|---|---|
| `ensemble-icon-mint-black.svg` | ink "e" on a mint rounded square | **primary** — favicon, HA icon, app icon. Best contrast at small sizes. |
| `ensemble-icon-mint-white.svg` | white "e" on a mint rounded square | soft alternate |
| `ensemble-icon-mint-black-square.svg` | ink "e" on a full-bleed mint square | apple-touch / PWA maskable (the OS applies its own rounding) |
| `ensemble-e-outline.svg` | white "e" with an ink keyline, transparent | large / standalone; the keyline lets it sit on any background |
| `ensemble-wordmark.svg` | e-badge + "ensemble" (Fraunces 600) + mint dot, ink text | horizontal lockup for **light** backgrounds; HA `logo.png` |
| `ensemble-wordmark-inverse.svg` | same lockup, light text | **dark** backgrounds; HA `dark_logo.png`; the site header |

`png/` holds the three marks at 128 / 256 / 512 / 1024 and the wordmarks at 160 / 320 / 640px tall.

## Regenerating

```sh
./generate.sh
```

Requires `python3` + `fonttools`, `inkscape`, and `imagemagick`. It:

1. re-extracts the glyph and writes the master SVGs (`extract_e.py`);
2. rasterizes `png/`;
3. distributes the favicon set into `site/src/assets/` (`favicon.svg`,
   `favicon.ico`, `apple-touch-icon.png`, `icon-192/512.png`) — wired into every
   page's `<head>` from `site/build.mjs`;
4. writes the Home Assistant brand icons into
   `integrations/homeassistant/brands/custom_integrations/ensemble/` — see that
   folder's README for how they reach the HA UI.
