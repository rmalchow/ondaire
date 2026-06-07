<script>
  // Nodes section (J arch §4): one NodeRow per known node.
  import NodeRow from "../components/NodeRow.svelte";

  let { snapshot, self } = $props();

  // alive first, then by name.
  let nodes = $derived(
    [...(snapshot.nodes || [])].sort((a, b) => {
      if (a.alive !== b.alive) return a.alive ? -1 : 1;
      return (a.name || "").localeCompare(b.name || "");
    }),
  );
</script>

<section class="section">
  <h2>Nodes</h2>
  {#if nodes.length === 0}
    <div class="empty">No nodes yet.</div>
  {:else}
    {#each nodes as node (node.id)}
      <NodeRow {node} {self} />
    {/each}
  {/if}
</section>
