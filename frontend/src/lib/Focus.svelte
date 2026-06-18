<script lang="ts">
  import { createEventDispatcher, onMount, onDestroy } from 'svelte';
  import type { main } from '../../wailsjs/go/models';
  import { RecordFocusSession } from '../../wailsjs/go/main/App.js';
  import { today } from './today';
  import { toISO, plural } from './dates';
  import { topicHex } from './colors';

  // Topics feed the dropdown and let us resolve a focus record's topic name and
  // colour; focusSessions is the completed-block log, owned by App so the Stats
  // tab sees the same data. active is whether the Focus tab is currently shown —
  // the timer pauses while it is false and resumes on return.
  export let topics: main.Topic[] = [];
  export let focusSessions: main.FocusSession[] = [];
  export let active = true;

  const dispatch = createEventDispatcher<{ recorded: main.FocusSession[]; error: string }>();

  // Durations are a UI preference, persisted locally like the theme. Clamped to
  // sane bounds wherever they feed the clock so a stray input can't break it.
  const FOCUS_MIN = 1, FOCUS_MAX = 180, BREAK_MIN = 1, BREAK_MAX = 60;
  const clamp = (n: number, lo: number, hi: number) =>
    Math.min(hi, Math.max(lo, Math.round(Number.isFinite(n) ? n : lo)));

  let focusMin = clamp(Number(localStorage.getItem('focusMin')) || 25, FOCUS_MIN, FOCUS_MAX);
  let breakMin = clamp(Number(localStorage.getItem('breakMin')) || 5, BREAK_MIN, BREAK_MAX);
  $: localStorage.setItem('focusMin', String(focusMin));
  $: localStorage.setItem('breakMin', String(breakMin));

  let selectedTopicId = ''; // '' = general focus

  // Alert sounds. Uploaded files are stored as Blobs in IndexedDB (no practical
  // size limit, unlike localStorage), and played from object URLs; with none set
  // a synthesised chime plays. studySound/breakSound hold the live object URLs.
  type SoundKind = 'study' | 'break';
  type SoundRecord = { blob: Blob; name: string };
  let studySound = '';
  let breakSound = '';
  let studySoundName = '';
  let breakSoundName = '';
  $: soundRows = [
    { kind: 'study' as const, label: 'Study end', name: studySoundName, custom: !!studySound },
    { kind: 'break' as const, label: 'Break end', name: breakSoundName, custom: !!breakSound },
  ];

  const SOUND_DB = 'study-planner-sounds';
  const SOUND_STORE = 'sounds';
  function openSoundDB(): Promise<IDBDatabase> {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(SOUND_DB, 1);
      req.onupgradeneeded = () => req.result.createObjectStore(SOUND_STORE);
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
  }
  async function soundDBPut(key: SoundKind, value: SoundRecord) {
    const db = await openSoundDB();
    try {
      await new Promise<void>((resolve, reject) => {
        const tx = db.transaction(SOUND_STORE, 'readwrite');
        tx.objectStore(SOUND_STORE).put(value, key);
        tx.oncomplete = () => resolve();
        tx.onerror = () => reject(tx.error);
      });
    } finally {
      db.close();
    }
  }
  async function soundDBGet(key: SoundKind): Promise<SoundRecord | undefined> {
    const db = await openSoundDB();
    try {
      return await new Promise<SoundRecord | undefined>((resolve, reject) => {
        const req = db.transaction(SOUND_STORE, 'readonly').objectStore(SOUND_STORE).get(key);
        req.onsuccess = () => resolve(req.result);
        req.onerror = () => reject(req.error);
      });
    } finally {
      db.close();
    }
  }
  async function soundDBDelete(key: SoundKind) {
    const db = await openSoundDB();
    try {
      await new Promise<void>((resolve, reject) => {
        const tx = db.transaction(SOUND_STORE, 'readwrite');
        tx.objectStore(SOUND_STORE).delete(key);
        tx.oncomplete = () => resolve();
        tx.onerror = () => reject(tx.error);
      });
    } finally {
      db.close();
    }
  }

  // Point a kind at a new object URL, revoking the one it replaces.
  function setSoundUrl(kind: SoundKind, url: string, name: string) {
    if (kind === 'study') {
      if (studySound) URL.revokeObjectURL(studySound);
      studySound = url;
      studySoundName = name;
    } else {
      if (breakSound) URL.revokeObjectURL(breakSound);
      breakSound = url;
      breakSoundName = name;
    }
  }

  onMount(async () => {
    for (const kind of ['study', 'break'] as const) {
      try {
        // One-time migration of any sound saved by the earlier localStorage version.
        const legacy = localStorage.getItem(`focusSound:${kind}`);
        if (legacy) {
          const name = localStorage.getItem(`focusSoundName:${kind}`) ?? 'sound';
          try {
            await soundDBPut(kind, { blob: await (await fetch(legacy)).blob(), name });
          } catch { /* ignore */ }
          localStorage.removeItem(`focusSound:${kind}`);
          localStorage.removeItem(`focusSoundName:${kind}`);
        }
        const rec = await soundDBGet(kind);
        if (rec) setSoundUrl(kind, URL.createObjectURL(rec.blob), rec.name);
      } catch { /* ignore load errors */ }
    }
  });

  onDestroy(() => {
    if (studySound) URL.revokeObjectURL(studySound);
    if (breakSound) URL.revokeObjectURL(breakSound);
  });

  let audioCtx: AudioContext | undefined;
  // Lazily create/resume the audio context (browsers require a user gesture to
  // start it — start() calls this so the chime can fire later when the timer ends).
  function ensureAudio(): AudioContext | undefined {
    try {
      if (!audioCtx) audioCtx = new (window.AudioContext || (window as any).webkitAudioContext)();
      if (audioCtx.state === 'suspended') void audioCtx.resume();
    } catch {
      return undefined; // audio unavailable; play calls become no-ops
    }
    return audioCtx;
  }
  // A short two-note chime so there's always a sound with nothing uploaded:
  // study rises, break falls.
  function playChime(kind: 'study' | 'break') {
    const ctx = ensureAudio();
    if (!ctx) return;
    const notes = kind === 'study' ? [660, 990] : [880, 587];
    notes.forEach((freq, i) => {
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = 'sine';
      osc.frequency.value = freq;
      const t = ctx.currentTime + i * 0.18;
      gain.gain.setValueAtTime(0.0001, t);
      gain.gain.exponentialRampToValueAtTime(0.32, t + 0.02);
      gain.gain.exponentialRampToValueAtTime(0.0001, t + 0.38);
      osc.connect(gain).connect(ctx.destination);
      osc.start(t);
      osc.stop(t + 0.42);
    });
  }
  function playSound(kind: 'study' | 'break') {
    const url = kind === 'study' ? studySound : breakSound;
    if (url) {
      new Audio(url).play().catch(() => playChime(kind)); // fall back if blocked
    } else {
      playChime(kind);
    }
  }
  async function uploadSound(kind: SoundKind, e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // allow re-picking the same file later
    if (!file) return;
    try {
      await soundDBPut(kind, { blob: file, name: file.name });
      setSoundUrl(kind, URL.createObjectURL(file), file.name);
    } catch (err) {
      dispatch('error', `Couldn't save that sound: ${err}`);
    }
  }
  async function clearSound(kind: SoundKind) {
    try {
      await soundDBDelete(kind);
    } catch { /* ignore */ }
    if (kind === 'study') {
      if (studySound) URL.revokeObjectURL(studySound);
      studySound = '';
      studySoundName = '';
    } else {
      if (breakSound) URL.revokeObjectURL(breakSound);
      breakSound = '';
      breakSoundName = '';
    }
  }

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
  let blockLen = focusMin * 60; // current block's full length, seconds
  let remaining = blockLen; // seconds left in the current block
  let justFinishedFocus = false; // drives the "nice focus" hand-off message

  $: fresh = !started;

  // While a block hasn't started, keep its length in step with the configured
  // durations so editing the inputs updates the clock immediately. Once started,
  // the length is locked until the block completes or resets.
  $: if (!started) {
    const len = (phase === 'focus' ? clamp(focusMin, FOCUS_MIN, FOCUS_MAX)
                                   : clamp(breakMin, BREAK_MIN, BREAK_MAX)) * 60;
    blockLen = len;
    remaining = len;
  }

  // Drift-free clock: while ticking we hold the wall-clock instant the block
  // ends and derive `remaining` from it, so throttled timers stay accurate. The
  // interval lives only while the tab is active AND running, which makes leaving
  // the Focus tab a clean pause (remaining freezes) and returning a clean resume
  // (endTime is recomputed from the frozen remaining — no elapsed time is lost).
  let endTime = 0;
  let intervalId: ReturnType<typeof setInterval> | undefined;

  $: syncInterval(active && running);
  function syncInterval(on: boolean) {
    clearInterval(intervalId);
    intervalId = undefined;
    if (!on) return;
    endTime = Date.now() + remaining * 1000;
    intervalId = setInterval(tick, 250);
  }

  function tick() {
    const left = Math.max(0, Math.round((endTime - Date.now()) / 1000));
    remaining = left;
    if (left <= 0) completeBlock();
  }

  async function completeBlock() {
    running = false; // stops the interval via syncInterval
    started = false; // back to a fresh, manually-gated next step
    if (phase === 'focus') {
      playSound('study'); // the study block's timer just hit zero
      const dur = blockLen;
      justFinishedFocus = true;
      try {
        const list = await RecordFocusSession(selectedTopicId, dur);
        dispatch('recorded', list);
      } catch (e) {
        dispatch('error', `Couldn't save that focus block: ${e}`);
      }
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
  function reset() {
    // Abandon the current block — nothing is recorded (only completed focus
    // blocks count). Returning started to false re-sizes it to a fresh block.
    running = false;
    started = false;
    justFinishedFocus = false;
  }
  function skipBreak() {
    // Back to a fresh focus block, starting from the ground.
    phase = 'focus';
    started = false;
    running = false;
    justFinishedFocus = false;
  }
  // End the focus phase early and drop straight into a fresh break: his wings
  // come off and he falls to the ground. The abandoned focus block isn't
  // recorded — only blocks that run to zero count.
  function skipToBreak() {
    detachClimb = climb; // wings come off at whatever height he'd reached
    detachSway = sway;
    phase = 'break';
    started = false;
    running = false;
    justFinishedFocus = false;
  }

  $: progress = blockLen > 0 ? (blockLen - remaining) / blockLen : 0;
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

  $: primaryLabel = running ? 'Pause' : fresh ? (phase === 'focus' ? 'Start focus' : 'Start break') : 'Resume';
  function primaryAction() {
    running ? pause() : start();
  }

  // ---- Stats derived from the focus log ----
  $: topicById = new Map(topics.map((t) => [t.id, t]));
  function topicMeta(id: string) {
    if (id === '') return { name: 'General focus', color: '' };
    const t = topicById.get(id);
    return { name: t?.name ?? 'Deleted topic', color: t?.color ?? '' };
  }

  $: todaySec = focusSessions
    .filter((f) => toISO(new Date(f.completedAt)) === $today)
    .reduce((n, f) => n + f.durationSec, 0);
  $: totalSec = focusSessions.reduce((n, f) => n + f.durationSec, 0);
  $: blockCount = focusSessions.length;
  $: todayCount = focusSessions.filter((f) => toISO(new Date(f.completedAt)) === $today).length;

  $: perTopic = (() => {
    const m = new Map<string, number>();
    for (const f of focusSessions) m.set(f.topicId, (m.get(f.topicId) ?? 0) + f.durationSec);
    return [...m.entries()]
      .map(([id, sec]) => ({ id, sec, ...topicMeta(id) }))
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

  $: selectedDot = selectedTopicId ? topicHex(topicById.get(selectedTopicId)?.color) : 'var(--muted)';
  $: activeTopics = topics.filter((t) => !t.archived);
  $: lockInputs = started; // don't let topic/length change mid-block
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
      <!-- Clock readout (kept out of the sky so it never covers Icarus). -->
      <div class="flex flex-col items-center gap-0.5 rounded-md border border-line bg-surface-2 py-3">
        <span class="text-[0.7rem] font-bold uppercase tracking-[0.18em] {phase === 'focus' ? 'text-accent-bright' : 'text-amber'}">
          {#if justFinishedFocus && fresh}Nice focus — take a break{:else}{phase === 'focus' ? 'Focus' : 'Break'}{/if}
        </span>
        <span class="tnum font-display text-[2.9rem] font-extrabold leading-none text-fg-strong tabular-nums">
          {fmtClock(remaining)}
        </span>
      </div>

      <div>
        <label class="mb-1 block text-[0.72rem] font-semibold uppercase tracking-[0.08em] text-fg-muted" for="focus-topic">Focusing on</label>
        <div class="flex items-center gap-2">
          <span class="h-[10px] w-[10px] shrink-0 rounded-full" style="background:{selectedDot}"></span>
          <select id="focus-topic" class="w-full" bind:value={selectedTopicId} disabled={lockInputs}>
            <option value="">General focus</option>
            {#each activeTopics as t (t.id)}
              <option value={t.id}>{t.name}</option>
            {/each}
          </select>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="mb-1 block text-[0.72rem] font-semibold uppercase tracking-[0.08em] text-fg-muted" for="focus-len">Focus (min)</label>
          <input id="focus-len" type="number" min={FOCUS_MIN} max={FOCUS_MAX} bind:value={focusMin} disabled={lockInputs} />
        </div>
        <div>
          <label class="mb-1 block text-[0.72rem] font-semibold uppercase tracking-[0.08em] text-fg-muted" for="break-len">Break (min)</label>
          <input id="break-len" type="number" min={BREAK_MIN} max={BREAK_MAX} bind:value={breakMin} disabled={lockInputs} />
        </div>
      </div>

      <div class="mt-1 flex flex-col gap-2">
        <button class="btn primary" on:click={primaryAction}>{primaryLabel}</button>
        <div class="flex gap-2">
          {#if started}
            <button class="btn ghost sm flex-1" on:click={reset}>Reset</button>
          {/if}
          {#if phase === 'focus'}
            <button class="btn ghost sm flex-1" on:click={skipToBreak}>Skip to break</button>
          {:else}
            <button class="btn ghost sm flex-1" on:click={skipBreak}>Skip break</button>
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

  <!-- Per-topic breakdown -->
  {#if perTopic.length}
    <div class="rounded-lg border border-line bg-surface px-5 py-4 shadow-1">
      <h3 class="m-0 mb-3 font-display text-[0.85rem] font-bold uppercase tracking-[0.04em] text-fg-faint">Focus by topic</h3>
      <ul class="m-0 flex list-none flex-col gap-[0.45rem] p-0">
        {#each perTopic as row (row.id)}
          <li class="flex items-center gap-[0.6rem] text-[0.9rem]">
            <span class="h-[9px] w-[9px] shrink-0 rounded-full" style="background:{row.color ? topicHex(row.color) : 'var(--muted)'}"></span>
            <span class="text-fg {row.id === '' || !topicById.has(row.id) ? 'italic text-fg-muted' : ''}">{row.name}</span>
            <span class="tnum ml-auto text-fg-muted">{fmtDuration(row.sec)}</span>
          </li>
        {/each}
      </ul>
    </div>
  {/if}

  <!-- Alert sounds (settings): an uploaded sound per timer end, else a chime. -->
  <div class="rounded-lg border border-line bg-surface px-5 py-4 shadow-1">
    <div class="mb-3 flex items-baseline justify-between gap-2">
      <h3 class="m-0 font-display text-[0.85rem] font-bold uppercase tracking-[0.04em] text-fg-faint">Alert sounds</h3>
      <span class="text-[0.74rem] text-fg-faint">Plays when a timer ends</span>
    </div>
    <div class="flex flex-col gap-[0.6rem]">
      {#each soundRows as row (row.kind)}
        <div class="flex items-center gap-2">
          <span class="w-[5.5rem] shrink-0 text-[0.8rem] font-semibold text-fg-muted">{row.label}</span>
          <span class="min-w-0 flex-1 truncate text-[0.82rem] {row.custom ? 'text-fg' : 'italic text-fg-muted'}">
            {row.custom ? row.name : 'Default chime'}
          </span>
          <button class="btn ghost sm" type="button" on:click={() => playSound(row.kind)}>Test</button>
          <label class="btn ghost sm cursor-pointer">
            Upload
            <input type="file" accept="audio/*" class="hidden" on:change={(e) => uploadSound(row.kind, e)} />
          </label>
          {#if row.custom}
            <button class="btn ghost sm" type="button" on:click={() => clearSound(row.kind)}>Reset</button>
          {/if}
        </div>
      {/each}
    </div>
  </div>
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
