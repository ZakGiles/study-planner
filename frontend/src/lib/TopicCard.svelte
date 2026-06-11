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
    SetTopicColor,
    SetTopicArchived,
  } from '../../wailsjs/go/main/App.js';
  import {
    formatDate,
    relativeLabel,
    sessionStatus,
    todayISO,
    parseIntervals,
    logOffsets,
    spacedPreview,
  } from './dates';
  import { TOPIC_COLORS, topicHex } from './colors';

  export let topic: main.Topic;
  export let allTags: string[] = [];
  export let draggable = false;

  const dispatch = createEventDispatcher<{
    changed: main.Topic[];
    error: string;
    arm: void;
    disarm: void;
  }>();

  let busy = false;
  let showColors = false;

  $: hex = topicHex(topic.color);

  async function pickColor(token: string) {
    showColors = false;
    await run(SetTopicColor(topic.id, token));
  }

  // Editing the topic name/description.
  let editing = false;
  let editName = '';
  let editDescription = '';
  let editTags: string[] = [];
  let tagDraft = '';

  // Adding study dates.
  let addMode: 'manual' | 'spaced' = 'spaced';
  let manualDate = todayISO();
  let spacedStart = todayISO();

  // Spaced repetition supports a fixed interval list or a logarithmic curve.
  let spacedCurve: 'fixed' | 'log' = 'fixed';
  let spacedIntervals = '0, 1, 3, 7, 14, 30';
  let logDilation = 10;
  let logFactor = 1.4;
  let logCount = 6;

  $: offsets =
    spacedCurve === 'fixed'
      ? parseIntervals(spacedIntervals)
      : logOffsets(logDilation, logFactor, logCount);
  // Unique, sorted offsets — what actually gets added (dates are de-duplicated).
  $: uniqueOffsets = Array.from(new Set(offsets)).sort((a, b) => a - b);
  $: preview = spacedPreview(spacedStart, offsets);
  $: doneCount = topic.sessions.filter((s) => s.done).length;
  $: total = topic.sessions.length;
  $: progress = total ? Math.round((doneCount / total) * 100) : 0;

  async function run(p: Promise<main.Topic[]>): Promise<boolean> {
    busy = true;
    try {
      dispatch('changed', await p);
      return true;
    } catch (e) {
      dispatch('error', String(e));
      return false;
    } finally {
      busy = false;
    }
  }

  function startEdit() {
    editName = topic.name;
    editDescription = topic.description;
    editTags = [...topic.tags];
    tagDraft = '';
    editing = true;
  }

  async function saveEdit() {
    if (!editName.trim()) return;
    addTag();
    if (await run(UpdateTopic(topic.id, editName, editDescription, editTags))) editing = false;
  }

  function addTag() {
    const t = tagDraft.trim();
    if (t && !editTags.some((x) => x.toLowerCase() === t.toLowerCase())) {
      editTags = [...editTags, t];
    }
    tagDraft = '';
  }
  function removeTag(t: string) {
    editTags = editTags.filter((x) => x !== t);
  }
  function tagKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      addTag();
    } else if (e.key === 'Backspace' && tagDraft === '' && editTags.length) {
      editTags = editTags.slice(0, -1);
    }
  }

  async function addManual() {
    if (!manualDate) return;
    await run(AddSession(topic.id, manualDate));
  }

  async function generateSpaced() {
    if (!spacedStart || offsets.length === 0) return;
    if (total > 0) {
      const ok = window.confirm(
        `This will clear the ${total} existing study date${total === 1 ? '' : 's'} for “${topic.name}” and replace ${total === 1 ? 'it' : 'them'} with the new schedule.`
      );
      if (!ok) return;
    }
    await run(AddSpacedSessions(topic.id, spacedStart, offsets));
  }
</script>

