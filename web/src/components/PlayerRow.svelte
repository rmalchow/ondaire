<script>
  // One player in a room card's roster (experiment). The roster lists every
  // playback-capable node in a STABLE alphabetical order with a UNIFORM footprint;
  // a node never moves or resizes when its state changes. The leading switch
  // toggles this node's "following" status for THIS room:
  //   - active  (a member here)      → switch on; right side shows its volume.
  //   - inactive (idle / elsewhere)  → switch off; right side shows where it is.
  // Clicking the switch flips this node's player: follow this room, or leave → idle.
  // The master is NOT locked — it sources the room regardless, but its OWN player
  // can independently play the room (follow self → "play own group", which the
  // server allows) or sit idle. So the master toggles like any other player.
  import { assignToGroup, leaveGroup, nodeSetVolume } from "../lib/api.js";
  import { playerZone } from "../lib/derive.js";
  import VolumeSlider from "./VolumeSlider.svelte";

  let { node, group, self, snapshot } = $props();

  let isMaster = $derived(node.id === group.master);
  let active = $derived((group.members || []).includes(node.id));
  let zone = $derived(playerZone(snapshot, node));

  function toggle() {
    // active → leave (idle); inactive → follow this room (for the master,
    // group.master === node.id, i.e. follow self = play its own room).
    if (active) leaveGroup(node).catch(() => {});
    else assignToGroup(node, group.master).catch(() => {});
  }
</script>

<div class="prow" class:active class:dim={!active}>
  <button
    class="sw"
    class:on={active}
    role="switch"
    aria-checked={active}
    onclick={toggle}
    title={active
      ? `${node.name} is playing this room — click to stop (→ idle)`
      : zone === "idle"
        ? `${node.name} is idle — click to play this room`
        : `${node.name} is following ${zone} — click to move it here`}
    aria-label="play this room on {node.name}"
  >
    <span class="knob"></span>
  </button>

  <span class="pname-wrap">
    <span class="dot {node.alive ? 'alive' : 'dead'}"></span>
    <span class="pname" title={node.name}>{node.name || "(unnamed)"}</span>
    {#if isMaster}<span class="tag master">master</span>{/if}
    {#if node.id === self.id}<span class="tag">this node</span>{/if}
  </span>

  <span class="pright">
    {#if active}
      <VolumeSlider value={node.volume} onchange={(v) => nodeSetVolume(node, v)} />
    {:else}
      <span class="pstate">
        {#if zone === "idle"}idle{:else}following <strong>{zone}</strong>{/if}
      </span>
    {/if}
  </span>
</div>

<style>
  /* fixed 3-column grid so every row aligns and keeps the same footprint whether
     it shows a volume slider or a state label. */
  .prow {
    display: grid;
    grid-template-columns: auto 1fr minmax(11rem, auto);
    align-items: center;
    gap: 10px;
    min-height: 38px;
    padding: 4px 0;
    border-top: 1px solid var(--border);
  }
  .prow:first-child {
    border-top: none;
  }
  .prow.dim {
    color: var(--muted);
  }
  .prow.dim .pname {
    color: var(--muted);
  }

  .pname-wrap {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .pname {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--fg);
  }

  /* consistent micro-tags (master / this node) — clean, subtle, same shape;
     master is accent-tinted to read as meaningful, the rest stay quiet. */
  .tag {
    flex: 0 0 auto;
    font-size: 9px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 1px 6px;
    border-radius: 4px;
    border: 1px solid var(--border);
    background: color-mix(in srgb, var(--fg) 6%, transparent);
    color: var(--muted);
  }
  .tag.master {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 40%, transparent);
    background: color-mix(in srgb, var(--accent) 12%, transparent);
  }

  .pright {
    justify-self: end;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    min-width: 0;
  }
  .pstate {
    font-size: 12px;
    color: var(--muted);
  }
  .pstate strong {
    color: var(--fg);
    font-weight: 600;
  }

  /* the toggle switch: a pill track + sliding knob, reflecting following state. */
  .sw {
    flex: 0 0 auto;
    width: 30px;
    height: 17px;
    padding: 0;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: color-mix(in srgb, var(--panel-2) 70%, transparent);
    position: relative;
    cursor: pointer;
    transition: background 0.16s ease, border-color 0.16s ease;
  }
  .sw .knob {
    position: absolute;
    top: 1px;
    left: 1px;
    width: 13px;
    height: 13px;
    border-radius: 50%;
    background: var(--muted);
    transition: transform 0.18s ease, background 0.16s ease;
  }
  .sw:not(.on):hover {
    border-color: color-mix(in srgb, var(--accent) 55%, var(--border));
  }
  .sw:not(.on):hover .knob {
    background: var(--accent);
  }
  .sw.on {
    background: var(--accent);
    border-color: var(--accent);
  }
  .sw.on .knob {
    transform: translateX(13px);
    background: var(--accent-ink);
  }
</style>
