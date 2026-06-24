<script lang="ts">
  import { onMount } from 'svelte';
  import type { main } from '../wailsjs/go/models';
  import {
    GetState,
    AddTask,
    AddSubject,
    ReorderSubjects,
    ToggleSession,
    ReorderTasks,
    RescheduleOverdueSessions,
    GradeSession,
    GetFocusSessions,
    SetDailyGoalMinutes,
  } from '../wailsjs/go/main/App.js';
  import TaskCard from './lib/TaskCard.svelte';
  import SubjectHeader from './lib/SubjectHeader.svelte';
  import Calendar from './lib/Calendar.svelte';
  import Stats from './lib/Stats.svelte';
  import Home from './lib/Home.svelte';
  import Focus from './lib/Focus.svelte';
  import Settings from './lib/Settings.svelte';
  import GradeModal from './lib/GradeModal.svelte';
  import { openModalCount } from './lib/ConfirmModal.svelte';
  import { makeMutator } from './lib/mutate';
  import { formatDate, relativeLabel, sessionStatus, plural } from './lib/dates';
  import { today } from './lib/today';
  import './lib/theme'; // side-effect: applies the saved theme on startup (control lives in Settings)
  import { loadSounds } from './lib/sounds';
  import { taskHex } from './lib/colors';
  import { dndzone } from 'svelte-dnd-action';
  import type { DndEvent } from 'svelte-dnd-action';

  let tasks: main.Task[] = [];
  let subjects: main.Subject[] = [];
  // Focus records live alongside tasks but on their own backend log; App owns
  // them so the Focus tab (which records) and the Stats tab (which reads) share
  // one source of truth.
  let focusSessions: main.FocusSession[] = [];
  // Configurable daily focus-time goal (minutes), persisted in the backend store
  // and shipped with every State; surfaced as the Home goal ring.
  let dailyGoalMinutes = 0;
  let activeTab: 'home' | 'tasks' | 'agenda' | 'calendar' | 'stats' | 'focus' | 'settings' = 'home';
  let loading = true;
  let errorMsg = '';
  let errorTimer: ReturnType<typeof setTimeout>;

  let newName = '';
  let newDescription = '';
  let newSubjectId = ''; // subject to drop the new task into ("" = ungrouped)
  let adding = false;

  let newSubjectName = '';
  let addingSubject = false;

  // Theme is a pure UI preference persisted locally; the store applies it (and
  // the no-transition swap) on change. The Settings control drives it.
  // See lib/theme.ts.

  // Keyboard shortcuts: n → new task, / → search (when not already typing).
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
    if (activeTab !== 'tasks') return;
    if (e.key === 'n') {
      e.preventDefault();
      nameInput?.focus();
    } else if (e.key === '/') {
      e.preventDefault();
      searchInput?.focus();
    }
  }

  // apply awaits a backend call, swaps in the returned State and toasts
  // failures; resolves to whether it succeeded.
  const apply = makeMutator({
    state: (s) => {
      tasks = s.tasks;
      subjects = s.subjects;
      dailyGoalMinutes = s.settings.dailyGoalMinutes;
    },
    error: showError,
  });

  onMount(async () => {
    await apply(GetState());
    // Focus history isn't part of the task graph, so it loads on its own; a
    // failure here shouldn't block the rest of the app from rendering.
    try {
      focusSessions = await GetFocusSessions();
    } catch (e) {
      showError(`Couldn't load focus history: ${e}`);
    }
    // Hydrate alert sounds from IndexedDB so both Focus and Settings share them.
    void loadSounds();
    loading = false;
  });

  function showError(msg: string) {
    errorMsg = msg;
    clearTimeout(errorTimer);
    errorTimer = setTimeout(() => (errorMsg = ''), 5000);
  }

  function onChanged(e: CustomEvent<main.State>) {
    tasks = e.detail.tasks;
    subjects = e.detail.subjects;
    dailyGoalMinutes = e.detail.settings.dailyGoalMinutes;
  }

  function onError(e: CustomEvent<string>) {
    showError(e.detail);
  }

  async function createTask() {
    if (!newName.trim()) return;
    adding = true;
    if (await apply(AddTask(newName, newDescription, newSubjectId))) {
      newName = '';
      newDescription = '';
    }
    adding = false;
  }

  async function createSubject() {
    if (!newSubjectName.trim()) return;
    addingSubject = true;
    if (await apply(AddSubject(newSubjectName))) newSubjectName = '';
    addingSubject = false;
  }

  // Move a subject one slot up or down by swapping it with its neighbour and
  // resending the full order.
  async function moveSubject(id: string, dir: -1 | 1) {
    const order = subjects.map((s) => s.id);
    const i = order.indexOf(id);
    const j = i + dir;
    if (i < 0 || j < 0 || j >= order.length) return;
    [order[i], order[j]] = [order[j], order[i]];
    await apply(ReorderSubjects(order));
  }

  // Collapsed subject groups, persisted locally like the theme. Keyed by subject
  // id (and '' for the Ungrouped group).
  let collapsed = new Set<string>(JSON.parse(localStorage.getItem('collapsedSubjects') ?? '[]'));
  function toggleCollapse(key: string) {
    if (collapsed.has(key)) collapsed.delete(key);
    else collapsed.add(key);
    collapsed = collapsed;
    localStorage.setItem('collapsedSubjects', JSON.stringify([...collapsed]));
  }

  // Guard against double-clicks per session: a second toggle for the SAME
  // session while the first is in flight would flip it straight back. Tracked
  // per id so toggling one session doesn't freeze the rest of the agenda.
  let agendaBusy: Record<string, boolean> = {};
  async function toggleFromAgenda(taskId: string, sessionId: string) {
    if (agendaBusy[sessionId]) return;
    agendaBusy = { ...agendaBusy, [sessionId]: true };
    await apply(ToggleSession(taskId, sessionId));
    const { [sessionId]: _, ...rest } = agendaBusy;
    agendaBusy = rest;
  }

  // Sessions of adaptive tasks are graded instead of plainly checked off; the
  // grade re-spaces the remaining schedule.
  let gradeTarget: { taskId: string; sessionId: string; taskName: string } | null = null;

  function agendaCheckClick(e: Event, item: AgendaItem) {
    if (!item.adaptive) return; // plain toggle proceeds via on:change
    e.preventDefault();
    gradeTarget = { taskId: item.taskId, sessionId: item.sessionId, taskName: item.taskName };
  }

  // No in-flight guard here: grading the same session twice is impossible (the
  // modal unmounts on the first choice) and concurrent grades of different
  // sessions are safe — a guard would only drop a grade silently.
  async function onGrade(e: CustomEvent<string>) {
    const target = gradeTarget;
    gradeTarget = null;
    if (target) await apply(GradeSession(target.taskId, target.sessionId, e.detail));
  }

  // One-click catch-up: every overdue session moves to today.
  let catchingUp = false;
  async function catchUpOverdue() {
    if (catchingUp) return;
    catchingUp = true;
    await apply(RescheduleOverdueSessions());
    catchingUp = false;
  }

  // Persist a new daily focus goal (minutes) from the Home ring's inline editor.
  async function setGoal(minutes: number) {
    await apply(SetDailyGoalMinutes(minutes));
  }

  // Personalised header for the Home tab: a time-of-day greeting and the date,
  // shown in place of the generic title/subtitle.
  const greeting = (() => {
    const h = new Date().getHours();
    return h < 12 ? 'Good morning' : h < 18 ? 'Good afternoon' : 'Good evening';
  })();
  const todayLabel = new Date().toLocaleDateString(undefined, {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
  });

  // Organisation: search text, selected tags and whether archived tasks show.
  let search = '';
  let selectedTags: string[] = [];
  let showArchived = false;

  function toggleTag(t: string) {
    selectedTags = selectedTags.includes(t)
      ? selectedTags.filter((x) => x !== t)
      : [...selectedTags, t];
  }

  $: allTags = Array.from(new Set(tasks.flatMap((t) => t.tags))).sort((a, b) => a.localeCompare(b));
  $: archivedCount = tasks.filter((t) => t.archived).length;

  // A task matches when the search text hits its name/description/tags and it
  // carries at least one selected tag (when any tags are selected).
  $: matches = (t: main.Task) => {
    const q = search.trim().toLowerCase();
    const okSearch =
      !q ||
      t.name.toLowerCase().includes(q) ||
      t.description.toLowerCase().includes(q) ||
      t.tags.some((tag) => tag.toLowerCase().includes(q));
    const okTags = selectedTags.length === 0 || t.tags.some((tag) => selectedTags.includes(tag));
    return okSearch && okTags;
  };

  // Active (non-archived) tasks that pass the filter feed every view; archived
  // tasks surface only in their own section.
  $: visibleActive = tasks.filter((t) => !t.archived && matches(t));
  $: visibleArchived = tasks.filter((t) => t.archived && matches(t));

  // Drag-to-reorder operates on the unfiltered active list only.
  $: hasFilter = search.trim() !== '' || selectedTags.length > 0;

  // Tasks grouped under their subject (in subject order), with an Ungrouped
  // bucket last for tasks with no subject (or one that no longer exists). Held
  // in a writable so svelte-dnd-action can mutate a group's list during a drag;
  // the reactive re-derive resets it whenever the backing state changes.
  type TaskGroup = { key: string; subject: main.Subject | null; tasks: main.Task[] };
  function buildGroups(subs: main.Subject[], list: main.Task[]): TaskGroup[] {
    const ids = new Set(subs.map((s) => s.id));
    const grouped: Record<string, main.Task[]> = {};
    const ungrouped: main.Task[] = [];
    for (const t of list) {
      if (t.subjectId && ids.has(t.subjectId)) (grouped[t.subjectId] ??= []).push(t);
      else ungrouped.push(t);
    }
    const groups: TaskGroup[] = subs.map((s) => ({ key: s.id, subject: s, tasks: grouped[s.id] ?? [] }));
    groups.push({ key: '', subject: null, tasks: ungrouped });
    return groups;
  }
  let dndGroups: TaskGroup[] = [];
  $: dndGroups = buildGroups(subjects, visibleActive);

  // Each subject group is its own dnd zone with a unique type, so cards reorder
  // within a group but never jump between groups by drag (use the card's subject
  // selector for that). Dragging is armed only while a card's drag handle is held
  // (and never while filtered), so it never hijacks text selection.
  let dragDisabled = true;
  function armDrag() {
    if (!hasFilter) dragDisabled = false;
  }
  function disarmDrag() {
    dragDisabled = true;
  }

  function handleConsider(gi: number, e: CustomEvent<DndEvent<main.Task>>) {
    dndGroups[gi] = { ...dndGroups[gi], tasks: e.detail.items };
    dndGroups = dndGroups;
  }
  async function handleFinalize(gi: number, e: CustomEvent<DndEvent<main.Task>>) {
    dndGroups[gi] = { ...dndGroups[gi], tasks: e.detail.items };
    dndGroups = dndGroups;
    dragDisabled = true;
    // Send the full active order (all groups, in display order) so global Order
    // stays consistent with the grouping.
    const ids = dndGroups.flatMap((g) => g.tasks.map((t) => t.id));
    if (!(await apply(ReorderTasks(ids)))) {
      dndGroups = buildGroups(subjects, visibleActive); // revert the optimistic order
    }
  }

  // Flattened, date-sorted list of incomplete sessions for the agenda view.
  type AgendaItem = {
    taskId: string;
    taskName: string;
    sessionId: string;
    date: string;
    taskColor: string;
    adaptive: boolean;
  };

  function agendaItems(from: main.Task[], done: boolean): AgendaItem[] {
    return from.flatMap((t) =>
      t.sessions
        .filter((s) => s.done === done)
        .map((s) => ({
          taskId: t.id,
          taskName: t.name,
          sessionId: s.id,
          date: s.date,
          taskColor: t.color,
          adaptive: t.adaptive,
        }))
    );
  }

  $: agenda = agendaItems(visibleActive, false).sort((a, b) => a.date.localeCompare(b.date));

  // Completed sessions, newest first — shown on demand below the agenda.
  let showPast = false;
  $: pastAgenda = agendaItems(visibleActive, true).sort((a, b) => b.date.localeCompare(a.date));

  // Cross-task scheduling load (date → planned sessions), used by the cards to
  // warn when a generated schedule would pile onto already-busy days.
  $: sessionLoad = (() => {
    const m: Record<string, number> = {};
    for (const t of tasks) {
      if (t.archived) continue;
      for (const s of t.sessions) {
        if (!s.done) m[s.date] = (m[s.date] ?? 0) + 1;
      }
    }
    return m;
  })();

  $: totalSessions = visibleActive.reduce((n, t) => n + t.sessions.length, 0);
  $: doneSessions = visibleActive.reduce((n, t) => n + t.sessions.filter((s) => s.done).length, 0);
  $: overallPct = totalSessions ? Math.round((doneSessions / totalSessions) * 100) : 0;

  // Group date-sorted agenda items by date, attaching each day's time-relative
  // status and label. `now` is the current date ($today): passing it in makes
  // the grouping recompute when the day rolls over, so the labels and overdue
  // styling stay correct in an app left open past midnight.
  type AgendaGroup = {
    date: string;
    items: AgendaItem[];
    status: ReturnType<typeof sessionStatus>;
    label: string;
  };
  function groupByDate(items: AgendaItem[], now: string): AgendaGroup[] {
    void now; // dependency marker — see comment above
    const groups: AgendaGroup[] = [];
    for (const item of items) {
      const last = groups[groups.length - 1];
      if (last && last.date === item.date) last.items.push(item);
      else
        groups.push({
          date: item.date,
          items: [item],
          status: sessionStatus(item.date),
          label: relativeLabel(item.date),
        });
    }
    return groups;
  }

  $: agendaGroups = groupByDate(agenda, $today);
  $: pastGroups = groupByDate(pastAgenda, $today);
  $: overdueCount = agendaGroups.reduce(
    (n, g) => n + (g.status === 'overdue' ? g.items.length : 0),
    0
  );

  // Per-view header copy for the sticky content header.
  const TAB_META = {
    home: { title: 'Home', sub: 'Your day at a glance' },
    tasks: { title: 'Tasks', sub: "Everything you're revising" },
    agenda: { title: 'Agenda', sub: "What's coming up next" },
    calendar: { title: 'Calendar', sub: 'Your month at a glance' },
    stats: { title: 'Stats', sub: 'Progress and streaks' },
    focus: { title: 'Focus', sub: 'Keep track of study time' },
    settings: { title: 'Settings', sub: 'Preferences and startup' },
  } as const;
  $: tabMeta = TAB_META[activeTab];

  const NAV = [
    { id: 'home', label: 'Home' },
    { id: 'tasks', label: 'Tasks' },
    { id: 'agenda', label: 'Agenda' },
    { id: 'calendar', label: 'Calendar' },
    { id: 'stats', label: 'Stats' },
    { id: 'focus', label: 'Focus' },
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
    <div class="px-[0.45rem] pt-[0.2rem] pb-[1.15rem] max-[720px]:hidden">
      <span class="block whitespace-nowrap text-[0.57rem] font-bold uppercase tracking-[0.16em] text-accent-bright">Spaced repetition</span>
      <h1 class="m-0 mt-0.5 whitespace-nowrap font-display text-[1.16rem] font-extrabold leading-[1.05] tracking-[-0.02em] text-fg-strong">Study Planner</h1>
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
            {#if item.id === 'home'}
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M3 10.5 12 3l9 7.5" /><path d="M5 9.5V20h14V9.5" /><path d="M9.5 20v-6h5v6" />
              </svg>
            {:else if item.id === 'tasks'}
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
            {:else if item.id === 'stats'}
              <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <rect x="3" y="11" width="4.5" height="9" rx="1.2" /><rect x="9.75" y="5" width="4.5" height="15" rx="1.2" /><rect x="16.5" y="8" width="4.5" height="12" rx="1.2" />
              </svg>
            {:else}
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <circle cx="12" cy="12" r="9" />
                <circle cx="12" cy="12" r="3" fill="currentColor" />
                <line x1="12" y1="1" x2="12" y2="4" />
                <line x1="12" y1="20" x2="12" y2="23" />
                <line x1="1" y1="12" x2="4" y2="12" />
                <line x1="20" y1="12" x2="23" y2="12" />
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
        class="relative flex w-full cursor-pointer items-center gap-[0.7rem] rounded-md px-[0.7rem] py-[0.58rem] text-left text-[0.9rem] font-semibold transition-colors hover:bg-surface-2 hover:text-fg-strong max-[720px]:justify-center max-[720px]:px-0 {activeTab === 'settings' ? 'bg-accent-soft text-fg-strong' : 'text-fg-muted'}"
        on:click={() => (activeTab = 'settings')}
        title="Settings"
      >
        {#if activeTab === 'settings'}
          <span class="absolute left-[-0.85rem] top-2 bottom-2 w-[3px] rounded-r-[3px] bg-accent max-[720px]:left-[-0.5rem]" aria-hidden="true"></span>
        {/if}
        <span class="shrink-0 [&_svg]:h-[18px] [&_svg]:w-[18px] {activeTab === 'settings' ? 'text-accent-bright' : 'opacity-[0.85]'}">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M19.4 13.5a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1.03 1.56V20a2 2 0 1 1-4 0v-.09A1.7 1.7 0 0 0 8.5 18.3a1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.7 1.7 0 0 0 .34-1.87 1.7 1.7 0 0 0-1.56-1.03H2a2 2 0 1 1 0-4h.09A1.7 1.7 0 0 0 3.7 8.5a1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.7 1.7 0 0 0 1.87.34H8.5a1.7 1.7 0 0 0 1.03-1.56V2a2 2 0 1 1 4 0v.09a1.7 1.7 0 0 0 1.03 1.56 1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.7 1.7 0 0 0-.34 1.87V8.5a1.7 1.7 0 0 0 1.56 1.03H22a2 2 0 1 1 0 4h-.09a1.7 1.7 0 0 0-1.51 1z" />
          </svg>
        </span>
        <span class="min-w-0 flex-1 max-[720px]:hidden">Settings</span>
      </button>
    </div>
  </aside>

  <main class="content h-full min-w-0 flex-1 overflow-y-auto [overscroll-behavior:none] [scrollbar-gutter:stable]">
    <div class="sticky top-0 z-10 border-b border-line bg-bg">
      <div class="mx-auto max-w-content px-[1.6rem] pb-4 pt-[1.15rem] max-[720px]:px-[1.1rem]">
        <h2 class="m-0 font-display text-[1.5rem] font-extrabold leading-[1.1] tracking-[-0.02em] text-fg-strong">{activeTab === 'home' ? greeting : tabMeta.title}</h2>
        <span class="mt-[0.12rem] block text-[0.82rem] text-fg-muted">{activeTab === 'home' ? todayLabel : tabMeta.sub}</span>
      </div>
    </div>

    <div class="mx-auto max-w-content px-[1.6rem] pb-20 pt-6 text-left max-[720px]:px-[1.1rem]">
    {#if loading}
      <div class="py-12 text-fg-muted">Loading…</div>
    {:else}
        <div>
          {#if activeTab === 'home'}
            <Home
              {tasks}
              {focusSessions}
              {dailyGoalMinutes}
              {agendaBusy}
              {catchingUp}
              onToggle={toggleFromAgenda}
              onGrade={(item) => (gradeTarget = { taskId: item.taskId, sessionId: item.sessionId, taskName: item.taskName })}
              onCatchUp={catchUpOverdue}
              onStartFocus={() => (activeTab = 'focus')}
              onViewAgenda={() => (activeTab = 'agenda')}
              onSetGoal={setGoal}
            />
          {:else if activeTab === 'tasks'}
            <section class="mb-[1.4rem] rounded-lg border border-line bg-surface px-[1.2rem] py-[1.1rem] shadow-1">
              <div class="mb-[0.8rem] flex items-baseline justify-between gap-2">
                <h2 class="m-0 font-display text-base font-bold tracking-[-0.01em] text-fg-strong">New task</h2>
                <span class="text-[0.78rem] text-fg-faint">Add something to revise</span>
              </div>
              <form class="flex flex-col gap-[0.6rem]" on:submit|preventDefault={createTask}>
                <input
                  type="text"
                  bind:this={nameInput}
                  bind:value={newName}
                  placeholder="Task name (e.g. Linear Algebra)"
                />
                <textarea
                  bind:value={newDescription}
                  rows="2"
                  placeholder="Description (optional) — what to cover, resources, goals…"
                ></textarea>
                <div class="flex flex-wrap items-center gap-[0.6rem]">
                  {#if subjects.length}
                    <select bind:value={newSubjectId} class="max-w-[14rem]" title="Add to subject">
                      <option value="">No subject</option>
                      {#each subjects as s}
                        <option value={s.id}>{s.name}</option>
                      {/each}
                    </select>
                  {/if}
                  <button class="btn primary" type="submit" disabled={adding || !newName.trim()}>
                    Add task
                  </button>
                </div>
              </form>
            </section>

            {#if tasks.length === 0}
              <div class="px-4 py-12 text-center text-fg">
                <div class="mb-[0.6rem] text-[1.6rem] text-accent-bright opacity-80" aria-hidden="true">✦</div>
                <p class="m-0 mb-[0.3rem] font-display text-[1.1rem] font-bold text-fg-strong">No tasks yet</p>
                <p class="muted mx-auto max-w-[44ch] text-[0.9rem] leading-[1.5]">
                  Add your first task above, then schedule study dates — manually or with a
                  spaced-repetition plan.
                </p>
              </div>
            {:else}
              <div class="mb-[1.1rem] flex flex-col gap-[0.6rem]">
                <div class="relative flex">
                  <input class="w-full pr-8" type="text" bind:this={searchInput} bind:value={search} placeholder="Search tasks… ( / )" />
                  {#if search}
                    <button class="absolute right-[0.35rem] top-1/2 -translate-y-1/2 cursor-pointer rounded-xs border-none bg-transparent px-[0.3rem] py-[0.15rem] text-[1.15rem] leading-none text-fg-muted hover:text-fg-strong" on:click={() => (search = '')} aria-label="Clear search">×</button>
                  {/if}
                </div>
                <div class="flex flex-wrap items-center gap-[0.4rem]">
                  <form class="flex items-center gap-[0.4rem]" on:submit|preventDefault={createSubject}>
                    <input class="w-[10rem] max-w-full" type="text" bind:value={newSubjectName} placeholder="New subject…" />
                    <button class="btn ghost sm" type="submit" disabled={addingSubject || !newSubjectName.trim()}>Add subject</button>
                  </form>
                  {#if archivedCount}
                    <button
                      class="ml-auto cursor-pointer rounded-sm border px-[0.6rem] py-[0.22rem] text-[0.74rem] font-semibold transition-colors {showArchived ? 'border-accent-bright bg-[var(--accent-grad)] text-white' : 'border-line bg-surface-2 text-fg-muted hover:border-line-strong hover:text-fg'}"
                      on:click={() => (showArchived = !showArchived)}
                    >
                      {showArchived ? 'Hide' : 'Show'} archived · {archivedCount}
                    </button>
                  {/if}
                </div>
                {#if allTags.length}
                  <div class="flex flex-wrap items-center gap-[0.4rem]">
                    {#each allTags as t}
                      <button
                        class="cursor-pointer rounded-sm border px-[0.6rem] py-[0.22rem] text-[0.74rem] font-semibold transition-colors {selectedTags.includes(t) ? 'border-accent-bright bg-[var(--accent-grad)] text-white' : 'border-line bg-surface-2 text-fg-muted hover:border-line-strong hover:text-fg'}"
                        on:click={() => toggleTag(t)}
                      >{t}</button>
                    {/each}
                  </div>
                {/if}
              </div>

              <div class="mb-[1.1rem] flex items-center gap-[1.25rem] px-[0.15rem]">
                <div class="flex shrink-0 items-baseline gap-[0.35rem]">
                  <span class="tnum font-display text-[1.5rem] font-extrabold leading-none text-fg-strong">{visibleActive.length}</span>
                  <span class="text-[0.82rem] text-fg-muted">task{plural(visibleActive.length)}</span>
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
                {#if subjects.length === 0}
                  <!-- No subjects yet: a plain, ungrouped list (the original view). -->
                  <div
                    class="flex flex-col gap-4"
                    use:dndzone={{ items: dndGroups[0].tasks, type: 'task-', flipDurationMs: 0, dragDisabled, dropTargetStyle: {} }}
                    on:consider={(e) => handleConsider(0, e)}
                    on:finalize={(e) => handleFinalize(0, e)}
                  >
                    {#each dndGroups[0].tasks as task (task.id)}
                      <div>
                        <TaskCard
                          {task}
                          {allTags}
                          {subjects}
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
                  <div class="flex flex-col gap-[1.1rem]">
                    {#each dndGroups as group, gi (group.key)}
                      {#if group.subject}
                        <section>
                          <SubjectHeader
                            subject={group.subject}
                            count={group.tasks.length}
                            collapsed={collapsed.has(group.key)}
                            canMoveUp={gi > 0}
                            canMoveDown={gi < subjects.length - 1}
                            on:toggle={() => toggleCollapse(group.key)}
                            on:changed={onChanged}
                            on:error={onError}
                            on:moveUp={() => moveSubject(group.key, -1)}
                            on:moveDown={() => moveSubject(group.key, 1)}
                          />
                          {#if !collapsed.has(group.key)}
                            <div
                              class="mt-3 flex flex-col gap-4"
                              use:dndzone={{ items: group.tasks, type: 'task-' + group.key, flipDurationMs: 0, dragDisabled, dropTargetStyle: {} }}
                              on:consider={(e) => handleConsider(gi, e)}
                              on:finalize={(e) => handleFinalize(gi, e)}
                            >
                              {#each group.tasks as task (task.id)}
                                <div>
                                  <TaskCard
                                    {task}
                                    {allTags}
                                    {subjects}
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
                            {#if group.tasks.length === 0}
                              <p class="muted mt-3 pl-[0.6rem] text-[0.82rem]">No tasks in this subject yet — assign one from its card, or add a new task above.</p>
                            {/if}
                          {/if}
                        </section>
                      {:else if group.tasks.length}
                        <!-- Ungrouped bucket, shown only when it holds tasks. -->
                        <section>
                          <button class="flex w-full cursor-pointer items-center gap-[0.55rem] rounded-md border border-line bg-surface-2 py-[0.5rem] pl-[0.6rem] pr-[0.5rem] text-left" on:click={() => toggleCollapse('')} title={collapsed.has('') ? 'Expand' : 'Collapse'}>
                            <span class="shrink-0 text-[0.7rem] text-fg-muted transition-transform {collapsed.has('') ? '' : 'rotate-90'}" aria-hidden="true">▶</span>
                            <span class="flex-1 font-display text-[0.98rem] font-bold tracking-[-0.01em] text-fg-muted">Ungrouped</span>
                            <span class="tnum shrink-0 text-[0.78rem] text-fg-muted">{group.tasks.length} task{plural(group.tasks.length)}</span>
                          </button>
                          {#if !collapsed.has('')}
                            <div
                              class="mt-3 flex flex-col gap-4"
                              use:dndzone={{ items: group.tasks, type: 'task-', flipDurationMs: 0, dragDisabled, dropTargetStyle: {} }}
                              on:consider={(e) => handleConsider(gi, e)}
                              on:finalize={(e) => handleFinalize(gi, e)}
                            >
                              {#each group.tasks as task (task.id)}
                                <div>
                                  <TaskCard
                                    {task}
                                    {allTags}
                                    {subjects}
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
                          {/if}
                        </section>
                      {/if}
                    {/each}
                  </div>
                {/if}
              {:else}
                <div class="px-4 py-12 text-center text-fg">
                  {#if hasFilter}
                    <p class="m-0 mb-[0.3rem] font-display text-[1.1rem] font-bold text-fg-strong">No matches</p>
                    <p class="muted mx-auto max-w-[44ch] text-[0.9rem] leading-[1.5]">No active tasks match your search or filters.</p>
                  {:else}
                    <p class="m-0 mb-[0.3rem] font-display text-[1.1rem] font-bold text-fg-strong">All tasks archived</p>
                    <p class="muted mx-auto max-w-[44ch] text-[0.9rem] leading-[1.5]">Every task is archived — use “Show archived” to see them.</p>
                  {/if}
                </div>
              {/if}

              {#if showArchived && visibleArchived.length}
                <div class="mt-[1.6rem]">
                  <h2 class="m-0 mb-[0.7rem] font-display text-[0.85rem] font-bold uppercase tracking-[0.04em] text-fg-faint">Archived</h2>
                  <div class="flex flex-col gap-4">
                    {#each visibleArchived as task (task.id)}
                      <TaskCard
                        {task}
                        {allTags}
                        {subjects}
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
              <div class="mb-[1.1rem] flex items-center gap-[0.75rem]">
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
                <div class="px-4 py-12 text-center text-fg">
                  <div class="mb-[0.6rem] text-[1.6rem] text-accent-bright opacity-80" aria-hidden="true">✓</div>
                  <p class="m-0 mb-[0.3rem] font-display text-[1.1rem] font-bold text-fg-strong">All caught up</p>
                  <p class="muted mx-auto max-w-[44ch] text-[0.9rem] leading-[1.5]">Nothing scheduled — add dates to your tasks to fill your agenda.</p>
                </div>
              {:else}
                <ul class="m-0 flex list-none flex-col gap-[0.7rem] p-0">
                  {#each agendaGroups as group (group.date)}
                    <li class="relative overflow-hidden rounded-md border border-line bg-surface py-[0.75rem] pl-[1.1rem] pr-[0.95rem] transition-colors hover:border-line-strong">
                      <span class="absolute bottom-0 left-0 top-0 w-[3px] {barClass(group.status)}" aria-hidden="true"></span>
                      <div class="mb-2 flex items-baseline justify-between gap-2">
                        <span class="tnum text-[0.92rem] font-semibold text-fg-strong">{formatDate(group.date)}</span>
                        <span class="tnum text-[0.76rem] {relClass(group.status)}">{group.label}</span>
                      </div>
                      <ul class="m-0 flex list-none flex-col gap-[0.4rem] p-0">
                        {#each group.items as item (item.sessionId)}
                          <li class="chk-row">
                            <label class="flex w-full cursor-pointer items-center gap-[0.6rem] text-[0.9rem] text-fg transition-colors hover:text-fg-strong">
                              <input
                                type="checkbox"
                                disabled={agendaBusy[item.sessionId]}
                                on:click={(e) => agendaCheckClick(e, item)}
                                on:change={() => toggleFromAgenda(item.taskId, item.sessionId)}
                              />
                              <span class="h-[9px] w-[9px] shrink-0 rounded-full" style="background:{taskHex(item.taskColor)}"></span>
                              <span>{item.taskName}</span>
                              {#if item.adaptive}<span class="text-[0.72rem] text-accent-bright opacity-80" title="Adaptive task — reviews are graded">◎</span>{/if}
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
                          <span class="tnum text-[0.76rem] text-fg-muted">{group.label}</span>
                        </div>
                        <ul class="m-0 flex list-none flex-col gap-[0.4rem] p-0">
                          {#each group.items as item (item.sessionId)}
                            <li class="chk-row">
                              <label class="flex w-full cursor-pointer items-center gap-[0.6rem] text-[0.9rem] text-fg transition-colors hover:text-fg-strong">
                                <!-- Unchecking a done session is always a plain
                                     toggle, even for adaptive tasks. -->
                                <input
                                  type="checkbox"
                                  checked
                                  disabled={agendaBusy[item.sessionId]}
                                  on:change={() => toggleFromAgenda(item.taskId, item.sessionId)}
                                />
                                <span class="h-[9px] w-[9px] shrink-0 rounded-full" style="background:{taskHex(item.taskColor)}"></span>
                                <span>{item.taskName}</span>
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
            <Calendar tasks={visibleActive} {subjects} on:changed={onChanged} on:error={onError} />
          {:else if activeTab === 'stats'}
            <Stats {tasks} {subjects} {focusSessions} />
          {:else if activeTab === 'settings'}
            <Settings {dailyGoalMinutes} onSetGoal={setGoal} on:error={onError} />
          {/if}

          <!-- Focus stays mounted (just hidden) across tab switches so a running
               timer pauses and resumes rather than being destroyed. -->
          <div class:hidden={activeTab !== 'focus'}>
            <Focus
              {tasks}
              {focusSessions}
              active={activeTab === 'focus'}
              on:recorded={(e) => (focusSessions = e.detail)}
              on:error={onError}
            />
          </div>
        </div>
    {/if}
    </div>
  </main>
</div>

{#if gradeTarget}
  <GradeModal
    taskName={gradeTarget.taskName}
    on:grade={onGrade}
    on:cancel={() => (gradeTarget = null)}
  />
{/if}

{#if errorMsg}
  <div class="fixed bottom-6 left-1/2 z-50 flex max-w-[min(90vw,460px)] -translate-x-1/2 items-center gap-[0.6rem] rounded-md border border-red-line bg-surface-3 px-4 py-[0.7rem] text-[0.88rem] text-fg-strong shadow-pop" role="alert">
    <span class="h-2 w-2 shrink-0 rounded-full bg-red" aria-hidden="true"></span>
    <span class="break-words leading-[1.4]">{errorMsg}</span>
  </div>
{/if}
