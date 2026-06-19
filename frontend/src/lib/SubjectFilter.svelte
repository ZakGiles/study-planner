<script lang="ts" context="module">
  // Shared "no subject" sentinel so parents and this control agree on the value.
  export const UNGROUPED = '\0ungrouped';
</script>

<script lang="ts">
  // A subject filter chip row: All · each subject (colour-dotted) · Ungrouped.
  // Bound via `value` so several instances can drive (and reflect) one filter —
  // e.g. repeated in the Stats section headers so the active subject stays
  // visible as you scroll. '' = all, a subject id, or the UNGROUPED sentinel.
  import type { main } from '../../wailsjs/go/models';
  import { taskHex } from './colors';

  export let subjects: main.Subject[] = [];
  export let hasUngrouped = false;
  export let value = '';

  const chip = 'cursor-pointer rounded-sm border px-[0.6rem] py-[0.22rem] text-[0.74rem] font-semibold transition-colors';
  const active = 'border-accent-bright bg-[var(--accent-grad)] text-white';
  const idle = 'border-line bg-surface-2 text-fg-muted hover:border-line-strong hover:text-fg';
</script>

<div class="flex flex-wrap items-center gap-[0.4rem]">
  <span class="mr-[0.2rem] text-[0.72rem] font-semibold uppercase tracking-[0.06em] text-fg-faint">Subject</span>
  <button class="{chip} {value === '' ? active : idle}" on:click={() => (value = '')}>All</button>
  {#each subjects as s (s.id)}
    <button
      class="inline-flex items-center gap-[0.35rem] {chip} {value === s.id ? active : idle}"
      on:click={() => (value = s.id)}
    ><span class="h-[8px] w-[8px] rounded-full" style="background:{taskHex(s.color)}"></span>{s.name}</button>
  {/each}
  {#if hasUngrouped}
    <button class="{chip} {value === UNGROUPED ? active : idle}" on:click={() => (value = UNGROUPED)}>Ungrouped</button>
  {/if}
</div>
