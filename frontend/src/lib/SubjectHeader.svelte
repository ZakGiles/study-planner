<script lang="ts">
  // The header of a subject group in the Tasks view: collapse toggle, colour,
  // rename, manual reorder (up/down) and delete. Self-contained like TaskCard —
  // it owns its rename/colour state and runs the backend calls itself, emitting
  // a `changed` State for the owner to swap in. Reordering needs the sibling
  // list, so it only emits `moveUp`/`moveDown` for App to resolve.
  import { createEventDispatcher } from 'svelte';
  import type { main } from '../../wailsjs/go/models';
  import { UpdateSubject, SetSubjectColor, DeleteSubject } from '../../wailsjs/go/main/App.js';
  import { makeMutator } from './mutate';
  import { plural } from './dates';
  import { TASK_COLORS, taskHex } from './colors';
  import ConfirmModal from './ConfirmModal.svelte';
  import type { ModalAction } from './ConfirmModal.svelte';

  export let subject: main.Subject;
  export let count = 0;
  export let collapsed = false;
  export let canMoveUp = false;
  export let canMoveDown = false;

  const dispatch = createEventDispatcher<{
    changed: main.State;
    error: string;
    toggle: void;
    moveUp: void;
    moveDown: void;
  }>();

  let busy = false;
  let editing = false;
  let editName = '';
  let showColors = false;
  let confirmDelete = false;

  $: hex = taskHex(subject.color);

  const run = makeMutator({
    state: (s) => dispatch('changed', s),
    error: (m) => dispatch('error', m),
    busy: (b) => (busy = b),
  });

  function startEdit() {
    editName = subject.name;
    editing = true;
  }
  async function saveEdit() {
    if (!editName.trim()) return;
    if (await run(UpdateSubject(subject.id, editName))) editing = false;
  }
  async function pickColor(token: string) {
    showColors = false;
    await run(SetSubjectColor(subject.id, token));
  }

  $: deleteActions = [
    { value: 'delete', label: 'Delete subject', kind: 'danger' },
    { value: 'cancel', label: 'Cancel', kind: 'ghost' },
  ] as ModalAction[];
  function onConfirm(e: CustomEvent<string>) {
    confirmDelete = false;
    if (e.detail === 'delete') void run(DeleteSubject(subject.id));
  }
</script>

<div class="flex items-center gap-[0.55rem] rounded-md border border-line bg-surface-2 py-[0.5rem] pl-[0.6rem] pr-[0.5rem]" style="--task:{hex}">
  {#if editing}
    <span class="h-[12px] w-[12px] shrink-0 rounded-full bg-[var(--task)]" aria-hidden="true"></span>
    <input class="min-w-0 flex-1" type="text" bind:value={editName} on:keydown={(e) => e.key === 'Enter' && saveEdit()} placeholder="Subject name" />
    <button class="btn primary sm" on:click={saveEdit} disabled={busy || !editName.trim()}>Save</button>
    <button class="btn ghost sm" on:click={() => (editing = false)} disabled={busy}>Cancel</button>
  {:else}
    <button class="flex min-w-0 flex-1 cursor-pointer items-center gap-[0.55rem] border-none bg-transparent p-0 text-left" on:click={() => dispatch('toggle')} title={collapsed ? 'Expand' : 'Collapse'}>
      <span class="shrink-0 text-[0.7rem] text-fg-muted transition-transform {collapsed ? '' : 'rotate-90'}" aria-hidden="true">▶</span>
      <span class="h-[12px] w-[12px] shrink-0 rounded-full bg-[var(--task)]" aria-hidden="true"></span>
      <span class="truncate font-display text-[0.98rem] font-bold tracking-[-0.01em] text-fg-strong">{subject.name}</span>
      <span class="tnum shrink-0 text-[0.78rem] text-fg-muted">{count} task{plural(count)}</span>
    </button>
    <div class="flex shrink-0 items-center gap-[0.2rem]">
      <button class="icon-btn small" title="Subject colour" on:click={() => (showColors = !showColors)} disabled={busy}><span class="block h-[12px] w-[12px] rounded-full bg-[var(--task)]"></span></button>
      <button class="icon-btn small" title="Rename subject" on:click={startEdit} disabled={busy}>✎</button>
      <button class="icon-btn small" title="Move up" on:click={() => dispatch('moveUp')} disabled={busy || !canMoveUp}>↑</button>
      <button class="icon-btn small" title="Move down" on:click={() => dispatch('moveDown')} disabled={busy || !canMoveDown}>↓</button>
      <button class="icon-btn small" title="Delete subject" on:click={() => (confirmDelete = true)} disabled={busy}>✕</button>
    </div>
  {/if}
</div>

{#if showColors}
  <div class="mt-[0.4rem] flex flex-wrap gap-[0.4rem] pl-[0.6rem]">
    {#each TASK_COLORS as c}
      <button
        class="h-5 w-5 cursor-pointer rounded-full border-none bg-[var(--task)] p-0 transition-transform hover:scale-110 {subject.color === c.token ? 'shadow-[0_0_0_2px_var(--surface),0_0_0_4px_var(--task)]' : ''}"
        style="--task:{c.hex}"
        title={c.label}
        aria-label={c.label}
        on:click={() => pickColor(c.token)}
        disabled={busy}
      ></button>
    {/each}
  </div>
{/if}

{#if confirmDelete}
  <ConfirmModal
    title="Delete “{subject.name}”?"
    message={count > 0
      ? `Its ${count} task${plural(count)} won't be deleted — they move to Ungrouped.`
      : 'This removes the subject.'}
    actions={deleteActions}
    on:choose={onConfirm}
  />
{/if}
