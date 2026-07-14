// TypeScript port of the Go domain model (models.go) for the browser-only
// backend. The shapes here are the plain JSON forms that cross the Wails
// boundary on desktop — timestamps are ISO strings, sessions omit completedAt
// while pending — so snapshots from this store are indistinguishable from the
// Go backend's. Date arithmetic is done on UTC midnights, mirroring the Go
// side's time.Parse("2006-01-02")/AddDate math, so it is DST-proof and the two
// implementations always agree on day gaps.
import { TASK_COLORS } from '../../colors';

export interface Session {
  id: string;
  date: string; // YYYY-MM-DD
  done: boolean;
  completedAt?: string; // ISO timestamp; absent while pending (Go: omitempty)
}

export interface Task {
  id: string;
  name: string;
  description: string;
  color: string; // palette token; '' = default
  subjectId: string; // owning subject; '' = ungrouped
  tags: string[];
  archived: boolean;
  adaptive: boolean; // grade reviews and re-space the schedule
  order: number; // manual sort position
  createdAt: string; // ISO timestamp
  sessions: Session[];
}

export interface Subject {
  id: string;
  name: string;
  color: string;
  order: number;
  createdAt: string;
}

export interface Settings {
  dailyGoalMinutes: number;
}

export interface State {
  subjects: Subject[];
  tasks: Task[];
  settings: Settings;
}

export interface FocusSession {
  id: string;
  taskId: string; // '' = general focus
  durationSec: number;
  completedAt: string;
}

// The focus goal a fresh store starts with (2h), so the Home ring is meaningful
// before the user ever opens settings. Mirrors defaultDailyGoalMinutes in Go.
export const DEFAULT_DAILY_GOAL_MINUTES = 120;

// Day offsets approximating a classic spaced-repetition schedule.
export const DEFAULT_INTERVALS = [0, 1, 3, 7, 14, 30];

// gradeFactors scales the gaps between today and each remaining review;
// "again" additionally forces the next review to tomorrow. Mirrors Go's
// gradeFactors map.
export const GRADE_FACTORS = new Map<string, number>([
  ['again', 0.5],
  ['hard', 0.7],
  ['good', 1.0],
  ['easy', 1.4],
]);

// The palette tokens, in the same order as Go's TaskColors — derived from the
// UI palette so the two can't drift.
export const COLOR_TOKENS: string[] = TASK_COLORS.map((c) => c.token);

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

// isValidDate matches Go's time.Parse("2006-01-02", …): exactly zero-padded
// YYYY-MM-DD digits AND a real calendar date (2026-02-30 is rejected). The
// round-trip through Date.UTC catches out-of-range months and days.
export function isValidDate(s: string): boolean {
  if (!DATE_RE.test(s)) return false;
  const [y, m, d] = s.split('-').map(Number);
  const dt = new Date(Date.UTC(y, m - 1, d));
  return dt.getUTCFullYear() === y && dt.getUTCMonth() === m - 1 && dt.getUTCDate() === d;
}

// utcMidnightMs returns the UTC-midnight epoch millis of a valid ISO date, the
// basis for whole-day gap arithmetic (identical to Go's parsed time values).
export function utcMidnightMs(iso: string): number {
  const [y, m, d] = iso.split('-').map(Number);
  return Date.UTC(y, m - 1, d);
}

// addDaysISO adds n days to an ISO date using UTC math (Date.UTC normalizes
// overflow), mirroring Go's AddDate on a UTC-parsed date.
export function addDaysISO(iso: string, n: number): string {
  const [y, m, d] = iso.split('-').map(Number);
  const dt = new Date(Date.UTC(y, m - 1, d + n));
  const mm = String(dt.getUTCMonth() + 1).padStart(2, '0');
  const dd = String(dt.getUTCDate()).padStart(2, '0');
  return `${dt.getUTCFullYear()}-${mm}-${dd}`;
}

// newId generates a v4 UUID — the same format as Go's uuid.NewString(), so ids
// from either backend are interchangeable. crypto.randomUUID needs a secure
// context (HTTPS/localhost); the fallback covers LAN-IP dev servers.
export function newId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}

// clone deep-copies a JSON-shaped value. The store holds only JSON-safe data
// (strings/numbers/booleans/arrays/objects), so a JSON round-trip is exact and
// also drops undefined properties — exactly how the wire format behaves.
export function clone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T;
}

// pickColor returns the palette token least used among the supplied colours,
// breaking ties by the palette's natural order (see Go pickColor). A reset ('')
// colour doesn't count against any token.
export function pickColor(used: string[]): string {
  const counts = new Map<string, number>();
  for (const c of used) counts.set(c, (counts.get(c) ?? 0) + 1);
  let best = COLOR_TOKENS[0];
  for (const c of COLOR_TOKENS.slice(1)) {
    if ((counts.get(c) ?? 0) < (counts.get(best) ?? 0)) best = c;
  }
  return best;
}

