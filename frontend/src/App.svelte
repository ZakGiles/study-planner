<script lang="ts">
  import { onMount } from 'svelte';
  import { fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import type { main } from '../wailsjs/go/models';
  import {
    GetTopics,
    AddTopic,
    ToggleSession,
    ReorderTopics,
    RescheduleOverdueSessions,
    GradeSession,
  } from '../wailsjs/go/main/App.js';
  import TopicCard from './lib/TopicCard.svelte';
  import Calendar from './lib/Calendar.svelte';
  import Stats from './lib/Stats.svelte';
  import GradeModal from './lib/GradeModal.svelte';
  import { openModalCount } from './lib/ConfirmModal.svelte';
  import { makeMutator } from './lib/mutate';
  import { formatDate, relativeLabel, daysFromToday, sessionStatus, plural } from './lib/dates';
  import { topicHex } from './lib/colors';
  import { dndzone } from 'svelte-dnd-action';
  import type { DndEvent } from 'svelte-dnd-action';
  import { flip } from 'svelte/animate';

  let topics: main.Topic[] = [];
  let activeTab: 'topics' | 'agenda' | 'calendar' | 'stats' = 'topics';
  let loading = true;
  let errorMsg = '';
  let errorTimer: ReturnType<typeof setTimeout>;

  let newName = '';
  let newDescription = '';
  let adding = false;

  // Theme is a pure UI preference, persisted locally rather than in the store.
  let theme: 'dark' | 'light' = localStorage.getItem('theme') === 'light' ? 'light' : 'dark';
  $: {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem('theme', theme);
  }

  // Keyboard shortcuts: n → new topic, / → search (when not already typing).
  let nameInput: HTMLInputElement;
  let searchInput: HTMLInputElement;

  function isTyping(): boolean {
    const el = document.activeElement;
    return (
      el instanceof HTMLInputElement ||
      el instanceof HTMLTextAreaElement ||
      el instanceof HTMLSelectElement
    );
  }

  function onKeydown(e: KeyboardEvent) {
    // $openModalCount covers every modal in the app, including ones owned by
    // child components — shortcuts must not steal focus from behind an overlay.
    if (e.metaKey || e.ctrlKey || e.altKey || isTyping() || $openModalCount > 0) return;
    if (activeTab !== 'topics') return;
    if (e.key === 'n') {
      e.preventDefault();
      nameInput?.focus();
    } else if (e.key === '/') {
      e.preventDefault();
      searchInput?.focus();
    }
  }

  // apply awaits a backend call, swaps in the returned topic list and toasts
  // failures; resolves to whether it succeeded.
  const apply = makeMutator({ topics: (t) => (topics = t), error: showError });

  onMount(async () => {
    await apply(GetTopics());
    loading = false;
  });

  function showError(msg: string) {
    errorMsg = msg;
    clearTimeout(errorTimer);
    errorTimer = setTimeout(() => (errorMsg = ''), 5000);
  }

  function onChanged(e: CustomEvent<main.Topic[]>) {
    topics = e.detail;
  }

  function onError(e: CustomEvent<string>) {
    showError(e.detail);
  }

  async function createTopic() {
    if (!newName.trim()) return;
    adding = true;
    if (await apply(AddTopic(newName, newDescription))) {
      newName = '';
      newDescription = '';
    }
    adding = false;
  }

  // Guard against double-clicks per session: a second toggle for the SAME
  // session while the first is in flight would flip it straight back. Tracked
  // per id so toggling one session doesn't freeze the rest of the agenda.
  let agendaBusy: Record<string, boolean> = {};
  async function toggleFromAgenda(topicId: string, sessionId: string) {
    if (agendaBusy[sessionId]) return;
    agendaBusy = { ...agendaBusy, [sessionId]: true };
    await apply(ToggleSession(topicId, sessionId));
    const { [sessionId]: _, ...rest } = agendaBusy;
    agendaBusy = rest;
  }

  // Sessions of adaptive topics are graded instead of plainly checked off; the
  // grade re-spaces the remaining schedule.
  let gradeTarget: { topicId: string; sessionId: string; topicName: string } | null = null;

  function agendaCheckClick(e: Event, item: AgendaItem) {
    if (!item.adaptive) return; // plain toggle proceeds via on:change
    e.preventDefault();
    gradeTarget = { topicId: item.topicId, sessionId: item.sessionId, topicName: item.topicName };
  }

  // No in-flight guard here: grading the same session twice is impossible (the
  // modal unmounts on the first choice) and concurrent grades of different
  // sessions are safe — a guard would only drop a grade silently.
  async function onGrade(e: CustomEvent<string>) {
    const target = gradeTarget;
    gradeTarget = null;
    if (target) await apply(GradeSession(target.topicId, target.sessionId, e.detail));
  }

  // One-click catch-up: every overdue session moves to today.
  let catchingUp = false;
  async function catchUpOverdue() {
    if (catchingUp) return;
    catchingUp = true;
    await apply(RescheduleOverdueSessions());
    catchingUp = false;
  }

  // Organisation: search text, selected tags and whether archived topics show.
  let search = '';
  let selectedTags: string[] = [];
  let showArchived = false;

  function toggleTag(t: string) {
    selectedTags = selectedTags.includes(t)
      ? selectedTags.filter((x) => x !== t)
      : [...selectedTags, t];
  }

  $: allTags = Array.from(new Set(topics.flatMap((t) => t.tags))).sort((a, b) => a.localeCompare(b));
  $: archivedCount = topics.filter((t) => t.archived).length;

  // A topic matches when the search text hits its name/description/tags and it
  // carries at least one selected tag (when any tags are selected).
  $: matches = (t: main.Topic) => {
    const q = search.trim().toLowerCase();
    const okSearch =
      !q ||
      t.name.toLowerCase().includes(q) ||
      t.description.toLowerCase().includes(q) ||
      t.tags.some((tag) => tag.toLowerCase().includes(q));
    const okTags = selectedTags.length === 0 || t.tags.some((tag) => selectedTags.includes(tag));
    return okSearch && okTags;
  };

  // Active (non-archived) topics that pass the filter feed every view; archived
  // topics surface only in their own section.
  $: visibleActive = topics.filter((t) => !t.archived && matches(t));
  $: visibleArchived = topics.filter((t) => t.archived && matches(t));

  // Drag-to-reorder operates on the unfiltered active list only.
  $: hasFilter = search.trim() !== '' || selectedTags.length > 0;
  let dndItems: main.Topic[] = [];
  $: dndItems = visibleActive;

  // Dragging is armed only while a card's drag handle is held (and never while
  // filtered), so it never hijacks text selection inside a card.
  let dragDisabled = true;
  function armDrag() {
    if (!hasFilter) dragDisabled = false;
  }
  function disarmDrag() {
    dragDisabled = true;
  }

  function handleConsider(e: CustomEvent<DndEvent<main.Topic>>) {
    dndItems = e.detail.items;
  }
  async function handleFinalize(e: CustomEvent<DndEvent<main.Topic>>) {
    dndItems = e.detail.items;
    dragDisabled = true;
    if (!(await apply(ReorderTopics(dndItems.map((t) => t.id))))) {
      dndItems = visibleActive; // revert the optimistic order
    }
  }

  // Flattened, date-sorted list of incomplete sessions for the agenda view.
  type AgendaItem = {
    topicId: string;
    topicName: string;
    sessionId: string;
    date: string;
    topicColor: string;
    adaptive: boolean;
  };

  function agendaItems(from: main.Topic[], done: boolean): AgendaItem[] {
    return from.flatMap((t) =>
      t.sessions
        .filter((s) => s.done === done)
        .map((s) => ({
          topicId: t.id,
          topicName: t.name,
          sessionId: s.id,
          date: s.date,
          topicColor: t.color,
          adaptive: t.adaptive,
        }))
    );
  }

  $: agenda = agendaItems(visibleActive, false).sort((a, b) => a.date.localeCompare(b.date));

  // Completed sessions, newest first — shown on demand below the agenda.
  let showPast = false;
  $: pastAgenda = agendaItems(visibleActive, true).sort((a, b) => b.date.localeCompare(a.date));

  // Cross-topic scheduling load (date → planned sessions), used by the cards to
  // warn when a generated schedule would pile onto already-busy days.
  $: sessionLoad = (() => {
    const m: Record<string, number> = {};
    for (const t of topics) {
      if (t.archived) continue;
      for (const s of t.sessions) {
        if (!s.done) m[s.date] = (m[s.date] ?? 0) + 1;
      }
    }
    return m;
  })();

  $: overdueCount = agenda.filter((a) => daysFromToday(a.date) < 0).length;
  $: totalSessions = visibleActive.reduce((n, t) => n + t.sessions.length, 0);
  $: doneSessions = visibleActive.reduce((n, t) => n + t.sessions.filter((s) => s.done).length, 0);
  $: overallPct = totalSessions ? Math.round((doneSessions / totalSessions) * 100) : 0;

  // Group date-sorted agenda items by date for display.
  function groupByDate(items: AgendaItem[]) {
    const groups: { date: string; items: AgendaItem[] }[] = [];
    for (const item of items) {
      const last = groups[groups.length - 1];
      if (last && last.date === item.date) last.items.push(item);
      else groups.push({ date: item.date, items: [item] });
    }
    return groups;
  }

  $: agendaGroups = groupByDate(agenda);
  $: pastGroups = groupByDate(pastAgenda);

  // Per-view header copy for the sticky content header.
  const TAB_META = {
    topics: { title: 'Topics', sub: "Everything you're revising" },
    agenda: { title: 'Agenda', sub: "What's coming up next" },
    calendar: { title: 'Calendar', sub: 'Your month at a glance' },
    stats: { title: 'Stats', sub: 'Progress and streaks' },
  } as const;
  $: tabMeta = TAB_META[activeTab];

  const NAV = [
    { id: 'topics', label: 'Topics' },
    { id: 'agenda', label: 'Agenda' },
    { id: 'calendar', label: 'Calendar' },
    { id: 'stats', label: 'Stats' },
  ] as const;

  // Status → Tailwind colour utilities for the agenda day cards.
  const barClass = (status: string) =>
    status === 'overdue' ? 'bg-red'
    : status === 'today' ? 'bg-amber'
    : status === 'past' ? 'bg-green'
    : status === 'upcoming' ? 'bg-accent'
    : 'bg-line-strong';
  const relClass = (status: string) =>
    status === 'overdue' ? 'text-red'
    : status === 'today' ? 'text-amber font-semibold'
    : 'text-fg-muted';
</script>

<svelte:window on:keydown={onKeydown} />

<div class="flex h-full min-h-0">
  <aside class="flex flex-[0_0_232px] flex-col gap-[0.4rem] bg-sidebar px-[0.85rem] pb-4 pt-[1.15rem] border-r border-line max-[720px]:flex-[0_0_60px] max-[720px]:items-center max-[720px]:px-2">
    <div class="flex items-center gap-[0.65rem] px-[0.45rem] pt-[0.2rem] pb-[1.15rem] max-[720px]:px-0">
      <span class="grid h-[34px] w-[34px] shrink-0 place-items-center rounded-[9px] bg-[var(--accent-grad)] [&_svg]:h-5 [&_svg]:w-5" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none">
          <path
            d="M3 18.5C6.5 18 8.5 13.5 12 10.5 15.5 7.5 18 5.6 21 5.2"
            stroke="white"
            stroke-width="2"
            stroke-linecap="round"
          />
          <circle cx="3" cy="18.5" r="2" fill="white" />
          <circle cx="11.4" cy="11.1" r="2" fill="white" />
          <circle cx="21" cy="5.2" r="2" fill="white" />
        </svg>
      </span>
      <div class="min-w-0 max-[720px]:hidden">
        <span class="block whitespace-nowrap text-[0.57rem] font-bold uppercase tracking-[0.16em] text-accent-bright">Spaced repetition</span>
        <h1 class="m-0 mt-0.5 whitespace-nowrap font-display text-[1.16rem] font-extrabold leading-[1.05] tracking-[-0.02em] text-fg-strong">Study Planner</h1>
      </div>
    </div>

    <nav class="flex flex-col gap-[0.12rem]">
      {#each NAV as item}
        <button
          class="relative flex w-full cursor-pointer items-center gap-[0.7rem] rounded-md px-[0.7rem] py-[0.58rem] text-left text-[0.9rem] font-semibold transition-colors hover:bg-surface-2 hover:text-fg-strong max-[720px]:justify-center max-[720px]:px-0 {activeTab === item.id ? 'bg-accent-soft text-fg-strong' : 'text-fg-muted'}"
          on:click={() => (activeTab = item.id)}
          title={item.label}
        >
          {#if activeTab === item.id}
            <span class="absolute left-[-0.85rem] top-2 bottom-2 w-[3px] rounded-r-[3px] bg-accent max-[720px]:left-[-0.5rem]" aria-hidden="true"></span>
          {/if}
          <span class="shrink-0 [&_svg]:h-[18px] [&_svg]:w-[18px] {activeTab === item.id ? 'text-accent-bright' : 'opacity-[0.85]'}">
            {#if item.id === 'topics'}
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M12 3 21 7.5 12 12 3 7.5z" /><path d="M3 12 12 16.5 21 12" /><path d="M3 16.5 12 21l9-4.5" />
              </svg>
            {:else if item.id === 'agenda'}
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M9 6h11" /><path d="M9 12h11" /><path d="M9 18h11" />
                <path d="M4 5.5l1.3 1.3L8 4" /><path d="M4 11.5l1.3 1.3L8 10" /><path d="M4 17.5l1.3 1.3L8 16" />
              </svg>
            {:else if item.id === 'calendar'}
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <rect x="3" y="4.5" width="18" height="16.5" rx="2.5" /><path d="M3 9.5h18" /><path d="M8 2.5v4" /><path d="M16 2.5v4" />
              </svg>
            {:else}
              <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <rect x="3" y="11" width="4.5" height="9" rx="1.2" /><rect x="9.75" y="5" width="4.5" height="15" rx="1.2" /><rect x="16.5" y="8" width="4.5" height="12" rx="1.2" />
              </svg>
            {/if}
          </span>
          <span class="min-w-0 flex-1 max-[720px]:hidden">{item.label}</span>
          {#if item.id === 'agenda' && overdueCount}
            <span class="tnum ml-auto rounded-full bg-red px-[0.4rem] py-[0.03rem] text-[0.64rem] font-bold leading-[1.5] text-white max-[720px]:absolute max-[720px]:right-[0.1rem] max-[720px]:top-[0.1rem] max-[720px]:ml-0">{overdueCount}</span>
          {/if}
        </button>
      {/each}
    </nav>

    <div class="mt-auto pt-[0.6rem]">
      <button
        class="flex w-full cursor-pointer items-center gap-[0.7rem] rounded-md border border-line bg-transparent px-[0.7rem] py-[0.52rem] text-[0.85rem] font-semibold text-fg-muted transition-colors hover:border-line-strong hover:bg-surface-2 hover:text-fg-strong max-[720px]:justify-center max-[720px]:px-0"
        title="Switch to {theme === 'dark' ? 'light' : 'dark'} theme"
        on:click={() => (theme = theme === 'dark' ? 'light' : 'dark')}
      >
        <span class="w-[18px] shrink-0 text-center text-base max-[720px]:w-auto" aria-hidden="true">{theme === 'dark' ? '☀' : '☾'}</span>
        <span class="max-[720px]:hidden">{theme === 'dark' ? 'Light mode' : 'Dark mode'}</span>
      </button>
    </div>
  </aside>

  <main class="content h-full min-w-0 flex-1 overflow-y-auto [overscroll-behavior:none] [scrollbar-gutter:stable]">
    <div class="sticky top-0 z-10 border-b border-line bg-bg">
      <div class="mx-auto max-w-content px-[1.6rem] pb-4 pt-[1.15rem] max-[720px]:px-[1.1rem]">
        <h2 class="m-0 font-display text-[1.5rem] font-extrabold leading-[1.1] tracking-[-0.02em] text-fg-strong">{tabMeta.title}</h2>
        <span class="mt-[0.12rem] block text-[0.82rem] text-fg-muted">{tabMeta.sub}</span>
      </div>
    </div>

    <div class="mx-auto max-w-content px-[1.6rem] pb-20 pt-6 text-left max-[720px]:px-[1.1rem]">
    {#if loading}
      <div class="flex items-center gap-[0.6rem] py-12 text-fg-muted">
        <span class="h-[18px] w-[18px] animate-spin rounded-full border-2 border-line-strong border-t-accent" aria-hidden="true"></span>
        <span>Loading…</span>
      </div>
    {:else}
      {#key activeTab}
        <div in:fly={{ y: 12, duration: 280, easing: cubicOut }}>
          {#if activeTab === 'topics'}
            <section class="reveal mb-[1.4rem] rounded-lg border border-line bg-surface px-[1.2rem] py-[1.1rem] shadow-1">
              <div class="mb-[0.8rem] flex items-baseline justify-between gap-2">
                <h2 class="m-0 font-display text-base font-bold tracking-[-0.01em] text-fg-strong">New topic</h2>
                <span class="text-[0.78rem] text-fg-faint">Add something to revise</span>
              </div>
              <form class="flex flex-col gap-[0.6rem]" on:submit|preventDefault={createTopic}>
                <input
                  type="text"
                  bind:this={nameInput}
                  bind:value={newName}
                  placeholder="Topic name (e.g. Linear Algebra)"
                />
                <textarea
                  bind:value={newDescription}
                  rows="2"
                  placeholder="Description (optional) — what to cover, resources, goals…"
                ></textarea>
                <button class="btn primary self-start" type="submit" disabled={adding || !newName.trim()}>
                  Add topic
                </button>
              </form>
            </section>

            {#if topics.length === 0}
              <div class="reveal px-4 py-12 text-center text-fg">
                <div class="mb-[0.6rem] text-[1.6rem] text-accent-bright opacity-80" aria-hidden="true">✦</div>
                <p class="m-0 mb-[0.3rem] font-display text-[1.1rem] font-bold text-fg-strong">No topics yet</p>
                <p class="muted mx-auto max-w-[44ch] text-[0.9rem] leading-[1.5]">
                  Add your first topic above, then schedule study dates — manually or with a
                  spaced-repetition plan.
                </p>
              </div>
            {:else}
              <div class="reveal mb-[1.1rem] flex flex-col gap-[0.6rem]">
                <div class="relative flex">
                  <input class="w-full pr-8" type="text" bind:this={searchInput} bind:value={search} placeholder="Search topics… ( / )" />
                  {#if search}
                    <button class="absolute right-[0.35rem] top-1/2 -translate-y-1/2 cursor-pointer rounded-xs border-none bg-transparent px-[0.3rem] py-[0.15rem] text-[1.15rem] leading-none text-fg-muted hover:text-fg-strong" on:click={() => (search = '')} aria-label="Clear search">×</button>
                  {/if}
                </div>
                {#if allTags.length || archivedCount}
                  <div class="flex flex-wrap items-center gap-[0.4rem]">
                    {#each allTags as t}
                      <button
                        class="cursor-pointer rounded-sm border px-[0.6rem] py-[0.22rem] text-[0.74rem] font-semibold transition-colors {selectedTags.includes(t) ? 'border-accent-bright bg-[var(--accent-grad)] text-white' : 'border-line bg-surface-2 text-fg-muted hover:border-line-strong hover:text-fg'}"
                        on:click={() => toggleTag(t)}
                      >{t}</button>
                    {/each}
                    {#if archivedCount}
                      <button
                        class="ml-auto cursor-pointer rounded-sm border px-[0.6rem] py-[0.22rem] text-[0.74rem] font-semibold transition-colors {showArchived ? 'border-accent-bright bg-[var(--accent-grad)] text-white' : 'border-line bg-surface-2 text-fg-muted hover:border-line-strong hover:text-fg'}"
                        on:click={() => (showArchived = !showArchived)}
                      >
                        {showArchived ? 'Hide' : 'Show'} archived · {archivedCount}
                      </button>
                    {/if}
                  </div>
                {/if}
              </div>

              <div class="reveal mb-[1.1rem] flex items-center gap-[1.25rem] px-[0.15rem]">
                <div class="flex shrink-0 items-baseline gap-[0.35rem]">
                  <span class="tnum font-display text-[1.5rem] font-extrabold leading-none text-fg-strong">{visibleActive.length}</span>
                  <span class="text-[0.82rem] text-fg-muted">topic{plural(visibleActive.length)}</span>
                </div>
                <div class="min-w-0 flex-1">
                  <div class="mb-[0.35rem] flex justify-between text-[0.74rem] text-fg-muted">
                    <span>Overall progress</span>
                    <span class="tnum">{doneSessions}/{totalSessions} · {overallPct}%</span>
                  </div>
                  <div class="bar">
                    <div class="fill" style="width:{overallPct}%"></div>
                  </div>
                </div>
              </div>

              {#if visibleActive.length}
                <div
                  class="flex flex-col gap-4"
                  use:dndzone={{ items: dndItems, flipDurationMs: 180, dragDisabled, dropTargetStyle: {} }}
                  on:consider={handleConsider}
                  on:finalize={handleFinalize}
                >
                  {#each dndItems as topic (topic.id)}
                    <div animate:flip={{ duration: 180 }}>
                      <TopicCard
                        {topic}
                        {allTags}
                        {sessionLoad}
                        draggable={!hasFilter}
                        on:changed={onChanged}
                        on:error={onError}
                        on:arm={armDrag}
                        on:disarm={disarmDrag}
                        on:filterTag={(e) => toggleTag(e.detail)}
                      />
                    </div>
                  {/each}
                </div>
              {:else}
                <div class="px-4 py-12 text-center text-fg">
                  {#if hasFilter}
                    <p class="m-0 mb-[0.3rem] font-display text-[1.1rem] font-bold text-fg-strong">No matches</p>
                    <p class="muted mx-auto max-w-[44ch] text-[0.9rem] leading-[1.5]">No active topics match your search or filters.</p>
                  {:else}
                    <p class="m-0 mb-[0.3rem] font-display text-[1.1rem] font-bold text-fg-strong">All topics archived</p>
                    <p class="muted mx-auto max-w-[44ch] text-[0.9rem] leading-[1.5]">Every topic is archived — use “Show archived” to see them.</p>
                  {/if}
                </div>
              {/if}

              {#if showArchived && visibleArchived.length}
                <div class="mt-[1.6rem]">
                  <h2 class="m-0 mb-[0.7rem] font-display text-[0.85rem] font-bold uppercase tracking-[0.04em] text-fg-faint">Archived</h2>
                  <div class="flex flex-col gap-4">
                    {#each visibleArchived as topic (topic.id)}
                      <TopicCard
                        {topic}
                        {allTags}
                        {sessionLoad}
                        on:changed={onChanged}
                        on:error={onError}
                        on:filterTag={(e) => toggleTag(e.detail)}
                      />
                    {/each}
                  </div>
                </div>
              {/if}
            {/if}
          {:else if activeTab === 'agenda'}
            <section>
              <div class="reveal mb-[1.1rem] flex items-center gap-[0.75rem]">
                <span class="inline-flex items-baseline gap-[0.4rem] text-[0.92rem] text-fg-muted">
                  <span class="tnum font-display text-[1.5rem] font-extrabold leading-none text-fg-strong">{agenda.length}</span>
                  upcoming session{plural(agenda.length)}
                </span>
                {#if overdueCount}
                  <span class="tnum rounded-sm border border-red-line bg-red-soft px-[0.5rem] py-[0.18rem] text-[0.72rem] font-semibold text-red">{overdueCount} overdue</span>
                  <button class="btn ghost sm" on:click={catchUpOverdue} disabled={catchingUp}>
                    Move all to today
                  </button>
                {/if}
                {#if pastAgenda.length}
                  <button class="btn ghost sm tnum ml-auto" on:click={() => (showPast = !showPast)}>
                    {showPast ? 'Hide' : 'Show'} past · {pastAgenda.length}
                  </button>
                {/if}
              </div>

              {#if agenda.length === 0}
                <div class="reveal px-4 py-12 text-center text-fg">
                  <div class="mb-[0.6rem] text-[1.6rem] text-accent-bright opacity-80" aria-hidden="true">✓</div>
                  <p class="m-0 mb-[0.3rem] font-display text-[1.1rem] font-bold text-fg-strong">All caught up</p>
                  <p class="muted mx-auto max-w-[44ch] text-[0.9rem] leading-[1.5]">Nothing scheduled — add dates to your topics to fill your agenda.</p>
                </div>
              {:else}
                <ul class="m-0 flex list-none flex-col gap-[0.7rem] p-0">
                  {#each agendaGroups as group (group.date)}
                    <li class="reveal relative overflow-hidden rounded-md border border-line bg-surface py-[0.75rem] pl-[1.1rem] pr-[0.95rem] transition-colors hover:border-line-strong">
                      <span class="absolute bottom-0 left-0 top-0 w-[3px] {barClass(sessionStatus(group.date))}" aria-hidden="true"></span>
                      <div class="mb-2 flex items-baseline justify-between gap-2">
                        <span class="tnum text-[0.92rem] font-semibold text-fg-strong">{formatDate(group.date)}</span>
                        <span class="tnum text-[0.76rem] {relClass(sessionStatus(group.date))}">{relativeLabel(group.date)}</span>
                      </div>
                      <ul class="m-0 flex list-none flex-col gap-[0.4rem] p-0">
                        {#each group.items as item (item.sessionId)}
                          <li>
                            <label class="flex cursor-pointer items-center gap-[0.6rem] text-[0.9rem] text-fg transition-colors hover:text-fg-strong">
                              <input
                                type="checkbox"
                                disabled={agendaBusy[item.sessionId]}
                                on:click={(e) => agendaCheckClick(e, item)}
                                on:change={() => toggleFromAgenda(item.topicId, item.sessionId)}
                              />
                              <span class="h-[9px] w-[9px] shrink-0 rounded-full" style="background:{topicHex(item.topicColor)}"></span>
                              <span>{item.topicName}</span>
                              {#if item.adaptive}<span class="text-[0.72rem] text-accent-bright opacity-80" title="Adaptive topic — reviews are graded">◎</span>{/if}
                            </label>
                          </li>
                        {/each}
                      </ul>
                    </li>
                  {/each}
                </ul>
              {/if}

              {#if showPast && pastGroups.length}
                <div class="mt-[1.6rem]">
                  <h2 class="m-0 mb-[0.7rem] font-display text-[0.85rem] font-bold uppercase tracking-[0.04em] text-fg-faint">Past sessions</h2>
                  <ul class="m-0 flex list-none flex-col gap-[0.7rem] p-0">
                    {#each pastGroups as group (group.date)}
                      <li class="relative overflow-hidden rounded-md border border-line bg-surface py-[0.75rem] pl-[1.1rem] pr-[0.95rem] opacity-75 transition-colors hover:border-line-strong">
                        <span class="absolute bottom-0 left-0 top-0 w-[3px] bg-green" aria-hidden="true"></span>
                        <div class="mb-2 flex items-baseline justify-between gap-2">
                          <span class="tnum text-[0.92rem] font-semibold text-fg-strong">{formatDate(group.date)}</span>
                          <span class="tnum text-[0.76rem] text-fg-muted">{relativeLabel(group.date)}</span>
                        </div>
                        <ul class="m-0 flex list-none flex-col gap-[0.4rem] p-0">
                          {#each group.items as item (item.sessionId)}
                            <li>
                              <label class="flex cursor-pointer items-center gap-[0.6rem] text-[0.9rem] text-fg transition-colors hover:text-fg-strong">
                                <!-- Unchecking a done session is always a plain
                                     toggle, even for adaptive topics. -->
                                <input
                                  type="checkbox"
                                  checked
                                  disabled={agendaBusy[item.sessionId]}
                                  on:change={() => toggleFromAgenda(item.topicId, item.sessionId)}
                                />
                                <span class="h-[9px] w-[9px] shrink-0 rounded-full" style="background:{topicHex(item.topicColor)}"></span>
                                <span>{item.topicName}</span>
                              </label>
                            </li>
                          {/each}
                        </ul>
                      </li>
                    {/each}
                  </ul>
                </div>
              {/if}
            </section>
          {:else if activeTab === 'calendar'}
            <Calendar topics={visibleActive} on:changed={onChanged} on:error={onError} />
          {:else}
            <Stats {topics} />
          {/if}
        </div>
      {/key}
    {/if}
    </div>
  </main>
</div>

{#if gradeTarget}
  <GradeModal
    topicName={gradeTarget.topicName}
    on:grade={onGrade}
    on:cancel={() => (gradeTarget = null)}
  />
{/if}

{#if errorMsg}
  <div class="fixed bottom-6 left-1/2 z-50 flex max-w-[min(90vw,460px)] -translate-x-1/2 items-center gap-[0.6rem] rounded-md border border-red-line bg-surface-3 px-4 py-[0.7rem] text-[0.88rem] text-fg-strong shadow-pop" role="alert" transition:fly={{ y: 24, duration: 260, easing: cubicOut }}>
    <span class="h-2 w-2 shrink-0 animate-pulse rounded-full bg-red" aria-hidden="true"></span>
    <span class="break-words leading-[1.4]">{errorMsg}</span>
  </div>
{/if}

