// Date helpers shared across the UI. All study dates are plain YYYY-MM-DD
// strings interpreted in the user's local timezone.

const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

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

export function relativeLabel(s: string): string {
  const n = daysFromToday(s);
  if (n === 0) return 'Today';
  if (n === 1) return 'Tomorrow';
  if (n === -1) return 'Yesterday';
  if (n < 0) return `${-n} days ago`;
  return `in ${n} days`;
}

// parseIntervals turns "0, 1, 3, 7" into [0, 1, 3, 7], ignoring junk.
export function parseIntervals(s: string): number[] {
  return s
    .split(/[\s,]+/)
    .map((x) => x.trim())
    .filter((x) => x !== '')
    .map(Number)
    .filter((n) => Number.isFinite(n) && n >= 0)
    .map((n) => Math.round(n));
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
