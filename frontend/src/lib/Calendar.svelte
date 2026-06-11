<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { main } from '../../wailsjs/go/models';
  import { ToggleSession } from '../../wailsjs/go/main/App.js';
  import { toISO, todayISO, relativeLabel, sessionStatus } from './dates';
  import { topicHex } from './colors';

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

  type DaySession = { topicId: string; topicName: string; sessionId: string; date: string; done: boolean; color: string };

  // Index every session by its date so each day cell is a cheap lookup.
  $: byDate = (() => {
    const m = new Map<string, DaySession[]>();
    for (const t of topics) {
      for (const s of t.sessions) {
        const list = m.get(s.date) ?? [];
        list.push({ topicId: t.id, topicName: t.name, sessionId: s.id, date: s.date, done: s.done, color: t.color });
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
  <div class="cal-head reveal">
    <div class="cal-nav">
      <button class="icon-btn nav" title="Previous month" on:click={prevMonth}>‹</button>
      <h2 class="tnum">{MONTHS[viewMonth]} {viewYear}</h2>
      <button class="icon-btn nav" title="Next month" on:click={nextMonth}>›</button>
    </div>
    <div class="cal-actions">
      <span class="cal-count tnum">{monthCount} session{monthCount === 1 ? '' : 's'}</span>
      <button class="btn ghost" on:click={goToday}>Today</button>
    </div>
  </div>

  <div class="grid reveal">
    {#each WEEKDAYS as wd}
      <div class="weekday">{wd}</div>
    {/each}
    {#each cells as cell (cell.iso)}
      <div class="cell" class:out={!cell.inMonth} class:today={cell.isToday}>
        <span class="day-num tnum">{cell.day}</span>
        {#if cell.sessions.length}
          <div class="cell-sessions">
            {#each cell.sessions as s (s.sessionId)}
              <button
                class="ev {sessionStatus(s.date, s.done)}"
                style="--topic:{topicHex(s.color)}"
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
    margin-bottom: 1rem;
  }

  .cal-nav {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .cal-nav h2 {
    margin: 0;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 1.2rem;
    letter-spacing: -0.01em;
    color: var(--text-strong);
    min-width: 10rem;
    text-align: center;
  }

  .icon-btn.nav {
    font-size: 1.35rem;
    padding: 0.1rem 0.55rem;
  }

  .cal-actions {
    display: flex;
    align-items: center;
    gap: 0.85rem;
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
    border-radius: var(--r-lg);
    overflow: hidden;
    box-shadow: var(--shadow-1);
  }

  .weekday {
    background: var(--surface-2);
    color: var(--muted);
    font-size: 0.68rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    text-align: center;
    padding: 0.5rem 0;
  }

  .cell {
    background: var(--surface);
    min-height: 94px;
    padding: 0.3rem 0.35rem 0.4rem;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    transition: background 0.14s ease;
  }

  .cell:hover {
    background: var(--surface-2);
  }

  .cell.out {
    background: var(--inset);
  }
  .cell.out:hover {
    background: var(--surface-2);
  }

  .cell.out .day-num {
    opacity: 0.4;
  }

  .day-num {
    font-size: 0.76rem;
    color: var(--text);
    line-height: 1.5;
    min-width: 1.55rem;
    height: 1.55rem;
    display: grid;
    place-items: center;
    align-self: flex-start;
    border-radius: var(--r-sm);
  }

  .cell.today .day-num {
    background: var(--accent-grad);
    color: #fff;
    font-weight: 700;
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.25),
      0 0 14px -2px var(--accent-glow);
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
    font-weight: 600;
    text-align: left;
    border: 1px solid;
    border-radius: var(--r-xs);
    padding: 0.12rem 0.35rem;
    cursor: pointer;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    background: color-mix(in srgb, var(--topic) 16%, transparent);
    border-color: color-mix(in srgb, var(--topic) 45%, transparent);
    color: color-mix(in srgb, var(--topic) 78%, white);
    transition: transform 0.12s var(--ease), filter 0.15s ease;
  }

  .ev:hover:not(:disabled) {
    transform: translateX(1px);
    filter: brightness(1.15);
  }

  .ev:disabled {
    cursor: not-allowed;
  }

  .ev.overdue {
    box-shadow: inset 3px 0 0 var(--red);
  }

  .ev.today {
    box-shadow: inset 3px 0 0 var(--amber);
    font-weight: 700;
  }

  .ev.done {
    opacity: 0.5;
    text-decoration: line-through;
  }
</style>
