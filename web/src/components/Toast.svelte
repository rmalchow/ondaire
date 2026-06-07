<script>
  import { toasts, dismissToast } from "../lib/toast.svelte.js";

  // auto-dismiss each toast after ~4s.
  $effect(() => {
    const timers = toasts.list.map((t) =>
      setTimeout(() => dismissToast(t.id), 4000),
    );
    return () => timers.forEach(clearTimeout);
  });
</script>

{#if toasts.list.length > 0}
  <div class="toasts">
    {#each toasts.list as t (t.id)}
      <button
        type="button"
        class="toast {t.kind}"
        onclick={() => dismissToast(t.id)}
      >
        {t.msg}
      </button>
    {/each}
  </div>
{/if}
