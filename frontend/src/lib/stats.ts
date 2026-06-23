// Shared study-progress aggregations used by both the Stats and Home views, so
// the streak, heatmap and goal numbers can't drift between them. Every function
// is pure and takes the local "today" (YYYY-MM-DD) so callers stay in sync with
// the rolling-midnight `today` store.
import type { main } from '../../wailsjs/go/models';
import { parseDate, toISO } from './dates';

// dayNum maps an ISO date to a whole-day index, so consecutive days differ by 1.
function dayNum(iso: string): number {
  return Math.round(parseDate(iso).getTime() / 86_400_000);
}

// computeDoneByDay maps each local day to how many sessions were completed on it.
// A done session is dated by completedAt; sessions checked off before completedAt
// existed fall back to their scheduled date, and the few legacy ones dated in the
// future are skipped.
export function computeDoneByDay(tasks: main.Task[], todayStr: string): Map<string, number> {
  const m = new Map<string, number>();
  for (const t of tasks) {
    for (const s of t.sessions) {
      if (!s.done) continue;
      const day = s.completedAt ? toISO(new Date(s.completedAt)) : s.date;
      if (day > todayStr) continue;
      m.set(day, (m.get(day) ?? 0) + 1);
    }
  }
  return m;
}

export interface Streaks {
  current: number;
  longest: number;
}

// computeStreaks counts consecutive days with at least one completion. The current
// streak survives until the end of today (an empty today doesn't break yesterday's).
export function computeStreaks(doneByDay: Map<string, number>, todayStr: string): Streaks {
  const days = [...doneByDay.keys()].map(dayNum).sort((a, b) => a - b);
  let longest = 0;
  let run = 0;
  let prev = NaN;
  for (const d of days) {
    run = d === prev + 1 ? run + 1 : 1;
    longest = Math.max(longest, run);
    prev = d;
  }
  const todayNum = dayNum(todayStr);
  const have = new Set(days);
  let current = 0;
  for (let d = have.has(todayNum) ? todayNum : todayNum - 1; have.has(d); d--) current++;
  return { current, longest };
}

// completedToday is the number of sessions completed today — the numerator of the
// Home goal ring. It shares computeDoneByDay's dating so it agrees with the streak.
export function completedToday(tasks: main.Task[], todayStr: string): number {
  return computeDoneByDay(tasks, todayStr).get(todayStr) ?? 0;
}

// dueToday counts pending sessions scheduled for today across non-archived tasks.
export function dueToday(tasks: main.Task[], todayStr: string): number {
  return tasks
    .filter((t) => !t.archived)
    .reduce((n, t) => n + t.sessions.filter((s) => !s.done && s.date === todayStr).length, 0);
}
