// TypeScript port of the Go App methods (app.go) over the localStorage-backed
// store — the browser build's whole backend. Every public method matches its
// Wails binding exactly: same name, same arguments, resolves with the same
// snapshot shape, and rejects with the same bare message string (Wails rejects
// with strings, and lib/mutate.ts renders errors via String(e) — an Error
// object here would gain an "Error: " prefix that desktop toasts don't have).
//
// The mutate() wrapper mirrors Go's: back up, run the mutation, persist, and
// roll memory back on ANY failure (including a localStorage quota error) so
// memory never drifts from storage. `now` is injectable so date-dependent
// scheduling is deterministic in tests, exactly like Go's App.now.
import type { main } from '../../../../wailsjs/go/models';
import { toISO } from '../../dates';
import {
  addDates,
  addDaysISO,
  clone,
  DEFAULT_INTERVALS,
  findSession,
  GRADE_FACTORS,
  hasPendingOn,
  isValidDate,
  newId,
  normalizeOrder,
  normalizeSubjectOrder,
  normalizeTags,
  pendingDates,
  pickColor,
  removeSession,
  sortSessions,
  sortSubjects,
  sortTasks,
  spacedDates,
  subjectColors,
  taskColors,
  utcMidnightMs,
  validColor,
  type FocusSession,
  type Session,
  type State,
  type Subject,
  type Task,
} from './models';
import { LocalStore, localStorageKV, type KV } from './store';
import { buildICS } from './ical';
import { downloadBlob } from './download';
import { applyBackup, buildBackupJSON } from './backup';

// The desktop bindings' surface (27 methods), plus the two web-only backup
// calls that have no desktop counterpart.
export interface LocalBackend {
  GetState(): Promise<main.State>;
  GetFocusSessions(): Promise<main.FocusSession[]>;
  AddTask(name: string, description: string, subjectID: string): Promise<main.State>;
  UpdateTask(id: string, name: string, description: string, tags: string[]): Promise<main.State>;
  SetTaskColor(id: string, color: string): Promise<main.State>;
  SetTaskArchived(id: string, archived: boolean): Promise<main.State>;
  SetTaskSubject(taskID: string, subjectID: string): Promise<main.State>;
  ReorderTasks(orderedIDs: string[]): Promise<main.State>;
  DeleteTask(id: string): Promise<main.State>;
  AddSubject(name: string): Promise<main.State>;
  UpdateSubject(id: string, name: string): Promise<main.State>;
  SetSubjectColor(id: string, color: string): Promise<main.State>;
  ReorderSubjects(orderedIDs: string[]): Promise<main.State>;
  DeleteSubject(id: string): Promise<main.State>;
  AddSession(taskID: string, date: string): Promise<main.State>;
  AddSpacedSessions(taskID: string, startDate: string, intervals: number[], replace: boolean): Promise<main.State>;
  DeleteSession(taskID: string, sessionID: string): Promise<main.State>;
  ToggleSession(taskID: string, sessionID: string): Promise<main.State>;
  RecordFocusSession(taskID: string, durationSec: number): Promise<main.FocusSession[]>;
  SetTaskAdaptive(id: string, adaptive: boolean): Promise<main.State>;
  RescheduleSession(taskID: string, sessionID: string, date: string): Promise<main.State>;
  RescheduleOverdueSessions(): Promise<main.State>;
  GradeSession(taskID: string, sessionID: string, grade: string): Promise<main.State>;
  SetDailyGoalMinutes(minutes: number): Promise<main.State>;
  ExportCalendar(): Promise<string>;
  GetAutoStart(): Promise<main.AutoStartStatus>;
  SetAutoStart(enabled: boolean): Promise<main.AutoStartStatus>;
  ExportBackup(): Promise<string>;
  ImportBackup(text: string): Promise<{ state: main.State; focusSessions: main.FocusSession[] }>;
}

