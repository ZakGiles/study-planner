<script lang="ts">
  import type { main } from '../../wailsjs/go/models';
  import { parseDate, toISO, formatDate, plural, MONTHS } from './dates';
  import { today } from './today';
  import { taskHex } from './colors';
  import { computeDoneByDay, computeStreaks, dueToday as countDueToday } from './stats';
  import SubjectFilter, { UNGROUPED } from './SubjectFilter.svelte';

  // All tasks, archived included — completed history shouldn't vanish when a
  // task is shelved.
  export let tasks: main.Task[] = [];
  // Subjects, for the optional subject filter (repeated in the section headers).
  export let subjects: main.Subject[] = [];
  // Completed focus blocks from the timer, owned by App.
  export let focusSessions: main.FocusSession[] = [];

  // Subject filter: '' = all subjects, a subject id, or the UNGROUPED sentinel.
  // Every stat below is computed from viewTasks/viewFocus, so the whole page
  // narrows to the chosen subject in one place.
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

  // Focus narrows the same way: a subject's blocks are those tied to its tasks;
  // the Ungrouped bucket also collects general focus (no task). "All" keeps every
  // block, including general focus.
  $: viewTaskIds = new Set(viewTasks.map((t) => t.id));
  $: viewFocus =
    subjectFilter === ''
      ? focusSessions
      : subjectFilter === UNGROUPED
        ? focusSessions.filter((f) => f.taskId === '' || viewTaskIds.has(f.taskId))
        : focusSessions.filter((f) => viewTaskIds.has(f.taskId));

  const WEEKS = 26;

  // doneByDay/streaks come from the shared stats module so Home and Stats agree.
  $: doneByDay = computeDoneByDay(viewTasks, $today);
  $: streaks = computeStreaks(doneByDay, $today);

  $: totalDone = viewTasks.reduce((n, t) => n + t.sessions.filter((s) => s.done).length, 0);
  $: dueToday = countDueToday(viewTasks, $today);

  type HeatCell = { iso: string; count: number; level: number; future: boolean };

  // A GitHub-style heatmap: WEEKS columns of Monday-first weeks ending in the
  // current week.
  $: weeks = (() => {
    const todayStr = $today;
    const t = parseDate(todayStr);
    const dow = (t.getDay() + 6) % 7; // 0 = Monday
    const cur = new Date(t);
    cur.setDate(cur.getDate() - dow - (WEEKS - 1) * 7);
    const out: HeatCell[][] = [];
    for (let w = 0; w < WEEKS; w++) {
      const col: HeatCell[] = [];
      for (let d = 0; d < 7; d++) {
        const iso = toISO(cur);
        const count = doneByDay.get(iso) ?? 0;
        col.push({ iso, count, level: Math.min(4, count), future: iso > todayStr });
        cur.setDate(cur.getDate() + 1);
      }
      out.push(col);
    }
    return out;
  })();

  // Label a column with its month when it contains the 1st.
  $: monthLabels = weeks.map((col) => {
    const first = col.find((c) => parseDate(c.iso).getDate() === 1);
    return first ? MONTHS[parseDate(first.iso).getMonth()] : '';
  });

  $: byTask = viewTasks
    .map((t) => ({
      id: t.id,
      name: t.name,
      archived: t.archived,
      hex: taskHex(t.color),
      done: t.sessions.filter((s) => s.done).length,
      total: t.sessions.length,
    }))
    .filter((t) => t.total > 0);

  function cellTitle(c: HeatCell): string {
    if (c.future) return formatDate(c.iso);
    return `${c.count} session${plural(c.count)} — ${formatDate(c.iso)}`;
  }

  // ---- Focus time (from the Pomodoro timer) ----
  function fmtDuration(sec: number): string {
    const m = Math.round(sec / 60);
    if (m < 60) return `${m}m`;
    const h = Math.floor(m / 60);
    const r = m % 60;
    return r ? `${h}h ${r}m` : `${h}h`;
  }

  // Focus seconds completed per local day, capped at today.
  $: focusByDay = (() => {
    const m = new Map<string, number>();
    const todayStr = $today;
    for (const f of viewFocus) {
      const day = toISO(new Date(f.completedAt));
      if (day > todayStr) continue;
      m.set(day, (m.get(day) ?? 0) + f.durationSec);
    }
    return m;
  })();

  $: totalFocusSec = viewFocus.reduce((n, f) => n + f.durationSec, 0);

  // Reuse the heatmap's date layout (weeks), swapping in focus minutes. Levels
  // step at 25/50/90 minutes so a single Pomodoro already registers.
  function focusLevel(sec: number): number {
    const m = sec / 60;
    if (m <= 0) return 0;
    if (m < 25) return 1;
    if (m < 50) return 2;
    if (m < 90) return 3;
    return 4;
  }
  $: focusWeeks = weeks.map((col) =>
    col.map((c) => {
      const sec = focusByDay.get(c.iso) ?? 0;
      return { iso: c.iso, sec, level: focusLevel(sec), future: c.future };
    })
  );
  function focusCellTitle(iso: string, sec: number, future: boolean): string {
    if (future) return formatDate(iso);
    return `${fmtDuration(sec)} focused — ${formatDate(iso)}`;
  }

  $: focusByTask = (() => {
    const m = new Map<string, number>();
    for (const f of viewFocus) m.set(f.taskId, (m.get(f.taskId) ?? 0) + f.durationSec);
    return [...m.entries()]
      .map(([id, sec]) => {
        const t = tasks.find((x) => x.id === id);
        return {
          id,
          sec,
          name: id === '' ? 'General focus' : t?.name ?? 'Deleted task',
          hex: id === '' || !t ? 'var(--muted)' : taskHex(t.color),
          known: id !== '' && !!t,
        };
      })
      .sort((a, b) => b.sec - a.sec);
  })();

  // Warm palette so the focus heatmap reads distinctly from the accent-blue
  // sessions one above it.
  const FOCUS_HEAT = [
    'border border-line-soft bg-inset',
    'border border-transparent [background:color-mix(in_srgb,var(--amber)_30%,var(--inset))]',
    'border border-transparent [background:color-mix(in_srgb,var(--amber)_55%,var(--inset))]',
    'border border-transparent [background:color-mix(in_srgb,var(--amber)_80%,var(--inset))]',
    'border border-transparent bg-amber',
  ];

  // Heatmap intensity (0–4) → Tailwind utilities, mixing the accent into the
  // inset so both themes stay legible.
  const HEAT = [
    'border border-line-soft bg-inset',
    'border border-transparent [background:color-mix(in_srgb,var(--accent)_30%,var(--inset))]',
    'border border-transparent [background:color-mix(in_srgb,var(--accent)_55%,var(--inset))]',
    'border border-transparent [background:color-mix(in_srgb,var(--accent)_78%,var(--inset))]',
    'border border-transparent bg-accent-bright',
  ];
