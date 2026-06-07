<script>
  // Media section (J arch §4): node picker → file list → Play here, plus URL
  // and Input play paths gated on the picked node's reported sources (§6.1).
  import { bytes, relTime } from "../lib/fmt.js";
  import { nodeById } from "../lib/derive.js";
  import { getMedia, play } from "../lib/api.js";

  let { snapshot, self } = $props();

  let pickedNodeId = $state("");
  let files = $state([]);
  let url = $state("");
  let loading = $state(false);

  // default the picker to self once self id is known.
  $effect(() => {
    if (!pickedNodeId && self.id) pickedNodeId = self.id;
  });

  // nodes that can decode local media (non-empty formats).
  let mediaNodes = $derived(
    (snapshot.nodes || []).filter(
      (n) => (n.capabilities?.formats ?? []).length > 0,
    ),
  );

  let picked = $derived(nodeById(snapshot, pickedNodeId));
  let sources = $derived(picked?.capabilities?.sources ?? []);
  let canHttp = $derived(sources.includes("http"));
  let canInput = $derived(sources.includes("input"));
  let urlValid = $derived(/^https?:\/\/\S+/.test(url.trim()));

  // refetch files when the picked node changes.
  $effect(() => {
    const id = pickedNodeId;
    if (!id) {
      files = [];
      return;
    }
    loading = true;
    getMedia(id)
      .then((list) => {
        if (pickedNodeId === id) files = Array.isArray(list) ? list : [];
      })
      .catch(() => {
        if (pickedNodeId === id) files = [];
      })
      .finally(() => {
        if (pickedNodeId === id) loading = false;
      });
  });

  function playFile(f) {
    play(pickedNodeId, "file:" + f.path).catch(() => {});
  }
  function playUrl() {
    if (urlValid) play(pickedNodeId, url.trim()).catch(() => {});
  }
  function playInput() {
    play(pickedNodeId, "input:").catch(() => {});
  }
</script>

<section class="section">
  <h2>Media</h2>
  <div class="card">
    <div class="row wrap" style="margin-bottom: 10px;">
      <label class="row">
        Node
        <select bind:value={pickedNodeId}>
          {#each mediaNodes as n (n.id)}
            <option value={n.id}>{n.name}</option>
          {/each}
        </select>
      </label>
      {#if loading}<span class="muted small">loading…</span>{/if}
    </div>

    {#if canHttp}
      <div class="row wrap" style="margin-bottom: 8px;">
        <input
          type="text"
          placeholder="http(s)://stream-url"
          bind:value={url}
          style="flex: 1; min-width: 200px;"
        />
        <button class="btn btn-accent" disabled={!urlValid} onclick={playUrl}>
          Play URL
        </button>
      </div>
    {/if}

    {#if canInput}
      <div class="row" style="margin-bottom: 8px;">
        <button class="btn" onclick={playInput}>Input (line-in / mic)</button>
      </div>
    {/if}

    {#if files.length === 0}
      <div class="empty">{loading ? "" : "No media files."}</div>
    {:else}
      {#each files as f (f.path)}
        <div class="media-file">
          <span title={f.path}>{f.name}</span>
          <span class="muted small">{bytes(f.sizeBytes)}</span>
          <span class="muted small">{relTime(f.modTime)}</span>
          <span class="spacer"></span>
          <button class="btn btn-accent" onclick={() => playFile(f)}>
            Play here
          </button>
        </div>
      {/each}
    {/if}
  </div>
</section>
