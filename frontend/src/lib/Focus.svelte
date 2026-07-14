<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { main } from '../../wailsjs/go/models';
  import { RecordFocusSession } from './backend';
  import { today } from './today';
  import { toISO, plural } from './dates';
  import { taskHex } from './colors';
  import { focusMin, breakMin, mode, clamp, FOCUS_MIN, FOCUS_MAX, BREAK_MIN, BREAK_MAX, type Mode } from './focusPrefs';
  import { ensureAudio, playSound } from './sounds';

  // Tasks feed the dropdown and let us resolve a focus record's task name and
  // colour; focusSessions is the completed-block log, owned by App so the Stats
  // tab sees the same data. active is whether the Focus tab is currently shown —
  // the timer pauses while it is false and resumes on return.
  export let tasks: main.Task[] = [];
  export let focusSessions: main.FocusSession[] = [];
  export let active = true;

  const dispatch = createEventDispatcher<{ recorded: main.FocusSession[]; error: string }>();

  // Focus durations and timer/stopwatch mode are shared, locally-persisted
  // preferences (lib/focusPrefs.ts) so the Settings tab edits the same defaults.
  // Alert sounds live in lib/sounds.ts; this tab only plays them.

  let selectedTaskId = ''; // '' = general focus (task the user is focusing on)

  // phase is the block we're on; running is whether the clock ticks. started
  // turns true once a block has been kicked off (running or paused mid-way) and
  // back to false when it completes or resets — it's the manual gate between
  // steps. A "fresh" block is one not yet started. Tracking this explicitly
  // (rather than deriving it from remaining === blockLen) keeps the
  // duration-sync block below out of a reactive cycle.
  type Phase = 'focus' | 'break';
  let phase: Phase = 'focus';
  let running = false;
  let started = false;
  let blockLen = $focusMin * 60; // current block's full length, seconds
  let remaining = blockLen; // seconds left in the current block
  let justFinishedFocus = false; // drives the "nice focus" hand-off message

  $: fresh = !started;

  // While a block hasn't started, keep its length in step with the configured
  // durations so editing the inputs updates the clock immediately. Once started,
  // the length is locked until the block completes or resets.
  $: if (!started) {
    const len = (phase === 'focus' ? clamp($focusMin, FOCUS_MIN, FOCUS_MAX)
                                   : clamp($breakMin, BREAK_MIN, BREAK_MAX)) * 60;
    blockLen = len;
    remaining = len;
    elapsed = 0; // a fresh stopwatch run starts from zero
  }

  // Drift-free clock: while ticking we hold the wall-clock instant the block
  // ends and derive `remaining` from it, so throttled timers stay accurate. The
  // interval lives only while the tab is active AND running, which makes leaving
  // the Focus tab a clean pause (remaining freezes) and returning a clean resume
  // (endTime is recomputed from the frozen remaining — no elapsed time is lost).
  let endTime = 0; // wall-clock instant a countdown block ends
  let startTime = 0; // wall-clock instant the running stopwatch began (less prior elapsed)
  let elapsed = 0; // seconds counted up so far (stopwatch focus only)
  let intervalId: ReturnType<typeof setInterval> | undefined;

  // Mode is captured when a block starts and held until it's fresh again, so
  // changing the timer/stopwatch default elsewhere — e.g. the Settings tab, whose
  // control isn't locked mid-block — can't reinterpret a block that's already
  // running. Like the durations above, it re-syncs the moment the block resets.
  let blockMode: Mode = $mode;
  $: if (!started) blockMode = $mode;

  // The focus phase counts up in stopwatch mode; everything else (timer focus,
  // and every break) counts down.
  $: countUp = blockMode === 'stopwatch' && phase === 'focus';

  $: syncInterval(active && running);
  function syncInterval(on: boolean) {
    clearInterval(intervalId);
    intervalId = undefined;
    if (!on) return;
    if (countUp) {
      startTime = Date.now() - elapsed * 1000;
    } else {
      endTime = Date.now() + remaining * 1000;
    }
    intervalId = setInterval(tick, 250);
  }

  function tick() {
    if (countUp) {
      elapsed = Math.max(0, Math.round((Date.now() - startTime) / 1000));
      return; // a stopwatch has no end — it runs until you stop it
    }
    const left = Math.max(0, Math.round((endTime - Date.now()) / 1000));
    remaining = left;
    if (left <= 0) completeBlock();
  }

  // Persist `dur` seconds of focus against the selected task and hand the updated
  // log back to the parent. Shared by every path that banks a focus block.
  async function bankFocus(dur: number) {
    try {
      const list = await RecordFocusSession(selectedTaskId, dur);
      dispatch('recorded', list);
    } catch (e) {
      dispatch('error', `Couldn't save that focus block: ${e}`);
    }
  }

  async function completeBlock() {
    running = false; // stops the interval via syncInterval
    started = false; // back to a fresh, manually-gated next step
    if (phase === 'focus') {
      playSound('study'); // the study block's timer just hit zero
      justFinishedFocus = true;
      await bankFocus(blockLen);
      detachClimb = climb; // remember where the wings come off (near the sun)
      detachSway = sway;
      phase = 'break'; // wings fall off and he drops from the sun
    } else {
      playSound('break'); // the break's timer just hit zero
      phase = 'focus';
      justFinishedFocus = false;
    }
  }

  function start() {
    ensureAudio(); // unlock audio on this gesture so the end chime can play
    justFinishedFocus = false;
    started = true;
    running = true;
  }
  function pause() {
    running = false;
  }
  async function reset() {
    // Abandon the current block. A timer focus block that ran at least
    // MIN_RECORD_SEC still banks the focus done so far before resetting to a
    // fresh block; shorter blocks, breaks, and the stopwatch's own Reset record
    // nothing. Returning started to false re-sizes it to a fresh block.
    const dur = blockLen - remaining; // capture before the !started reactive flush
    running = false;
    started = false;
    justFinishedFocus = false; // Reset returns to a fresh focus block, not a break
    if (blockMode === 'timer' && phase === 'focus' && dur >= MIN_RECORD_SEC) {
      await bankFocus(dur);
    }
  }
  function skipBreak() {
    // Back to a fresh focus block, starting from the ground.
    phase = 'focus';
    started = false;
    running = false;
    justFinishedFocus = false;
  }
  // End the focus phase early and drop straight into a fresh break: his wings
  // come off and he falls to the ground. The focus time done so far is banked
  // (like the stopwatch's Stop) when it's at least MIN_RECORD_SEC; shorter
  // attempts record nothing.
  async function skipToBreak() {
    detachClimb = climb; // wings come off at whatever height he'd reached
    detachSway = sway;
    running = false;
    started = false;
    const dur = blockLen - remaining; // focus elapsed before the early exit
    if (dur >= MIN_RECORD_SEC) {
      playSound('study');
      justFinishedFocus = true; // drives the "Nice focus — take a break" message
      await bankFocus(dur);
    } else {
      justFinishedFocus = false;
    }
    phase = 'break';
  }

  // What the big clock shows: time counted up (stopwatch focus) or time left.
  $: displaySec = countUp ? elapsed : remaining;
  // Icarus climbs with progress through the block. A timer's progress is linear
  // to its end; a stopwatch has no end, so he follows a fixed asymptotic curve
  // (1 - e^(-t/τ)) that always approaches the sun but never reaches it — the same
  // climb shape no matter how long you run. τ sets how quickly he nears the top.
  const STOPWATCH_TAU = 25 * 60; // seconds; ~63% of the way up at this mark
  const MIN_RECORD_SEC = 60; // shortest early-exit focus block we bother logging
  $: progress = countUp
    ? 1 - Math.exp(-elapsed / STOPWATCH_TAU)
    : (blockLen > 0 ? (blockLen - remaining) / blockLen : 0);
  // Icarus climbs with focus progress; during a break his wings have come off
  // and he has dropped to the ground, so his climb is 0.
  $: climb = phase === 'focus' ? progress : 0;
  const ICARUS_LOW = 315, ICARUS_HIGH = 88;
  $: icarusY = ICARUS_LOW - climb * (ICARUS_LOW - ICARUS_HIGH);
  $: sway = phase === 'focus' ? Math.sin(progress * Math.PI * 6) * 12 : 0; // drift while flying
  $: sunGlow = 0.35 + climb * 0.65;
  // Tumble: upright while flying, rotated over as he falls into a break.
  $: fallRot = phase === 'break' ? 165 : 0;

  // Wings track the figure while flying, but freeze at the height/position they
  // detached from when a break begins — so they drop a FIXED distance (the
  // wing-fall CSS) instead of riding the body all the way to the ground.
  let detachClimb = 0;
  let detachSway = 0;
  $: detachY = ICARUS_LOW - detachClimb * (ICARUS_LOW - ICARUS_HIGH);
  $: wingX = phase === 'focus' ? 110 + sway : 110 + detachSway;
  $: wingY = phase === 'focus' ? icarusY : detachY;

  $: primaryLabel = countUp
    ? (running ? 'Stop' : fresh ? 'Start focus' : 'Resume')
    : (running ? 'Pause' : fresh ? (phase === 'focus' ? 'Start focus' : 'Start break') : 'Resume');
  function primaryAction() {
    if (countUp && running) {
      stopStopwatch(); // a running stopwatch's primary action is stop → break
      return;
    }
    running ? pause() : start();
  }

  // Stop the stopwatch: bank the elapsed focus time as a completed session and
  // drop straight into a break, mirroring how a timer block completes. A run of
  // under a second records nothing (RecordFocusSession rejects zero durations).
  async function stopStopwatch() {
    detachClimb = climb; // capture the height before the reset zeroes progress
    detachSway = sway;
    running = false;
    started = false;
    const dur = elapsed;
    if (dur >= 1) {
      playSound('study');
      justFinishedFocus = true;
      await bankFocus(dur);
    }
    phase = 'break'; // wings fall off and he drops from wherever he'd reached
  }

  // ---- Stats derived from the focus log ----
  $: taskById = new Map(tasks.map((t) => [t.id, t]));
  function taskMeta(id: string) {
    if (id === '') return { name: 'General focus', color: '' };
    const t = taskById.get(id);
    return { name: t?.name ?? 'Deleted task', color: t?.color ?? '' };
  }

  $: todaySec = focusSessions
    .filter((f) => toISO(new Date(f.completedAt)) === $today)
    .reduce((n, f) => n + f.durationSec, 0);
  $: totalSec = focusSessions.reduce((n, f) => n + f.durationSec, 0);
  $: blockCount = focusSessions.length;
  $: todayCount = focusSessions.filter((f) => toISO(new Date(f.completedAt)) === $today).length;

  $: perTask = (() => {
    const m = new Map<string, number>();
    for (const f of focusSessions) m.set(f.taskId, (m.get(f.taskId) ?? 0) + f.durationSec);
    return [...m.entries()]
      .map(([id, sec]) => ({ id, sec, ...taskMeta(id) }))
      .sort((a, b) => b.sec - a.sec);
  })();

  function fmtClock(sec: number): string {
    const m = Math.floor(sec / 60);
    const s = sec % 60;
    return `${m}:${String(s).padStart(2, '0')}`;
  }
  function fmtDuration(sec: number): string {
    const m = Math.round(sec / 60);
    if (m < 60) return `${m}m`;
    const h = Math.floor(m / 60);
    const r = m % 60;
    return r ? `${h}h ${r}m` : `${h}h`;
  }

  $: selectedDot = selectedTaskId ? taskHex(taskById.get(selectedTaskId)?.color) : 'var(--muted)';
  $: activeTasks = tasks.filter((t) => !t.archived);
  $: lockInputs = started; // don't let task/length change mid-block
