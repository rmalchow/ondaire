"""Standalone self-check for the HA-independent model logic.

Not shipped (lives outside custom_components/ondaire). Run: `python3 selfcheck.py`.
Exercises the contracts.go -> dataclass mapping and the Snapshot helpers the
coordinator/entity/config-flow all depend on. No Home Assistant needed.
"""

import sys

sys.path.insert(0, "custom_components/ondaire")

from models import Snapshot  # noqa: E402

# A cluster with two masters (bbbb..., aaaa...) and one playback node following
# the aaaa master. Field names mirror internal/contracts/contracts.go verbatim.
RAW = {
    "nodes": [
        {"id": "b" * 32, "name": "Kitchen", "volume": 0.4, "httpPort": 8080,
         "addrs": ["192.168.1.10/24"], "alive": True, "stale": False,
         "appVersion": "v0.25.2"},
        {"id": "a" * 32, "name": "Living", "volume": 0.8, "httpPort": 8080,
         "addrs": ["192.168.1.11/24"], "alive": True, "following": ""},
        {"id": "c" * 32, "name": "Speaker", "playbackNode": True, "volume": 0.5,
         "alive": True, "following": "a" * 32},
    ],
    "groups": [
        {"id": "b" * 32, "master": "b" * 32, "members": ["b" * 32],
         "playback": {"state": "idle"}},
        {"id": "a" * 32, "master": "a" * 32, "members": ["a" * 32, "c" * 32],
         "playback": {"state": "playing", "uri": "file:x/y.flac",
                      "positionSec": 12.5, "seekable": True,
                      "metadata": {"title": "T", "artist": "A", "hasArt": True,
                                   "durationSec": 200}}},
    ],
    "streamPresets": [{"id": "p1", "name": "Radio", "url": "http://r/s",
                       "hasAuth": True, "authScheme": "basic"}],
}


def main() -> None:
    snap = Snapshot.from_json(RAW)

    # field mapping (JSON tag -> python attr)
    kitchen = snap.node("b" * 32)
    assert kitchen and kitchen.name == "Kitchen" and kitchen.http_port == 8080
    assert kitchen.app_version == "v0.25.2" and not kitchen.playback_node

    # group_of follows membership, not the node's own id
    grp = snap.group_of("c" * 32)
    assert grp and grp.master == "a" * 32, "playback node routes to its group master"
    assert grp.playback.state == "playing" and grp.playback.seekable
    assert grp.playback.position_sec == 12.5
    md = grp.playback.metadata
    assert md and md.title == "T" and md.has_art and md.duration_sec == 200

    # a master finds its own group even though the daemon does NOT list the
    # master in `members` (observed v0.31.1) — regression guard: a playing
    # room read as idle when group_of matched membership only
    solo = Snapshot.from_json({
        "nodes": [{"id": "d" * 32, "name": "Solo", "alive": True}],
        "groups": [{"id": "d" * 32, "master": "d" * 32, "members": [],
                    "playback": {"state": "playing", "uri": "file:z.mp3"}}],
    })
    grp = solo.group_of("d" * 32)
    assert grp and grp.playback.state == "playing", "master must find its own group"

    # masters() excludes the playback-only node; dedup key is the lowest id
    master_ids = {n.id for n in snap.masters()}
    assert master_ids == {"a" * 32, "b" * 32}, master_ids
    assert snap.smallest_master_id() == "a" * 32

    # capabilities.playback -> playback_capable (independent of playback_node).
    # A dual-role node (a master that also plays, e.g. "study") reports
    # capabilities.playback=True while a room-only master reports False.
    caps = Snapshot.from_json({
        "nodes": [
            {"id": "e" * 32, "name": "Study", "alive": True,
             "capabilities": {"playback": True}},          # room + player
            {"id": "f" * 32, "name": "Hallway", "alive": True,
             "capabilities": {"playback": False}},          # room-only master
            {"id": "g" * 32, "name": "Satellite", "alive": True,
             "playbackNode": True, "capabilities": {"playback": True}},
            {"id": "h" * 32, "name": "Legacy", "alive": True},  # no capabilities key
        ],
        "groups": [],
    })
    study, hallway, sat, legacy = (caps.node(c * 32) for c in "efgh")
    assert study.playback_capable and not study.playback_node, "dual-role node is playback-capable"
    assert not hallway.playback_capable, "room-only master is not playback-capable"
    assert sat.playback_capable and sat.playback_node, "satellite is both"
    assert not legacy.playback_capable, "missing capabilities defaults to False"

    # presets carry the secret-free auth signal only
    assert snap.stream_presets[0].has_auth and snap.stream_presets[0].auth_scheme == "basic"

    print("selfcheck OK")


if __name__ == "__main__":
    main()
