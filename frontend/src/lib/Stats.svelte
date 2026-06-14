<script lang="ts">
  import type { main } from '../../wailsjs/go/models';
  import { parseDate, toISO, formatDate, plural, MONTHS } from './dates';
  import { today } from './today';
  import { topicHex } from './colors';

  // All topics, archived included — completed history shouldn't vanish when a
  // topic is shelved.
  export let topics: main.Topic[] = [];

  const WEEKS = 26;

  // completionDay maps a done session to the local day it was completed.
  // Sessions checked off before completedAt existed fall back to their
  // scheduled date; the few legacy ones dated in the future are skipped.
  $: doneByDay = (() => {
    const m = new Map<string, number>();
    const todayStr = $today;
    for (const t of topics) {
      for (const s of t.sessions) {
        if (!s.done) continue;
        const day = s.completedAt ? toISO(new Date(s.completedAt)) : s.date;
        if (day > todayStr) continue;
        m.set(day, (m.get(day) ?? 0) + 1);
      }
    }
    return m;
  })();

  function dayNum(iso: string): number {
    return Math.round(parseDate(iso).getTime() / 86_400_000);
  }

  // Streaks: consecutive days with at least one completion. The current streak
  // survives until the end of today (an empty today doesn't break yesterday's).
  $: streaks = (() => {
    const days = [...doneByDay.keys()].map(dayNum).sort((a, b) => a - b);
    let longest = 0;
    let run = 0;
    let prev = NaN;
    for (const d of days) {
      run = d === prev + 1 ? run + 1 : 1;
      longest = Math.max(longest, run);
      prev = d;
    }
    const todayNum = dayNum($today);
    const have = new Set(days);
    let current = 0;
    for (let d = have.has(todayNum) ? todayNum : todayNum - 1; have.has(d); d--) current++;
    return { current, longest };
  })();

  $: totalDone = topics.reduce((n, t) => n + t.sessions.filter((s) => s.done).length, 0);
  $: dueToday = topics
    .filter((t) => !t.archived)
    .reduce((n, t) => n + t.sessions.filter((s) => !s.done && s.date === $today).length, 0);

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

  $: byTopic = topics
    .map((t) => ({
      id: t.id,
      name: t.name,
      archived: t.archived,
      hex: topicHex(t.color),
      done: t.sessions.filter((s) => s.done).length,
      total: t.sessions.length,
    }))
    .filter((t) => t.total > 0);

  function cellTitle(c: HeatCell): string {
    if (c.future) return formatDate(c.iso);
    return `${c.count} session${plural(c.count)} — ${formatDate(c.iso)}`;
  }

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
  </div>

  <div class="rounded-lg border border-line bg-surface px-[1.2rem] pb-[1.1rem] pt-4 shadow-1">
    <div class="mb-[0.85rem] flex items-baseline justify-between gap-2">
      <h2 class="m-0 font-display text-base font-bold text-fg-strong">Last {WEEKS} weeks</h2>
      <span class="inline-flex items-center gap-[3px] text-[0.7rem] text-fg-faint">
        less
        {#each [0, 1, 2, 3, 4] as l}<span class="h-[13px] w-[13px] rounded-[3px] {HEAT[l]}"></span>{/each}
        more
      </span>
    </div>
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

  {#if byTopic.length}
    <div class="rounded-lg border border-line bg-surface px-[1.2rem] pb-[1.1rem] pt-4 shadow-1">
      <div class="mb-[0.85rem] flex items-baseline justify-between gap-2">
        <h2 class="m-0 font-display text-base font-bold text-fg-strong">By topic</h2>
      </div>
      <ul class="m-0 flex list-none flex-col gap-[0.55rem] p-0">
        {#each byTopic as t (t.id)}
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
</section>

