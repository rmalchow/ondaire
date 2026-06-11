<script>
  // The header node pill, made interactive: click to drop down every known node
  // (each node masters its own group 1:1, so this is the master list) and jump to
  // another node's UI. The pill itself keeps showing this node's name + health.
  let { snapshot, self, statusLevel, statusTitle } = $props();

  let open = $state(false);
  // id -> reachable origin ("" for this node). Only nodes present here are listed,
  // so a node with no UI reachable from this browser never shows up.
  let reach = $state({});
  let probing = $state(false);

  // Candidate origins for a node, same-/24 first (most likely reachable from here).
  function originsFor(n) {
    const port = n.httpPort;
    if (!port) return [];
    const proto = window.location.protocol === "https:" ? "https:" : "http:";
    const addrs = (n.addrs || []).map((c) => String(c).split("/")[0].trim()).filter(Boolean);
    const host = window.location.hostname;
    if (host.includes(".")) {
      const net = host.split(".").slice(0, 3).join(".") + ".";
      addrs.sort((a, b) => (b.startsWith(net) ? 1 : 0) - (a.startsWith(net) ? 1 : 0));
    }
    return addrs.map((ip) => `${proto}//${ip.includes(":") ? `[${ip}]` : ip}:${port}`);
  }

  // no-cors GET resolves on a live connection, rejects on network/timeout — a true
  // reachability test that needs no CORS on the peer.
  async function reachable(origin, ms = 2500) {
    try {
      await fetch(origin + "/api/status", { mode: "no-cors", cache: "no-store", signal: AbortSignal.timeout(ms) });
      return true;
    } catch {
      return false;
    }
  }

  async function probeAll() {
    const list = (snapshot && snapshot.nodes) || [];
    probing = true;
    const found = {};
    await Promise.all(
      list.map(async (n) => {
        if (self.id && n.id === self.id) {
          found[n.id] = ""; // this node = the current page, reachable by definition
          return;
        }
        for (const o of originsFor(n)) {
          if (await reachable(o)) {
            found[n.id] = o;
            return;
          }
        }
      }),
    );
    reach = found;
    probing = false;
  }

  $effect(() => {
    if (open) probeAll(); // probe once each time the menu opens
  });

  let nodes = $derived(
    ((snapshot && snapshot.nodes) || [])
      .filter((n) => n.id in reach)
      .map((n) => ({
        id: n.id,
        name: n.name || (n.id || "").slice(0, 8) || "node",
        isSelf: !!self.id && n.id === self.id,
        origin: reach[n.id],
      }))
      .sort((a, b) => (a.isSelf ? -1 : b.isSelf ? 1 : a.name.localeCompare(b.name))),
  );

  function toggle() {
    open = !open;
  }
  function close() {
    open = false;
  }
  function pick(n) {
    if (n.isSelf || !n.origin) {
      close();
      return;
    }
    window.location.href = n.origin;
  }
  function onKey(e) {
    if (e.key === "Escape") close();
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="switcher">
  <button
    class="status-pill {statusLevel}"
    title={statusTitle}
    onclick={toggle}
    aria-haspopup="listbox"
    aria-expanded={open}
  >
    {self.name || "…"}<span class="caret" aria-hidden="true">▾</span>
  </button>

  {#if open}
    <button class="sw-scrim" aria-label="Close node list" onclick={close}></button>
    <ul class="sw-menu" role="listbox">
      <li class="sw-head">Switch node</li>
      {#each nodes as n (n.id)}
        <li>
          <button
            class="sw-item"
            class:is-self={n.isSelf}
            disabled={n.isSelf}
            onclick={() => pick(n)}
          >
            <span class="sw-dot good"></span>
            <span class="sw-name">{n.name}</span>
            {#if n.isSelf}<span class="sw-tag">this node</span>{/if}
          </button>
        </li>
      {/each}
      {#if probing && nodes.length <= 1}
        <li class="sw-empty">Checking for reachable nodes…</li>
      {:else if nodes.length <= 1}
        <li class="sw-empty">No other nodes reachable.</li>
      {/if}
    </ul>
  {/if}
</div>

<style>
  .switcher {
    position: relative;
    display: inline-flex;
  }
  button.status-pill {
    font: inherit;
    font-size: 12.5px;
    cursor: pointer;
  }
  .caret {
    font-size: 9px;
    color: var(--muted);
    margin-left: 1px;
  }
  /* full-viewport invisible catcher so an outside click/tap closes the menu */
  .sw-scrim {
    position: fixed;
    inset: 0;
    z-index: 40;
    border: 0;
    background: transparent;
    cursor: default;
  }
  .sw-menu {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 41;
    min-width: 200px;
    max-width: 80vw;
    margin: 0;
    padding: 4px;
    list-style: none;
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 10px;
    box-shadow: 0 12px 32px -12px rgba(0, 0, 0, 0.6);
    max-height: 60vh;
    overflow-y: auto;
  }
  .sw-head {
    padding: 6px 10px 4px;
    font-size: 11px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--muted);
  }
  .sw-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 7px 10px;
    border: 0;
    border-radius: 7px;
    background: transparent;
    color: var(--fg);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .sw-item:hover:not(:disabled) {
    background: color-mix(in srgb, var(--accent) 12%, transparent);
  }
  .sw-item:disabled {
    cursor: default;
  }
  .sw-item.is-self {
    color: var(--muted);
  }
  .sw-dot {
    flex: 0 0 auto;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--muted);
  }
  .sw-dot.good {
    background: var(--ok);
  }
  .sw-empty {
    padding: 8px 10px;
    color: var(--muted);
    font-size: 12.5px;
  }
  .sw-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sw-tag {
    flex: 0 0 auto;
    font-size: 11px;
    color: var(--muted);
  }
</style>
