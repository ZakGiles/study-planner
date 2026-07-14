// Storage-layer behavior: first-run defaults, persistence round-trips, the
// mutate() rollback contract (port of TestMutateRollsBackOnSaveFailure), and
// the corrupt-blob recovery path that replaces the Go store's SQLite-specific
// tests.
import { describe, expect, it } from 'vitest';
import { day, memKV, newTestApp, sessionDates } from './testUtil';

describe('LocalStore', () => {
  it('first run: defaults with no eager write', async () => {
    const { app, kv } = newTestApp();
    const st = await app.GetState();
    expect(st.tasks).toEqual([]);
    expect(st.subjects).toEqual([]);
    expect(st.settings.dailyGoalMinutes).toBe(120);
    expect(await app.GetFocusSessions()).toEqual([]);
    // Loading and reading never write; the first mutation does.
    expect(kv.raw()).toBeNull();
  });

  it('mutations persist across a reload', async () => {
    const { app, kv } = newTestApp();
    const tasks = (await app.AddTask('Physics', 'notes', '')).tasks;
    await app.AddSession(tasks[0].id, day(1));
    await app.SetDailyGoalMinutes(90);
    await app.RecordFocusSession('', 600);

    const { app: reopened } = newTestApp(kv);
    const st = await reopened.GetState();
    expect(st.tasks).toHaveLength(1);
    expect(st.tasks[0].name).toBe('Physics');
    expect(st.tasks[0].description).toBe('notes');
    expect(sessionDates(st.tasks[0])).toEqual([day(1)]);
    expect(st.settings.dailyGoalMinutes).toBe(90);
    expect(await reopened.GetFocusSessions()).toHaveLength(1);
  });

  it('rolls the in-memory graph back when a save fails', async () => {
    const { app, kv } = newTestApp();
    await app.AddTask('Keep', '', '');

    kv.failWrites(true);
    await expect(app.AddTask('Doomed', '', '')).rejects.toBe('kv write failed');
    const st = await app.GetState();
    expect(st.tasks.map((t) => t.name)).toEqual(['Keep']);

    // Memory matches storage again once writes recover.
    kv.failWrites(false);
    const { app: reopened } = newTestApp(kv);
    expect((await reopened.GetState()).tasks.map((t) => t.name)).toEqual(['Keep']);
  });

  it('rejects a negative daily goal', async () => {
    const { app } = newTestApp();
    await expect(app.SetDailyGoalMinutes(-1)).rejects.toBe('daily goal cannot be negative');
  });

  it('preserves an unreadable blob under a backup and starts fresh', async () => {
    const kv = memKV('not json{');
    const { app } = newTestApp(kv);
    const st = await app.GetState();
    expect(st.tasks).toEqual([]);
    expect(kv.backups).toEqual(['not json{']);
  });

  it('treats a newer schema version as unreadable rather than guessing', async () => {
    const blob = JSON.stringify({
      version: 99,
      state: { subjects: [], tasks: [], settings: { dailyGoalMinutes: 60 } },
      focusSessions: [],
    });
    const kv = memKV(blob);
    const { app } = newTestApp(kv);
    expect((await app.GetState()).settings.dailyGoalMinutes).toBe(120);
    expect(kv.backups).toEqual([blob]);
  });
});
