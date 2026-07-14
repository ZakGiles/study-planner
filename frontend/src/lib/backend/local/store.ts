// localStorage persistence for the browser backend. Mirrors the Go store's
// model: the in-memory state is authoritative and every mutation rewrites the
// whole blob (Go rewrites the whole SQLite graph the same way). The KV seam
// exists so tests can run against an in-memory map and inject write failures.
import {
  DEFAULT_DAILY_GOAL_MINUTES,
  normalizeOrder,
  normalizeSubjectOrder,
  type FocusSession,
  type Session,
  type State,
  type Subject,
  type Task,
} from './models';

export interface KV {
  get(): string | null;
  set(value: string): void;
  // backup preserves an unreadable blob out-of-band before starting fresh.
  backup?(value: string): void;
}

export const STORAGE_KEY = 'study-planner:data';
export const SCHEMA_VERSION = 1;

// localStorageKV binds the store to one localStorage key. Reads degrade to
// "no data" when storage is unavailable (privacy modes); writes surface a
// readable error for the mutation rollback path (quota, lockdown).
export function localStorageKV(key = STORAGE_KEY): KV {
  return {
    get() {
      try {
        return localStorage.getItem(key);
      } catch {
        return null;
      }
    },
    set(value) {
      try {
        localStorage.setItem(key, value);
      } catch {
        throw new Error("couldn't save: browser storage is full or unavailable");
      }
    },
    backup(value) {
      try {
        localStorage.setItem(`${key}.corrupt-${Date.now()}`, value);
      } catch {
        // Best effort — starting fresh matters more than the backup.
      }
    },
  };
}

export function defaultState(): State {
  return { subjects: [], tasks: [], settings: { dailyGoalMinutes: DEFAULT_DAILY_GOAL_MINUTES } };
}

const str = (v: unknown, fallback = ''): string => (typeof v === 'string' ? v : fallback);
const num = (v: unknown, fallback: number): number =>
  typeof v === 'number' && Number.isFinite(v) ? v : fallback;

// normalizeState rebuilds a State from untrusted JSON (the stored blob or an
// imported backup), defaulting missing fields and dropping entries without the
// identity fields nothing can work without. Returns null when the payload is
// not recognizably a task graph — callers treat that as corrupt. Guarantees the
// invariants components rely on: arrays are always materialized (never
// null/undefined) and order fields are contiguous, mirroring the Go store's
// load() normalization.
export function normalizeState(raw: unknown): State | null {
  if (!raw || typeof raw !== 'object') return null;
  const r = raw as Record<string, unknown>;
  if (!Array.isArray(r.tasks) || !Array.isArray(r.subjects)) return null;

  const tasks: Task[] = [];
  for (const item of r.tasks) {
    if (!item || typeof item !== 'object') continue;
    const t = item as Record<string, unknown>;
    if (typeof t.id !== 'string' || !t.id) continue;
    const sessions: Session[] = [];
    if (Array.isArray(t.sessions)) {
      for (const sItem of t.sessions) {
        if (!sItem || typeof sItem !== 'object') continue;
        const s = sItem as Record<string, unknown>;
        if (typeof s.id !== 'string' || typeof s.date !== 'string') continue;
        const session: Session = { id: s.id, date: s.date, done: !!s.done };
        if (typeof s.completedAt === 'string') session.completedAt = s.completedAt;
        sessions.push(session);
      }
    }
    tasks.push({
      id: t.id,
      name: str(t.name),
      description: str(t.description),
      color: str(t.color),
      subjectId: str(t.subjectId),
      tags: Array.isArray(t.tags) ? t.tags.filter((x): x is string => typeof x === 'string') : [],
      archived: !!t.archived,
      adaptive: !!t.adaptive,
      order: num(t.order, 0),
      createdAt: str(t.createdAt, new Date(0).toISOString()),
      sessions,
    });
  }

  const subjects: Subject[] = [];
  for (const item of r.subjects) {
    if (!item || typeof item !== 'object') continue;
    const s = item as Record<string, unknown>;
    if (typeof s.id !== 'string' || !s.id) continue;
    subjects.push({
      id: s.id,
      name: str(s.name),
      color: str(s.color),
      order: num(s.order, 0),
      createdAt: str(s.createdAt, new Date(0).toISOString()),
    });
  }

  const settings = (r.settings ?? {}) as Record<string, unknown>;
  const goal = num(settings.dailyGoalMinutes, DEFAULT_DAILY_GOAL_MINUTES);

  const state: State = {
    subjects,
    tasks,
    settings: { dailyGoalMinutes: goal >= 0 ? Math.round(goal) : DEFAULT_DAILY_GOAL_MINUTES },
  };
  normalizeOrder(state.tasks);
  normalizeSubjectOrder(state.subjects);
  return state;
}

// normalizeFocus rebuilds the focus log from untrusted JSON, dropping records
// missing their identity or stamp. A malformed log never blocks a load.
export function normalizeFocus(raw: unknown): FocusSession[] {
  if (!Array.isArray(raw)) return [];
  const out: FocusSession[] = [];
  for (const item of raw) {
    if (!item || typeof item !== 'object') continue;
    const f = item as Record<string, unknown>;
    if (typeof f.id !== 'string' || !f.id) continue;
    if (typeof f.completedAt !== 'string') continue;
    const dur = num(f.durationSec, 0);
    if (dur <= 0) continue;
    out.push({ id: f.id, taskId: str(f.taskId), durationSec: dur, completedAt: f.completedAt });
  }
  return out;
}

// LocalStore owns the in-memory state and its localStorage mirror. Loading
// never throws and never silently destroys: an unreadable or newer-versioned
// blob is preserved under a timestamped backup key and the app starts fresh.
// A missing key means first run — defaults, with no eager write (the first
// mutation writes, as in Go).
export class LocalStore {
  state: State;
  focus: FocusSession[];

  constructor(private kv: KV) {
    const raw = kv.get();
    if (raw === null) {
      this.state = defaultState();
      this.focus = [];
      return;
    }
    let parsed: unknown = null;
    try {
      parsed = JSON.parse(raw);
    } catch {
      parsed = null;
    }
    const blob = parsed && typeof parsed === 'object' ? (parsed as Record<string, unknown>) : null;
    const versionOK = blob !== null && (blob.version === undefined || blob.version === SCHEMA_VERSION);
    const state = versionOK ? normalizeState(blob.state) : null;
    if (!state) {
      kv.backup?.(raw);
      this.state = defaultState();
      this.focus = [];
      return;
    }
    this.state = state;
    this.focus = normalizeFocus(blob!.focusSessions);
  }

  // save rewrites the whole blob — task graph, settings and focus log — in one
  // write, the browser analogue of the Go store's full-graph save().
  save(): void {
    this.kv.set(
      JSON.stringify({ version: SCHEMA_VERSION, state: this.state, focusSessions: this.focus })
    );
  }
}
