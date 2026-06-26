<script>
  // Add/edit modal for a cluster-wide stream preset. Used for both "Add" (preset
  // null) and "Edit" (preset = the StreamPresetView). Secrets are write-only: the
  // snapshot never returns them, so edit pre-fills name/url + auth scheme but the
  // secret inputs start blank; leaving them blank on save keeps the stored secret.
  import { setStreamPreset, deleteStreamPreset } from "../lib/api.js";

  let { preset = null, onclose } = $props();

  const editing = !!preset;
  let name = $state(preset?.name ?? "");
  let url = $state(preset?.url ?? "");
  let scheme = $state(preset?.authScheme ?? "none");
  let user = $state("");
  let pass = $state("");
  let token = $state("");
  let busy = $state(false);

  let urlValid = $derived(/^https?:\/\/\S+/.test(url.trim()));
  let nameValid = $derived(name.trim().length > 0);
  let canSave = $derived(nameValid && urlValid && !busy);

  function buildAuth() {
    if (scheme === "basic") return { scheme: "basic", user: user.trim(), pass };
    if (scheme === "bearer") return { scheme: "bearer", token };
    return undefined; // "none" → clear auth
  }

  async function save() {
    if (!canSave) return;
    busy = true;
    try {
      await setStreamPreset({
        id: preset?.id,
        name: name.trim(),
        url: url.trim(),
        auth: buildAuth(),
      });
      onclose?.();
    } catch {
      busy = false; // toast already shown by api layer; keep modal open
    }
  }

  async function remove() {
    if (!editing || busy) return;
    busy = true;
    try {
      await deleteStreamPreset(preset.id);
      onclose?.();
    } catch {
      busy = false;
    }
  }

  function onKey(e) {
    if (e.key === "Escape") onclose?.();
  }
</script>

<svelte:window onkeydown={onKey} />

<div
  class="backdrop"
  role="button"
  tabindex="-1"
  aria-label="close"
  onclick={() => onclose?.()}
  onkeydown={(e) => (e.key === "Enter" || e.key === " ") && onclose?.()}
>
  <div
    class="dialog"
    role="dialog"
    aria-modal="true"
    aria-label={editing ? "Edit stream" : "Add stream"}
    onclick={(e) => e.stopPropagation()}
    onkeydown={(e) => e.stopPropagation()}
  >
    <h3>{editing ? "Edit stream" : "Add stream"}</h3>

    <label>
      <span>Name</span>
      <input type="text" placeholder="My radio station" bind:value={name} />
    </label>

    <label>
      <span>URL</span>
      <input type="text" placeholder="http(s)://host/stream(.pls/.m3u)" bind:value={url} />
    </label>

    <label>
      <span>Authentication</span>
      <select bind:value={scheme}>
        <option value="none">None</option>
        <option value="basic">HTTP Basic</option>
        <option value="bearer">Bearer token</option>
      </select>
    </label>

    {#if scheme === "basic"}
      <label>
        <span>Username</span>
        <input type="text" autocomplete="off" bind:value={user} />
      </label>
      <label>
        <span>Password</span>
        <input
          type="password"
          autocomplete="new-password"
          placeholder={editing && preset?.hasAuth ? "•••••• (unchanged)" : ""}
          bind:value={pass}
        />
      </label>
    {:else if scheme === "bearer"}
      <label>
        <span>Token</span>
        <input
          type="password"
          autocomplete="new-password"
          placeholder={editing && preset?.hasAuth ? "•••••• (unchanged)" : ""}
          bind:value={token}
        />
      </label>
    {/if}

    <div class="actions row wrap">
      {#if editing}
        <button class="btn btn-danger" onclick={remove} disabled={busy}>Delete</button>
      {/if}
      <span class="spacer"></span>
      <button class="btn" onclick={() => onclose?.()} disabled={busy}>Cancel</button>
      <button class="btn btn-accent" onclick={save} disabled={!canSave}>Save</button>
    </div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.55);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
    border: none;
  }
  .dialog {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 18px;
    width: min(420px, 92vw);
    display: flex;
    flex-direction: column;
    gap: 10px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
  }
  h3 {
    margin: 0 0 4px;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 13px;
    color: var(--muted);
  }
  label input,
  label select {
    font: inherit;
  }
  .actions {
    margin-top: 8px;
    align-items: center;
  }
  .spacer {
    flex: 1;
  }
  .btn-danger {
    border-color: var(--danger, #c0392b);
    color: var(--danger, #c0392b);
  }
</style>