</script>

<section class="flex flex-col gap-5">
  <div class="grid grid-cols-[minmax(0,1fr)_300px] gap-5 max-[820px]:grid-cols-1">
    <!-- Icarus sky -->
    <div
      class="overflow-hidden rounded-lg border border-line shadow-1 min-h-[380px] max-[820px]:min-h-[300px]"
      style="background: linear-gradient(to top, var(--surface-2), color-mix(in srgb, var(--accent) 24%, var(--surface)));"
    >
      <!-- h-full makes the SVG fill the (grid-stretched) panel, and xMidYMax
           anchors the art to its bottom — so Icarus always stands on the floor of
           the panel rather than the top of a fixed-height box. -->
      <svg viewBox="0 0 220 340" preserveAspectRatio="xMidYMax meet" class="block h-full min-h-[380px] w-full max-[820px]:min-h-[300px]" aria-hidden="true">
        <defs>
          <!-- The original simple wing: one smooth white crescent. Drawn as the
               left wing; the right is a mirrored instance. -->
          <g id="wingL">
            <path d="M0 -8 C -22 -20 -34 -16 -36 -4 C -22 -8 -10 -4 0 2 Z" fill="var(--text-strong)" />
          </g>
        </defs>
        <!-- Sun -->
        <g style="opacity:{sunGlow}; transition:opacity 0.4s ease;">
          <circle cx="110" cy="44" r="40" fill="var(--amber)" opacity="0.2" />
          <circle cx="110" cy="44" r="24" fill="var(--amber)" />
        </g>
        <!-- Clouds: white so they read as clouds in either theme (always the
             lightest thing in the sky), each a couple of overlapping puffs. -->
        <g fill="#ffffff" opacity="0.4">
          <ellipse cx="50" cy="120" rx="20" ry="9" />
          <ellipse cx="66" cy="124" rx="15" ry="8" />
          <ellipse cx="168" cy="182" rx="18" ry="8" />
          <ellipse cx="182" cy="186" rx="13" ry="7" />
        </g>
        <!-- Icarus ascends toward the sun as the focus block progresses; entering
             a break he falls to the ground while tumbling over. The transform is
             driven via CSS (not the SVG attribute) so WebKit actually transitions
             the drop — the outer group carries the fall, the inner figure spins.
             An accelerating curve gives the drop some gravity. -->
        <!-- Wings are their own group, positioned to the figure while flying but
             frozen at the detach point during a break, so they drop a fixed
             distance via the wing-fall transition rather than riding the body
             down. The fall is a transition (not a keyframe), so re-showing the
             tab — which restarts CSS animations — doesn't replay it. -->
        <g
          class:flying={running && phase === 'focus'}
          style="transform: translate({wingX}px, {wingY}px); transition: transform {running && phase === 'focus' ? '0.25s ease' : '0.45s ease'};"
        >
          <g class="wing-fall" class:breaking={phase === 'break'}>
            <g class="wing"><use href="#wingL" /></g>
            <g transform="scale(-1 1)"><g class="wing"><use href="#wingL" /></g></g>
          </g>
        </g>
        <!-- The body falls to the ground and tumbles about its own centre. -->
        <g
          style="transform: translate({110 + sway}px, {icarusY}px); transition: transform {phase === 'break' ? '0.85s cubic-bezier(0.45,0,0.9,0.9)' : running ? '0.25s ease' : '0.45s ease'};"
        >
          <g class="figure" style="transform: rotate({fallRot}deg);">
            <path d="M0 -14 C -4 -14 -5 -6 -4 1 L -2 25 L 2 25 L 4 1 C 5 -6 4 -14 0 -14 Z" fill="var(--text-strong)" />
            <circle cx="0" cy="-19" r="5" fill="var(--text-strong)" />
          </g>
        </g>
      </svg>
    </div>

    <!-- Controls -->
    <div class="flex flex-col gap-4 rounded-lg border border-line bg-surface px-5 py-5 shadow-1">
      <!-- Mode switch: a countdown timer (default) or a count-up stopwatch.
           Locked mid-block so the running clock can't change under you. -->
      <div class="flex gap-2">
        <button
          class="btn sm flex-1 {blockMode === 'timer' ? 'primary' : 'ghost'}"
          on:click={() => mode.set('timer')}
          disabled={lockInputs}
        >Timer</button>
        <button
          class="btn sm flex-1 {blockMode === 'stopwatch' ? 'primary' : 'ghost'}"
          on:click={() => mode.set('stopwatch')}
          disabled={lockInputs}
        >Stopwatch</button>
      </div>

      <!-- Clock readout (kept out of the sky so it never covers Icarus). -->
      <div class="flex flex-col items-center gap-0.5 rounded-md border border-line bg-surface-2 py-3">
        <span class="text-[0.7rem] font-bold uppercase tracking-[0.18em] {phase === 'focus' ? 'text-accent-bright' : 'text-amber'}">
          {#if justFinishedFocus && fresh}Nice focus — take a break{:else}{phase === 'focus' ? 'Focus' : 'Break'}{/if}
        </span>
        <span class="tnum font-display text-[2.9rem] font-extrabold leading-none text-fg-strong tabular-nums">
          {fmtClock(displaySec)}
        </span>
      </div>

      <div>
        <label class="mb-1 block text-[0.72rem] font-semibold uppercase tracking-[0.08em] text-fg-muted" for="focus-task">Focusing on</label>
        <div class="flex items-center gap-2">
          <span class="h-[10px] w-[10px] shrink-0 rounded-full" style="background:{selectedDot}"></span>
          <select id="focus-task" class="w-full" bind:value={selectedTaskId} disabled={lockInputs}>
            <option value="">General focus</option>
            {#each activeTasks as t (t.id)}
              <option value={t.id}>{t.name}</option>
            {/each}
          </select>
        </div>
      </div>

      <!-- Length inputs only matter for the timer; a stopwatch runs until you
           stop it, so they're hidden in that mode. -->
      {#if blockMode === 'timer'}
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="mb-1 block text-[0.72rem] font-semibold uppercase tracking-[0.08em] text-fg-muted" for="focus-len">Focus (min)</label>
            <input id="focus-len" type="number" min={FOCUS_MIN} max={FOCUS_MAX} bind:value={$focusMin} disabled={lockInputs} />
          </div>
          <div>
            <label class="mb-1 block text-[0.72rem] font-semibold uppercase tracking-[0.08em] text-fg-muted" for="break-len">Break (min)</label>
            <input id="break-len" type="number" min={BREAK_MIN} max={BREAK_MAX} bind:value={$breakMin} disabled={lockInputs} />
          </div>
        </div>
      {/if}

      <div class="mt-1 flex flex-col gap-2">
        <button class="btn primary" on:click={primaryAction}>{primaryLabel}</button>
        <div class="flex gap-2">
          {#if started}
            <button class="btn ghost sm flex-1" on:click={reset}>Reset</button>
          {/if}
          {#if phase === 'break'}
            <button class="btn ghost sm flex-1" on:click={skipBreak}>Skip break</button>
          {:else if blockMode === 'timer'}
            <button class="btn ghost sm flex-1" on:click={skipToBreak}>Skip to break</button>
          {/if}
        </div>
      </div>

      <div class="mt-1 grid grid-cols-2 gap-3 border-t border-line pt-4">
        <div>
          <div class="tnum font-display text-[1.4rem] font-extrabold leading-none text-fg-strong">{fmtDuration(todaySec)}</div>
          <div class="mt-0.5 text-[0.74rem] text-fg-muted">today · {todayCount} block{plural(todayCount)}</div>
        </div>
        <div>
          <div class="tnum font-display text-[1.4rem] font-extrabold leading-none text-fg-strong">{fmtDuration(totalSec)}</div>
          <div class="mt-0.5 text-[0.74rem] text-fg-muted">all time · {blockCount} block{plural(blockCount)}</div>
        </div>
      </div>
    </div>
  </div>

  <!-- Per-task breakdown -->
  {#if perTask.length}
    <div class="rounded-lg border border-line bg-surface px-5 py-4 shadow-1">
      <h3 class="m-0 mb-3 font-display text-[0.85rem] font-bold uppercase tracking-[0.04em] text-fg-faint">Focus by task</h3>
      <ul class="m-0 flex list-none flex-col gap-[0.45rem] p-0">
        {#each perTask as row (row.id)}
          <li class="flex items-center gap-[0.6rem] text-[0.9rem]">
            <span class="h-[9px] w-[9px] shrink-0 rounded-full" style="background:{row.color ? taskHex(row.color) : 'var(--muted)'}"></span>
            <span class="text-fg {row.id === '' || !taskById.has(row.id) ? 'italic text-fg-muted' : ''}">{row.name}</span>
            <span class="tnum ml-auto text-fg-muted">{fmtDuration(row.sec)}</span>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
</section>

<style>
  /* Both wings carry left-wing geometry and pivot at the shoulder (the root, at
     the right edge of the bounding box); the right wing is mirrored by its
     parent group, so a single flap keyframe drives a symmetric beat. */
  .wing {
    transform-box: fill-box;
    transform-origin: 100% 42%;
  }
  .flying .wing {
    animation: flap 0.55s ease-in-out infinite;
  }
  @keyframes flap {
    0%, 100% { transform: rotate(0deg); }
    50% { transform: rotate(-12deg); }
  }
  /* The body tumbles about its own centre as it falls into a break. */
  .figure {
    transform-box: fill-box;
    transform-origin: center;
    transition: transform 0.85s ease-in;
  }
  /* Entering a break the wings tear off, drop a FIXED distance and fade, then
     stay gone for the rest of the break; they reappear when the next focus block
     resets the phase. A transition (not a keyframe) means re-showing the Focus
     tab — which restarts CSS animations — doesn't replay the drop. */
  .wing-fall {
    transition: transform 0.7s ease-in, opacity 0.7s ease-in;
  }
  .wing-fall.breaking {
    transform: translateY(60px);
    opacity: 0;
  }
  @media (prefers-reduced-motion: reduce) {
    .flying .wing { animation: none; }
    .wing-fall { transition: none; }
    .wing-fall.breaking { opacity: 0; }
    .figure { transition: none; }
  }
</style>
