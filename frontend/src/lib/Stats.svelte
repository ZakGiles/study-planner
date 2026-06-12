<script lang="ts">
  import type { main } from '../../wailsjs/go/models';
  import { parseDate, toISO, todayISO, formatDate, plural, MONTHS } from './dates';
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
    const today = todayISO();
    for (const t of topics) {
      for (const s of t.sessions) {
        if (!s.done) continue;
        const day = s.completedAt ? toISO(new Date(s.completedAt)) : s.date;
        if (day > today) continue;
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
    const today = dayNum(todayISO());
    const have = new Set(days);
    let current = 0;
    for (let d = have.has(today) ? today : today - 1; have.has(d); d--) current++;
    return { current, longest };
  })();

  $: totalDone = topics.reduce((n, t) => n + t.sessions.filter((s) => s.done).length, 0);
  $: dueToday = topics
    .filter((t) => !t.archived)
    .reduce((n, t) => n + t.sessions.filter((s) => !s.done && s.date === todayISO()).length, 0);

  type HeatCell = { iso: string; count: number; level: number; future: boolean };

  // A GitHub-style heatmap: WEEKS columns of Monday-first weeks ending in the
  // current week.
  $: weeks = (() => {
    const today = todayISO();
    const t = parseDate(today);
    const dow = (t.getDay() + 6) % 7; // 0 = Monday
    const cur = new Date(t);
    cur.setDate(cur.getDate() - dow - (WEEKS - 1) * 7);
    const out: HeatCell[][] = [];
    for (let w = 0; w < WEEKS; w++) {
      const col: HeatCell[] = [];
      for (let d = 0; d < 7; d++) {
        const iso = toISO(cur);
        const count = doneByDay.get(iso) ?? 0;
        col.push({ iso, count, level: Math.min(4, count), future: iso > today });
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
</script>

<section class="stats">
  <div class="tiles reveal">
    <div class="tile">
      <span class="tile-num tnum">{streaks.current}</span>
      <span class="tile-label">day streak{streaks.current > 0 ? ' 🔥' : ''}</span>
    </div>
    <div class="tile">
      <span class="tile-num tnum">{streaks.longest}</span>
      <span class="tile-label">longest streak</span>
    </div>
    <div class="tile">
      <span class="tile-num tnum">{totalDone}</span>
      <span class="tile-label">sessions completed</span>
    </div>
    <div class="tile">
      <span class="tile-num tnum">{dueToday}</span>
      <span class="tile-label">due today</span>
    </div>
  </div>

  <div class="panel reveal">
    <div class="panel-head">
      <h2>Last {WEEKS} weeks</h2>
      <span class="legend">
        less
        {#each [0, 1, 2, 3, 4] as l}<span class="cell l{l}"></span>{/each}
        more
      </span>
    </div>
    <div class="heatmap">
      <div class="gutter">
        <span></span><span>Mon</span><span></span><span>Wed</span><span></span><span>Fri</span><span></span>
      </div>
      <div class="weeks">
        <div class="months" style="--weeks:{WEEKS}">
          {#each monthLabels as m}<span>{m}</span>{/each}
        </div>
        <div class="grid-cols">
          {#each weeks as col}
            <div class="week">
              {#each col as c (c.iso)}
                <span class="cell l{c.level}" class:future={c.future} title={cellTitle(c)}></span>
              {/each}
            </div>
          {/each}
        </div>
      </div>
    </div>
  </div>

  {#if byTopic.length}
    <div class="panel reveal">
      <div class="panel-head">
        <h2>By topic</h2>
      </div>
      <ul class="topic-rows">
        {#each byTopic as t (t.id)}
          <li class:archived={t.archived}>
            <span class="dot" style="--topic:{t.hex}"></span>
            <span class="name">{t.name}{#if t.archived}<span class="arch-tag">archived</span>{/if}</span>
            <span class="bar"><span class="fill" style="width:{(t.done / t.total) * 100}%; background:{t.hex}"></span></span>
            <span class="count tnum">{t.done}/{t.total}</span>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
</section>

<style>
  .stats {
    display: flex;
    flex-direction: column;
    gap: 1.1rem;
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 0.8rem;
  }

  .tile {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-1);
    padding: 0.9rem 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .tile-num {
    font-family: var(--font-display);
    font-weight: 800;
    font-size: 1.7rem;
    color: var(--text-strong);
    line-height: 1;
  }

  .tile-label {
    font-size: 0.78rem;
    color: var(--muted);
  }

  .panel {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-1);
    padding: 1rem 1.2rem 1.1rem;
  }

  .panel-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.5rem;
    margin-bottom: 0.85rem;
  }

  .panel-head h2 {
    margin: 0;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 1rem;
    color: var(--text-strong);
  }

  .legend {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 0.7rem;
    color: var(--faint);
  }

  .heatmap {
    display: flex;
    gap: 6px;
    overflow-x: auto;
    padding-bottom: 0.2rem;
  }

  .gutter {
    display: grid;
    grid-template-rows: repeat(7, 13px);
    gap: 3px;
    margin-top: 17px; /* aligns with cells below the month labels */
    font-size: 0.62rem;
    color: var(--faint);
  }
  .gutter span {
    line-height: 13px;
  }

  .weeks {
    min-width: 0;
  }

  .months {
    display: grid;
    grid-template-columns: repeat(var(--weeks), 13px);
    gap: 3px;
    height: 14px;
    margin-bottom: 3px;
    font-size: 0.62rem;
    color: var(--faint);
    white-space: nowrap;
  }

  .grid-cols {
    display: flex;
    gap: 3px;
  }

  .week {
    display: grid;
    grid-template-rows: repeat(7, 13px);
    gap: 3px;
  }

  .cell {
    width: 13px;
    height: 13px;
    border-radius: 3px;
    background: var(--inset);
    border: 1px solid var(--border-soft);
  }

  .cell.l1 {
    background: color-mix(in srgb, var(--accent) 30%, var(--inset));
    border-color: transparent;
  }
  .cell.l2 {
    background: color-mix(in srgb, var(--accent) 55%, var(--inset));
    border-color: transparent;
  }
  .cell.l3 {
    background: color-mix(in srgb, var(--accent) 78%, var(--inset));
    border-color: transparent;
  }
  .cell.l4 {
    background: var(--accent-bright);
    border-color: transparent;
    box-shadow: 0 0 8px -2px var(--accent-glow);
  }

  .cell.future {
    opacity: 0.35;
  }

  .topic-rows {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
  }

  .topic-rows li {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .topic-rows li.archived {
    opacity: 0.55;
  }

  .dot {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--topic);
    flex: 0 0 auto;
  }

  .name {
    flex: 0 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.88rem;
    color: var(--text);
  }

  .arch-tag {
    margin-left: 0.4rem;
    font-size: 0.64rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--faint);
  }

  .bar {
    flex: 1;
    height: 6px;
    min-width: 60px;
    background: var(--inset);
    border: 1px solid var(--border-soft);
    border-radius: 99px;
    overflow: hidden;
  }

  .fill {
    display: block;
    height: 100%;
    border-radius: 99px;
  }

  .count {
    font-size: 0.76rem;
    color: var(--muted);
    flex: 0 0 auto;
  }
</style>
