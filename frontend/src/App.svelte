<script lang="ts">
  import { onMount } from 'svelte';
  import type { main } from '../wailsjs/go/models';
  import { GetTopics, AddTopic, ToggleSession } from '../wailsjs/go/main/App.js';
  import TopicCard from './lib/TopicCard.svelte';
  import { formatDate, relativeLabel, daysFromToday } from './lib/dates';

  let topics: main.Topic[] = [];
  let activeTab: 'topics' | 'agenda' = 'topics';
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

<main>
  <header class="app-head">
    <div class="brand">
      <h1>📚 Study Planner</h1>
      <p class="tagline">Plan topics and space out your revision.</p>
    </div>
    <nav class="tabs">
      <button class:active={activeTab === 'topics'} on:click={() => (activeTab = 'topics')}>Topics</button>
      <button class:active={activeTab === 'agenda'} on:click={() => (activeTab = 'agenda')}>
        Agenda{#if overdueCount}<span class="badge">{overdueCount}</span>{/if}
      </button>
    </nav>
  </header>

  {#if errorMsg}
    <div class="error" role="alert">{errorMsg}</div>
  {/if}

  {#if loading}
    <p class="loading">Loading…</p>
  {:else if activeTab === 'topics'}
    <section class="new-topic">
      <h2>New topic</h2>
      <form on:submit|preventDefault={createTopic}>
        <input class="name-input" bind:value={newName} placeholder="Topic name (e.g. Linear Algebra)" />
        <textarea
          bind:value={newDescription}
          rows="2"
          placeholder="Description (optional) — what to cover, resources, goals…"
        ></textarea>
        <button class="btn primary" type="submit" disabled={adding || !newName.trim()}>Add topic</button>
      </form>
    </section>

    {#if topics.length === 0}
      <div class="empty">
        <p>No topics yet.</p>
        <p class="muted">Add your first topic above, then schedule study dates — manually or with a spaced-repetition plan.</p>
      </div>
    {:else}
      <div class="stats">
        {topics.length} topic{topics.length === 1 ? '' : 's'} · {doneSessions}/{totalSessions} sessions done
      </div>
      <div class="topic-list">
        {#each topics as topic (topic.id)}
          <TopicCard {topic} on:changed={onChanged} on:error={onError} />
        {/each}
      </div>
    {/if}
  {:else}
    <!-- Agenda -->
    <section class="agenda">
      <div class="agenda-summary">
        <span>{agenda.length} upcoming session{agenda.length === 1 ? '' : 's'}</span>
        {#if overdueCount}<span class="pill danger">{overdueCount} overdue</span>{/if}
      </div>

      {#if agenda.length === 0}
        <div class="empty">
          <p>Nothing scheduled. 🎉</p>
          <p class="muted">All caught up — add dates to your topics to fill your agenda.</p>
        </div>
      {:else}
        <ul class="agenda-list">
          {#each agendaGroups as group (group.date)}
            <li class="agenda-day {dateClass(group.date)}">
              <div class="day-head">
                <span class="day-date">{formatDate(group.date)}</span>
                <span class="day-rel">{relativeLabel(group.date)}</span>
              </div>
              <ul class="day-items">
                {#each group.items as item (item.sessionId)}
                  <li class="agenda-item">
                    <label>
                      <input type="checkbox" on:change={() => toggleFromAgenda(item.topicId, item.sessionId)} />
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
  {/if}
</main>

<style>
  main {
    max-width: 760px;
    margin: 0 auto;
    padding: 1.5rem 1.25rem 4rem;
    text-align: left;
  }

  .app-head {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1rem;
    flex-wrap: wrap;
    margin-bottom: 1.25rem;
  }

  .brand h1 {
    margin: 0;
    font-size: 1.5rem;
    color: var(--text);
  }

  .tagline {
    margin: 0.2rem 0 0;
    color: var(--muted);
    font-size: 0.9rem;
  }

  .tabs {
    display: inline-flex;
    background: var(--chip);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 3px;
  }

  .tabs button {
    border: none;
    background: transparent;
    color: var(--muted);
    padding: 0.4rem 1rem;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.9rem;
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }

  .tabs button.active {
    background: var(--card);
    color: var(--text);
    box-shadow: 0 1px 2px rgba(16, 24, 40, 0.08);
  }

  .badge {
    background: var(--danger);
    color: #fff;
    font-size: 0.7rem;
    border-radius: 99px;
    padding: 0 0.4rem;
    line-height: 1.4;
  }

  .error {
    background: #fef2f1;
    border: 1px solid #fcaca7;
    color: var(--danger);
    padding: 0.6rem 0.85rem;
    border-radius: 10px;
    margin-bottom: 1rem;
    font-size: 0.88rem;
  }

  .loading {
    color: var(--muted);
  }

  .new-topic {
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: 14px;
    padding: 1rem 1.1rem;
    margin-bottom: 1.25rem;
    box-shadow: 0 1px 2px rgba(16, 24, 40, 0.04);
  }

  .new-topic h2 {
    margin: 0 0 0.6rem;
    font-size: 0.95rem;
    color: var(--text);
  }

  .new-topic form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .new-topic .btn {
    align-self: flex-start;
  }

  .stats {
    color: var(--muted);
    font-size: 0.82rem;
    margin-bottom: 0.75rem;
  }

  .topic-list {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .empty {
    text-align: center;
    padding: 2.5rem 1rem;
    color: var(--text);
  }

  .empty .muted {
    color: var(--muted);
    font-size: 0.9rem;
    max-width: 42ch;
    margin: 0.4rem auto 0;
  }

  .agenda-summary {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    color: var(--muted);
    font-size: 0.85rem;
    margin-bottom: 0.9rem;
  }

  .pill {
    font-size: 0.74rem;
    border-radius: 99px;
    padding: 0.1rem 0.55rem;
  }

  .pill.danger {
    background: #fef2f1;
    color: var(--danger);
    border: 1px solid #fcaca7;
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
    background: var(--card);
    border: 1px solid var(--border);
    border-left: 3px solid var(--border);
    border-radius: 12px;
    padding: 0.7rem 0.9rem;
  }

  .agenda-day.overdue {
    border-left-color: var(--danger);
  }
  .agenda-day.today {
    border-left-color: var(--warn);
  }
  .agenda-day.upcoming {
    border-left-color: var(--accent);
  }

  .day-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 0.45rem;
  }

  .day-date {
    font-weight: 600;
    color: var(--text);
    font-size: 0.9rem;
  }

  .day-rel {
    font-size: 0.76rem;
    color: var(--muted);
  }

  .agenda-day.overdue .day-rel {
    color: var(--danger);
  }
  .agenda-day.today .day-rel {
    color: var(--warn);
    font-weight: 600;
  }

  .day-items {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .agenda-item label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
    font-size: 0.88rem;
    color: var(--text);
  }

  .agenda-item input {
    width: 16px;
    height: 16px;
    accent-color: var(--accent);
    cursor: pointer;
  }
</style>
