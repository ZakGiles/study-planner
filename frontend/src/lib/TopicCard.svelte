<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { main } from '../../wailsjs/go/models';
  import {
    AddSession,
    AddSpacedSessions,
    DeleteSession,
    DeleteTopic,
    ToggleSession,
    UpdateTopic,
  } from '../../wailsjs/go/main/App.js';
  import {
    formatDate,
    relativeLabel,
    daysFromToday,
    todayISO,
    parseIntervals,
    spacedPreview,
  } from './dates';

  export let topic: main.Topic;

  const dispatch = createEventDispatcher<{ changed: main.Topic[]; error: string }>();

  let busy = false;

  // Editing the topic name/description.
  let editing = false;
  let editName = '';
  let editDescription = '';

  // Adding study dates.
  let addMode: 'manual' | 'spaced' = 'spaced';
  let manualDate = todayISO();
  let spacedStart = todayISO();
  let spacedIntervals = '0, 1, 3, 7, 14, 30';

  $: intervals = parseIntervals(spacedIntervals);
  $: preview = spacedPreview(spacedStart, intervals);
  $: doneCount = topic.sessions.filter((s) => s.done).length;
  $: total = topic.sessions.length;
  $: progress = total ? Math.round((doneCount / total) * 100) : 0;

  async function run(p: Promise<main.Topic[]>) {
    busy = true;
    try {
      dispatch('changed', await p);
    } catch (e) {
      dispatch('error', String(e));
    } finally {
      busy = false;
    }
  }

  function startEdit() {
    editName = topic.name;
    editDescription = topic.description;
    editing = true;
  }

  async function saveEdit() {
    if (!editName.trim()) return;
    await run(UpdateTopic(topic.id, editName, editDescription));
    editing = false;
  }

  async function addManual() {
    if (!manualDate) return;
    await run(AddSession(topic.id, manualDate));
  }

  async function generateSpaced() {
    if (!spacedStart) return;
    await run(AddSpacedSessions(topic.id, spacedStart, intervals));
  }

  function sessionState(date: string, done: boolean): string {
    if (done) return 'done';
    const n = daysFromToday(date);
    if (n < 0) return 'overdue';
    if (n === 0) return 'today';
    return 'upcoming';
  }
</script>

