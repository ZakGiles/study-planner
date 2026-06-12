// Date helpers shared across the UI. All study dates are plain YYYY-MM-DD
// strings interpreted in the user's local timezone.

const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
export const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

// parseDate reads a YYYY-MM-DD string as a local date (new Date("YYYY-MM-DD")
// would parse as UTC and can shift the day).
export function parseDate(s: string): Date {
  const [y, m, d] = s.split('-').map(Number);
  return new Date(y, m - 1, d);
}

export function toISO(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

export function todayISO(): string {
  return toISO(new Date());
}

// formatDate -> "Mon 8 Jun 2026"
export function formatDate(s: string): string {
  const d = parseDate(s);
  return `${WEEKDAYS[d.getDay()]} ${d.getDate()} ${MONTHS[d.getMonth()]} ${d.getFullYear()}`;
}

// daysFromToday returns whole days between today and s (negative = in the past).
export function daysFromToday(s: string): number {
  const today = parseDate(todayISO()).getTime();
  const d = parseDate(s).getTime();
  return Math.round((d - today) / 86_400_000);
}

// plural returns the "s" suffix for a count, e.g. `session${plural(n)}`.
export function plural(n: number): string {
  return n === 1 ? '' : 's';
}

export function relativeLabel(s: string): string {
  const n = daysFromToday(s);
  if (n === 0) return 'Today';
  if (n === 1) return 'Tomorrow';
  if (n === -1) return 'Yesterday';
  if (n < 0) return `${-n} days ago`;
  return `in ${n} days`;
}

// sessionStatus classifies a session for styling: done wins, otherwise the
// date is overdue, today or upcoming.
export function sessionStatus(date: string, done = false): 'done' | 'overdue' | 'today' | 'upcoming' {
  if (done) return 'done';
  const n = daysFromToday(date);
  if (n < 0) return 'overdue';
  if (n === 0) return 'today';
  return 'upcoming';
}

// parseIntervals turns "0, 1, 3, 7" into [0, 1, 3, 7], ignoring junk.
export function parseIntervals(s: string): number[] {
  return s
    .split(/[\s,]+/)
    .filter(Boolean)
    .map(Number)
    .filter((n) => Number.isFinite(n) && n >= 0)
    .map((n) => Math.round(n));
}

// logOffsets builds day-offsets from a logarithmic curve. For each step n
// (starting at 0) the offset is:
//
//     offset(n) = dilation * factor^n * ln(n + 1)
//
// The dilation is scaled by `factor` at every step (so it compounds across all
// steps), and ln(n+1) is the partial log-graph term — step 0 is always the
// start date because ln(1) = 0. Offsets are rounded to whole days; negatives
// (only possible with a negative dilation) are dropped to match the backend.
export function logOffsets(dilation: number, factor: number, count: number): number[] {
  const out: number[] = [];
  const n = Math.floor(count);
  if (!Number.isFinite(dilation) || !Number.isFinite(factor) || !Number.isFinite(n) || n < 1) {
    return out;
  }
  for (let i = 0; i < n; i++) {
    const scaled = dilation * Math.pow(factor, i);
    const offset = Math.round(scaled * Math.log(i + 1));
    if (Number.isFinite(offset) && offset >= 0) out.push(offset);
  }
  return out;
}

// spacedPreview mirrors the Go backend: start date + day offsets -> sorted,
// de-duplicated ISO dates.
export function spacedPreview(startISO: string, intervals: number[]): string[] {
  if (!startISO) return [];
  const start = parseDate(startISO);
  const set = new Set<string>();
  for (const n of intervals) {
    const d = new Date(start);
    d.setDate(d.getDate() + n);
    set.add(toISO(d));
  }
  return [...set].sort();
}

// smoothOffsets nudges day-offsets off busy days so that, where possible, no
// day ends up with more than maxPerDay planned sessions (existing load plus
// the new ones). Candidates are tried at ±1 then ±2 days, preferring later;
// the first offset (the start session) never moves, and a session is kept on
// its busy day rather than dropped when no nearby slot is free.
export function smoothOffsets(
  startISO: string,
  offsets: number[],
  load: Record<string, number>,
  maxPerDay = 2
): number[] {
  if (!startISO) return [...offsets];
  const start = parseDate(startISO);
  const dateOf = (n: number) => {
    const d = new Date(start);
    d.setDate(d.getDate() + n);
    return toISO(d);
  };
  const placed = new Set<number>();
  const free = (m: number) => m >= 0 && !placed.has(m) && (load[dateOf(m)] ?? 0) < maxPerDay;
  const out: number[] = [];
  const sorted = Array.from(new Set(offsets)).sort((a, b) => a - b);
  sorted.forEach((n, i) => {
    // The first offset (the start session) is pinned; the rest may shift.
    let pick: number | null = i === 0 || free(n) ? n : null;
    // Widen the search outward (preferring later), honouring the load cap, up
    // to a generous window before giving up.
    for (let delta = 1; pick === null && delta <= 14; delta++) {
      if (free(n + delta)) pick = n + delta;
      else if (free(n - delta)) pick = n - delta;
    }
    // Last resort, only when no day within the window is under the cap: take
    // the nearest day not already used, so the session is never dropped.
    for (let m = n; pick === null; m++) {
      if (!placed.has(m)) pick = m;
    }
    placed.add(pick);
    out.push(pick);
  });
  return out.sort((a, b) => a - b);
}
