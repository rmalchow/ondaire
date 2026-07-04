# Home Assistant brand icons

Home Assistant does **not** load an integration's logo from its `custom_components/`
folder. The integrations list, the "Add integration" dialog, and the config-flow
header all fetch brand images from `brands.home-assistant.io`, which is populated
from the [`home-assistant/brands`](https://github.com/home-assistant/brands) repo.
Until the icon is merged there, ondaire shows the generic puzzle-piece placeholder
in the HA UI — the integration itself works regardless.

## The assets

`custom_integrations/ondaire/` here mirrors the exact layout the brands repo
expects for a **custom** integration:

| File | Size | Source |
|---|---|---|
| `icon.png` | 256×256 | ink "o" badge on mint (`branding/ondaire-mark-mint-dark.svg`) |
| `icon@2x.png` | 512×512 | same, 2× |

Regenerate with `branding/generate.sh` (it re-renders and re-strips these).

## Submitting to home-assistant/brands

1. Fork and clone `home-assistant/brands`.
2. Copy this folder in: `cp -r custom_integrations/ondaire <brands>/custom_integrations/`.
3. From the brands repo, run its checker: `python3 -m script.hassfest` (or the
   `pytest` in that repo) — it verifies square dimensions, exact 2× ratio,
   trimming, and file size.
4. Open a PR. Once merged, HA fetches the icon automatically (frontend caches it,
   so allow a little time / a hard refresh).

The integration's `manifest.json` domain (`ondaire`) already matches the folder
name here, which is what the brands lookup keys on.
