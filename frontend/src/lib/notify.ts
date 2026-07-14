// Web-only due-today notifications — the browser analogue of the desktop
// backend's notifyLoop (which fires natively at launch and each midnight, and
// never runs through this code). Wiring rides the existing `today` live store:
// it emits once on subscribe (app load) and again when the date rolls over via
// its minute/focus/visibility re-checks, which are exactly the desktop loop's
// two moments. Unlike the desktop, notifications are deduped per day — web
// page loads are far more frequent than app launches.
import { writable, type Writable } from 'svelte/store';
import { capabilities, GetState } from './backend';
import { plural, todayISO } from './dates';
import { today } from './today';

export const notifySupported = capabilities.notifications;

const ENABLED_KEY = 'study-planner:notifyEnabled';
const LAST_KEY = 'study-planner:lastNotifiedDate';

// persistedBool mirrors focusPrefs.ts's persisted(): a writable mirrored to
// localStorage, tolerating storage being unavailable.
function persistedBool(key: string, initial: boolean): Writable<boolean> {
  let raw: string | null = null;
  try {
    raw = localStorage.getItem(key);
  } catch {
    // Privacy modes: fall back to a session-only store.
  }
  const store = writable<boolean>(raw !== null ? raw === 'true' : initial);
  store.subscribe((v) => {
    try {
      localStorage.setItem(key, String(v));
    } catch {
      // Best effort.
    }
  });
  return store;
}

// Whether the user has turned reminders on (their preference — separate from
// whether the browser has granted permission).
export const notifyEnabled = persistedBool(ENABLED_KEY, false);

// requestPermission asks the browser for notification permission. Must be
// called from a user gesture (the Settings toggle); resolves to whether
// notifications may now be shown.
export async function requestPermission(): Promise<boolean> {
  if (!notifySupported) return false;
  if (Notification.permission === 'granted') return true;
  if (Notification.permission === 'denied') return false;
  try {
    return (await Notification.requestPermission()) === 'granted';
  } catch {
    return false;
  }
}

// maybeNotifyDueToday shows a summary of today's workload — counted exactly
// like the Go backend's notifyDueToday: not-done sessions of non-archived
// tasks, due (== today) and overdue (< today). Silent when there's nothing to
// say, when already notified today (unless force), or when the platform
// rejects page-scope notifications (Android Chrome throws — degrade quietly).
export async function maybeNotifyDueToday(force = false): Promise<void> {
  if (!notifySupported || Notification.permission !== 'granted') return;
  const day = todayISO();
  if (!force) {
    try {
      if (localStorage.getItem(LAST_KEY) === day) return;
    } catch {
      // No dedupe without storage; still notify.
    }
  }

  let due = 0;
  let overdue = 0;
  try {
    const st = await GetState();
    for (const t of st.tasks) {
      if (t.archived) continue;
      for (const s of t.sessions) {
        if (s.done) continue;
        if (s.date === day) due++;
        else if (s.date < day) overdue++;
      }
    }
  } catch {
    return; // a failed load should never surface as a notification error
  }
  if (due === 0 && overdue === 0) return;

  const parts: string[] = [];
  if (due > 0) parts.push(`${due} session${plural(due)} due today`);
  if (overdue > 0) parts.push(`${overdue} overdue`);
  try {
    new Notification('Study Planner', {
      body: parts.join(' · '),
      tag: 'study-planner-due-today',
    });
    try {
      localStorage.setItem(LAST_KEY, day);
    } catch {
      // Best effort.
    }
  } catch {
    // Page-scope notifications unsupported here; nothing to do.
  }
}

// initNotifications subscribes the reminder to the app lifecycle; returns the
// unsubscriber. Safe to call anywhere — a no-op unless this host supports
// notifications.
export function initNotifications(): () => void {
  if (!notifySupported) return () => {};
  let enabled = false;
  const unsubEnabled = notifyEnabled.subscribe((v) => (enabled = v));
  const unsubToday = today.subscribe(() => {
    if (enabled) void maybeNotifyDueToday();
  });
  return () => {
    unsubEnabled();
    unsubToday();
  };
}
