import { readable } from 'svelte/store';
import { todayISO } from './dates';

// today holds the current local date as a YYYY-MM-DD string, re-checked on a
// one-minute interval and whenever the window regains focus or visibility.
// Time-relative UI (overdue badges, "today" highlight, relative labels) reads
// the live date through normal helpers; referencing $today in the reactive
// blocks that feed those views makes them recompute when the day rolls over,
// so an app left open past midnight refreshes instead of showing a stale day.
export const today = readable(todayISO(), (set) => {
  const update = () => set(todayISO());
  const id = setInterval(update, 60_000);
  window.addEventListener('focus', update);
  document.addEventListener('visibilitychange', update);
  return () => {
    clearInterval(id);
    window.removeEventListener('focus', update);
    document.removeEventListener('visibilitychange', update);
  };
});
