"""Media browsing: flat /api/media → a path trie + stream presets."""

from __future__ import annotations

from homeassistant.components.media_player import (
    BrowseError,
    BrowseMedia,
    MediaClass,
    MediaType,
)

from .coordinator import OndaireCoordinator

# media_content_id scheme:
#   "root"          → the two top-level sections
#   "library"       → media library root
#   "dir:<rel>/"    → a library subdirectory
#   "file:<rel>"    → a playable file (also the play URI, sans normalization)
#   "presets"       → the stream-preset section
#   "stream:<id>"   → a playable stream preset
_LIBRARY = "library"
_PRESETS = "presets"


async def async_browse_media(
    coordinator: OndaireCoordinator,
    master: str,
    media_content_id: str | None,
) -> BrowseMedia:
    """Build a BrowseMedia node for the given content id."""
    if not media_content_id or media_content_id == "root":
        return _root()
    if media_content_id == _LIBRARY or media_content_id.startswith("dir:"):
        prefix = "" if media_content_id == _LIBRARY else media_content_id[len("dir:") :]
        if prefix and not prefix.endswith("/"):
            prefix += "/"
        files = await coordinator.async_get_media(master)
        return _library_dir(media_content_id, prefix, [f.path for f in files])
    if media_content_id == _PRESETS:
        return _presets(coordinator)
    raise BrowseError(f"unknown media id: {media_content_id}")


def search_results(files: list[dict]) -> list[BrowseMedia]:
    """Map search-hit MediaFile dicts to playable BrowseMedia items. Title prefers
    the tag title, falling back to the filename; artist is prepended when known."""
    out: list[BrowseMedia] = []
    for f in files:
        path = f.get("path")
        if not isinstance(path, str) or not path:
            continue
        title = f.get("title") or f.get("name") or path
        artist = f.get("artist")
        if artist:
            title = f"{artist} — {title}"
        out.append(
            BrowseMedia(
                media_class=MediaClass.MUSIC,
                media_content_id=f"file:{path}",
                media_content_type=MediaType.MUSIC,
                title=title,
                can_play=True,
                can_expand=False,
            )
        )
    return out


def resolve_play_uri(media_content_id: str) -> str:
    """Normalize a browsed/served id to an ondaire media-source URI.

    file:/stream:/http(s):///spotify/input: pass through; a bare relative path
    folds to file: (matches resolvePlayURI in internal/api/handlers.go).
    """
    cid = media_content_id
    for scheme in ("file:", "stream:", "http://", "https://", "spotify", "input:"):
        if cid.startswith(scheme):
            return cid
    return "file:" + cid


def _root() -> BrowseMedia:
    return BrowseMedia(
        media_class=MediaClass.DIRECTORY,
        media_content_id="root",
        media_content_type="root",
        title="ondaire",
        can_play=False,
        can_expand=True,
        children_media_class=MediaClass.DIRECTORY,
        children=[
            BrowseMedia(
                media_class=MediaClass.DIRECTORY,
                media_content_id=_LIBRARY,
                media_content_type="library",
                title="Library",
                can_play=False,
                can_expand=True,
            ),
            BrowseMedia(
                media_class=MediaClass.DIRECTORY,
                media_content_id=_PRESETS,
                media_content_type="presets",
                title="Stream presets",
                can_play=False,
                can_expand=True,
            ),
        ],
    )


def _library_dir(content_id: str, prefix: str, paths: list[str]) -> BrowseMedia:
    """List immediate children (subdirs + files) of one library directory."""
    subdirs: dict[str, bool] = {}
    files: list[str] = []
    for path in paths:
        if not path.startswith(prefix):
            continue
        rest = path[len(prefix) :]
        head, sep, _ = rest.partition("/")
        if sep:
            subdirs[head] = True  # dedup subdirectory names
        else:
            files.append(path)

    children = [
        BrowseMedia(
            media_class=MediaClass.DIRECTORY,
            media_content_id=f"dir:{prefix}{name}",
            media_content_type="directory",
            title=name,
            can_play=False,
            can_expand=True,
        )
        for name in sorted(subdirs)
    ]
    children += [
        BrowseMedia(
            media_class=MediaClass.MUSIC,
            media_content_id=f"file:{path}",
            media_content_type=MediaType.MUSIC,
            title=path[len(prefix) :],
            can_play=True,
            can_expand=False,
        )
        for path in sorted(files)
    ]

    title = "Library" if content_id == _LIBRARY else prefix.rstrip("/").split("/")[-1]
    return BrowseMedia(
        media_class=MediaClass.DIRECTORY,
        media_content_id=content_id,
        media_content_type="directory",
        title=title,
        can_play=False,
        can_expand=True,
        children_media_class=MediaClass.MUSIC,
        children=children,
    )


def _presets(coordinator: OndaireCoordinator) -> BrowseMedia:
    presets = coordinator.data.stream_presets if coordinator.data else ()
    children = [
        BrowseMedia(
            media_class=MediaClass.MUSIC,
            media_content_id=f"stream:{preset.id}",
            media_content_type=MediaType.MUSIC,
            title=preset.name or preset.url,
            can_play=True,
            can_expand=False,
        )
        for preset in presets
    ]
    return BrowseMedia(
        media_class=MediaClass.DIRECTORY,
        media_content_id=_PRESETS,
        media_content_type="presets",
        title="Stream presets",
        can_play=False,
        can_expand=True,
        children_media_class=MediaClass.MUSIC,
        children=children,
    )
