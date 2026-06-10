<script>
  // One member inside a group card (J arch §4): name, volume, master badge,
  // source stats (master row only when playing), remove. Renaming a node happens
  // on the Nodes page, not here; adding one uses the room's assign roster.
  import { nodeSetVolume, leaveGroup } from "../lib/api.js";
  import VolumeSlider from "./VolumeSlider.svelte";

  let { member, group, self } = $props();

  let isThisMaster = $derived(member.id === group.master);
  let pb = $derived(group.playback || { state: "idle" });
  let src = $derived(pb.source || {});
  let showStats = $derived(isThisMaster && pb.state === "playing");
</script>

<div class="member">
  <span class="member-id">
    <span class="dot {member.alive ? 'alive' : 'dead'}"></span>
    <span class="mname" title={member.name}>{member.name || "(unnamed)"}</span>
    {#if isThisMaster}<span class="badge">master</span>{/if}
    {#if member.id === self.id}<span class="chip">this node</span>{/if}
  </span>

  <VolumeSlider
    value={member.volume}
    onchange={(v) => nodeSetVolume(member, v)}
  />

  <button
    class="btn icon-btn"
    onclick={() => leaveGroup(member)}
    title="remove from room"
    aria-label="remove from room">✕</button
  >

  <!-- Source stats (master, playing): own line below on wide, hidden when narrow.
       Rams: silence is not a signal — show counts only when they mean something. -->
  {#if showStats && ((src.clients ?? 0) > 0 || (src.restarts ?? 0) > 0)}
    <span class="member-stats">
      {#if (src.clients ?? 0) > 0}<span class="chip">{src.clients} listeners</span>{/if}
      {#if (src.restarts ?? 0) > 0}<span class="chip">{src.restarts} reconnects</span>{/if}
    </span>
  {/if}
</div>

<style>
  /* Line 1: name (col 1) | volume (col 2) | remove (col 3, right). Source stats
     get their OWN line below (col 4, full width). Leading column prefers 16rem so
     sliders line up across rows, but shrinks — the name ellipsises. */
  .member-id {
    order: 1;
    display: inline-flex;
    align-items: center;
    gap: 8px;
    flex: 0 1 16rem;
    min-width: 0;
    overflow: hidden;
  }
  .mname {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .member-id :global(.badge),
  .member-id :global(.chip) {
    flex: 0 0 auto;
  }

  /* volume fills the middle of the first line */
  .member :global(.vol) {
    order: 2;
    flex: 1 1 auto;
    min-width: 6rem;
  }

  /* Leave control: icon-only, compact, pinned to the right of the first line. */
  .icon-btn {
    order: 3;
    flex: 0 0 auto;
    margin-left: auto;
    line-height: 1;
    padding: 4px 7px;
  }

  /* Source stats on their own line below (full-width forces the wrap). */
  .member-stats {
    order: 4;
    flex: 1 1 100%;
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  /* Narrow cards: name + remove on line 1 (badges hidden), volume on line 2,
     source stats hidden entirely. */
  @media (max-width: 560px) {
    .member-id {
      flex: 1 1 auto;
    }
    .member-id :global(.badge),
    .member-id :global(.chip) {
      display: none;
    }
    .icon-btn {
      order: 2;
    }
    .member :global(.vol) {
      order: 3;
      flex: 1 1 100%;
    }
    .member-stats {
      display: none;
    }
  }
</style>
