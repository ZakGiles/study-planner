<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { main } from '../../wailsjs/go/models';
  import { AddSession, GradeSession, ToggleSession } from './backend';
  import { toISO, formatDate, relativeLabel, sessionStatus, plural } from './dates';
  import { today } from './today';
  import { taskHex } from './colors';
  import { makeMutator } from './mutate';
  import ConfirmModal from './ConfirmModal.svelte';
  import type { ModalAction } from './ConfirmModal.svelte';
  import GradeModal from './GradeModal.svelte';

  export let tasks: main.Task[] = [];
  export let subjects: main.Subject[] = [];

  const dispatch = createEventDispatcher<{ changed: main.State; error: string }>();

  // Subject filter: '' = all subjects, a subject id, or the UNGROUPED sentinel.
  // Everything the calendar shows (and the quick-add task picker) derives from
  // viewTasks, so the month narrows to the chosen subject in one place.
  const UNGROUPED = '\0ungrouped';
  let subjectFilter = '';

  $: subjectIds = new Set(subjects.map((s) => s.id));
  const isUngrouped = (t: main.Task, ids: Set<string>) => !t.subjectId || !ids.has(t.subjectId);
  $: hasUngrouped = tasks.some((t) => isUngrouped(t, subjectIds));

  // Reset a stale filter (its subject was deleted, or the Ungrouped bucket emptied).
  $: if (subjectFilter === UNGROUPED ? !hasUngrouped : subjectFilter && !subjectIds.has(subjectFilter)) {
    subjectFilter = '';
  }

  $: viewTasks =
    subjectFilter === ''
      ? tasks
      : subjectFilter === UNGROUPED
        ? tasks.filter((t) => isUngrouped(t, subjectIds))
        : tasks.filter((t) => t.subjectId === subjectFilter);

  // Monday-first week, matching the day-first date format used elsewhere.
  const WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
  const MONTHS = [
    'January', 'February', 'March', 'April', 'May', 'June',
    'July', 'August', 'September', 'October', 'November', 'December',
  ];

  const now = new Date();
  let viewYear = now.getFullYear();
  let viewMonth = now.getMonth(); // 0-11

  let busy = false;

  type DaySession = {
    taskId: string;
    taskName: string;
    sessionId: string;
    date: string;
    done: boolean;
    color: string;
    adaptive: boolean;
  };

  // Index every session by its date so each day cell is a cheap lookup.
  $: byDate = (() => {
    const m = new Map<string, DaySession[]>();
    for (const t of viewTasks) {
      for (const s of t.sessions) {
        const list = m.get(s.date) ?? [];
        list.push({
          taskId: t.id,
          taskName: t.name,
          sessionId: s.id,
          date: s.date,
          done: s.done,
          color: t.color,
          adaptive: t.adaptive,
        });
        m.set(s.date, list);
      }
    }
    return m;
  })();

  type Cell = { iso: string; day: number; inMonth: boolean; isToday: boolean; sessions: DaySession[] };

  // A fixed 6-week grid (42 cells) so the calendar height stays stable while
  // paging between months. Leading/trailing days spill into adjacent months.
  $: cells = (() => {
    const first = new Date(viewYear, viewMonth, 1);
    const lead = (first.getDay() + 6) % 7; // days from the preceding Monday to the 1st
    // $today (not a one-off read) so the "today" cell + event statuses refresh
    // when the day rolls over while the calendar is open.
    const todayStr = $today;
    const out: Cell[] = [];
    const cur = new Date(viewYear, viewMonth, 1 - lead);
    for (let i = 0; i < 42; i++) {
      const iso = toISO(cur);
      out.push({
        iso,
        day: cur.getDate(),
        inMonth: cur.getMonth() === viewMonth,
        isToday: iso === todayStr,
        sessions: byDate.get(iso) ?? [],
      });
      cur.setDate(cur.getDate() + 1);
    }
    return out;
  })();

  // Count of sessions that actually fall inside the month on screen.
  $: monthCount = cells.reduce((n, c) => n + (c.inMonth ? c.sessions.length : 0), 0);

  function prevMonth() {
    if (viewMonth === 0) {
      viewMonth = 11;
      viewYear -= 1;
    } else viewMonth -= 1;
  }

  function nextMonth() {
    if (viewMonth === 11) {
      viewMonth = 0;
      viewYear += 1;
    } else viewMonth += 1;
  }

  function goToday() {
    const t = new Date();
    viewYear = t.getFullYear();
    viewMonth = t.getMonth();
  }

  const run = makeMutator({
    state: (s) => dispatch('changed', s),
    error: (m) => dispatch('error', m),
    busy: (b) => (busy = b),
  });

  // Checking off a session of an adaptive task asks for a grade instead.
  let gradeTarget: DaySession | null = null;

  function toggle(s: DaySession) {
    if (s.adaptive && !s.done) {
      gradeTarget = s;
      return;
    }
    void run(ToggleSession(s.taskId, s.sessionId));
  }

  // No `busy` pre-check: it is also set by toggles and quick-add, so guarding
  // on it would silently drop a grade picked while an unrelated call is in
  // flight. Double-grading the same session is impossible — the modal unmounts
  // on the first choice.
  function onGrade(e: CustomEvent<string>) {
    const target = gradeTarget;
    gradeTarget = null;
    if (target) void run(GradeSession(target.taskId, target.sessionId, e.detail));
  }

  // Quick-add: the "+" on a day cell picks a task for a session on that date.
  let pickDate: string | null = null;

  $: taskActions = [
    ...viewTasks.map((t) => ({ value: t.id, label: t.name, color: taskHex(t.color) })),
    { value: 'cancel', label: 'Cancel', kind: 'ghost' },
  ] as ModalAction[];

  function onPickTask(e: CustomEvent<string>) {
    const date = pickDate;
    pickDate = null;
    if (date && e.detail !== 'cancel') void run(AddSession(e.detail, date));
  }

  // Status → Tailwind utilities for a calendar event chip.
  const evClass = (status: string) =>
    status === 'overdue' ? 'shadow-[inset_3px_0_0_var(--red)]'
    : status === 'today' ? 'shadow-[inset_3px_0_0_var(--amber)] font-bold'
    : status === 'done' ? 'opacity-50 line-through'
    : '';
