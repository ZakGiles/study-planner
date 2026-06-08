<script lang="ts">
  import { onMount } from 'svelte';
  import { fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import type { main } from '../wailsjs/go/models';
  import { GetTopics, AddTopic, ToggleSession } from '../wailsjs/go/main/App.js';
  import TopicCard from './lib/TopicCard.svelte';
  import Calendar from './lib/Calendar.svelte';
  import { formatDate, relativeLabel, daysFromToday } from './lib/dates';

  let topics: main.Topic[] = [];
  let activeTab: 'topics' | 'agenda' | 'calendar' = 'topics';
  let loading = true;
  let errorMsg = '';
  let errorTimer: ReturnType<typeof setTimeout>;

  let newName = '';
  let newDescription = '';
  let adding = false;

  onMount(async () => {
    try {
      topics = await GetTopics();
    } catch (e) {
      showError(String(e));
    } finally {
      loading = false;
    }
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
    try {
      topics = await AddTopic(newName, newDescription);
      newName = '';
      newDescription = '';
    } catch (e) {
      showError(String(e));
    } finally {
      adding = false;
    }
  }

  async function toggleFromAgenda(topicId: string, sessionId: string) {
    try {
      topics = await ToggleSession(topicId, sessionId);
    } catch (e) {
      showError(String(e));
    }
  }

  // Flattened, date-sorted list of incomplete sessions for the agenda view.
  type AgendaItem = { topicId: string; topicName: string; sessionId: string; date: string };

  $: agenda = topics
    .flatMap((t) =>
      t.sessions
        .filter((s) => !s.done)
        .map((s) => ({ topicId: t.id, topicName: t.name, sessionId: s.id, date: s.date }))
    )
    .sort((a, b) => a.date.localeCompare(b.date)) as AgendaItem[];

  $: overdueCount = agenda.filter((a) => daysFromToday(a.date) < 0).length;
  $: totalSessions = topics.reduce((n, t) => n + t.sessions.length, 0);
  $: doneSessions = topics.reduce((n, t) => n + t.sessions.filter((s) => s.done).length, 0);
  $: overallPct = totalSessions ? Math.round((doneSessions / totalSessions) * 100) : 0;

  // Group agenda items by date for display.
  $: agendaGroups = (() => {
    const groups: { date: string; items: AgendaItem[] }[] = [];
    for (const item of agenda) {
      const last = groups[groups.length - 1];
      if (last && last.date === item.date) last.items.push(item);
      else groups.push({ date: item.date, items: [item] });
    }
    return groups;
  })();

  function dateClass(date: string): string {
    const n = daysFromToday(date);
    if (n < 0) return 'overdue';
    if (n === 0) return 'today';
    return 'upcoming';
  }
</script>

<div class="shell">
  <header class="topbar">
    <div class="topbar-inner">
      <div class="brand reveal">
        <span class="logo" aria-hidden="true">
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
        <div class="brand-text">
          <span class="eyebrow">Spaced repetition</span>
          <h1>Study Planner</h1>
        </div>
      </div>

      <nav class="tabs reveal">
        <button class:active={activeTab === 'topics'} on:click={() => (activeTab = 'topics')}>
          Topics
        </button>
        <button class:active={activeTab === 'agenda'} on:click={() => (activeTab = 'agenda')}>
          Agenda{#if overdueCount}<span class="badge tnum">{overdueCount}</span>{/if}
        </button>
        <button class:active={activeTab === 'calendar'} on:click={() => (activeTab = 'calendar')}>
          Calendar
        </button>
      </nav>
    </div>
  </header>

  <main>
    {#if loading}
      <div class="loader">
        <span class="spinner" aria-hidden="true"></span>
        <span>Loading…</span>
      </div>
    {:else}
      {#key activeTab}
        <div class="view" in:fly={{ y: 12, duration: 280, easing: cubicOut }}>
          {#if activeTab === 'topics'}
            <section class="panel new-topic reveal">
              <div class="panel-head">
                <h2>New topic</h2>
                <span class="panel-hint">Add something to revise</span>
              </div>
              <form on:submit|preventDefault={createTopic}>
                <input
                  class="name-input"
                  bind:value={newName}
                  placeholder="Topic name (e.g. Linear Algebra)"
                />
                <textarea
                  bind:value={newDescription}
                  rows="2"
                  placeholder="Description (optional) — what to cover, resources, goals…"
                ></textarea>
                <button class="btn primary" type="submit" disabled={adding || !newName.trim()}>
                  Add topic
                </button>
              </form>
            </section>

            {#if topics.length === 0}
              <div class="empty reveal">
                <div class="empty-mark" aria-hidden="true">✦</div>
                <p class="empty-title">No topics yet</p>
                <p class="muted">
                  Add your first topic above, then schedule study dates — manually or with a
                  spaced-repetition plan.
                </p>
              </div>
            {:else}
              <div class="overview reveal">
                <div class="overview-stat">
                  <span class="stat-num tnum">{topics.length}</span>
                  <span class="stat-label">topic{topics.length === 1 ? '' : 's'}</span>
                </div>
                <div class="overview-bar">
                  <div class="overview-bar-head">
                    <span>Overall progress</span>
                    <span class="tnum">{doneSessions}/{totalSessions} · {overallPct}%</span>
                  </div>
                  <div class="bar">
                    <div class="fill" style="width:{overallPct}%"></div>
                  </div>
                </div>
              </div>
              <div class="topic-list">
                {#each topics as topic (topic.id)}
                  <TopicCard {topic} on:changed={onChanged} on:error={onError} />
                {/each}
              </div>
            {/if}
          {:else if activeTab === 'agenda'}
            <section class="agenda">
              <div class="agenda-summary reveal">
                <span class="agenda-count">
                  <span class="stat-num tnum">{agenda.length}</span>
                  upcoming session{agenda.length === 1 ? '' : 's'}
                </span>
                {#if overdueCount}
                  <span class="pill danger tnum">{overdueCount} overdue</span>
                {/if}
              </div>

              {#if agenda.length === 0}
                <div class="empty reveal">
                  <div class="empty-mark" aria-hidden="true">✓</div>
                  <p class="empty-title">All caught up</p>
                  <p class="muted">Nothing scheduled — add dates to your topics to fill your agenda.</p>
                </div>
              {:else}
                <ul class="agenda-list">
                  {#each agendaGroups as group (group.date)}
                    <li class="agenda-day reveal {dateClass(group.date)}">
                      <div class="day-head">
                        <span class="day-date tnum">{formatDate(group.date)}</span>
                        <span class="day-rel tnum">{relativeLabel(group.date)}</span>
                      </div>
                      <ul class="day-items">
                        {#each group.items as item (item.sessionId)}
                          <li class="agenda-item">
                            <label>
                              <input
                                type="checkbox"
                                on:change={() => toggleFromAgenda(item.topicId, item.sessionId)}
                              />
                              <span>{item.topicName}</span>
                            </label>
                          </li>
                        {/each}
                      </ul>
                    </li>
                  {/each}
                </ul>
              {/if}
            </section>
          {:else}
            <Calendar {topics} on:changed={onChanged} on:error={onError} />
          {/if}
        </div>
      {/key}
    {/if}
  </main>
</div>

{#if errorMsg}
  <div class="toast" role="alert" transition:fly={{ y: 24, duration: 260, easing: cubicOut }}>
    <span class="toast-dot" aria-hidden="true"></span>
    <span class="toast-msg">{errorMsg}</span>
  </div>
{/if}

<style>
  .shell {
    min-height: 100%;
  }

  /* ---- Top bar ---- */
  .topbar {
    position: sticky;
    top: 0;
    z-index: 20;
    background: rgba(12, 18, 25, 0.78);
    backdrop-filter: blur(14px) saturate(140%);
    -webkit-backdrop-filter: blur(14px) saturate(140%);
    border-bottom: 1px solid var(--border);
  }

  .topbar-inner {
    max-width: var(--content);
    margin: 0 auto;
    padding: 0.85rem 1.5rem 0;
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1rem 1.5rem;
    flex-wrap: wrap;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding-bottom: 0.8rem;
  }

  .logo {
    width: 34px;
    height: 34px;
    border-radius: 8px;
    background: var(--accent-grad);
    display: grid;
    place-items: center;
    flex: 0 0 auto;
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.3),
      0 4px 16px -4px var(--accent-glow);
  }
  .logo svg {
    width: 20px;
    height: 20px;
  }

  .eyebrow {
    display: block;
    font-size: 0.62rem;
    font-weight: 700;
    letter-spacing: 0.2em;
    text-transform: uppercase;
    color: var(--accent-bright);
    opacity: 0.92;
  }

  .brand h1 {
    margin: 2px 0 0;
    font-family: var(--font-display);
    font-weight: 800;
    font-size: 1.4rem;
    letter-spacing: -0.02em;
    color: var(--text-strong);
    line-height: 1;
  }

  .tabs {
    display: flex;
    gap: 0.15rem;
  }

  .tabs button {
    position: relative;
    border: none;
    background: transparent;
    color: var(--muted);
    font-family: var(--font-body);
    font-weight: 700;
    font-size: 0.78rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    padding: 0.55rem 0.85rem 1.1rem;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    transition: color 0.18s ease;
  }

  .tabs button::after {
    content: '';
    position: absolute;
    left: 0.85rem;
    right: 0.85rem;
    bottom: -1px;
    height: 2px;
    background: var(--accent);
    border-radius: 2px 2px 0 0;
    transform: scaleX(0);
    opacity: 0;
    box-shadow: 0 0 12px var(--accent-glow);
    transition: transform 0.26s var(--ease), opacity 0.2s ease;
  }

  .tabs button:hover {
    color: var(--text);
  }
  .tabs button:hover::after {
    transform: scaleX(0.55);
    opacity: 0.4;
  }
  .tabs button.active {
    color: var(--text-strong);
  }
  .tabs button.active::after {
    transform: scaleX(1);
    opacity: 1;
  }

  .badge {
    background: var(--red);
    color: #fff;
    font-size: 0.66rem;
    font-weight: 700;
    border-radius: var(--r-xs);
    padding: 0.05rem 0.32rem;
    line-height: 1.45;
    box-shadow: 0 0 12px -2px rgba(255, 107, 107, 0.6);
  }

  /* ---- Main column ---- */
  main {
    max-width: var(--content);
    margin: 0 auto;
    padding: 1.5rem 1.5rem 5rem;
    text-align: left;
  }

  .loader {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    color: var(--muted);
    padding: 3rem 0;
  }

  .spinner {
    width: 18px;
    height: 18px;
    border: 2px solid var(--border-strong);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  /* ---- Panels ---- */
  .panel {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    padding: 1.1rem 1.2rem;
    box-shadow: var(--shadow-1);
  }

  .new-topic {
    margin-bottom: 1.4rem;
  }

  .panel-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.5rem;
    margin-bottom: 0.8rem;
  }

  .panel-head h2 {
    margin: 0;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 1rem;
    letter-spacing: -0.01em;
    color: var(--text-strong);
  }

  .panel-hint {
    font-size: 0.78rem;
    color: var(--faint);
  }

  .new-topic form {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .new-topic .btn {
    align-self: flex-start;
  }

  /* ---- Overview stat row ---- */
  .overview {
    display: flex;
    align-items: center;
    gap: 1.25rem;
    margin-bottom: 1.1rem;
    padding: 0 0.15rem;
  }

  .overview-stat {
    display: flex;
    align-items: baseline;
    gap: 0.35rem;
    flex: 0 0 auto;
  }

  .stat-num {
    font-family: var(--font-display);
    font-weight: 800;
    font-size: 1.5rem;
    color: var(--text-strong);
    line-height: 1;
  }

  .stat-label {
    font-size: 0.82rem;
    color: var(--muted);
  }

  .overview-bar {
    flex: 1;
    min-width: 0;
  }

  .overview-bar-head {
    display: flex;
    justify-content: space-between;
    font-size: 0.74rem;
    color: var(--muted);
    margin-bottom: 0.35rem;
  }

  .bar {
    height: 7px;
    background: var(--inset);
    border: 1px solid var(--border-soft);
    border-radius: 99px;
    overflow: hidden;
  }

  .fill {
    height: 100%;
    background: var(--accent-grad);
    border-radius: 99px;
    box-shadow: 0 0 12px -2px var(--accent-glow);
    transition: width 0.4s var(--ease);
  }

  .topic-list {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  /* ---- Empty states ---- */
  .empty {
    text-align: center;
    padding: 3rem 1rem;
    color: var(--text);
  }

  .empty-mark {
    font-size: 1.6rem;
    color: var(--accent-bright);
    margin-bottom: 0.6rem;
    opacity: 0.8;
  }

  .empty-title {
    margin: 0 0 0.3rem;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 1.1rem;
    color: var(--text-strong);
  }

  .muted {
    color: var(--muted);
  }

  .empty .muted {
    font-size: 0.9rem;
    max-width: 44ch;
    margin: 0 auto;
    line-height: 1.5;
  }

  /* ---- Agenda ---- */
  .agenda-summary {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 1.1rem;
  }

  .agenda-count {
    display: inline-flex;
    align-items: baseline;
    gap: 0.4rem;
    color: var(--muted);
    font-size: 0.92rem;
  }

  .pill {
    font-size: 0.72rem;
    font-weight: 600;
    border-radius: var(--r-sm);
    padding: 0.18rem 0.5rem;
  }

  .pill.danger {
    background: var(--red-soft);
    color: var(--red);
    border: 1px solid var(--red-line);
  }

  .agenda-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }

  .agenda-day {
    position: relative;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    padding: 0.75rem 0.95rem 0.75rem 1.1rem;
    overflow: hidden;
    transition: border-color 0.16s ease, transform 0.16s var(--ease);
  }

  /* Colored accent rail on the left edge. */
  .agenda-day::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 3px;
    background: var(--border-strong);
  }
  .agenda-day.overdue::before {
    background: var(--red);
    box-shadow: 0 0 14px var(--red);
  }
  .agenda-day.today::before {
    background: var(--amber);
    box-shadow: 0 0 14px var(--amber);
  }
  .agenda-day.upcoming::before {
    background: var(--accent);
    box-shadow: 0 0 14px var(--accent-glow);
  }

  .agenda-day:hover {
    border-color: var(--border-strong);
    transform: translateX(2px);
  }

  .day-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
  }

  .day-date {
    font-weight: 600;
    color: var(--text-strong);
    font-size: 0.92rem;
  }

  .day-rel {
    font-size: 0.76rem;
    color: var(--muted);
  }

  .agenda-day.overdue .day-rel {
    color: var(--red);
  }
  .agenda-day.today .day-rel {
    color: var(--amber);
    font-weight: 600;
  }

  .day-items {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .agenda-item label {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    cursor: pointer;
    font-size: 0.9rem;
    color: var(--text);
    transition: color 0.15s ease;
  }
  .agenda-item label:hover {
    color: var(--text-strong);
  }

  /* ---- Error toast ---- */
  .toast {
    position: fixed;
    left: 50%;
    bottom: 1.5rem;
    transform: translateX(-50%);
    z-index: 50;
    display: flex;
    align-items: center;
    gap: 0.6rem;
    max-width: min(90vw, 460px);
    background: var(--surface-3);
    border: 1px solid var(--red-line);
    border-radius: var(--r-md);
    padding: 0.7rem 1rem;
    color: var(--text-strong);
    font-size: 0.88rem;
    box-shadow: var(--shadow-pop);
  }

  .toast-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--red);
    flex: 0 0 auto;
    box-shadow: 0 0 10px var(--red);
    animation: pulse 1.4s ease-in-out infinite;
  }
  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
    }
    50% {
      opacity: 0.4;
    }
  }

  .toast-msg {
    line-height: 1.4;
    word-break: break-word;
  }
</style>
