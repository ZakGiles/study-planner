<script lang="ts">
  // The Settings tab: appearance, the daily focus goal, focus-timer defaults
  // (lengths, mode, alert sounds) and launch-on-login. Theme and focus defaults
  // are device-local (shared stores); the daily goal lives in the backend store
  // and flows through App via the onSetGoal callback. Auto-start is an OS-level
  // toggle handled entirely by the backend.
  import { createEventDispatcher, onMount } from 'svelte';
  import { GetAutoStart, SetAutoStart, ExportCalendar, capabilities, exportBackup, importBackup } from './backend';
  import type { main } from '../../wailsjs/go/models';
  import { theme } from './theme';
  import { focusMin, breakMin, mode, FOCUS_MIN, FOCUS_MAX, BREAK_MIN, BREAK_MAX } from './focusPrefs';
  import { sounds, uploadSound, clearSound, playSound, type SoundKind } from './sounds';
  import { maybeNotifyDueToday, notifyEnabled, requestPermission } from './notify';
  import ConfirmModal from './ConfirmModal.svelte';

  // The daily focus-time goal (minutes) and the backend setter, owned by App.
  export let dailyGoalMinutes = 0;
  export let onSetGoal: (minutes: number) => void;

  const dispatch = createEventDispatcher<{
    error: string;
    changed: main.State;
    focusChanged: main.FocusSession[];
  }>();

  // Goal is shown in hours (people set goals like "2h"); stored as minutes. This
  // tab remounts on each visit, so seeding once from the prop is enough.
  let goalHours = dailyGoalMinutes / 60;
  function saveGoal() {
    const minutes = Math.max(0, Math.round((goalHours || 0) * 60));
    if (minutes !== dailyGoalMinutes) onSetGoal(minutes);
  }

  // Alert-sound rows derived from the shared store so uploads/resets reflect live.
  $: soundRows = [
    { kind: 'study' as const, label: 'Study end', name: $sounds.study.name, custom: !!$sounds.study.url },
    { kind: 'break' as const, label: 'Break end', name: $sounds.break.name, custom: !!$sounds.break.url },
  ];
  async function onUpload(kind: SoundKind, e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // allow re-picking the same file later
    if (!file) return;
    try {
      await uploadSound(kind, file);
    } catch (err) {
      dispatch('error', `Couldn't save that sound: ${err}`);
    }
  }

  // Launch on login. The whole card only exists on desktop
  // (capabilities.autoStart); `available` is additionally false in unbundled
  // dev builds, where we show the toggle disabled rather than wiring it to a
  // transient path.
  let autoStart: main.AutoStartStatus = { enabled: false, available: false };
  let autoBusy = false;
  onMount(async () => {
    if (!capabilities.autoStart) return;
    try {
      autoStart = await GetAutoStart();
    } catch (e) {
      dispatch('error', String(e));
    }
  });
  async function toggleAutoStart() {
    if (autoBusy || !autoStart.available) return;
    autoBusy = true;
    try {
      autoStart = await SetAutoStart(!autoStart.enabled);
    } catch (e) {
      dispatch('error', String(e));
    } finally {
      autoBusy = false;
    }
  }

  // Calendar export: writes the outstanding schedule to an .ics file the user
  // imports into Google/Apple/Outlook. The result path is shown inline; the
  // export is a point-in-time snapshot, so re-export to pick up later changes.
  let exporting = false;
  let exportMsg = '';
  async function exportCalendar() {
    if (exporting) return;
    exporting = true;
    exportMsg = '';
    try {
      const path = await ExportCalendar();
      exportMsg = path ? `Saved to ${path}` : ''; // "" = the user cancelled
    } catch (e) {
      dispatch('error', `Couldn't export calendar: ${e}`);
    } finally {
      exporting = false;
    }
  }

  // Backup (web only): this browser's storage is the only copy of the data, so
  // an exported JSON file is the user's insurance against cleared site data.
  // Importing replaces everything, behind an explicit confirm; App swaps in the
  // returned state via the changed/focusChanged events.
  let backupMsg = '';
  let importFile: File | null = null; // non-null = confirm modal is open
  let importing = false;

  async function exportData() {
    backupMsg = '';
    try {
      const name = await exportBackup();
      backupMsg = `Saved ${name}`;
    } catch (e) {
      dispatch('error', `Couldn't export your data: ${e}`);
    }
  }

  function onImportPick(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // allow re-picking the same file later
    if (file) importFile = file;
  }

  async function onImportChoose(e: CustomEvent<string>) {
    const file = importFile;
    importFile = null;
    if (e.detail !== 'replace' || !file || importing) return;
    importing = true;
    backupMsg = '';
    try {
      const res = await importBackup(await file.text());
      dispatch('changed', res.state);
      dispatch('focusChanged', res.focusSessions);
      // The goal input is seeded once per mount, so re-seed it by hand.
      goalHours = res.state.settings.dailyGoalMinutes / 60;
      backupMsg = 'Data restored.';
    } catch (err) {
      dispatch('error', `Couldn't import that backup: ${err}`);
    } finally {
      importing = false;
    }
  }

  // Web notifications: the permission request must come from the toggle's
  // click (a user gesture). The user's preference (notifyEnabled) and the
  // browser's grant are tracked separately — a blocked grant shows inline.
  let notifyBusy = false;
  let notifyBlocked = false;

  async function toggleNotify() {
    if (notifyBusy) return;
    notifyBusy = true;
    try {
      if ($notifyEnabled) {
        notifyEnabled.set(false);
      } else {
        notifyBlocked = false;
        if (await requestPermission()) {
          notifyEnabled.set(true);
          void maybeNotifyDueToday(true); // immediate proof it works
        } else {
          notifyBlocked = true;
        }
      }
    } finally {
      notifyBusy = false;
    }
  }
