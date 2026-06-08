<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { main } from '../../wailsjs/go/models';
  import { ToggleSession } from '../../wailsjs/go/main/App.js';
  import { parseDate, toISO, todayISO, daysFromToday, relativeLabel } from './dates';

  export let topics: main.Topic[] = [];

  const dispatch = createEventDispatcher<{ changed: main.Topic[]; error: string }>();

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

  type DaySession = { topicId: string; topicName: string; sessionId: string; date: string; done: boolean };

  // Index every session by its date so each day cell is a cheap lookup.
  $: byDate = (() => {
    const m = new Map<string, DaySession[]>();
    for (const t of topics) {
      for (const s of t.sessions) {
        const list = m.get(s.date) ?? [];
        list.push({ topicId: t.id, topicName: t.name, sessionId: s.id, date: s.date, done: s.done });
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
    const today = todayISO();
    const out: Cell[] = [];
    const cur = new Date(viewYear, viewMonth, 1 - lead);
    for (let i = 0; i < 42; i++) {
      const iso = toISO(cur);
      out.push({
        iso,
        day: cur.getDate(),
        inMonth: cur.getMonth() === viewMonth,
        isToday: iso === today,
        sessions: byDate.get(iso) ?? [],
      });
      cur.setDate(cur.getDate() + 1);
    }
    return out;
  })();

  // Count of sessions that actually fall inside the month on screen.
  $: monthCount = topics.reduce(
    (n, t) =>
      n +
      t.sessions.filter((s) => {
        const d = parseDate(s.date);
        return d.getFullYear() === viewYear && d.getMonth() === viewMonth;
      }).length,
    0
  );

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

  function sessionClass(date: string, done: boolean): string {
    if (done) return 'done';
    const n = daysFromToday(date);
    if (n < 0) return 'overdue';
    if (n === 0) return 'today';
    return 'upcoming';
  }

  async function toggle(s: DaySession) {
    busy = true;
    try {
      dispatch('changed', await ToggleSession(s.topicId, s.sessionId));
    } catch (e) {
      dispatch('error', String(e));
    } finally {
      busy = false;
    }
  }
</script>

<section class="calendar">
  <div class="cal-head">
    <div class="cal-nav">
      <button class="icon-btn nav" title="Previous month" on:click={prevMonth}>‹</button>
      <h2>{MONTHS[viewMonth]} {viewYear}</h2>
      <button class="icon-btn nav" title="Next month" on:click={nextMonth}>›</button>
    </div>
    <div class="cal-actions">
      <span class="cal-count">{monthCount} session{monthCount === 1 ? '' : 's'}</span>
      <button class="btn ghost" on:click={goToday}>Today</button>
    </div>
  </div>

  <div class="grid">
    {#each WEEKDAYS as wd}
      <div class="weekday">{wd}</div>
    {/each}
    {#each cells as cell (cell.iso)}
      <div class="cell" class:out={!cell.inMonth} class:today={cell.isToday}>
        <span class="day-num">{cell.day}</span>
        {#if cell.sessions.length}
          <div class="cell-sessions">
            {#each cell.sessions as s (s.sessionId)}
              <button
                class="ev {sessionClass(s.date, s.done)}"
                title={`${s.topicName} — ${s.done ? 'done' : relativeLabel(s.date)} (click to toggle)`}
                on:click={() => toggle(s)}
                disabled={busy}
              >
                {s.topicName}
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  </div>
</section>

<style>
  .cal-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    flex-wrap: wrap;
    margin-bottom: 0.9rem;
  }

  .cal-nav {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }

  .cal-nav h2 {
    margin: 0;
    font-size: 1.05rem;
    color: var(--text);
    min-width: 9.5rem;
    text-align: center;
  }

  .icon-btn.nav {
    font-size: 1.4rem;
    color: var(--muted);
    padding: 0.1rem 0.5rem;
  }

  .cal-actions {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .cal-count {
    font-size: 0.82rem;
    color: var(--muted);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(7, minmax(0, 1fr));
    gap: 1px;
    background: var(--border);
    border: 1px solid var(--border);
    border-radius: 12px;
    overflow: hidden;
  }

  .weekday {
    background: var(--chip);
    color: var(--muted);
    font-size: 0.72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    text-align: center;
    padding: 0.45rem 0;
  }

  .cell {
    background: var(--card);
    min-height: 92px;
    padding: 0.3rem 0.35rem 0.4rem;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .cell.out {
    background: var(--chip);
  }

  .cell.out .day-num {
    opacity: 0.5;
  }

  .day-num {
    font-size: 0.78rem;
    color: var(--text);
    line-height: 1.5;
    min-width: 1.5rem;
    height: 1.5rem;
    text-align: center;
    align-self: flex-start;
  }

  .cell.today .day-num {
    background: var(--accent);
    color: #fff;
    border-radius: 99px;
    font-weight: 700;
  }

  .cell-sessions {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .ev {
    font: inherit;
    font-size: 0.72rem;
    text-align: left;
    border: 1px solid transparent;
    border-radius: 6px;
    padding: 0.1rem 0.32rem;
    cursor: pointer;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    transition: opacity 0.15s ease;
  }

  .ev:hover:not(:disabled) {
    opacity: 0.82;
  }

  .ev:disabled {
    cursor: not-allowed;
  }

  .ev.upcoming {
    background: #eef0fd;
    border-color: #d6dafc;
    color: var(--accent);
  }

  .ev.today {
    background: #fff7ec;
    border-color: #fcd9a3;
    color: var(--warn);
  }

  .ev.overdue {
    background: #fef2f1;
    border-color: #fcaca7;
    color: var(--danger);
  }

  .ev.done {
    background: var(--chip);
    border-color: var(--border);
    color: var(--muted);
    text-decoration: line-through;
  }
</style>
