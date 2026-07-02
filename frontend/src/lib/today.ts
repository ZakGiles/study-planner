import { readable, type Readable } from 'svelte/store';
import { todayISO } from './dates';

// live wraps a cheap read() in a store that re-checks it on a one-minute
// interval and whenever the window regains focus or visibility. readable
// dedupes unchanged primitive values, so subscribers only fire when the
// answer actually changes.
function live<T>(read: () => T): Readable<T> {
  return readable(read(), (set) => {
    const update = () => set(read());
    const id = setInterval(update, 60_000);
    window.addEventListener('focus', update);
    document.addEventListener('visibilitychange', update);
    return () => {
      clearInterval(id);
      window.removeEventListener('focus', update);
      document.removeEventListener('visibilitychange', update);
    };
  });
}

// today holds the current local date as a YYYY-MM-DD string. Time-relative UI
// (overdue badges, "today" highlight, relative labels) reads the live date
// through normal helpers; referencing $today in the reactive blocks that feed
// those views makes them recompute when the day rolls over, so an app left
// open past midnight refreshes instead of showing a stale day.
export const today = live(todayISO);

// dayPeriod is the greeting bucket for the Home header. It gets its own store
// (rather than deriving from today) because it flips at noon and 18:00, not at
// midnight.
export type DayPeriod = 'morning' | 'afternoon' | 'evening';
export const dayPeriod = live<DayPeriod>(() => {
  const h = new Date().getHours();
  return h < 12 ? 'morning' : h < 18 ? 'afternoon' : 'evening';
});