</script>

<section class="flex max-w-[640px] flex-col gap-[1.1rem]">
  <!-- Appearance -->
  <div class="rounded-lg border border-line bg-surface px-5 py-[1.1rem] shadow-1">
    <h3 class="m-0 mb-[0.2rem] font-display text-base font-bold text-fg-strong">Appearance</h3>
    <p class="mb-[0.8rem] text-[0.8rem] text-fg-muted">Theme used across the app.</p>
    <div class="seg max-w-[220px]">
      <button class:active={$theme === 'dark'} on:click={() => theme.set('dark')}>☾ Dark</button>
      <button class:active={$theme === 'light'} on:click={() => theme.set('light')}>☀ Light</button>
    </div>
  </div>

  <!-- Daily focus goal -->
  <div class="rounded-lg border border-line bg-surface px-5 py-[1.1rem] shadow-1">
    <h3 class="m-0 mb-[0.2rem] font-display text-base font-bold text-fg-strong">Daily focus goal</h3>
    <p class="mb-[0.8rem] text-[0.8rem] text-fg-muted">Target focus time per day, shown as the Home progress ring. Set 0 to disable it.</p>
    <label class="flex items-center gap-[0.5rem] text-[0.85rem] text-fg-muted">
      <input
        class="w-[5rem] rounded-sm border border-line bg-inset px-[0.5rem] py-[0.3rem] text-[0.9rem] text-fg-strong"
        type="number"
        min="0"
        step="0.5"
        bind:value={goalHours}
        on:change={saveGoal}
        on:blur={saveGoal}
      />
      hours / day
    </label>
  </div>

  <!-- Focus timer defaults -->
  <div class="rounded-lg border border-line bg-surface px-5 py-[1.1rem] shadow-1">
    <h3 class="m-0 mb-[0.2rem] font-display text-base font-bold text-fg-strong">Focus timer</h3>
    <p class="mb-[0.8rem] text-[0.8rem] text-fg-muted">Defaults for the Focus tab. Changing a running block is still done there.</p>

    <div class="mb-[0.9rem]">
      <span class="mb-1 block text-[0.72rem] font-semibold uppercase tracking-[0.08em] text-fg-muted">Mode</span>
      <div class="seg max-w-[260px]">
        <button class:active={$mode === 'timer'} on:click={() => mode.set('timer')}>Timer</button>
        <button class:active={$mode === 'stopwatch'} on:click={() => mode.set('stopwatch')}>Stopwatch</button>
      </div>
    </div>

    <div class="grid max-w-[320px] grid-cols-2 gap-3">
      <div>
        <label class="mb-1 block text-[0.72rem] font-semibold uppercase tracking-[0.08em] text-fg-muted" for="set-focus-len">Focus (min)</label>
        <input id="set-focus-len" class="w-full" type="number" min={FOCUS_MIN} max={FOCUS_MAX} bind:value={$focusMin} />
      </div>
      <div>
        <label class="mb-1 block text-[0.72rem] font-semibold uppercase tracking-[0.08em] text-fg-muted" for="set-break-len">Break (min)</label>
        <input id="set-break-len" class="w-full" type="number" min={BREAK_MIN} max={BREAK_MAX} bind:value={$breakMin} />
      </div>
    </div>

    <div class="mt-[1.1rem] border-t border-line pt-[0.9rem]">
      <span class="mb-[0.6rem] block text-[0.72rem] font-semibold uppercase tracking-[0.08em] text-fg-muted">Alert sounds</span>
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
              <input type="file" accept="audio/*" class="hidden" on:change={(e) => onUpload(row.kind, e)} />
            </label>
            {#if row.custom}
              <button class="btn ghost sm" type="button" on:click={() => clearSound(row.kind)}>Reset</button>
            {/if}
          </div>
        {/each}
      </div>
    </div>
  </div>

  {#if capabilities.autoStart}
    <!-- Startup (desktop only) -->
    <div class="rounded-lg border border-line bg-surface px-5 py-[1.1rem] shadow-1">
      <h3 class="m-0 mb-[0.2rem] font-display text-base font-bold text-fg-strong">Startup</h3>
      <p class="mb-[0.8rem] text-[0.8rem] text-fg-muted">
        {#if autoStart.available}
          Open Study Planner automatically when you sign in.
        {:else}
          Launching on login is available in the installed app only.
        {/if}
      </p>
      <label class="inline-flex cursor-pointer items-center gap-[0.5rem] text-[0.88rem] text-fg {autoStart.available ? '' : 'cursor-not-allowed opacity-60'}">
        <input
          type="checkbox"
          checked={autoStart.enabled}
          disabled={!autoStart.available || autoBusy}
          on:click|preventDefault={toggleAutoStart}
        />
        <span>Launch on login</span>
      </label>
    </div>
  {/if}

  {#if capabilities.notifications}
    <!-- Notifications (web only; the desktop app notifies natively) -->
    <div class="rounded-lg border border-line bg-surface px-5 py-[1.1rem] shadow-1">
      <h3 class="m-0 mb-[0.2rem] font-display text-base font-bold text-fg-strong">Notifications</h3>
      <p class="mb-[0.8rem] text-[0.8rem] text-fg-muted">A browser notification summarising due and overdue sessions, shown when the app first sees each new day.</p>
      <label class="inline-flex cursor-pointer items-center gap-[0.5rem] text-[0.88rem] text-fg">
        <input
          type="checkbox"
          checked={$notifyEnabled}
          disabled={notifyBusy}
          on:click|preventDefault={toggleNotify}
        />
        <span>Due-today reminders</span>
      </label>
      {#if notifyBlocked}
        <p class="mt-[0.5rem] text-[0.78rem] text-red">Notifications are blocked for this site in your browser settings.</p>
      {/if}
    </div>
  {/if}

  <!-- Calendar export -->
  <div class="rounded-lg border border-line bg-surface px-5 py-[1.1rem] shadow-1">
    <h3 class="m-0 mb-[0.2rem] font-display text-base font-bold text-fg-strong">Calendar export</h3>
    <p class="mb-[0.8rem] text-[0.8rem] text-fg-muted">Download an .ics file of your outstanding study schedule to import into Google, Apple or Outlook calendars. It's a snapshot — export again to reflect later changes.</p>
    <div class="flex items-center gap-3">
      <button class="btn primary" type="button" on:click={exportCalendar} disabled={exporting}>
        {exporting ? 'Exporting…' : 'Export .ics'}
      </button>
      {#if exportMsg}
        <span class="min-w-0 truncate text-[0.8rem] text-fg-muted" title={exportMsg}>{exportMsg}</span>
      {/if}
    </div>
  </div>

  {#if capabilities.backup}
    <!-- Backup (web only) -->
    <div class="rounded-lg border border-line bg-surface px-5 py-[1.1rem] shadow-1">
      <h3 class="m-0 mb-[0.2rem] font-display text-base font-bold text-fg-strong">Backup</h3>
      <p class="mb-[0.8rem] text-[0.8rem] text-fg-muted">Your data lives only in this browser. Export a backup file to keep it safe or move it elsewhere; import one to replace everything here.</p>
      <div class="flex items-center gap-3">
        <button class="btn primary" type="button" on:click={exportData}>Export data</button>
        <label class="btn ghost cursor-pointer">
          Import data…
          <input type="file" accept="application/json,.json" class="hidden" disabled={importing} on:change={onImportPick} />
        </label>
        {#if backupMsg}
          <span class="min-w-0 truncate text-[0.8rem] text-fg-muted" title={backupMsg}>{backupMsg}</span>
        {/if}
      </div>
    </div>
  {/if}
</section>

{#if importFile}
  <ConfirmModal
    title="Replace all data?"
    message={`Importing "${importFile.name}" replaces every task, subject, session and focus record in this browser with the backup's contents.`}
    actions={[
      { value: 'replace', label: 'Replace everything', kind: 'danger' },
      { value: 'cancel', label: 'Cancel' },
    ]}
    on:choose={onImportChoose}
  />
{/if}
