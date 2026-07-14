// Ports of the session-scheduling tests in app_test.go.
import { describe, expect, it } from 'vitest';
import { countByDate, day, newTestApp, sessionDates } from './testUtil';

describe('AddSpacedSessions', () => {
  it('merges into existing sessions and replaces on demand', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('Maths', '', '')).tasks;
    const id = tasks[0].id;

    // Seed one manual session and mark it done.
    tasks = (await app.AddSession(id, '2026-06-01')).tasks;
    const doneID = tasks[0].sessions[0].id;
    await app.ToggleSession(id, doneID);

    // Merge: existing date kept (same id, still done), new dates added around it.
    tasks = (await app.AddSpacedSessions(id, '2026-06-01', [0, 2], false)).tasks;
    expect(sessionDates(tasks[0])).toEqual(['2026-06-01', '2026-06-03']);
    expect(tasks[0].sessions[0].id).toBe(doneID);
    expect(tasks[0].sessions[0].done).toBe(true);

    // Replace: everything cleared, only the new schedule remains.
    tasks = (await app.AddSpacedSessions(id, '2026-06-10', [0, 1], true)).tasks;
    expect(sessionDates(tasks[0])).toEqual(['2026-06-10', '2026-06-11']);
    for (const s of tasks[0].sessions) expect(s.done).toBe(false);
  });

  it('rejects a malformed start date', async () => {
    const { app } = newTestApp();
    const id = (await app.AddTask('Maths', '', '')).tasks[0].id;
    await expect(app.AddSpacedSessions(id, '2026-6-1', [], false)).rejects.toBe(
      'start date must be in YYYY-MM-DD format'
    );
  });
});

describe('AddSession', () => {
  it('rejects malformed and impossible dates', async () => {
    const { app } = newTestApp();
    const id = (await app.AddTask('Maths', '', '')).tasks[0].id;
    for (const bad of ['2026-02-30', '12-06-2026', 'garbage']) {
      await expect(app.AddSession(id, bad)).rejects.toBe('date must be in YYYY-MM-DD format');
    }
  });
});

describe('ToggleSession', () => {
  it('stamps completedAt on done and clears it on undo', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('Biology', '', '')).tasks;
    tasks = (await app.AddSession(tasks[0].id, day(0))).tasks;
    const id = tasks[0].id;
    const sid = tasks[0].sessions[0].id;

    tasks = (await app.ToggleSession(id, sid)).tasks;
    expect(tasks[0].sessions[0].done).toBe(true);
    expect(tasks[0].sessions[0].completedAt).toBeDefined();

    tasks = (await app.ToggleSession(id, sid)).tasks;
    expect(tasks[0].sessions[0].done).toBe(false);
    expect(tasks[0].sessions[0].completedAt).toBeUndefined();
  });
});

describe('RescheduleSession', () => {
  it('merges onto an occupied date and moves to a free one', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('History', '', '')).tasks;
    const id = tasks[0].id;
    await app.AddSession(id, day(-3));
    tasks = (await app.AddSession(id, day(0))).tasks;
    const overdueID = tasks[0].sessions[0].id;

    // Moving onto an occupied date drops the moved session.
    tasks = (await app.RescheduleSession(id, overdueID, day(0))).tasks;
    expect(sessionDates(tasks[0])).toEqual([day(0)]);

    // Moving to a free date just changes the date.
    const sid = tasks[0].sessions[0].id;
    tasks = (await app.RescheduleSession(id, sid, day(2))).tasks;
    expect(sessionDates(tasks[0])).toEqual([day(2)]);
  });

  it('coexists with a done session on the target day', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('History', '', '')).tasks;
    const id = tasks[0].id;
    await app.AddSession(id, day(-2)); // overdue, pending
    tasks = (await app.AddSession(id, day(0))).tasks;
    const overdueID = tasks[0].sessions[0].id;
    const todayID = tasks[0].sessions[1].id;
    await app.ToggleSession(id, todayID); // mark today's session done

    // Moving the overdue review onto today (which has only a DONE session)
    // must NOT drop it — it coexists with the completed one.
    tasks = (await app.RescheduleSession(id, overdueID, day(0))).tasks;
    expect(countByDate(tasks[0], day(0))).toEqual({ done: 1, pending: 1 });
  });
});

describe('RescheduleOverdueSessions', () => {
  it('moves overdue reviews to today with at most one pending per task', async () => {
    const { app } = newTestApp();

    let tasks = (await app.AddTask('Catching up', '', '')).tasks;
    const lone = tasks[0].id;
    await app.AddSession(lone, day(-2)); // only overdue → moves to today

    tasks = (await app.AddTask('Covered', '', '')).tasks;
    const covered = tasks[1].id;
    await app.AddSession(covered, day(-3)); // overdue but today exists → dropped
    await app.AddSession(covered, day(0));

    tasks = (await app.AddTask('Shelved', '', '')).tasks;
    const shelved = tasks[2].id;
    await app.AddSession(shelved, day(-5));
    await app.SetTaskArchived(shelved, true); // archived → untouched

    // Studied-today task: an overdue pending session plus a DONE session today.
    // The done one doesn't cover the overdue review, so it still moves to today.
    tasks = (await app.AddTask('Studied today', '', '')).tasks;
    const studied = tasks[3].id;
    await app.AddSession(studied, day(-1));
    tasks = (await app.AddSession(studied, day(0))).tasks;
    await app.ToggleSession(studied, tasks[3].sessions[1].id); // day(0) done

    tasks = (await app.RescheduleOverdueSessions()).tasks;
    const byID = new Map(tasks.map((t) => [t.id, t]));
    expect(sessionDates(byID.get(lone)!)).toEqual([day(0)]);
    expect(sessionDates(byID.get(covered)!)).toEqual([day(0)]);
    expect(sessionDates(byID.get(shelved)!)).toEqual([day(-5)]);
    expect(countByDate(byID.get(studied)!, day(0))).toEqual({ done: 1, pending: 1 });
  });
});

describe('DeleteSession', () => {
  it('rejects an unknown session id', async () => {
    const { app } = newTestApp();
    const tasks = (await app.AddTask('Chemistry', '', '')).tasks;
    await expect(app.DeleteSession(tasks[0].id, 'nope')).rejects.toBe('session not found');
  });
});
