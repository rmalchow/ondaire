<script>
  // Media section (J arch §4): the SELECTED group (clicked card) is the play
  // target — no node picker. File list → Play, plus URL and Input play paths gated
  // on the target group master's reported sources (§6.1).
  import { bytes, relTime } from "../lib/fmt.js";
  import { nodeById, activeGroup, groupLabel } from "../lib/derive.js";
  import { getMedia, playOnNode } from "../lib/api.js";
  import { entriesFor, crumbs, parentDir, joinDir } from "../lib/tree.js";

  let { snapshot, self, selectedMaster, selectTick } = $props();

  let pickedNodeId = $state("");
  let files = $state([]);
  let url = $state("");
  let loading = $state(false);
  // current directory within the picked node's media tree ("" == root).
  let dir = $state("");

  // Clicking a group card (App selectTick bump) points the picker at that
  // group's MASTER. Tracked by tick so re-selecting the same group re-applies,
  // while a manual pick from the dropdown below still sticks in between.
  let appliedTick = -1;
  $effect(() => {
    if (selectTick !== appliedTick) {
      appliedTick = selectTick;
      if (selectedMaster) pickedNodeId = selectedMaster;
    }
  });

  // Before any explicit selection, default to the active group's master
  // (a playing group, else self's group, else the first).
  $effect(() => {
    if (!pickedNodeId) {
      const g = activeGroup(snapshot, self.id);
      pickedNodeId = (g && g.master) || self.id || "";
    }
  });

  // The group this media plays into = the group mastered by the picked node (the
  // selected card). Its label gives the section its context.
  let targetGroup = $derived(
    (snapshot.groups || []).find((g) => g.master === pickedNodeId),
  );
  let targetLabel = $derived(targetGroup ? groupLabel(targetGroup) : "");

  let picked = $derived(nodeById(snapshot, pickedNodeId));
  let sources = $derived(picked?.capabilities?.sources ?? []);
  let canHttp = $derived(sources.includes("http"));
  let canInput = $derived(sources.includes("input"));
  let urlValid = $derived(/^https?:\/\/\S+/.test(url.trim()));

  // capture devices on the picked node (D48): pick which input to play. "" =
  // system default.
  let inputDevices = $derived(picked?.inputDevices ?? []);
  let inputDeviceId = $state("");
  $effect(() => {
    const ids = inputDevices.map((d) => d.id);
    if (!ids.includes(inputDeviceId)) inputDeviceId = inputDevices.length ? inputDevices[0].id : "";
  });

  // directory view derived from the flat file list + current dir (pure, tree.js).
  let view = $derived(entriesFor(files, dir));
  let trail = $derived(crumbs(dir));

  // refetch files when the picked node changes; reset to the root directory.
  $effect(() => {
    const id = pickedNodeId;
    dir = "";
    if (!id) {
      files = [];
      return;
    }
    loading = true;
    getMedia(id)
      .then((list) => {
        // Spec §6: bare array; tolerate an older node's {files:[...]} envelope.
        if (pickedNodeId === id) files = Array.isArray(list) ? list : Array.isArray(list?.files) ? list.files : [];
      })
      .catch(() => {
        if (pickedNodeId === id) files = [];
      })
      .finally(() => {
        if (pickedNodeId === id) loading = false;
      });
  });

  // play `uri` into the selected group: its master sources it directly (every node
  // masters its own group — no takeover).
  function playHere(uri) {
    if (pickedNodeId) playOnNode(pickedNodeId, uri).catch(() => {});
  }
  function playFile(f) {
    playHere("file:" + f.path);
  }
  function playUrl() {
    if (urlValid) playHere(url.trim());
  }
  function playInput() {
    playHere("input:" + inputDeviceId);
  }

  function enter(folder) {
    dir = joinDir(dir, folder.name);
  }
  function goUp() {
    dir = parentDir(dir);
  }
  function goTo(d) {
    dir = d;
  }
</script>

<section class="section">
  <h2>Media</h2>
  <div class="card">
    <div class="row wrap" style="margin-bottom: 10px;">
      {#if targetLabel}
        <span class="muted">Playing into <strong>{targetLabel}</strong></span>
      {:else}
        <span class="muted">Select a group above to play into it.</span>
      {/if}
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
      <div class="row wrap" style="margin-bottom: 8px;">
        {#if inputDevices.length > 0}
          <select bind:value={inputDeviceId} aria-label="input device" title="capture device">
            {#each inputDevices as d (d.id)}
              <option value={d.id}>{d.desc}</option>
            {/each}
          </select>
        {/if}
        <button class="btn" onclick={playInput}>Input (line-in / mic)</button>
      </div>
    {/if}

    <div class="crumbs row wrap">
      {#each trail as c, i (c.dir)}
        {#if i > 0}<span class="crumb-sep">/</span>{/if}
        {#if i === trail.length - 1}
          <span class="crumb here">{c.name}</span>
        {:else}
          <button class="crumb link" onclick={() => goTo(c.dir)}>
            {c.name}
          </button>
        {/if}
      {/each}
    </div>

    {#if view.folders.length === 0 && view.files.length === 0 && dir === ""}
      <div class="empty">{loading ? "" : "No media files."}</div>
    {:else}
      {#if dir !== ""}
        <button class="media-file folder-row up" onclick={goUp}>
          <span class="glyph">▸</span>
          <span>..</span>
        </button>
      {/if}
      {#each view.folders as folder (folder.name)}
        <button class="media-file folder-row" onclick={() => enter(folder)}>
          <span class="glyph">📁</span>
          <span>{folder.name}</span>
          <span class="muted small">{folder.count} file{folder.count === 1 ? "" : "s"}</span>
        </button>
      {/each}
      {#each view.files as f (f.path)}
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
