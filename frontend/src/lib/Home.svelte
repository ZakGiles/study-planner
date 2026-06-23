<script lang="ts">
  import type { main } from '../../wailsjs/go/models';
  import { today } from './today';
  import { toISO, plural } from './dates';
  import { taskHex } from './colors';
  import { computeDoneByDay, computeStreaks } from './stats';

  // The full task graph and focus log, owned by App and passed straight through.
  export let tasks: main.Task[] = [];
  export let focusSessions: main.FocusSession[] = [];
  // The configurable daily focus-time target, in minutes (0 disables the ring).
  export let dailyGoalMinutes = 0;

  // Per-session in-flight guard and catch-up flag, shared with the Agenda tab so
  // a toggle started here can't double-fire.
  export let agendaBusy: Record<string, boolean> = {};
  export let catchingUp = false;

  // Actions wired back to App, which owns the grade modal and backend calls.
  export let onToggle: (taskId: string, sessionId: string) => void;
  export let onGrade: (item: DueItem) => void;
  export let onCatchUp: () => void;
  export let onStartFocus: () => void;
  export let onViewAgenda: () => void;
  export let onSetGoal: (goal: number) => void;

  type DueItem = {
    taskId: string;
    taskName: string;
    sessionId: string;
    taskColor: string;
    adaptive: boolean;
  };

  // Pending sessions on active tasks, split into today's reviews and the overdue
  // backlog. Today's drive the actionable list; overdue drives the catch-up banner.
  $: active = tasks.filter((t) => !t.archived);
  function dueItems(when: (date: string) => boolean): DueItem[] {
    return active.flatMap((t) =>
      t.sessions
        .filter((s) => !s.done && when(s.date))
        .map((s) => ({
          taskId: t.id,
          taskName: t.name,
          sessionId: s.id,
          taskColor: t.color,
          adaptive: t.adaptive,
        }))
    );
  }
  $: todayItems = dueItems((d) => d === $today);
  $: overdueCount = dueItems((d) => d < $today).length;

  // Adaptive tasks are graded (opening App's modal); plain tasks toggle directly.
  function check(item: DueItem) {
    if (item.adaptive) onGrade(item);
    else onToggle(item.taskId, item.sessionId);
  }

  // Streak from the shared module so it matches the Stats page exactly.
  $: streaks = computeStreaks(computeDoneByDay(tasks, $today), $today);

  // Today's focused time from the Pomodoro log — the numerator of the goal ring.
  $: focusTodaySec = focusSessions.reduce(
    (n, f) => (toISO(new Date(f.completedAt)) === $today ? n + f.durationSec : n),
    0
  );
  function fmtDuration(sec: number): string {
    const m = Math.round(sec / 60);
    if (m < 60) return `${m}m`;
    const h = Math.floor(m / 60);
    const r = m % 60;
    return r ? `${h}h ${r}m` : `${h}h`;
  }

  // Goal ring: focus time logged today vs. the target. Clamp the arc at 100% but
  // keep the raw time so overachieving still reads (e.g. "2h 30m / 2h").
  $: goalSec = dailyGoalMinutes * 60;
  $: goalPct = goalSec > 0 ? Math.min(1, focusTodaySec / goalSec) : 0;
  $: goalMet = goalSec > 0 && focusTodaySec >= goalSec;
  const R = 34;
  const CIRC = 2 * Math.PI * R;

  // Inline goal editing, in hours (people set focus goals like "2h"); stored as
  // minutes. A half-hour step keeps fractional goals easy.
  let editingGoal = false;
  let goalDraft = 0;
  function startEditGoal() {
    goalDraft = dailyGoalMinutes / 60;
    editingGoal = true;
  }
  function saveGoal() {
    editingGoal = false;
    const minutes = Math.max(0, Math.round((goalDraft || 0) * 60));
    if (minutes !== dailyGoalMinutes) onSetGoal(minutes);
  }
</script>