export function createLocalBackend(
  kv: KV = localStorageKV(),
  now: () => Date = () => new Date()
): LocalBackend {
  const store = new LocalStore(kv);

  // The wailsjs model types are generated classes (with an unused convertValues
  // method) that are never instantiated anywhere in src/ — at runtime, Wails
  // delivers plain parsed JSON, and so do we. These casts assert that shared
  // wire shape once, at the boundary.
  const asState = (s: State) => s as unknown as main.State;
  const asFocus = (f: FocusSession[]) => f as unknown as main.FocusSession[];

  const findTask = (id: string) => store.state.tasks.find((t) => t.id === id);
  const findSubject = (id: string) => store.state.subjects.find((s) => s.id === id);

  // snapshot returns a sorted, deep-copied view of the whole graph, so callers
  // can never mutate store memory through a returned State (Go: App.snapshot).
  function snapshot(): main.State {
    for (const t of store.state.tasks) sortSessions(t.sessions);
    sortTasks(store.state.tasks);
    sortSubjects(store.state.subjects);
    return asState(clone(store.state));
  }

  // focusSnapshot returns the focus log sorted oldest-first, deep-copied.
  // Stamps are compared via Date.parse, never lexicographically: records
  // imported from the desktop app may carry non-UTC offsets.
  function focusSnapshot(): main.FocusSession[] {
    const out = clone(store.focus);
    out.sort((a, b) => (Date.parse(a.completedAt) || 0) - (Date.parse(b.completedAt) || 0));
    return asFocus(out);
  }

  // guard adapts a synchronous operation to the Wails promise convention:
  // resolve with its result, reject with a bare message string.
  function guard<T>(fn: () => T): Promise<T> {
    try {
      return Promise.resolve(fn());
    } catch (e) {
      return Promise.reject(e instanceof Error ? e.message : String(e));
    }
  }

  // mutate runs fn, persists, and returns the new snapshot. The graph (and
  // settings, which share the blob) is restored from a pre-mutation backup
  // when fn or the save throws, so memory never drifts from storage.
  function mutate(fn: () => void): main.State {
    const backupTasks = clone(store.state.tasks);
    const backupSubjects = clone(store.state.subjects);
    const backupSettings = { ...store.state.settings };
    try {
      fn();
      store.save();
    } catch (e) {
      store.state.tasks = backupTasks;
      store.state.subjects = backupSubjects;
      store.state.settings = backupSettings;
      throw e;
    }
    return snapshot();
  }

  function mutateTask(id: string, fn: (t: Task) => void): main.State {
    return mutate(() => {
      const t = findTask(id);
      if (!t) throw new Error('task not found');
      fn(t);
    });
  }

  function mutateSubject(id: string, fn: (s: Subject) => void): main.State {
    return mutate(() => {
      const s = findSubject(id);
      if (!s) throw new Error('subject not found');
      fn(s);
    });
  }

  return {
    GetState: () => guard(snapshot),

    GetFocusSessions: () => guard(focusSnapshot),

    AddTask: (name, description, subjectID) =>
      guard(() => {
        name = name.trim();
        if (!name) throw new Error('task name is required');
        return mutate(() => {
          if (subjectID !== '' && !findSubject(subjectID)) throw new Error('subject not found');
          store.state.tasks.push({
            id: newId(),
            name,
            description: description.trim(),
            color: pickColor(taskColors(store.state.tasks)),
            subjectId: subjectID,
            tags: [],
            archived: false,
            adaptive: false,
            order: store.state.tasks.length,
            createdAt: now().toISOString(),
            sessions: [],
          });
        });
      }),

    UpdateTask: (id, name, description, tags) =>
      guard(() => {
        name = name.trim();
        if (!name) throw new Error('task name is required');
        return mutateTask(id, (t) => {
          t.name = name;
          t.description = description.trim();
          t.tags = normalizeTags(tags);
        });
      }),

    SetTaskColor: (id, color) =>
      guard(() => {
        if (!validColor(color)) throw new Error('unknown colour');
        return mutateTask(id, (t) => {
          t.color = color;
        });
      }),

    SetTaskArchived: (id, archived) =>
      guard(() =>
        mutateTask(id, (t) => {
          t.archived = archived;
        })
      ),

    SetTaskSubject: (taskID, subjectID) =>
      guard(() =>
        mutate(() => {
          const t = findTask(taskID);
          if (!t) throw new Error('task not found');
          if (subjectID !== '' && !findSubject(subjectID)) throw new Error('subject not found');
          t.subjectId = subjectID;
        })
      ),

    // ReorderTasks applies a new manual order; unlisted tasks keep their
    // relative order after the listed ones (Go: ReorderTasks).
    ReorderTasks: (orderedIDs) =>
      guard(() =>
        mutate(() => {
          const pos = new Map(orderedIDs.map((id, i) => [id, i]));
          sortTasks(store.state.tasks);
          let next = orderedIDs.length;
          for (const t of store.state.tasks) {
            const p = pos.get(t.id);
            t.order = p !== undefined ? p : next++;
          }
          normalizeOrder(store.state.tasks);
        })
      ),

    DeleteTask: (id) =>
      guard(() =>
        mutate(() => {
          const kept = store.state.tasks.filter((t) => t.id !== id);
          if (kept.length === store.state.tasks.length) throw new Error('task not found');
          store.state.tasks = kept;
          normalizeOrder(store.state.tasks);
        })
      ),

    AddSubject: (name) =>
      guard(() => {
        name = name.trim();
        if (!name) throw new Error('subject name is required');
        return mutate(() => {
          store.state.subjects.push({
            id: newId(),
            name,
            color: pickColor(subjectColors(store.state.subjects)),
            order: store.state.subjects.length,
            createdAt: now().toISOString(),
          });
        });
      }),

    UpdateSubject: (id, name) =>
      guard(() => {
        name = name.trim();
        if (!name) throw new Error('subject name is required');
        return mutateSubject(id, (s) => {
          s.name = name;
        });
      }),

    SetSubjectColor: (id, color) =>
      guard(() => {
        if (!validColor(color)) throw new Error('unknown colour');
        return mutateSubject(id, (s) => {
          s.color = color;
        });
      }),

    ReorderSubjects: (orderedIDs) =>
      guard(() =>
        mutate(() => {
          const pos = new Map(orderedIDs.map((id, i) => [id, i]));
          sortSubjects(store.state.subjects);
          let next = orderedIDs.length;
          for (const s of store.state.subjects) {
            const p = pos.get(s.id);
            s.order = p !== undefined ? p : next++;
          }
          normalizeSubjectOrder(store.state.subjects);
        })
      ),

    // DeleteSubject ungroups (does not delete) the subject's tasks.
    DeleteSubject: (id) =>
      guard(() =>
        mutate(() => {
          const kept = store.state.subjects.filter((s) => s.id !== id);
          if (kept.length === store.state.subjects.length) throw new Error('subject not found');
          store.state.subjects = kept;
          for (const t of store.state.tasks) {
            if (t.subjectId === id) t.subjectId = '';
          }
          normalizeSubjectOrder(store.state.subjects);
        })
      ),

    AddSession: (taskID, date) =>
      guard(() => {
        date = date.trim();
        if (!isValidDate(date)) throw new Error('date must be in YYYY-MM-DD format');
        return mutateTask(taskID, (t) => addDates(t, [date]));
      }),

    AddSpacedSessions: (taskID, startDate, intervals, replace) =>
      guard(() => {
        startDate = startDate.trim();
        if (!isValidDate(startDate)) throw new Error('start date must be in YYYY-MM-DD format');
        if (intervals.length === 0) intervals = DEFAULT_INTERVALS;
        return mutateTask(taskID, (t) => {
          if (replace) t.sessions = [];
          addDates(t, spacedDates(startDate, intervals));
        });
      }),

    DeleteSession: (taskID, sessionID) =>
      guard(() =>
        mutateTask(taskID, (t) => {
          if (!removeSession(t, sessionID)) throw new Error('session not found');
        })
      ),

    ToggleSession: (taskID, sessionID) =>
      guard(() =>
        mutateTask(taskID, (t) => {
          const s = findSession(t, sessionID);
          if (!s) throw new Error('session not found');
          s.done = !s.done;
          if (s.done) {
            s.completedAt = now().toISOString();
          } else {
            delete s.completedAt;
          }
        })
      ),

    RecordFocusSession: (taskID, durationSec) =>
      guard(() => {
        if (!(durationSec > 0)) throw new Error('focus duration must be positive');
        if (taskID !== '' && !findTask(taskID)) throw new Error('task not found');
        const backup = store.focus;
        store.focus = [
          ...store.focus,
          { id: newId(), taskId: taskID, durationSec, completedAt: now().toISOString() },
        ];
        try {
          store.save();
        } catch (e) {
          store.focus = backup;
          throw e;
        }
        return focusSnapshot();
      }),

    SetTaskAdaptive: (id, adaptive) =>
      guard(() =>
        mutateTask(id, (t) => {
          t.adaptive = adaptive;
        })
      ),

    // RescheduleSession moves a session to a new date, merging (dropping the
    // moved one) when a pending session already sits on the target day. A done
    // session there is historical and coexists.
    RescheduleSession: (taskID, sessionID, date) =>
      guard(() => {
        date = date.trim();
        if (!isValidDate(date)) throw new Error('date must be in YYYY-MM-DD format');
        return mutateTask(taskID, (t) => {
          const target = findSession(t, sessionID);
          if (!target) throw new Error('session not found');
          if (target.date === date) return;
          if (hasPendingOn(t, date)) {
            removeSession(t, sessionID);
            return;
          }
          target.date = date;
        });
      }),

    // RescheduleOverdueSessions moves every overdue, not-done session of every
    // active task to today; a task ends up with at most one pending session
    // today and surplus overdue ones are removed as covered. A done session on
    // today doesn't count as covering.
    RescheduleOverdueSessions: () =>
      guard(() =>
        mutate(() => {
          const today = toISO(now());
          for (const t of store.state.tasks) {
            if (t.archived) continue;
            let hasToday = hasPendingOn(t, today);
            const kept: Session[] = [];
            for (const s of t.sessions) {
              if (!s.done && s.date < today) {
                if (hasToday) continue;
                s.date = today;
                hasToday = true;
              }
              kept.push(s);
            }
            t.sessions = kept;
          }
        })
      ),

    // GradeSession marks a session done with a recall grade and re-spaces the
    // task's remaining schedule (SM-2 lite) — see Go GradeSession for the
    // full reasoning; this is a line-for-line port.
    GradeSession: (taskID, sessionID, grade) =>
      guard(() => {
        const factor = GRADE_FACTORS.get(grade);
        if (factor === undefined) throw new Error('grade must be one of: again, hard, good, easy');
        return mutateTask(taskID, (t) => {
          const graded = findSession(t, sessionID);
          if (!graded) throw new Error('session not found');
          if (graded.done) throw new Error('session is already done');
          // Validate before mutating: a malformed date (possible via import)
          // must not leave the session half-graded.
          if (!isValidDate(graded.date)) throw new Error('session has an invalid date');
          const gradedMs = utcMidnightMs(graded.date);
          const nowDate = now();
          graded.done = true;
          graded.completedAt = nowDate.toISOString();
          // Only pending sessions occupy a day; done ones are historical, so
          // re-spaced reviews may land on (and coexist with) a completed day.
          // Sessions after the graded one are about to be rewritten, so their
          // old dates leave the collision set.
          const occupied = pendingDates(t);
          const future: Session[] = [];
          for (const s of t.sessions) {
            if (!s.done && s.date > graded.date) {
              future.push(s);
              occupied.delete(s.date);
            }
          }
          future.sort((a, b) => (a.date < b.date ? -1 : a.date > b.date ? 1 : 0));

          // Go anchors to the local calendar date, then does pure day math.
          const todayLocal = toISO(nowDate);
          const dateAt = (n: number) => addDaysISO(todayLocal, n);
          let prev = 0;
          for (const s of future) {
            if (!isValidDate(s.date)) continue;
            const gap = Math.round((utcMidnightMs(s.date) - gradedMs) / 86_400_000);
            let next = Math.round(gap * factor);
            if (grade === 'again' && prev === 0) next = 1;
            if (next <= prev) next = prev + 1;
            while (occupied.has(dateAt(next))) next++;
            s.date = dateAt(next);
            occupied.add(s.date);
            prev = next;
          }
          if (grade === 'again' && future.length === 0 && !occupied.has(dateAt(1))) {
            t.sessions.push({ id: newId(), date: dateAt(1), done: false });
          }
        });
      }),

    SetDailyGoalMinutes: (minutes) =>
      guard(() => {
        if (!(minutes >= 0)) throw new Error('daily goal cannot be negative');
        return mutate(() => {
          store.state.settings.dailyGoalMinutes = Math.round(minutes);
        });
      }),

    // ExportCalendar builds the .ics in-browser and hands it over as a
    // download — the web stand-in for the desktop's native save dialog. The
    // returned name feeds the same "Saved to …" message the desktop shows.
    ExportCalendar: () =>
      guard(() => {
        const ics = buildICS(store.state.tasks, now());
        downloadBlob('study-planner.ics', ics, 'text/calendar;charset=utf-8');
        return 'study-planner.ics';
      }),

    // Launch-on-login is an OS feature with no web equivalent; Settings hides
    // the card entirely on the web (capabilities.autoStart).
    GetAutoStart: () => Promise.resolve({ enabled: false, available: false }),
    SetAutoStart: () => Promise.reject('launch on login is only available in the installed app'),

    ExportBackup: () =>
      guard(() => {
        const name = `study-planner-backup-${toISO(now())}.json`;
        downloadBlob(name, buildBackupJSON(store, now()), 'application/json');
        return name;
      }),

    ImportBackup: (text) =>
      guard(() => {
        applyBackup(store, text);
        return { state: snapshot(), focusSessions: focusSnapshot() };
      }),
  };
}
