"""Downscale proxied cover art.

Cover bytes are pulled through HA (media_player.async_get_media_image) so the
frontend never has to reach the ondaire host or an art CDN. Those originals can
be large — Spotify serves 1000px+, embedded FLAC art is often full-resolution —
so we downscale them to a small JPEG thumbnail before handing them on.

This is BLOCKING (PIL decode) — call it from the executor
(hass.async_add_executor_job), never directly on the event loop.
"""

from __future__ import annotations

import logging
from io import BytesIO

try:
    from PIL import Image, UnidentifiedImageError
except ImportError:  # pragma: no cover - Pillow ships with HA core
    Image = None  # type: ignore[assignment]
    UnidentifiedImageError = Exception  # type: ignore[assignment,misc]

_LOGGER = logging.getLogger(__name__)

MAX_DIM = 128  # px — longest edge of the thumbnail
JPEG_QUALITY = 85


def scale_cover(raw: bytes) -> bytes | None:
    """Downscale `raw` to a JPEG thumbnail and return the encoded bytes. Returns
    None if scaling isn't possible (no Pillow, undecodable bytes) so the caller
    can fall back to serving the original."""
    if Image is None:
        return None
    try:
        img = Image.open(BytesIO(raw))
        img.draft("RGB", (MAX_DIM, MAX_DIM))  # cheap pre-scale on JPEG decode
        img = img.convert("RGB")  # drop alpha/palette so JPEG can encode it
        img.thumbnail((MAX_DIM, MAX_DIM))  # in place, aspect-preserving, shrink-only
        buf = BytesIO()
        img.save(buf, format="JPEG", quality=JPEG_QUALITY)
        return buf.getvalue()
    except (UnidentifiedImageError, OSError, ValueError) as err:
        _LOGGER.debug("cover scale failed: %s", err)
        return None