<section class="flex flex-col gap-[1.1rem]">
  <div class="grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-[0.8rem]">
    <!-- Daily focus goal ring -->
    <div class="flex items-center gap-[1rem] rounded-lg border border-line bg-surface px-4 py-[1rem] shadow-1">
      <div class="relative shrink-0">
        <svg width="84" height="84" viewBox="0 0 84 84" class="-rotate-90">
          <circle cx="42" cy="42" r={R} fill="none" stroke="var(--inset)" stroke-width="8" />
          <circle
            cx="42"
            cy="42"
            r={R}
            fill="none"
            stroke={goalMet ? 'var(--green)' : 'var(--accent-bright)'}
            stroke-width="8"
            stroke-linecap="round"
            stroke-dasharray={CIRC}
            stroke-dashoffset={CIRC * (1 - goalPct)}
            class="transition-[stroke-dashoffset] duration-500"
          />
        </svg>
        <span class="absolute inset-0 flex items-center justify-center">
          {#if goalMet}<span class="text-[1.2rem]">✓</span>{:else}<span class="tnum font-display text-[0.95rem] font-extrabold text-fg-strong">{Math.round(goalPct * 100)}%</span>{/if}
        </span>
      </div>
      <div class="flex min-w-0 flex-1 flex-col gap-[0.2rem]">
        <span class="text-[0.78rem] font-semibold text-fg-strong">Daily focus goal</span>
        {#if editingGoal}
          <label class="flex items-center gap-[0.35rem] text-[0.78rem] text-fg-muted">
            <!-- svelte-ignore a11y-autofocus -->
            <input
              class="w-[3.6rem] rounded-sm border border-line bg-inset px-[0.4rem] py-[0.15rem] text-[0.85rem] text-fg-strong"
              type="number"
              min="0"
              step="0.5"
              bind:value={goalDraft}
              on:blur={saveGoal}
              on:keydown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
              autofocus
            />
            hours / day
          </label>
        {:else}
          <span class="tnum text-[0.82rem] text-fg-muted">{fmtDuration(focusTodaySec)} / {fmtDuration(goalSec)}</span>
          <div class="mt-[0.15rem] flex items-center gap-[0.7rem]">
            <button class="btn ghost sm" on:click={onStartFocus}>Start focus</button>
            <button class="text-[0.72rem] text-accent-bright hover:underline" on:click={startEditGoal}>Edit goal</button>
          </div>
        {/if}
      </div>
    </div>

    <!-- Streak -->
    <div class="flex items-center gap-[0.9rem] rounded-lg border border-line bg-surface px-4 py-[1rem] shadow-1">
      <span class="text-[2rem] leading-none {streaks.current > 0 ? '' : 'opacity-40 grayscale'}" aria-hidden="true">🔥</span>
      <div class="flex min-w-0 flex-col gap-[0.1rem]">
        <span class="tnum font-display text-[1.7rem] font-extrabold leading-none text-fg-strong">{streaks.current}</span>
        <span class="text-[0.78rem] text-fg-muted">day streak</span>
        <span class="text-[0.72rem] text-fg-faint">longest {streaks.longest}</span>
      </div>
    </div>
  </div>

  <!-- Due today -->
  <div class="rounded-lg border border-line bg-surface px-[1.2rem] pb-[1.1rem] pt-4 shadow-1">
    <div class="mb-[0.85rem] flex items-baseline justify-between gap-2">
      <h2 class="m-0 font-display text-base font-bold text-fg-strong">
        Due today{#if todayItems.length}<span class="tnum ml-[0.4rem] text-[0.82rem] font-semibold text-fg-muted">{todayItems.length}</span>{/if}
      </h2>
      <button class="text-[0.76rem] text-accent-bright hover:underline" on:click={onViewAgenda}>View agenda</button>
    </div>

    {#if overdueCount}
      <div class="mb-[0.85rem] flex items-center justify-between gap-2 rounded-md border border-red-line bg-red-soft px-[0.8rem] py-[0.55rem]">
        <span class="text-[0.82rem] text-red">{overdueCount} overdue review{plural(overdueCount)}</span>
        <button class="btn ghost sm" on:click={onCatchUp} disabled={catchingUp}>Reschedule overdue</button>
      </div>
    {/if}

    {#if todayItems.length}
      <ul class="m-0 flex list-none flex-col gap-[0.4rem] p-0">
        {#each todayItems as item (item.sessionId)}
          <li class="flex items-center gap-[0.6rem]">
            <input
              type="checkbox"
              class="h-[18px] w-[18px] shrink-0 cursor-pointer accent-accent"
              checked={false}
              disabled={agendaBusy[item.sessionId]}
              on:click|preventDefault={() => check(item)}
              aria-label="Complete {item.taskName}"
            />
            <span class="h-[9px] w-[9px] shrink-0 rounded-full" style="background:{taskHex(item.taskColor)}"></span>
            <span class="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap text-[0.9rem] text-fg">{item.taskName}</span>
            {#if item.adaptive}<span class="shrink-0 text-[0.64rem] uppercase tracking-[0.06em] text-fg-faint">grade</span>{/if}
          </li>
        {/each}
      </ul>
    {:else if !overdueCount}
      <p class="muted m-0 text-[0.86rem]">All clear for today — nothing due. 🎉</p>
    {:else}
      <p class="muted m-0 text-[0.86rem]">Nothing scheduled for today; clear the overdue backlog above.</p>
    {/if}
  </div>
</section>