export function taskColors(tasks: Task[]): string[] {
  return tasks.map((t) => t.color);
}

export function subjectColors(subjects: Subject[]): string[] {
  return subjects.map((s) => s.color);
}

// validColor reports whether c is a known palette token; '' means "default".
export function validColor(c: string): boolean {
  return c === '' || COLOR_TOKENS.includes(c);
}

// normalizeTags trims, drops empties and de-duplicates tags case-insensitively
// (keeping the first-seen casing), capping both the count and each tag's
// length. Length is measured in code points ([...t]), matching Go's runes.
export function normalizeTags(tags: string[]): string[] {
  const maxTags = 12;
  const maxLen = 30;
  const out: string[] = [];
  const seen = new Set<string>();
  for (let t of tags) {
    t = t.trim();
    if (!t) continue;
    const runes = [...t];
    if (runes.length > maxLen) t = runes.slice(0, maxLen).join('');
    const key = t.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(t);
    if (out.length >= maxTags) break;
  }
  return out;
}

// createdMs parses a stored timestamp for order tie-breaks. Never compare ISO
// strings lexicographically: stamps imported from the desktop app may carry
// non-UTC offsets.
function createdMs(iso: string): number {
  const ms = Date.parse(iso);
  return Number.isNaN(ms) ? 0 : ms;
}

// sortTasks orders tasks by their manual order, breaking ties by creation time.
// Array.prototype.sort is stable (ES2019), matching Go's SliceStable.
export function sortTasks(tasks: Task[]): void {
  tasks.sort((a, b) => a.order - b.order || createdMs(a.createdAt) - createdMs(b.createdAt));
}

// normalizeOrder sorts tasks and reassigns a contiguous 0..n-1 order.
export function normalizeOrder(tasks: Task[]): void {
  sortTasks(tasks);
  tasks.forEach((t, i) => (t.order = i));
}

export function sortSubjects(subjects: Subject[]): void {
  subjects.sort((a, b) => a.order - b.order || createdMs(a.createdAt) - createdMs(b.createdAt));
}

export function normalizeSubjectOrder(subjects: Subject[]): void {
  sortSubjects(subjects);
  subjects.forEach((s, i) => (s.order = i));
}

// sortSessions orders a task's sessions chronologically. YYYY-MM-DD strings
// compare lexicographically in date order, as in Go.
export function sortSessions(sessions: Session[]): void {
  sessions.sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0));
}

// hasPendingOn reports whether the task has a not-done session on date. Done
// sessions are historical records and never block scheduling a new review.
export function hasPendingOn(t: Task, date: string): boolean {
  return t.sessions.some((s) => !s.done && s.date === date);
}

// pendingDates returns the set of dates holding a not-done session — the
// domain of the one-pending-session-per-day invariant.
export function pendingDates(t: Task): Set<string> {
  const m = new Set<string>();
  for (const s of t.sessions) if (!s.done) m.add(s.date);
  return m;
}

export function findSession(t: Task, id: string): Session | undefined {
  return t.sessions.find((s) => s.id === id);
}

// removeSession drops the session with the given id, returning whether it was
// found.
export function removeSession(t: Task, id: string): boolean {
  const i = t.sessions.findIndex((s) => s.id === id);
  if (i === -1) return false;
  t.sessions.splice(i, 1);
  return true;
}

// addDates appends new sessions for any dates the task does not already have.
// Unlike hasPendingOn this dedupes against ALL dates: generating a schedule
// should not re-add a day already completed.
export function addDates(t: Task, dates: string[]): void {
  const existing = new Set(t.sessions.map((s) => s.date));
  for (const d of dates) {
    if (existing.has(d)) continue;
    existing.add(d);
    t.sessions.push({ id: newId(), date: d, done: false });
  }
}

// spacedDates turns a start date and a set of day offsets into sorted,
// de-duplicated YYYY-MM-DD strings. Negative offsets are dropped (this differs
// from the UI's spacedPreview in dates.ts, which keeps them — port of Go's
// spacedDates, which is the behavior that actually persists).
export function spacedDates(startISO: string, intervals: number[]): string[] {
  const seen = new Set<string>();
  const dates: string[] = [];
  for (const raw of intervals) {
    const n = Math.round(raw);
    if (!Number.isFinite(n) || n < 0) continue;
    const date = addDaysISO(startISO, n);
    if (seen.has(date)) continue;
    seen.add(date);
    dates.push(date);
  }
  return dates.sort();
}
