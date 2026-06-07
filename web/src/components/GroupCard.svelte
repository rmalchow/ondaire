<script>
  // One derived group (J arch §4): name, playback bar, members, settings text.
  import { nodeById, groupLabel } from "../lib/derive.js";
  import { renameGroup } from "../lib/api.js";
  import EditableText from "./EditableText.svelte";
  import PlaybackBar from "./PlaybackBar.svelte";
  import MemberRow from "./MemberRow.svelte";

  let { group, snapshot, self } = $props();

  let label = $derived(groupLabel(group));
  let members = $derived(
    group.members.map((id) => nodeById(snapshot, id)).filter(Boolean),
  );
  let settings = $derived(group.settings || {});
</script>

<div class="card">
  <div class="row between">
    <h3>
      <EditableText value={label} onsave={(n) => renameGroup(group.id, n)} />
    </h3>
  </div>

  <PlaybackBar {group} />

  {#each members as member (member.id)}
    <MemberRow {member} {group} {self} {snapshot} />
  {/each}

  <div class="hint" style="margin-top: 8px;">
    codec {settings.codec ?? "pcm"} · transport {settings.transport ?? "udp"} ·
    buffer {settings.bufferMs ?? 150} ms
  </div>
</div>
