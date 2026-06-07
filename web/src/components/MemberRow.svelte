<script>
  // One member inside a group card (J arch §4): name, volume, master badge,
  // source stats (master row only when playing), make-master, leave, join.
  import { relTime } from "../lib/fmt.js";
  import { renameNode, setVolume, makeMaster, unfollow } from "../lib/api.js";
  import EditableText from "./EditableText.svelte";
  import VolumeSlider from "./VolumeSlider.svelte";
  import JoinDropdown from "./JoinDropdown.svelte";

  let { member, group, self, snapshot } = $props();

  let isThisMaster = $derived(member.id === group.master);
  let solo = $derived(group.members.length <= 1);
  let pb = $derived(group.playback || { state: "idle" });
  let src = $derived(pb.source || {});
  let showStats = $derived(isThisMaster && pb.state === "playing");
</script>

<div class="member">
  <span class="dot {member.alive ? 'alive' : 'dead'}"></span>
  <EditableText
    value={member.name}
    onsave={(n) => renameNode(member.id, n)}
    placeholder="(unnamed)"
  />
  {#if isThisMaster}<span class="badge">master</span>{/if}
  {#if member.id === self.id}<span class="chip">this node</span>{/if}

  <VolumeSlider
    value={member.volume}
    onchange={(v) => setVolume(member.id, v)}
  />

  {#if showStats}
    <span class="chip">{src.clients ?? 0} listeners</span>
    <span class="chip">{src.restarts ?? 0} reconnects</span>
  {/if}

  <span class="spacer"></span>

  <span class="muted small">
    {member.alive ? relTime(member.lastSeen) : "offline"}
    {#if member.stale}<span class="offline"> stale</span>{/if}
  </span>

  {#if !isThisMaster}
    <button
      class="btn btn-accent"
      onclick={() => makeMaster(member.id, member.id)}
      title="take mastership to this node">Make master</button
    >
  {/if}
  {#if !solo}
    <button class="btn" onclick={() => unfollow(member.id)}>Leave</button>
  {/if}
  <JoinDropdown {member} {snapshot} />
</div>
