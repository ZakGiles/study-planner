// Backup export/import: the export shape, a full round-trip between two
// stores, and the guarantee that a rejected import touches nothing.
import { describe, expect, it } from 'vitest';
import { buildBackupJSON } from './backup';
import { LocalStore } from './store';
import { day, newTestApp, TEST_CLOCK } from './testUtil';

describe('backup', () => {
  it('exports the whole store with version and stamp', async () => {
    const { app, kv } = newTestApp();
    await app.AddTask('Maths', '', '');
    await app.RecordFocusSession('', 600);

    // A fresh LocalStore over the same KV sees exactly what was persisted.
    const parsed = JSON.parse(buildBackupJSON(new LocalStore(kv), TEST_CLOCK));
    expect(parsed.version).toBe(1);
    expect(parsed.exportedAt).toBe(TEST_CLOCK.toISOString());
    expect(parsed.state.tasks).toHaveLength(1);
    expect(parsed.state.tasks[0].name).toBe('Maths');
    expect(parsed.focusSessions).toHaveLength(1);
  });

  it('imports a backup produced by another store', async () => {
    const { app: source, kv: sourceKV } = newTestApp();
    await source.AddSubject('Mathematics');
    const st = await source.AddTask('Maths', '', '');
    await source.AddSession(st.tasks[0].id, day(1));
    await source.RecordFocusSession(st.tasks[0].id, 900);
    const backup = sourceKV.raw()!; // the storage blob doubles as a backup file

    const { app: dest, kv: destKV } = newTestApp();
    const res = await dest.ImportBackup(backup);
    expect(res.state.tasks.map((t) => t.name)).toEqual(['Maths']);
    expect(res.state.subjects.map((s) => s.name)).toEqual(['Mathematics']);
    expect(res.focusSessions).toHaveLength(1);

    // …and it persisted: a reload over the same storage sees the import.
    const { app: reopened } = newTestApp(destKV);
    expect((await reopened.GetState()).tasks.map((t) => t.name)).toEqual(['Maths']);
    expect(await reopened.GetFocusSessions()).toHaveLength(1);
  });

  it('rejects garbage without touching the store', async () => {
    const { app, kv } = newTestApp();
    await app.AddTask('Keep', '', '');
    const before = kv.raw();

    for (const garbage of ['not json', '{"nope":1}', '{"state":{"tasks":"x","subjects":[]}}']) {
      await expect(app.ImportBackup(garbage)).rejects.toBe(
        "that file doesn't look like a Study Planner backup"
      );
    }
    expect(kv.raw()).toBe(before);
    expect((await app.GetState()).tasks.map((t) => t.name)).toEqual(['Keep']);
  });
});
