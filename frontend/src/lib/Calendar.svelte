<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { main } from '../../wailsjs/go/models';
  import { AddSession, GradeSession, ToggleSession } from '../../wailsjs/go/main/App.js';
  import { toISO, todayISO, formatDate, relativeLabel, sessionStatus } from './dates';
  import { topicHex } from './colors';
  import ConfirmModal from './ConfirmModal.svelte';
  import type { ModalAction } from './ConfirmModal.svelte';
  import { GRADE_ACTIONS, GRADE_VALUES } from './grades';

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

  type DaySession = {
    topicId: string;
    topicName: string;
    sessionId: string;
    date: string;
    done: boolean;
    color: string;
    adaptive: boolean;
  };

  // Index every session by its date so each day cell is a cheap lookup.
  $: byDate = (() => {
    const m = new Map<string, DaySession[]>();
    for (const t of topics) {
      for (const s of t.sessions) {
        const list = m.get(s.date) ?? [];
        list.push({
          topicId: t.id,
          topicName: t.name,
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

  // Checking off a session of an adaptive topic asks for a grade instead.
  let gradeTarget: DaySession | null = null;

  async function toggle(s: DaySession) {
    if (s.adaptive && !s.done) {
      gradeTarget = s;
      return;
    }
    busy = true;
    try {
      dispatch('changed', await ToggleSession(s.topicId, s.sessionId));
    } catch (e) {
      dispatch('error', String(e));
    } finally {
      busy = false;
    }
  }

  async function onGradeChoose(e: CustomEvent<string>) {
    const target = gradeTarget;
    gradeTarget = null;
    if (!target || !GRADE_VALUES.includes(e.detail)) return;
    busy = true;
    try {
      dispatch('changed', await GradeSession(target.topicId, target.sessionId, e.detail));
    } catch (err) {
      dispatch('error', String(err));
    } finally {
      busy = false;
    }
  }

  // Quick-add: the "+" on a day cell picks a topic for a session on that date.
  let pickDate: string | null = null;

  $: topicActions = [
    ...topics.map((t) => ({ value: t.id, label: t.name, color: topicHex(t.color) })),
    { value: 'cancel', label: 'Cancel', kind: 'ghost' },
  ] as ModalAction[];

  async function onPickTopic(e: CustomEvent<string>) {
    const date = pickDate;
    pickDate = null;
    if (!date || e.detail === 'cancel') return;
    busy = true;
    try {
      dispatch('changed', await AddSession(e.detail, date));
    } catch (err) {
      dispatch('error', String(err));
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
        <div class="cell-head">
          <span class="day-num tnum">{cell.day}</span>
          {#if topics.length}
            <button
              class="add-day"
              title="Add a session on {formatDate(cell.iso)}"
              aria-label="Add a session on {formatDate(cell.iso)}"
              on:click={() => (pickDate = cell.iso)}
              disabled={busy}
            >+</button>
          {/if}
        </div>
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

{#if pickDate}
  <ConfirmModal
    title="Add session — {formatDate(pickDate)}"
    message="Pick a topic to study that day."
    actions={topicActions}
    on:choose={onPickTopic}
  />
{/if}
{#if gradeTarget}
  <ConfirmModal
    title="How did “{gradeTarget.topicName}” go?"
    message="Your grade re-spaces the remaining reviews, starting from today."
    actions={GRADE_ACTIONS}
    on:choose={onGradeChoose}
  />
{/if}

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

  .cell-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.2rem;
  }

  .day-num {
    font-size: 0.76rem;
    color: var(--text);
    line-height: 1.5;
    min-width: 1.55rem;
    height: 1.55rem;
    display: grid;
    place-items: center;
    border-radius: var(--r-sm);
  }

  .add-day {
    border: 1px solid transparent;
    background: transparent;
    color: var(--muted);
    font-size: 0.95rem;
    line-height: 1;
    width: 1.35rem;
    height: 1.35rem;
    border-radius: var(--r-sm);
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.13s ease, background 0.15s ease, color 0.15s ease;
  }
  .cell:hover .add-day,
  .add-day:focus-visible {
    opacity: 1;
  }
  .add-day:hover:not(:disabled) {
    background: var(--surface-3);
    border-color: var(--border);
    color: var(--text-strong);
  }
  .add-day:disabled {
    cursor: not-allowed;
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
    /* Mixing toward --text-strong keeps the label readable on both themes. */
    color: color-mix(in srgb, var(--topic) 60%, var(--text-strong));
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
