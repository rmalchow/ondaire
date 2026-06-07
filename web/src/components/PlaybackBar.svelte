<script>
  // Playback state + position + stop button for a group (J arch §4).
  import { position } from "../lib/fmt.js";
  import { stop } from "../lib/api.js";

  let { group } = $props();

  let pb = $derived(group.playback || { state: "idle" });
  let playing = $derived(pb.state === "playing");

  async function onstop() {
    try {
      await stop(group.master);
    } catch {
      // toast shown by api.js
    }
  }
</script>

<div class="row wrap" style="margin: 4px 0 8px;">
  {#if playing}
    <span class="badge">playing</span>
    <span class="muted small" title={pb.uri}>{pb.uri}</span>
    <span class="small">{position(pb.positionSec)}</span>
    <span class="chip">{pb.codec}</span>
    <span class="chip">{pb.transport}</span>
    <span class="spacer"></span>
    <button class="btn btn-danger" onclick={onstop}>Stop</button>
  {:else}
    <span class="muted small">idle</span>
  {/if}
</div>
