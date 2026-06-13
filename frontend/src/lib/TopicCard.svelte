<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { main } from '../../wailsjs/go/models';
  import {
    AddSession,
    AddSpacedSessions,
    DeleteSession,
    DeleteTopic,
    GradeSession,
    RescheduleSession,
    ToggleSession,
    UpdateTopic,
    SetTopicAdaptive,
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
    smoothOffsets,
    plural,
  } from './dates';
  import { makeMutator } from './mutate';
  import { TOPIC_COLORS, topicHex } from './colors';
  import ConfirmModal from './ConfirmModal.svelte';
  import type { ModalAction } from './ConfirmModal.svelte';
  import GradeModal from './GradeModal.svelte';

  export let topic: main.Topic;
  export let allTags: string[] = [];
  export let draggable = false;
  // Cross-topic planned-session counts per date, for busy-day warnings.
  export let sessionLoad: Record<string, number> = {};

  const dispatch = createEventDispatcher<{
    changed: main.Topic[];
    error: string;
    arm: void;
    disarm: void;
    filterTag: string;
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

  // Load on each day from *other* topics: this topic's own sessions don't
  // count against its own regenerated schedule.
  $: otherLoad = (() => {
    const m: Record<string, number> = { ...sessionLoad };
    if (!topic.archived) {
      for (const s of topic.sessions) {
        if (!s.done && m[s.date]) m[s.date] -= 1;
      }
    }
    return m;
  })();

  // Days the schedule would land on that already carry 2+ sessions from other
  // topics. rawBusyDays decides whether smoothing is worth offering; busyDays
  // reflects what actually remains busy after smoothing, so the warning clears
  // when the shift resolves every conflict (and persists when it can't).
  let smooth = false;
  $: unsmoothedDates = spacedPreview(spacedStart, uniqueOffsets);
  $: rawBusyDays = unsmoothedDates.filter((d) => (otherLoad[d] ?? 0) >= 2);
  $: effectiveOffsets = smooth ? smoothOffsets(spacedStart, uniqueOffsets, otherLoad) : uniqueOffsets;
  // When not smoothing, effectiveOffsets === uniqueOffsets, so reuse the dates
  // already computed above instead of expanding them a second time.
  $: preview = smooth ? spacedPreview(spacedStart, effectiveOffsets) : unsmoothedDates;
  $: busyDays = preview.filter((d) => (otherLoad[d] ?? 0) >= 2);
  $: doneCount = topic.sessions.filter((s) => s.done).length;
  $: total = topic.sessions.length;
  $: progress = total ? Math.round((doneCount / total) * 100) : 0;

  const run = makeMutator({
    topics: (t) => dispatch('changed', t),
    error: (m) => dispatch('error', m),
    busy: (b) => (busy = b),
  });

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

  // Destructive actions go through an in-app modal. (window.confirm is a no-op
  // in the macOS webview — WKWebView returns false when the host app doesn't
  // implement the JS dialog delegate, which Wails doesn't.)
  let confirmKind: 'generate' | 'delete' | null = null;

  $: confirmTitle = confirmKind === 'delete' ? `Delete “${topic.name}”?` : 'Topic already has study dates';
  $: confirmMessage =
    confirmKind === 'delete'
      ? total > 0
        ? `This permanently removes the topic and its ${total} study date${plural(total)}.`
        : 'This permanently removes the topic.'
      : `“${topic.name}” has ${total} study date${plural(total)}. What should the new schedule do with ${total === 1 ? 'it' : 'them'}?`;
  $: confirmActions =
    confirmKind === 'delete'
      ? ([
          { value: 'delete', label: 'Delete topic', kind: 'danger' },
          { value: 'cancel', label: 'Cancel', kind: 'ghost' },
        ] as ModalAction[])
      : ([
          {
            value: 'merge',
            label: 'Keep both',
            kind: 'primary',
            detail: 'Add the new dates alongside the current ones — days already scheduled stay as they are.',
          },
          {
            value: 'replace',
            label: 'Replace schedule',
            kind: 'danger',
            detail: `Clear the ${total} current date${plural(total)} (including completed ones) and start fresh.`,
          },
          { value: 'cancel', label: 'Cancel', kind: 'ghost' },
        ] as ModalAction[]);

  function requestGenerate() {
    if (!spacedStart || effectiveOffsets.length === 0) return;
    if (total === 0) {
      void run(AddSpacedSessions(topic.id, spacedStart, effectiveOffsets, true));
      return;
    }
    confirmKind = 'generate';
  }

  function onConfirmChoose(e: CustomEvent<string>) {
    const kind = confirmKind;
    confirmKind = null;
    const choice = e.detail;
    if (kind === 'generate' && (choice === 'merge' || choice === 'replace')) {
      void run(AddSpacedSessions(topic.id, spacedStart, effectiveOffsets, choice === 'replace'));
    } else if (kind === 'delete' && choice === 'delete') {
      void run(DeleteTopic(topic.id));
    }
  }

  // Adaptive topics grade each completed review instead of a plain check-off;
  // the grade re-spaces the remaining schedule.
  let gradeSid: string | null = null;

  function sessionCheckClick(e: Event, s: main.Session) {
    if (!topic.adaptive || s.done) return; // unchecking stays a plain toggle
    e.preventDefault();
    gradeSid = s.id;
  }

  function onGrade(e: CustomEvent<string>) {
    const sid = gradeSid;
    gradeSid = null;
    if (sid) void run(GradeSession(topic.id, sid, e.detail));
  }

  // Status → Tailwind utilities for a session row and its relative-time label.
  const sessionClass = (status: string) =>
    status === 'overdue' ? '!border-red-line !border-l-red !bg-red-soft'
    : status === 'today' ? '!border-amber-line !border-l-amber !bg-amber-soft'
    : status === 'done' ? 'opacity-[0.55] !border-l-green'
    : status === 'upcoming' ? '!border-l-accent'
    : '';
  const relColor = (status: string) =>
    status === 'overdue' ? 'text-red'
    : status === 'today' ? 'text-amber font-semibold'
    : 'text-fg-muted';
</script>

<article
  class="relative rounded-lg border border-line bg-surface px-[1.2rem] py-[1.1rem] text-left shadow-1 transition-[border-color,box-shadow] hover:border-line-strong hover:shadow-2 {topic.archived ? 'opacity-60' : ''}"
  style="--topic:{hex}"
>
  <span class="absolute bottom-0 left-0 top-0 w-[3px] rounded-l-lg bg-[var(--topic)] opacity-90" aria-hidden="true"></span>
  <header class="flex items-start justify-between gap-[0.75rem]">
    {#if editing}
      <div class="flex w-full flex-col gap-[0.5rem]">
        <input class="w-full" type="text" bind:value={editName} placeholder="Topic name" />
        <textarea class="w-full" bind:value={editDescription} rows="2" placeholder="Description"></textarea>
        <div class="flex flex-col gap-[0.4rem]">
          {#if editTags.length}
            <div class="flex flex-wrap gap-[0.3rem]">
              {#each editTags as t}
                <span class="inline-flex items-center gap-[0.25rem] rounded-sm border border-line bg-surface-2 px-[0.45rem] py-[0.1rem] text-[0.7rem] font-semibold text-fg">{t}<button type="button" class="cursor-pointer border-none bg-transparent p-0 text-[0.95rem] leading-none text-fg-muted hover:text-red" on:click={() => removeTag(t)} aria-label="Remove {t}">×</button></span>
              {/each}
            </div>
          {/if}
          <input
            type="text"
            bind:value={tagDraft}
            on:keydown={tagKeydown}
            list="alltags-{topic.id}"
            placeholder="Add tags — press Enter"
          />
          <datalist id="alltags-{topic.id}">
            {#each allTags as t}<option value={t}></option>{/each}
          </datalist>
        </div>
        <div class="flex gap-[0.4rem]">
          <button class="btn primary" on:click={saveEdit} disabled={busy || !editName.trim()}>Save</button>
          <button class="btn ghost" on:click={() => (editing = false)} disabled={busy}>Cancel</button>
        </div>
      </div>
    {:else}
      <div>
        <h3 class="m-0 font-display text-[1.08rem] font-bold tracking-[-0.01em] text-fg-strong">{topic.name}</h3>
        {#if topic.description}
          <p class="mt-[0.3rem] whitespace-pre-wrap text-[0.875rem] leading-[1.45] text-fg-muted">{topic.description}</p>
        {/if}
        {#if topic.tags.length}
          <div class="mt-[0.5rem] flex flex-wrap gap-[0.3rem]">
            {#each topic.tags as t}
              <button class="cursor-pointer rounded-sm border border-line bg-surface-2 px-[0.45rem] py-[0.1rem] text-[0.7rem] font-semibold text-fg-muted transition-colors hover:border-accent-line hover:text-fg-strong" title="Filter by “{t}”" on:click={() => dispatch('filterTag', t)}>{t}</button>
            {/each}
          </div>
        {/if}
      </div>
      <div class="flex shrink-0 gap-[0.25rem]">
        {#if draggable}
          <button
            class="icon-btn cursor-grab touch-none active:cursor-grabbing"
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
        <button class="icon-btn" title="Topic colour" on:click={() => (showColors = !showColors)} disabled={busy}><span class="block h-[14px] w-[14px] rounded-full bg-[var(--topic)] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.25)]"></span></button>
        <button class="icon-btn" title={topic.archived ? 'Restore topic' : 'Archive topic'} on:click={() => run(SetTopicArchived(topic.id, !topic.archived))} disabled={busy}>{topic.archived ? '⤒' : '⤓'}</button>
        <button class="icon-btn" title="Edit topic" on:click={startEdit} disabled={busy}>✎</button>
        <button class="icon-btn" title="Delete topic" on:click={() => (confirmKind = 'delete')} disabled={busy}>✕</button>
      </div>
    {/if}
  </header>

  {#if showColors}
    <div class="mb-[0.4rem] mt-[0.2rem] flex flex-wrap gap-[0.4rem]">
      {#each TOPIC_COLORS as c}
        <button
          class="h-5 w-5 cursor-pointer rounded-full border-none bg-[var(--topic)] p-0 transition-transform hover:scale-110 {topic.color === c.token ? 'shadow-[0_0_0_2px_var(--surface),0_0_0_4px_var(--topic)]' : ''}"
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
    <div class="mb-[0.6rem] mt-4 flex items-center gap-[0.7rem]">
      <div class="bar"><div class="fill" style="width:{progress}%"></div></div>
      <span class="tnum whitespace-nowrap text-[0.74rem] text-fg-muted">{doneCount}/{total}</span>
    </div>
    <ul class="m-0 mt-[0.5rem] flex list-none flex-col gap-[0.35rem] p-0">
      {#each topic.sessions as s (s.id)}
        <li class="chk-row flex items-center justify-between gap-2 rounded-sm border border-line-soft border-l-2 border-l-line-strong bg-surface-2 py-[0.45rem] pl-[0.6rem] pr-[0.5rem] transition-colors {sessionClass(sessionStatus(s.date, s.done))}">
          <label class="flex min-w-0 flex-1 cursor-pointer items-center gap-[0.55rem]">
            <input
              type="checkbox"
              checked={s.done}
              on:click={(e) => sessionCheckClick(e, s)}
              on:change={() => run(ToggleSession(topic.id, s.id))}
              disabled={busy}
            />
            <span class="tnum text-[0.86rem] {s.done ? 'text-fg-muted line-through' : 'text-fg-strong'}">{formatDate(s.date)}</span>
            <span class="tnum ml-auto whitespace-nowrap pl-2 text-[0.72rem] {relColor(sessionStatus(s.date, s.done))}">{s.done ? 'done' : relativeLabel(s.date)}</span>
          </label>
          {#if sessionStatus(s.date, s.done) === 'overdue'}
            <button
              class="icon-btn small"
              title="Move to today"
              on:click={() => run(RescheduleSession(topic.id, s.id, todayISO()))}
              disabled={busy}
            >↷</button>
          {/if}
          <button class="icon-btn small" title="Remove date" on:click={() => run(DeleteSession(topic.id, s.id))} disabled={busy}>×</button>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="mb-[0.2rem] mt-[0.9rem] text-[0.85rem] text-fg-muted">No study dates yet — add some below.</p>
  {/if}

  <div class="mt-4 border-t border-line-soft pt-[0.95rem]">
    <div class="mb-[0.55rem] flex items-center justify-between gap-[0.6rem]">
      <span class="block text-[0.66rem] font-bold uppercase tracking-[0.12em] text-fg-faint">Schedule dates</span>
      <label class="inline-flex cursor-pointer items-center gap-[0.35rem] text-[0.74rem] font-semibold text-fg-muted hover:text-fg" title="Grade each review (Again / Hard / Good / Easy) and let the schedule adapt">
        <input
          type="checkbox"
          checked={topic.adaptive}
          disabled={busy}
          on:change={() => run(SetTopicAdaptive(topic.id, !topic.adaptive))}
        />
        <span>Adaptive</span>
      </label>
    </div>
    <div class="seg">
      <button class:active={addMode === 'spaced'} on:click={() => (addMode = 'spaced')}>Spaced</button>
      <button class:active={addMode === 'manual'} on:click={() => (addMode = 'manual')}>Manual</button>
    </div>

    {#if addMode === 'manual'}
      <div class="mt-[0.6rem] flex flex-wrap items-end gap-2">
        <input type="date" bind:value={manualDate} />
        <button class="btn primary" on:click={addManual} disabled={busy || !manualDate}>Add date</button>
      </div>
      {#if manualDate && topic.sessions.some((s) => s.date === manualDate)}
        <p class="muted mt-[0.6rem] text-[0.78rem]">Already scheduled on this day for this topic.</p>
      {:else if manualDate && (otherLoad[manualDate] ?? 0) >= 2}
        <p class="mt-[0.55rem] text-[0.76rem] leading-[1.45] text-amber">This day already has {otherLoad[manualDate]} sessions across topics.</p>
      {/if}
    {:else}
      <div class="mt-[0.6rem] flex flex-wrap items-end gap-2">
        <label class="flex flex-col gap-[0.25rem] text-[0.68rem] font-semibold uppercase tracking-[0.04em] text-fg-faint">
          <span>Start</span>
          <input type="date" bind:value={spacedStart} />
        </label>
      </div>

      <div class="seg my-[0.6rem]">
        <button class:active={spacedCurve === 'fixed'} on:click={() => (spacedCurve = 'fixed')}>Fixed intervals</button>
        <button class:active={spacedCurve === 'log'} on:click={() => (spacedCurve = 'log')}>Logarithmic</button>
      </div>

      {#if spacedCurve === 'fixed'}
        <div class="mt-[0.6rem] flex flex-wrap items-end gap-2">
          <label class="flex min-w-[150px] flex-1 flex-col gap-[0.25rem] text-[0.68rem] font-semibold uppercase tracking-[0.04em] text-fg-faint">
            <span>Days from start</span>
            <input type="text" bind:value={spacedIntervals} placeholder="0, 1, 3, 7, 14, 30" />
          </label>
          <button class="btn primary" on:click={requestGenerate} disabled={busy || preview.length === 0}>Generate</button>
        </div>
      {:else}
        <div class="mt-[0.6rem] flex flex-wrap items-end gap-2">
          <label class="flex w-[92px] flex-col gap-[0.25rem] text-[0.68rem] font-semibold uppercase tracking-[0.04em] text-fg-faint">
            <span>Dilation</span>
            <input class="w-full" type="number" min="0" step="0.5" bind:value={logDilation} />
          </label>
          <label class="flex w-[92px] flex-col gap-[0.25rem] text-[0.68rem] font-semibold uppercase tracking-[0.04em] text-fg-faint">
            <span>Factor</span>
            <input class="w-full" type="number" min="0" step="0.1" bind:value={logFactor} />
          </label>
          <label class="flex w-[92px] flex-col gap-[0.25rem] text-[0.68rem] font-semibold uppercase tracking-[0.04em] text-fg-faint">
            <span>Sessions</span>
            <input class="w-full" type="number" min="1" max="60" step="1" bind:value={logCount} />
          </label>
          <button class="btn primary" on:click={requestGenerate} disabled={busy || preview.length === 0}>Generate</button>
        </div>
        <p class="mt-[0.4rem] text-[0.74rem] text-fg-muted">offset(n) = dilation × factor<sup class="text-[0.6rem]">n</sup> × ln(n+1) days from start</p>
      {/if}

      {#if preview.length}
        <p class="tnum mt-[0.6rem] text-[0.78rem] text-accent-bright">→ {preview.length} session{plural(preview.length)}: {formatDate(preview[0])}{preview.length > 1 ? ` … ${formatDate(preview[preview.length - 1])}` : ''}</p>
        {#if spacedCurve === 'log'}
          <p class="muted tnum mt-[0.6rem] text-[0.78rem]">offsets: {effectiveOffsets.join(', ')} days</p>
        {/if}
        {#if rawBusyDays.length}
          <p class="mt-[0.55rem] text-[0.76rem] leading-[1.45] {smooth && busyDays.length === 0 ? 'text-green' : 'text-amber'}">
            {#if smooth && busyDays.length === 0}
              Shifted off {rawBusyDays.length === 1 ? 'a busy day' : `${rawBusyDays.length} busy days`}. ✓
            {:else if busyDays.length === 1}
              {formatDate(busyDays[0])} still has 2+ sessions across topics.
            {:else}
              {busyDays.length} of these days still have 2+ sessions across topics.
            {/if}
            <label class="ml-[0.45rem] inline-flex cursor-pointer items-center gap-[0.3rem] font-semibold text-fg hover:text-fg-strong">
              <input type="checkbox" bind:checked={smooth} />
              <span>shift busy days</span>
            </label>
          </p>
        {/if}
      {:else if spacedCurve === 'fixed'}
        <p class="muted mt-[0.6rem] text-[0.78rem]">Enter day offsets, e.g. <code class="rounded-xs border border-line-soft bg-inset px-[0.3rem] py-[0.05rem] text-fg">0, 1, 3, 7, 14, 30</code></p>
      {:else}
        <p class="muted mt-[0.6rem] text-[0.78rem]">Set a dilation, factor and session count.</p>
      {/if}
    {/if}
  </div>
</article>

<!-- Siblings of the card, not children: a transform on .card (e.g. from a
     hover effect) would turn the modals' position:fixed into card-relative
     positioning, so they stay outside it. -->
{#if confirmKind}
  <ConfirmModal title={confirmTitle} message={confirmMessage} actions={confirmActions} on:choose={onConfirmChoose} />
{/if}
{#if gradeSid}
  <GradeModal topicName={topic.name} on:grade={onGrade} on:cancel={() => (gradeSid = null)} />
{/if}

