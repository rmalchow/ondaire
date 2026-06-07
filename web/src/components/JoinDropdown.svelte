<script>
  // A member's "Join group…" select → POST /api/follow (J arch §4).
  import { joinTargets } from "../lib/derive.js";
  import { follow } from "../lib/api.js";

  let { member, snapshot } = $props();

  let selected = $state("");
  let targets = $derived(joinTargets(snapshot, member));

  async function onchange(e) {
    const target = e.target.value;
    selected = "";
    e.target.value = "";
    if (target) {
      try {
        await follow(member.id, target);
      } catch {
        // toast shown by api.js
      }
    }
  }
</script>

{#if targets.length > 0}
  <select value={selected} {onchange} title="follow another master">
    <option value="">Join group…</option>
    {#each targets as t (t.id)}
      <option value={t.id}>{t.name}</option>
    {/each}
  </select>
{/if}