</script>

<section>
  <div class="mb-4 flex flex-wrap items-center justify-between gap-4">
    <div class="flex items-center gap-2">
      <button class="icon-btn px-[0.55rem] py-[0.1rem] text-[1.35rem]" title="Previous month" on:click={prevMonth}>‹</button>
      <h2 class="tnum m-0 min-w-[10rem] text-center font-display text-[1.2rem] font-bold tracking-[-0.01em] text-fg-strong">{MONTHS[viewMonth]} {viewYear}</h2>
      <button class="icon-btn px-[0.55rem] py-[0.1rem] text-[1.35rem]" title="Next month" on:click={nextMonth}>›</button>
    </div>
    <div class="flex items-center gap-[0.85rem]">
      <span class="tnum text-[0.82rem] text-fg-muted">{monthCount} session{plural(monthCount)}</span>
      <button class="btn ghost" on:click={goToday}>Today</button>
    </div>
  </div>

  {#if subjects.length}
    <div class="mb-4 flex flex-wrap items-center gap-[0.4rem]">
      <span class="mr-[0.2rem] text-[0.72rem] font-semibold uppercase tracking-[0.06em] text-fg-faint">Subject</span>
      <button
        class="cursor-pointer rounded-sm border px-[0.6rem] py-[0.22rem] text-[0.74rem] font-semibold transition-colors {subjectFilter === '' ? 'border-accent-bright bg-[var(--accent-grad)] text-white' : 'border-line bg-surface-2 text-fg-muted hover:border-line-strong hover:text-fg'}"
        on:click={() => (subjectFilter = '')}
      >All</button>
      {#each subjects as s (s.id)}
        <button
          class="inline-flex cursor-pointer items-center gap-[0.35rem] rounded-sm border px-[0.6rem] py-[0.22rem] text-[0.74rem] font-semibold transition-colors {subjectFilter === s.id ? 'border-accent-bright bg-[var(--accent-grad)] text-white' : 'border-line bg-surface-2 text-fg-muted hover:border-line-strong hover:text-fg'}"
          on:click={() => (subjectFilter = s.id)}
        ><span class="h-[8px] w-[8px] rounded-full" style="background:{taskHex(s.color)}"></span>{s.name}</button>
      {/each}
      {#if hasUngrouped}
        <button
          class="cursor-pointer rounded-sm border px-[0.6rem] py-[0.22rem] text-[0.74rem] font-semibold transition-colors {subjectFilter === UNGROUPED ? 'border-accent-bright bg-[var(--accent-grad)] text-white' : 'border-line bg-surface-2 text-fg-muted hover:border-line-strong hover:text-fg'}"
          on:click={() => (subjectFilter = UNGROUPED)}
        >Ungrouped</button>
      {/if}
    </div>
  {/if}

  <div class="grid grid-cols-7 gap-px overflow-hidden rounded-lg border border-line bg-line shadow-1">
    {#each WEEKDAYS as wd}
      <div class="bg-surface-2 py-2 text-center text-[0.68rem] font-bold uppercase tracking-[0.08em] text-fg-muted">{wd}</div>
    {/each}
    {#each cells as cell (cell.iso)}
      <div class="group flex min-h-[94px] flex-col gap-[0.25rem] px-[0.35rem] pb-[0.4rem] pt-[0.3rem] transition-colors hover:bg-surface-2 {cell.inMonth ? 'bg-surface' : 'bg-inset'}">
        <div class="flex items-start justify-between gap-[0.2rem]">
          <span class="tnum grid h-[1.55rem] min-w-[1.55rem] place-items-center rounded-sm text-[0.76rem] leading-[1.5] {cell.isToday ? 'bg-[var(--accent-grad)] font-bold text-white' : cell.inMonth ? 'text-fg' : 'text-fg opacity-40'}">{cell.day}</span>
          {#if viewTasks.length}
            <button
              class="h-[1.35rem] w-[1.35rem] cursor-pointer rounded-sm border border-transparent bg-transparent text-[0.95rem] leading-none text-fg-muted opacity-0 transition group-hover:opacity-100 focus-visible:opacity-100 hover:border-line hover:bg-surface-3 hover:text-fg-strong disabled:cursor-not-allowed"
              title="Add a session on {formatDate(cell.iso)}"
              aria-label="Add a session on {formatDate(cell.iso)}"
              on:click={() => (pickDate = cell.iso)}
              disabled={busy}
            >+</button>
          {/if}
        </div>
        {#if cell.sessions.length}
          <div class="flex min-w-0 flex-col gap-[0.2rem]">
            {#each cell.sessions as s (s.sessionId)}
              <button
                class="cursor-pointer overflow-hidden text-ellipsis whitespace-nowrap rounded-xs border px-[0.35rem] py-[0.12rem] text-left text-[0.72rem] font-semibold transition-[transform,filter] [background:color-mix(in_srgb,var(--task)_16%,transparent)] [border-color:color-mix(in_srgb,var(--task)_45%,transparent)] [color:color-mix(in_srgb,var(--task)_60%,var(--text-strong))] hover:translate-x-[1px] hover:brightness-[1.15] disabled:cursor-not-allowed {evClass(sessionStatus(s.date, s.done))}"
                style="--task:{taskHex(s.color)}"
                title={`${s.taskName} — ${s.done ? 'done' : relativeLabel(s.date)} (click to toggle)`}
                on:click={() => toggle(s)}
                disabled={busy}
              >
                {s.taskName}
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  </div>
</section>

{#if pickDate}
  <ConfirmModal
    title="Add session — {formatDate(pickDate)}"
    message="Pick a task to study that day."
    actions={taskActions}
    on:choose={onPickTask}
  />
{/if}
{#if gradeTarget}
  <GradeModal taskName={gradeTarget.taskName} on:grade={onGrade} on:cancel={() => (gradeTarget = null)} />
{/if}
