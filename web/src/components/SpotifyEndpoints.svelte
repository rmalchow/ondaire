<script>
  // Spotify Connect endpoints editor (D57), shown on a node that runs go-librespot.
  // The DEFAULT device ("ensemble <node>") is implicit + read-only (legacy
  // behavior). Below it, the operator manages named PRESETS: each is a name + a
  // row of toggleable players (speakers). Playing to a preset's Connect device
  // regroups those players and plays. All edits persist via PATCH /node, which
  // reconciles the live bridges (start/stop/rename) on the target node.
  import EditableText from "./EditableText.svelte";
  import { setSpotifyEndpoints } from "../lib/api.js";

  let { node, snapshot } = $props();

  // Local editable copy — source of truth while the editor is open. Seeded ONCE
  // from the node record (guarded like MediaBrowser) so snapshot ticks never clobber
  // an in-progress edit; ids are client-generated + stable so the backend keeps them.
  let seeded = false;
  let endpoints = $state([]);
  $effect(() => {
    if (seeded) return;
    seeded = true;
    endpoints = (node.spotifyEndpoints ?? []).map((e) => ({
      id: e.id,
      name: e.name,
      players: [...(e.players ?? [])],
    }));
  });

  // Player candidates: alive, playback-capable nodes (the speakers).
  let players = $derived(
    (snapshot?.nodes ?? [])
      .filter((n) => n.alive && n.capabilities && n.capabilities.playback)
      .sort((a, b) => (a.name || "").localeCompare(b.name || "")),
  );

  let baseName = $derived("ensemble " + (node.name || "node"));

  function save() {
    setSpotifyEndpoints(node.id, endpoints).catch(() => {});
  }
  function newId() {
    // stable slug the backend keeps as-is (normalize lowercases + keeps [a-z0-9-]).
    const r =
      (globalThis.crypto && crypto.randomUUID && crypto.randomUUID()) ||
      Math.random().toString(16).slice(2);
    return "ep-" + r.replace(/-/g, "").slice(0, 8);
  }

  function addEndpoint() {
    endpoints = [...endpoints, { id: newId(), name: "New endpoint", players: [] }];
    save();
  }
  function removeEndpoint(i) {
    endpoints = endpoints.filter((_, j) => j !== i);
    save();
  }
  // Immutable updates (reassign the $state array) so the keyed {#each} item — and
  // its EditableText value prop — reliably re-render after an edit.
  function renameEndpoint(i, name) {
    endpoints = endpoints.map((e, j) => (j === i ? { ...e, name } : e));
    save();
  }
  function togglePlayer(i, pid) {
    endpoints = endpoints.map((e, j) => {
      if (j !== i) return e;
      const players = e.players.includes(pid)
        ? e.players.filter((x) => x !== pid)
        : [...e.players, pid];
      return { ...e, players };
    });
    save();
  }
</script>

<div class="spotify-endpoints">
  <div class="ep">
    <div class="ep-head">
      <span class="ep-name">{baseName}</span>
    </div>
    <span class="muted small">default · plays this node's current group</span>
  </div>

  {#each endpoints as ep, i (ep.id)}
    <div class="ep">
      <div class="ep-head">
        <span class="ep-name">{baseName}:</span>
        <EditableText value={ep.name} onsave={(v) => renameEndpoint(i, v)} placeholder="endpoint name" />
        <span class="spacer"></span>
        <button class="btn btn-danger ep-remove" title="remove endpoint" aria-label="remove endpoint" onclick={() => removeEndpoint(i)}>
          <svg width="10" height="10" viewBox="0 0 10 10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" fill="none" aria-hidden="true"><line x1="2" y1="2" x2="8" y2="8" /><line x1="8" y1="2" x2="2" y2="8" /></svg>
        </button>
      </div>
      {#if players.length === 0}
        <span class="muted small">No playback-capable nodes yet.</span>
      {:else}
        <div class="row wrap players">
          {#each players as p (p.id)}
            {@const on = ep.players.includes(p.id)}
            <button
              type="button"
              class="chip player"
              class:on
              aria-pressed={on}
              title={on ? `remove ${p.name}` : `add ${p.name}`}
              onclick={() => togglePlayer(i, p.id)}
            >
              <span class="glyph" aria-hidden="true">{on ? "●" : "○"}</span>{p.name}
            </button>
          {/each}
        </div>
      {/if}
    </div>
  {/each}

  <button class="btn add-ep" onclick={addEndpoint}>+ Add endpoint</button>
</div>

<style>
  .spotify-endpoints {
    display: flex;
    flex-direction: column;
  }
  .ep {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px 0;
    border-bottom: 1px solid var(--border);
  }
  .ep-name {
    font-size: 14px;
  }
  .ep-head {
    display: flex;
    align-items: center;
    gap: 6px;
    /* the editable part after the colon matches the prefix / default name size */
    font-size: 14px;
  }
  .ep-head .spacer {
    flex: 1;
  }
  .ep-remove {
    width: 36px;
    height: 36px;
    padding: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .players {
    gap: 6px;
  }
  .chip.player {
    cursor: pointer;
    background: var(--panel-2);
    border: 1px solid var(--border);
  }
  .chip.player.on {
    background: #15803d;
    border-color: #15803d;
    color: #fff;
  }
  .chip.player.on:hover {
    background: #166534;
  }
  .chip.player .glyph {
    margin-right: 4px;
  }
  .add-ep {
    margin-top: 8px;
  }
</style>
