<script>
  import { onMount } from "svelte";
  import { cluster, connect, disconnect } from "./lib/ws.svelte.js";
  import { getStatus, getCluster, setSelfId } from "./lib/api.js";
  import { startStatsPolling, stopStatsPolling } from "./lib/stats.svelte.js";
  import Groups from "./sections/Groups.svelte";
  import Nodes from "./sections/Nodes.svelte";
  import Toast from "./components/Toast.svelte";
  import UnreachableBanner from "./components/UnreachableBanner.svelte";
  import NodeSwitcher from "./components/NodeSwitcher.svelte";

  // self {id, name}; id/name seeded once from GET /api/status.
  let self = $state({ id: "", name: "", role: "" });

  // Theme switcher (experimental): a theme is a token swap on <html data-theme>.
  // Persisted to localStorage; applied synchronously at init so there's no flash.
  const THEMES = ["mint", "studio", "nocturne", "paper", "8bit", "xp"];
  const initialTheme = localStorage.getItem("ensemble-theme") || "mint";
  if (typeof document !== "undefined")
    document.documentElement.dataset.theme = initialTheme;
  let theme = $state(initialTheme);
  $effect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("ensemble-theme", theme);
  });

  // re-render trigger for the "stale connection" hint (no interval needed —
  // heartbeats drive re-render; this just reads receivedAt).
  let stale = $derived(
    cluster.receivedAt > 0 && Date.now() - cluster.receivedAt > 10000,
  );

  // Connection health folded into one signal for the node-name pill: green when
  // open + fresh, red when the socket is closed, amber in between (connecting /
  // reconnecting / stale data).
  let statusLevel = $derived(
    cluster.status === "closed"
      ? "bad"
      : cluster.status === "open" && !stale
        ? "good"
        : "warn",
  );
  let statusTitle = $derived(stale ? cluster.status + " · stale" : cluster.status);

  // When the serving node stays unreachable past a short grace (so brief reconnect
  // blips don't flash), surface the fallback banner with links to reachable peers.
  let showFallback = $state(false);
  $effect(() => {
    if (cluster.status === "open") {
      showFallback = false;
      return;
    }
    const id = setTimeout(() => (showFallback = true), 5000);
    return () => clearTimeout(id);
  });

  // Two pages via hash routing (bulletproof under the embedded SPA — no server
  // route needed): "" → overview (groups + media), "nodes" → the node list.
  let route = $state(parseHash());
  function parseHash() {
    return location.hash.replace(/^#\/?/, "");
  }
  function onHash() {
    route = parseHash();
  }

  // Clicking a group card selects it → that card reveals its operational controls
  // (assign roster, inline media browser, settings). group.id === group.master
  // (D42), so the selected master id doubles as the selected-group highlight key.
  let selectedMaster = $state("");
  function selectGroup(masterId) {
    selectedMaster = masterId;
  }

  onMount(() => {
    window.addEventListener("hashchange", onHash);
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
      startStatsPolling();
    })();

    return () => {
      window.removeEventListener("hashchange", onHash);
      disconnect();
      stopStatsPolling();
    };
  });
</script>

<header class="app-header">
  <span class="brand-mark">
    <span class="wordmark">ensemble</span>
    <span class="brand-dot"></span>
  </span>
  <span class="spacer"></span>
  <div class="header-right">
    <select
      class="theme-select"
      bind:value={theme}
      aria-label="color theme"
      title="color theme (experimental)"
    >
      {#each THEMES as t (t)}
        <option value={t}>{t}</option>
      {/each}
    </select>
    <NodeSwitcher snapshot={cluster.snapshot} {self} {statusLevel} {statusTitle} />
  </div>
</header>

<style>
  /* experimental theme picker — a compact pill that matches the chip language */
  .theme-select {
    appearance: none;
    -webkit-appearance: none;
    background: color-mix(in srgb, var(--panel) 80%, transparent);
    color: var(--muted);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 4px 24px 4px 12px;
    font-family: var(--mono);
    font-size: 12px;
    cursor: pointer;
    backdrop-filter: blur(8px);
    background-image: linear-gradient(
        45deg,
        transparent 50%,
        var(--muted) 50%
      ),
      linear-gradient(135deg, var(--muted) 50%, transparent 50%);
    background-position:
      right 11px center,
      right 7px center;
    background-size:
      4px 4px,
      4px 4px;
    background-repeat: no-repeat;
    transition: border-color 0.18s ease, color 0.18s ease;
  }
  .theme-select:hover {
    border-color: color-mix(in srgb, var(--accent) 55%, var(--border));
    color: var(--fg);
  }
</style>

{#if showFallback}
  <UnreachableBanner {self} />
{/if}

<Toast />

<h1 class="page-switch">
  <a
    href="#/"
    class="switch-item"
    class:active={route !== "nodes"}
    aria-current={route !== "nodes" ? "page" : undefined}>rooms</a>
  <span class="switch-sep" aria-hidden="true">|</span>
  <a
    href="#/nodes"
    class="switch-item"
    class:active={route === "nodes"}
    aria-current={route === "nodes" ? "page" : undefined}>nodes</a>
</h1>

{#if route === "nodes"}
  <Nodes snapshot={cluster.snapshot} {self} />
{:else}
  <Groups
    snapshot={cluster.snapshot}
    {self}
    {selectedMaster}
    onselect={selectGroup}
  />
{/if}