<article class="card reveal" class:archived={topic.archived} style="--topic:{hex}">
  <header class="card-head">
    {#if editing}
      <div class="edit">
        <input class="edit-name" bind:value={editName} placeholder="Topic name" />
        <textarea class="edit-desc" bind:value={editDescription} rows="2" placeholder="Description"></textarea>
        <div class="tag-edit">
          {#if editTags.length}
            <div class="tag-chips">
              {#each editTags as t}
                <span class="tag removable">{t}<button type="button" class="tag-x" on:click={() => removeTag(t)} aria-label="Remove {t}">×</button></span>
              {/each}
            </div>
          {/if}
          <input
            type="text"
            bind:value={tagDraft}
            on:keydown={tagKeydown}
            on:blur={addTag}
            list="alltags-{topic.id}"
            placeholder="Add tags — press Enter"
          />
          <datalist id="alltags-{topic.id}">
            {#each allTags as t}<option value={t}></option>{/each}
          </datalist>
        </div>
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
        {#if topic.tags.length}
          <div class="tags">
            {#each topic.tags as t}<span class="tag">{t}</span>{/each}
          </div>
        {/if}
      </div>
      <div class="head-actions">
        {#if draggable}
          <button
            class="icon-btn handle"
            title="Drag to reorder"
            aria-label="Drag to reorder"
            on:mousedown={() => dispatch('arm')}
            on:touchstart={() => dispatch('arm')}
            on:mouseup={() => dispatch('disarm')}
          >
            <svg viewBox="0 0 16 16" width="13" height="13" fill="currentColor" aria-hidden="true">
              <circle cx="6" cy="4" r="1.3" /><circle cx="10" cy="4" r="1.3" />
              <circle cx="6" cy="8" r="1.3" /><circle cx="10" cy="8" r="1.3" />
              <circle cx="6" cy="12" r="1.3" /><circle cx="10" cy="12" r="1.3" />
            </svg>
          </button>
        {/if}
        <button class="icon-btn swatch" title="Topic colour" on:click={() => (showColors = !showColors)} disabled={busy}><span class="swatch-dot"></span></button>
        <button class="icon-btn" title={topic.archived ? 'Restore topic' : 'Archive topic'} on:click={() => run(SetTopicArchived(topic.id, !topic.archived))} disabled={busy}>{topic.archived ? '↩️' : '📦'}</button>
        <button class="icon-btn" title="Edit topic" on:click={startEdit} disabled={busy}>✏️</button>
        <button class="icon-btn" title="Delete topic" on:click={() => run(DeleteTopic(topic.id))} disabled={busy}>🗑️</button>
      </div>
    {/if}
  </header>

  {#if showColors}
    <div class="swatches">
      {#each TOPIC_COLORS as c}
        <button
          class="swatch-opt"
          class:active={topic.color === c.token}
          style="--topic:{c.hex}"
          title={c.label}
          aria-label={c.label}
          on:click={() => pickColor(c.token)}
          disabled={busy}
        ></button>
      {/each}
    </div>
  {/if}

  {#if total > 0}
    <div class="progress-row">
      <div class="bar"><div class="fill" style="width:{progress}%"></div></div>
      <span class="progress-label tnum">{doneCount}/{total}</span>
    </div>
    <ul class="sessions">
      {#each topic.sessions as s (s.id)}
        <li class="session {sessionStatus(s.date, s.done)}">
          <label class="chk">
            <input type="checkbox" checked={s.done} on:change={() => run(ToggleSession(topic.id, s.id))} disabled={busy} />
            <span class="date tnum">{formatDate(s.date)}</span>
            <span class="rel tnum">{s.done ? 'done' : relativeLabel(s.date)}</span>
          </label>
          <button class="icon-btn small" title="Remove date" on:click={() => run(DeleteSession(topic.id, s.id))} disabled={busy}>×</button>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="empty-sessions">No study dates yet — add some below.</p>
  {/if}

  <div class="adder">
    <span class="adder-label">Schedule dates</span>
    <div class="seg">
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
      </div>

      <div class="seg curve">
        <button class:active={spacedCurve === 'fixed'} on:click={() => (spacedCurve = 'fixed')}>Fixed intervals</button>
        <button class:active={spacedCurve === 'log'} on:click={() => (spacedCurve = 'log')}>Logarithmic</button>
      </div>

      {#if spacedCurve === 'fixed'}
        <div class="row">
          <label class="field grow">
            <span>Days from start</span>
            <input type="text" bind:value={spacedIntervals} placeholder="0, 1, 3, 7, 14, 30" />
          </label>
          <button class="btn primary" on:click={generateSpaced} disabled={busy || preview.length === 0}>Generate</button>
        </div>
      {:else}
        <div class="row">
          <label class="field num">
            <span>Dilation</span>
            <input type="number" min="0" step="0.5" bind:value={logDilation} />
          </label>
          <label class="field num">
            <span>Factor</span>
            <input type="number" min="0" step="0.1" bind:value={logFactor} />
          </label>
          <label class="field num">
            <span>Sessions</span>
            <input type="number" min="1" max="60" step="1" bind:value={logCount} />
          </label>
          <button class="btn primary" on:click={generateSpaced} disabled={busy || preview.length === 0}>Generate</button>
        </div>
        <p class="hint">offset(n) = dilation × factor<sup>n</sup> × ln(n+1) days from start</p>
      {/if}

      {#if preview.length}
        <p class="preview tnum">→ {preview.length} session{preview.length === 1 ? '' : 's'}: {formatDate(preview[0])}{preview.length > 1 ? ` … ${formatDate(preview[preview.length - 1])}` : ''}</p>
        {#if spacedCurve === 'log'}
          <p class="preview muted tnum">offsets: {uniqueOffsets.join(', ')} days</p>
        {/if}
      {:else if spacedCurve === 'fixed'}
        <p class="preview muted">Enter day offsets, e.g. <code>0, 1, 3, 7, 14, 30</code></p>
      {:else}
        <p class="preview muted">Set a dilation, factor and session count.</p>
      {/if}
    {/if}
  </div>
</article>

<style>
  .card {
    position: relative;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    padding: 1.1rem 1.2rem;
    box-shadow: var(--shadow-1);
    text-align: left;
    transition: border-color 0.18s ease, box-shadow 0.18s ease, transform 0.18s var(--ease);
  }

  .card:hover {
    border-color: var(--border-strong);
    box-shadow: var(--shadow-2);
    transform: translateY(-2px);
  }

  .card.archived {
    opacity: 0.6;
  }

  .card::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
    border-radius: var(--r-lg) 0 0 var(--r-lg);
    background: var(--topic);
    opacity: 0.9;
  }

  .swatch-dot {
    display: block;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    background: var(--topic);
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.25);
  }

  .swatches {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    margin: 0.2rem 0 0.4rem;
  }

  .swatch-opt {
    width: 20px;
    height: 20px;
    padding: 0;
    border: none;
    border-radius: 50%;
    background: var(--topic);
    cursor: pointer;
    transition: transform 0.12s var(--ease), box-shadow 0.15s ease;
  }
  .swatch-opt:hover {
    transform: scale(1.12);
  }
  .swatch-opt.active {
    box-shadow: 0 0 0 2px var(--surface), 0 0 0 4px var(--topic);
  }

  .card-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .title-block h3 {
    margin: 0;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 1.08rem;
    letter-spacing: -0.01em;
    color: var(--text-strong);
  }

  .desc {
    margin: 0.3rem 0 0;
    color: var(--muted);
    font-size: 0.875rem;
    line-height: 1.45;
    white-space: pre-wrap;
  }

  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
    margin-top: 0.5rem;
  }

  .tag {
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--muted);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    padding: 0.1rem 0.45rem;
  }

  .tag.removable {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    color: var(--text);
  }

  .tag-x {
    border: none;
    background: transparent;
    color: var(--muted);
    cursor: pointer;
    padding: 0;
    font-size: 0.95rem;
    line-height: 1;
  }
  .tag-x:hover {
    color: var(--red);
  }

  .tag-edit {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .tag-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }

  .head-actions {
    display: flex;
    gap: 0.25rem;
    flex-shrink: 0;
  }

  .handle {
    cursor: grab;
    touch-action: none;
  }
  .handle:active {
    cursor: grabbing;
  }

  .progress-row {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    margin: 1rem 0 0.6rem;
  }

  .progress-label {
    font-size: 0.74rem;
    color: var(--muted);
    white-space: nowrap;
  }

  .sessions {
    list-style: none;
    margin: 0.5rem 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .session {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0.45rem 0.5rem 0.45rem 0.6rem;
    border-radius: var(--r-sm);
    border: 1px solid var(--border-soft);
    background: var(--surface-2);
    border-left: 2px solid var(--border-strong);
    transition: border-color 0.15s ease, background 0.15s ease;
  }

  .session.upcoming {
    border-left-color: var(--accent);
  }
  .session.overdue {
    border-color: var(--red-line);
    border-left-color: var(--red);
    background: var(--red-soft);
  }
  .session.today {
    border-color: var(--amber-line);
    border-left-color: var(--amber);
    background: var(--amber-soft);
  }
  .session.done {
    opacity: 0.55;
    border-left-color: var(--green);
  }

  .chk {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    cursor: pointer;
    flex: 1;
    min-width: 0;
  }

  .date {
    font-size: 0.86rem;
    color: var(--text-strong);
  }

  .session.done .date {
    text-decoration: line-through;
    color: var(--muted);
  }

  .rel {
    font-size: 0.72rem;
    color: var(--muted);
    margin-left: auto;
    padding-left: 0.5rem;
    white-space: nowrap;
  }

  .session.overdue .rel {
    color: var(--red);
  }
  .session.today .rel {
    color: var(--amber);
    font-weight: 600;
  }

  .empty-sessions {
    margin: 0.9rem 0 0.2rem;
    color: var(--muted);
    font-size: 0.85rem;
  }

  .adder {
    margin-top: 1rem;
    padding-top: 0.95rem;
    border-top: 1px solid var(--border-soft);
  }

  .adder-label {
    display: block;
    font-size: 0.66rem;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--faint);
    margin-bottom: 0.55rem;
  }

  .seg {
    display: inline-flex;
    background: var(--inset);
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    padding: 2px;
    gap: 2px;
  }

  .seg.curve {
    margin: 0.6rem 0;
  }

  .seg button {
    border: none;
    background: transparent;
    color: var(--muted);
    padding: 0.34rem 0.8rem;
    border-radius: var(--r-xs);
    cursor: pointer;
    font-family: var(--font-body);
    font-size: 0.8rem;
    font-weight: 600;
    transition: color 0.15s ease, background 0.15s ease;
  }

  .seg button:hover {
    color: var(--text);
  }

  .seg button.active {
    background: var(--surface-3);
    color: var(--text-strong);
    box-shadow: var(--shadow-1);
  }

  .row {
    display: flex;
    align-items: flex-end;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin-top: 0.55rem;
  }

  .seg + .row,
  .adder > .seg:first-of-type + .row {
    margin-top: 0.6rem;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.68rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--faint);
  }

  .field.grow {
    flex: 1;
    min-width: 150px;
  }

  .field.num {
    width: 92px;
  }

  .field.num input {
    width: 100%;
  }

  .hint {
    margin: 0.4rem 0 0;
    font-size: 0.74rem;
    color: var(--muted);
  }

  .hint sup {
    font-size: 0.6rem;
  }

  .preview {
    margin: 0.6rem 0 0;
    font-size: 0.78rem;
    color: var(--accent-bright);
  }

  .preview.muted {
    color: var(--muted);
  }

  .preview code {
    background: var(--inset);
    border: 1px solid var(--border-soft);
    padding: 0.05rem 0.3rem;
    border-radius: var(--r-xs);
    color: var(--text);
  }

  .edit {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
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
