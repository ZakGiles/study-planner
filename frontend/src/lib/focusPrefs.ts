// Focus-timer defaults (lengths and mode) as shared, locally-persisted stores so
// both the Focus tab (operational use) and the Settings tab (as defaults) read
// and write one source of truth. Durations are clamped to sane bounds wherever
// they feed the clock so a stray input can't break it.
import { writable, type Writable } from 'svelte/store';

export const FOCUS_MIN = 1,
  FOCUS_MAX = 180,
  BREAK_MIN = 1,
  BREAK_MAX = 60;

export const clamp = (n: number, lo: number, hi: number) =>
  Math.min(hi, Math.max(lo, Math.round(Number.isFinite(n) ? n : lo)));

export type Mode = 'timer' | 'stopwatch';

// persisted builds a writable mirrored to localStorage under `key`, seeding from
// the stored value (run through `parse`) or `initial` when absent.
function persisted<T>(key: string, initial: T, parse: (raw: string) => T): Writable<T> {
  const raw = localStorage.getItem(key);
  const store = writable<T>(raw !== null ? parse(raw) : initial);
  store.subscribe((v) => localStorage.setItem(key, String(v)));
  return store;
}

export const focusMin = persisted('focusMin', 25, (r) => clamp(Number(r) || 25, FOCUS_MIN, FOCUS_MAX));
export const breakMin = persisted('breakMin', 5, (r) => clamp(Number(r) || 5, BREAK_MIN, BREAK_MAX));
export const mode = persisted<Mode>('focusMode', 'timer', (r) => (r === 'stopwatch' ? 'stopwatch' : 'timer'));
