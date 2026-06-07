<script>
  // Click-to-rename inline editor (J arch §4). Reused by group + node names.
  let { value, onsave, placeholder = "" } = $props();

  let editing = $state(false);
  let draft = $state("");
  let inputEl = $state(null);

  function start() {
    draft = value || "";
    editing = true;
  }

  $effect(() => {
    if (editing && inputEl) inputEl.focus();
  });

  async function commit() {
    if (!editing) return;
    const next = draft.trim();
    editing = false;
    if (next && next !== value) {
      try {
        await onsave(next);
      } catch {
        // toast already shown by api.js; snapshot reverts the display
      }
    }
  }

  function cancel() {
    editing = false;
  }

  function onkey(e) {
    if (e.key === "Enter") {
      e.preventDefault();
      commit();
    } else if (e.key === "Escape") {
      e.preventDefault();
      cancel();
    }
  }
</script>

{#if editing}
  <input
    class="editable-input"
    bind:this={inputEl}
    bind:value={draft}
    onkeydown={onkey}
    onblur={commit}
    {placeholder}
  />
{:else}
  <span
    class="editable"
    role="button"
    tabindex="0"
    onclick={start}
    onkeydown={(e) => e.key === "Enter" && start()}
    title="click to rename"
  >
    {value || placeholder || "—"}
  </span>
{/if}
