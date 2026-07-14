// Ports of TestRecordFocusSession and TestFocusSurvivesTaskMutationAndReload
// from app_test.go — a "reload" here is a second backend over the same KV.
import { describe, expect, it } from 'vitest';
import { newTestApp, TEST_CLOCK } from './testUtil';

describe('RecordFocusSession', () => {
  it('logs blocks against tasks and general focus, rejecting bad input', async () => {
    const { app } = newTestApp();
    const tasks = (await app.AddTask('Maths', '', '')).tasks;
    const taskID = tasks[0].id;

    // A focus block against a task is logged and stamped with the clock.
    let focus = await app.RecordFocusSession(taskID, 1500);
    expect(focus).toHaveLength(1);
    expect(focus[0].taskId).toBe(taskID);
    expect(focus[0].durationSec).toBe(1500);
    expect(new Date(focus[0].completedAt).getTime()).toBe(TEST_CLOCK.getTime());

    // General focus ('' task) is allowed.
    focus = await app.RecordFocusSession('', 600);
    expect(focus).toHaveLength(2);

    // Non-positive duration and unknown task are rejected.
    await expect(app.RecordFocusSession(taskID, 0)).rejects.toBe('focus duration must be positive');
    await expect(app.RecordFocusSession('nope', 60)).rejects.toBe('task not found');
  });

  it('survives task mutations and a reload', async () => {
    const { app, kv } = newTestApp();
    const tasks = (await app.AddTask('Maths', '', '')).tasks;
    const taskID = tasks[0].id;
    await app.RecordFocusSession(taskID, 1500);

    // A task mutation rewrites the whole blob; the focus log must not be wiped
    // by that. Deleting the task leaves the record intact (its taskId dangles).
    await app.DeleteTask(taskID);
    const focus = await app.GetFocusSessions();
    expect(focus).toHaveLength(1);
    expect(focus[0].taskId).toBe(taskID);

    // And it persists across a reload.
    const { app: reopened } = newTestApp(kv);
    const focus2 = await reopened.GetFocusSessions();
    expect(focus2).toHaveLength(1);
    expect(focus2[0].durationSec).toBe(1500);
  });
});
