<script>
  import { onMount } from "svelte";
  import { cluster, connect, disconnect } from "./lib/ws.svelte.js";
  import { getStatus, getCluster, setSelfId } from "./lib/api.js";
  import Groups from "./sections/Groups.svelte";
  import Nodes from "./sections/Nodes.svelte";
  import Media from "./sections/Media.svelte";
  import Toast from "./components/Toast.svelte";

  // self {id, name, role}; seeded once from GET /api/status.
  let self = $state({ id: "", name: "", role: "" });

  // re-render trigger for the "stale connection" hint (no interval needed —
  // heartbeats drive re-render; this just reads receivedAt).
  let stale = $derived(
    cluster.receivedAt > 0 && Date.now() - cluster.receivedAt > 10000,
  );

  onMount(() => {
    (async () => {
      try {
        const s = await getStatus();
        if (s) {
          self = { id: s.id || "", name: s.name || "", role: s.role || "" };
          setSelfId(self.id);
        }
      } catch {
        // non-fatal: "this node" markers simply absent
      }
      try {
        const snap = await getCluster();
        if (snap) cluster.snapshot = snap;
      } catch {
        // non-fatal: ws will seed
      }
      connect();
    })();

    return () => disconnect();
  });
</script>

<header class="app-header">
  <span class="title">ensemble</span>
  <span class="self">
    {#if self.name}
      {self.name}
      {#if self.role}<span class="badge">{self.role}</span>{/if}
    {:else}
      …
    {/if}
  </span>
  <span class="spacer"></span>
  {#if stale}<span class="muted small">stale</span>{/if}
  <span class="dot {cluster.status}" title={cluster.status}></span>
  <span class="muted small">{cluster.status}</span>
</header>

<Toast />

<Groups snapshot={cluster.snapshot} {self} />
<Nodes snapshot={cluster.snapshot} {self} />
<Media snapshot={cluster.snapshot} {self} />
