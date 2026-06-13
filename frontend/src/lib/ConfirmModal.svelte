<script lang="ts" context="module">
  import { writable } from 'svelte/store';

  // Count of mounted ConfirmModals (GradeModal wraps one too). Global keyboard
  // shortcuts subscribe to this so they stay inert while any modal is open —
  // each component owns its modal state, so no single component can know.
  export const openModalCount = writable(0);

  // A modal action. Actions with a `detail` or a `color` render as stacked
  // choice buttons (color adds a tinted dot); the rest form a regular button
  // row underneath. Dismissing the modal (Escape / backdrop click) dispatches
  // `choose` with the value "cancel".
  export type ModalAction = {
    value: string;
    label: string;
    kind?: 'primary' | 'danger' | 'ghost';
    detail?: string;
    color?: string;
  };
</script>

<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';

  export let title: string;
  export let message = '';
  export let actions: ModalAction[] = [];

  const dispatch = createEventDispatcher<{ choose: string }>();

  $: choices = actions.filter((a) => a.detail || a.color);
  $: row = actions.filter((a) => !a.detail && !a.color);

  let panel: HTMLElement;
  onMount(() => {
    openModalCount.update((n) => n + 1);
    panel?.querySelector<HTMLElement>('button')?.focus();
    return () => openModalCount.update((n) => n - 1);
  });

  function focusables(): HTMLElement[] {
    return panel
      ? Array.from(panel.querySelectorAll<HTMLElement>('button, [href], input, select, textarea'))
      : [];
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation();
      dispatch('choose', 'cancel');
      return;
    }
    // Trap Tab inside the panel so focus can't reach controls behind the
    // overlay (which would otherwise let a keyboard user act on the page —
    // e.g. overwrite the session being graded — while the modal is open).
    if (e.key === 'Tab') {
      const items = focusables();
      if (items.length === 0) return;
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement as HTMLElement | null;
      if (!panel.contains(active)) {
        e.preventDefault();
        first.focus();
      } else if (e.shiftKey && active === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && active === last) {
        e.preventDefault();
        first.focus();
      }
    }
  }
</script>

<svelte:window on:keydown={onKey} />

<div
  class="fixed inset-0 z-[60] grid place-items-center bg-[rgba(5,9,14,0.6)] p-6"
  role="presentation"
  transition:fade={{ duration: 150 }}
  on:mousedown|self={() => dispatch('choose', 'cancel')}
>
  <div
    class="w-[min(92vw,440px)] max-h-[calc(100vh-3rem)] overflow-y-auto [overscroll-behavior:contain] rounded-lg border border-line-strong bg-surface-2 px-[1.25rem] pb-[1.2rem] pt-[1.15rem] text-left shadow-pop"
    role="dialog"
    aria-modal="true"
    aria-label={title}
    bind:this={panel}
    transition:fly={{ y: 14, duration: 220, easing: cubicOut }}
  >
    <h3 class="m-0 font-display text-[1.05rem] font-bold tracking-[-0.01em] text-fg-strong">{title}</h3>
    {#if message}
      <p class="mt-[0.45rem] text-[0.86rem] leading-[1.5] text-fg-muted">{message}</p>
    {/if}

    {#if choices.length}
      <div class="mt-[0.95rem] flex flex-col gap-[0.5rem]">
        {#each choices as c (c.value)}
          <button
            class="flex cursor-pointer flex-col gap-[0.18rem] rounded-md border border-line bg-inset px-[0.8rem] py-[0.65rem] text-left transition hover:-translate-y-px {c.color ? 'hover:[background:color-mix(in_srgb,var(--choice)_12%,transparent)] hover:[border-color:color-mix(in_srgb,var(--choice)_50%,transparent)]' : c.kind === 'primary' ? 'hover:border-accent-line hover:bg-accent-soft' : c.kind === 'danger' ? 'hover:border-red-line hover:bg-red-soft' : ''}"
            style={c.color ? `--choice:${c.color}` : ''}
            on:click={() => dispatch('choose', c.value)}
          >
            <span class="text-[0.9rem] font-bold {c.kind === 'primary' ? 'text-accent-bright' : c.kind === 'danger' ? 'text-red' : 'text-fg-strong'}">
              {#if c.color}<span class="mr-[0.5rem] inline-block h-[9px] w-[9px] rounded-full bg-[var(--choice)]"></span>{/if}{c.label}
            </span>
            {#if c.detail}<span class="text-[0.76rem] leading-[1.4] text-fg-muted">{c.detail}</span>{/if}
          </button>
        {/each}
      </div>
    {/if}

    {#if row.length}
      <div class="mt-4 flex justify-end gap-[0.5rem]">
        {#each row as c (c.value)}
          <button class="btn {c.kind ?? 'ghost'}" on:click={() => dispatch('choose', c.value)}>
            {c.label}
          </button>
        {/each}
      </div>
    {/if}
  </div>
</div>