</script>

<section class="flex flex-col gap-[1.1rem]">
  {#if subjects.length}
    <SubjectFilter {subjects} {hasUngrouped} bind:value={subjectFilter} />
  {/if}

  <div class="grid grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-[0.8rem]">
    <div class="flex flex-col gap-[0.2rem] rounded-lg border border-line bg-surface px-4 py-[0.9rem] shadow-1">
      <span class="tnum font-display text-[1.7rem] font-extrabold leading-none text-fg-strong">{streaks.current}</span>
      <span class="text-[0.78rem] text-fg-muted">day streak{streaks.current > 0 ? ' ✦' : ''}</span>
    </div>
    <div class="flex flex-col gap-[0.2rem] rounded-lg border border-line bg-surface px-4 py-[0.9rem] shadow-1">
      <span class="tnum font-display text-[1.7rem] font-extrabold leading-none text-fg-strong">{streaks.longest}</span>
      <span class="text-[0.78rem] text-fg-muted">longest streak</span>
    </div>
    <div class="flex flex-col gap-[0.2rem] rounded-lg border border-line bg-surface px-4 py-[0.9rem] shadow-1">
      <span class="tnum font-display text-[1.7rem] font-extrabold leading-none text-fg-strong">{totalDone}</span>
      <span class="text-[0.78rem] text-fg-muted">sessions completed</span>
    </div>
    <div class="flex flex-col gap-[0.2rem] rounded-lg border border-line bg-surface px-4 py-[0.9rem] shadow-1">
      <span class="tnum font-display text-[1.7rem] font-extrabold leading-none text-fg-strong">{dueToday}</span>
      <span class="text-[0.78rem] text-fg-muted">due today</span>
    </div>
    <div class="flex flex-col gap-[0.2rem] rounded-lg border border-line bg-surface px-4 py-[0.9rem] shadow-1">
      <span class="tnum font-display text-[1.7rem] font-extrabold leading-none text-fg-strong">{fmtDuration(totalFocusSec)}</span>
      <span class="text-[0.78rem] text-fg-muted">time focused</span>
    </div>
  </div>

  {#if byTask.length}
    <div class="rounded-lg border border-line bg-surface px-[1.2rem] pb-[1.1rem] pt-4 shadow-1">
      <div class="mb-[0.85rem] flex items-baseline justify-between gap-2">
        <h2 class="m-0 font-display text-base font-bold text-fg-strong">By task</h2>
      </div>
      <ul class="m-0 flex list-none flex-col gap-[0.55rem] p-0">
        {#each byTask as t (t.id)}
          <li class="flex items-center gap-[0.6rem] {t.archived ? 'opacity-[0.55]' : ''}">
            <span class="h-[9px] w-[9px] shrink-0 rounded-full" style="background:{t.hex}"></span>
            <span class="min-w-0 flex-[0_1_auto] overflow-hidden text-ellipsis whitespace-nowrap text-[0.88rem] text-fg">{t.name}{#if t.archived}<span class="ml-[0.4rem] text-[0.64rem] uppercase tracking-[0.06em] text-fg-faint">archived</span>{/if}</span>
            <span class="bar min-w-[60px]"><span class="fill" style="width:{(t.done / t.total) * 100}%; background:{t.hex}"></span></span>
            <span class="tnum shrink-0 text-[0.76rem] text-fg-muted">{t.done}/{t.total}</span>
          </li>
        {/each}
      </ul>
    </div>
  {/if}

  <div class="rounded-lg border border-line bg-surface px-[1.2rem] pb-[1.1rem] pt-4 shadow-1">
    <div class="mb-[0.85rem] flex items-baseline justify-between gap-2">
      <h2 class="m-0 font-display text-base font-bold text-fg-strong">Last {WEEKS} weeks</h2>
      <span class="inline-flex items-center gap-[3px] text-[0.7rem] text-fg-faint">
        less
        {#each [0, 1, 2, 3, 4] as l}<span class="h-[13px] w-[13px] rounded-[3px] {HEAT[l]}"></span>{/each}
        more
      </span>
    </div>
    {#if subjects.length}
      <div class="mb-[0.85rem]"><SubjectFilter {subjects} {hasUngrouped} bind:value={subjectFilter} /></div>
    {/if}
    <div class="flex gap-[6px] overflow-x-auto pb-[0.2rem]">
      <div class="mt-[17px] grid grid-rows-[repeat(7,13px)] gap-[3px] text-[0.62rem] leading-[13px] text-fg-faint">
        <span></span><span>Mon</span><span></span><span>Wed</span><span></span><span>Fri</span><span></span>
      </div>
      <div class="min-w-0">
        <div class="mb-[3px] grid h-[14px] grid-cols-[repeat(var(--weeks),13px)] gap-[3px] whitespace-nowrap text-[0.62rem] text-fg-faint" style="--weeks:{WEEKS}">
          {#each monthLabels as m}<span>{m}</span>{/each}
        </div>
        <div class="flex gap-[3px]">
          {#each weeks as col}
            <div class="grid grid-rows-[repeat(7,13px)] gap-[3px]">
              {#each col as c (c.iso)}
                <span class="h-[13px] w-[13px] rounded-[3px] {HEAT[c.level]} {c.future ? 'opacity-[0.35]' : ''}" title={cellTitle(c)}></span>
              {/each}
            </div>
          {/each}
        </div>
      </div>
    </div>
  </div>

  <div class="rounded-lg border border-line bg-surface px-[1.2rem] pb-[1.1rem] pt-4 shadow-1">
    <div class="mb-[0.85rem] flex items-baseline justify-between gap-2">
      <h2 class="m-0 font-display text-base font-bold text-fg-strong">Focus time</h2>
      <span class="inline-flex items-center gap-[3px] text-[0.7rem] text-fg-faint">
        less
        {#each [0, 1, 2, 3, 4] as l}<span class="h-[13px] w-[13px] rounded-[3px] {FOCUS_HEAT[l]}"></span>{/each}
        more
      </span>
    </div>
    {#if subjects.length}
      <div class="mb-[0.85rem]"><SubjectFilter {subjects} {hasUngrouped} bind:value={subjectFilter} /></div>
    {/if}
    <div class="flex gap-[6px] overflow-x-auto pb-[0.2rem]">
      <div class="mt-[17px] grid grid-rows-[repeat(7,13px)] gap-[3px] text-[0.62rem] leading-[13px] text-fg-faint">
        <span></span><span>Mon</span><span></span><span>Wed</span><span></span><span>Fri</span><span></span>
      </div>
      <div class="min-w-0">
        <div class="mb-[3px] grid h-[14px] grid-cols-[repeat(var(--weeks),13px)] gap-[3px] whitespace-nowrap text-[0.62rem] text-fg-faint" style="--weeks:{WEEKS}">
          {#each monthLabels as m}<span>{m}</span>{/each}
        </div>
        <div class="flex gap-[3px]">
          {#each focusWeeks as col}
            <div class="grid grid-rows-[repeat(7,13px)] gap-[3px]">
              {#each col as c (c.iso)}
                <span class="h-[13px] w-[13px] rounded-[3px] {FOCUS_HEAT[c.level]} {c.future ? 'opacity-[0.35]' : ''}" title={focusCellTitle(c.iso, c.sec, c.future)}></span>
              {/each}
            </div>
          {/each}
        </div>
      </div>
    </div>

    {#if focusByTask.length}
      <ul class="m-0 mt-[1.1rem] flex list-none flex-col gap-[0.55rem] p-0">
        {#each focusByTask as row (row.id)}
          <li class="flex items-center gap-[0.6rem]">
            <span class="h-[9px] w-[9px] shrink-0 rounded-full" style="background:{row.hex}"></span>
            <span class="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap text-[0.88rem] {row.known ? 'text-fg' : 'italic text-fg-muted'}">{row.name}</span>
            <span class="tnum shrink-0 text-[0.76rem] text-fg-muted">{fmtDuration(row.sec)}</span>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="muted mt-[1.1rem] text-[0.84rem]">No focus time logged{subjectFilter === '' ? ' yet' : ' for this subject yet'} — start a block on the Focus tab.</p>
    {/if}
  </div>
</section>

