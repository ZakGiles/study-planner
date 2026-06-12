<script lang="ts" context="module">
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
  onMount(() => panel?.querySelector<HTMLElement>('button')?.focus());

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
  class="overlay"
  role="presentation"
  transition:fade={{ duration: 150 }}
  on:mousedown|self={() => dispatch('choose', 'cancel')}
>
  <div
    class="panel"
    role="dialog"
    aria-modal="true"
    aria-label={title}
    bind:this={panel}
    transition:fly={{ y: 14, duration: 220, easing: cubicOut }}
  >
    <h3>{title}</h3>
    {#if message}
      <p class="msg">{message}</p>
    {/if}

    {#if choices.length}
      <div class="choices">
        {#each choices as c (c.value)}
          <button
            class="choice {c.kind ?? ''}"
            class:tinted={!!c.color}
            style={c.color ? `--choice:${c.color}` : ''}
            on:click={() => dispatch('choose', c.value)}
          >
            <span class="choice-label">
              {#if c.color}<span class="choice-dot"></span>{/if}{c.label}
            </span>
            {#if c.detail}<span class="choice-detail">{c.detail}</span>{/if}
          </button>
        {/each}
      </div>
    {/if}

    {#if row.length}
      <div class="row">
        {#each row as c (c.value)}
          <button class="btn {c.kind ?? 'ghost'}" on:click={() => dispatch('choose', c.value)}>
            {c.label}
          </button>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 60;
    display: grid;
    place-items: center;
    padding: 1.5rem;
    background: rgba(5, 9, 14, 0.6);
    backdrop-filter: blur(3px);
    -webkit-backdrop-filter: blur(3px);
  }

  .panel {
    width: min(92vw, 440px);
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-pop);
    padding: 1.15rem 1.25rem 1.2rem;
    text-align: left;
  }

  h3 {
    margin: 0;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 1.05rem;
    letter-spacing: -0.01em;
    color: var(--text-strong);
  }

  .msg {
    margin: 0.45rem 0 0;
    color: var(--muted);
    font-size: 0.86rem;
    line-height: 1.5;
  }

  .choices {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin-top: 0.95rem;
  }

  .choice {
    font: inherit;
    display: flex;
    flex-direction: column;
    gap: 0.18rem;
    text-align: left;
    background: var(--inset);
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    padding: 0.65rem 0.8rem;
    cursor: pointer;
    transition: background 0.15s ease, border-color 0.15s ease, transform 0.12s var(--ease);
  }
  .choice:hover {
    transform: translateY(-1px);
  }

  .choice.primary:hover {
    background: var(--accent-soft);
    border-color: var(--accent-line);
  }
  .choice.danger:hover {
    background: var(--red-soft);
    border-color: var(--red-line);
  }

  .choice-label {
    font-weight: 700;
    font-size: 0.9rem;
    color: var(--text-strong);
  }
  .choice.primary .choice-label {
    color: var(--accent-bright);
  }
  .choice.danger .choice-label {
    color: var(--red);
  }

  .choice.tinted:hover {
    background: color-mix(in srgb, var(--choice) 12%, transparent);
    border-color: color-mix(in srgb, var(--choice) 50%, transparent);
  }

  .choice-dot {
    display: inline-block;
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--choice);
    margin-right: 0.5rem;
    box-shadow: 0 0 8px -1px var(--choice);
  }

  .choice-detail {
    font-size: 0.76rem;
    color: var(--muted);
    line-height: 1.4;
  }

  .row {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-top: 1rem;
  }
</style>