<article class="card">
  <header class="card-head">
    {#if editing}
      <div class="edit">
        <input class="edit-name" bind:value={editName} placeholder="Topic name" />
        <textarea class="edit-desc" bind:value={editDescription} rows="2" placeholder="Description"></textarea>
        <div class="edit-actions">
          <button class="btn primary" on:click={saveEdit} disabled={busy || !editName.trim()}>Save</button>
          <button class="btn ghost" on:click={() => (editing = false)} disabled={busy}>Cancel</button>
        </div>
      </div>
    {:else}
      <div class="title-block">
        <h3>{topic.name}</h3>
        {#if topic.description}
          <p class="desc">{topic.description}</p>
        {/if}
      </div>
      <div class="head-actions">
        <button class="icon-btn" title="Edit topic" on:click={startEdit} disabled={busy}>✏️</button>
        <button class="icon-btn" title="Delete topic" on:click={() => run(DeleteTopic(topic.id))} disabled={busy}>🗑️</button>
      </div>
    {/if}
  </header>

  {#if total > 0}
    <div class="progress-row">
      <div class="bar"><div class="fill" style="width:{progress}%"></div></div>
      <span class="progress-label">{doneCount}/{total} done</span>
    </div>
    <ul class="sessions">
      {#each topic.sessions as s (s.id)}
        <li class="session {sessionState(s.date, s.done)}">
          <label class="chk">
            <input type="checkbox" checked={s.done} on:change={() => run(ToggleSession(topic.id, s.id))} disabled={busy} />
            <span class="date">{formatDate(s.date)}</span>
            <span class="rel">{s.done ? 'done' : relativeLabel(s.date)}</span>
          </label>
          <button class="icon-btn small" title="Remove date" on:click={() => run(DeleteSession(topic.id, s.id))} disabled={busy}>×</button>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="empty-sessions">No study dates yet — add some below.</p>
  {/if}

  <div class="adder">
    <div class="mode-toggle">
      <button class:active={addMode === 'spaced'} on:click={() => (addMode = 'spaced')}>Spaced</button>
      <button class:active={addMode === 'manual'} on:click={() => (addMode = 'manual')}>Manual</button>
    </div>

    {#if addMode === 'manual'}
      <div class="row">
        <input type="date" bind:value={manualDate} />
        <button class="btn primary" on:click={addManual} disabled={busy || !manualDate}>Add date</button>
      </div>
    {:else}
      <div class="row">
        <label class="field">
          <span>Start</span>
          <input type="date" bind:value={spacedStart} />
        </label>
        <label class="field grow">
          <span>Days from start</span>
          <input type="text" bind:value={spacedIntervals} placeholder="0, 1, 3, 7, 14, 30" />
        </label>
        <button class="btn primary" on:click={generateSpaced} disabled={busy || preview.length === 0}>Generate</button>
      </div>
      {#if preview.length}
        <p class="preview">→ {preview.length} session{preview.length === 1 ? '' : 's'}: {formatDate(preview[0])}{preview.length > 1 ? ` … ${formatDate(preview[preview.length - 1])}` : ''}</p>
      {:else}
        <p class="preview muted">Enter day offsets, e.g. <code>0, 1, 3, 7, 14, 30</code></p>
      {/if}
    {/if}
  </div>
</article>

<style>
  .card {
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: 14px;
    padding: 1rem 1.1rem;
    box-shadow: 0 1px 2px rgba(16, 24, 40, 0.04);
    text-align: left;
  }

  .card-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .title-block h3 {
    margin: 0;
    font-size: 1.05rem;
    color: var(--text);
  }

  .desc {
    margin: 0.25rem 0 0;
    color: var(--muted);
    font-size: 0.88rem;
    line-height: 1.35;
    white-space: pre-wrap;
  }

  .head-actions {
    display: flex;
    gap: 0.25rem;
    flex-shrink: 0;
  }

  .progress-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin: 0.9rem 0 0.4rem;
  }

  .bar {
    flex: 1;
    height: 6px;
    background: var(--border);
    border-radius: 99px;
    overflow: hidden;
  }

  .fill {
    height: 100%;
    background: var(--accent);
    border-radius: 99px;
    transition: width 0.2s ease;
  }

  .progress-label {
    font-size: 0.75rem;
    color: var(--muted);
    white-space: nowrap;
  }

  .sessions {
    list-style: none;
    margin: 0.4rem 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .session {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.4rem 0.55rem;
    border-radius: 9px;
    border: 1px solid var(--border);
    background: var(--chip);
  }

  .session.overdue {
    border-color: #fcaca7;
    background: #fef2f1;
  }
  .session.today {
    border-color: #fcd9a3;
    background: #fff7ec;
  }
  .session.done {
    opacity: 0.6;
  }

  .chk {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
    flex: 1;
    min-width: 0;
  }

  .chk input {
    width: 16px;
    height: 16px;
    accent-color: var(--accent);
    cursor: pointer;
  }

  .date {
    font-size: 0.86rem;
    color: var(--text);
  }

  .session.done .date {
    text-decoration: line-through;
  }

  .rel {
    font-size: 0.72rem;
    color: var(--muted);
    margin-left: auto;
    padding-left: 0.5rem;
    white-space: nowrap;
  }

  .session.overdue .rel {
    color: var(--danger);
  }
  .session.today .rel {
    color: var(--warn);
    font-weight: 600;
  }

  .empty-sessions {
    margin: 0.8rem 0 0.2rem;
    color: var(--muted);
    font-size: 0.85rem;
  }

  .adder {
    margin-top: 0.9rem;
    padding-top: 0.85rem;
    border-top: 1px dashed var(--border);
  }

  .mode-toggle {
    display: inline-flex;
    background: var(--chip);
    border: 1px solid var(--border);
    border-radius: 9px;
    padding: 2px;
    margin-bottom: 0.6rem;
  }

  .mode-toggle button {
    border: none;
    background: transparent;
    color: var(--muted);
    padding: 0.3rem 0.8rem;
    border-radius: 7px;
    cursor: pointer;
    font-size: 0.82rem;
  }

  .mode-toggle button.active {
    background: var(--card);
    color: var(--text);
    box-shadow: 0 1px 2px rgba(16, 24, 40, 0.08);
  }

  .row {
    display: flex;
    align-items: flex-end;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    font-size: 0.72rem;
    color: var(--muted);
  }

  .field.grow {
    flex: 1;
    min-width: 140px;
  }

  .preview {
    margin: 0.5rem 0 0;
    font-size: 0.78rem;
    color: var(--accent);
  }

  .preview.muted {
    color: var(--muted);
  }

  .preview code {
    background: var(--chip);
    padding: 0 4px;
    border-radius: 4px;
  }

  .edit {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    width: 100%;
  }

  .edit-name,
  .edit-desc {
    width: 100%;
  }

  .edit-actions {
    display: flex;
    gap: 0.4rem;
  }
</style>
