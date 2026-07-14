// Ports of TestGradeSession (all seven subtests, same names) and
// TestGradeSessionBadDateRollsBack from app_test.go — the SM-2-lite spec.
import { describe, expect, it } from 'vitest';
import { day, memKV, newTestApp, sessionDates, countByDate, TEST_CLOCK } from './testUtil';

describe('GradeSession', () => {
  it('good re-anchors remaining gaps to today', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('Maths', '', '')).tasks;
    const id = tasks[0].id;
    await app.AddSession(id, day(-4));
    await app.AddSession(id, day(-1));
    tasks = (await app.AddSession(id, day(2))).tasks;
    const gradedID = tasks[0].sessions[0].id; // the day(-4) session

    tasks = (await app.GradeSession(id, gradedID, 'good')).tasks;
    // Gaps from day(-4) were 3 and 6 days; ×1.0 re-anchored to today.
    expect(sessionDates(tasks[0])).toEqual([day(-4), day(3), day(6)]);
    expect(tasks[0].sessions[0].done).toBe(true);
    expect(tasks[0].sessions[0].completedAt).toBeDefined();
  });

  it('again forces tomorrow and compresses', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('Maths', '', '')).tasks;
    const id = tasks[0].id;
    await app.AddSession(id, day(0));
    await app.AddSession(id, day(2));
    tasks = (await app.AddSession(id, day(6))).tasks;
    const gradedID = tasks[0].sessions[0].id;

    tasks = (await app.GradeSession(id, gradedID, 'again')).tasks;
    expect(sessionDates(tasks[0])).toEqual([day(0), day(1), day(3)]);
  });

  it('again with no future sessions schedules tomorrow', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('Maths', '', '')).tasks;
    const id = tasks[0].id;
    tasks = (await app.AddSession(id, day(0))).tasks;
    const gradedID = tasks[0].sessions[0].id;

    tasks = (await app.GradeSession(id, gradedID, 'again')).tasks;
    expect(sessionDates(tasks[0])).toEqual([day(0), day(1)]);
  });

  it('again schedules tomorrow even if tomorrow is already done', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('Maths', '', '')).tasks;
    const id = tasks[0].id;
    await app.AddSession(id, day(0));
    tasks = (await app.AddSession(id, day(1))).tasks;
    await app.ToggleSession(id, tasks[0].sessions[1].id); // day(1) reviewed early, done
    const gradedID = tasks[0].sessions[0].id;

    tasks = (await app.GradeSession(id, gradedID, 'again')).tasks;
    // A done session on tomorrow must not swallow the forced re-review:
    // a new pending session is scheduled for tomorrow, coexisting with it.
    expect(countByDate(tasks[0], day(1))).toEqual({ done: 1, pending: 1 });
  });

  it('again forces tomorrow past a done session, not after it', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('Maths', '', '')).tasks;
    const id = tasks[0].id;
    await app.AddSession(id, day(0));
    await app.AddSession(id, day(1));
    tasks = (await app.AddSession(id, day(5))).tasks;
    await app.ToggleSession(id, tasks[0].sessions[1].id); // day(1) done
    const gradedID = tasks[0].sessions[0].id;

    tasks = (await app.GradeSession(id, gradedID, 'again')).tasks;
    // The remaining future review (was day(5)) re-anchors to tomorrow; the
    // done day(1) no longer pushes it out to day(2).
    expect(countByDate(tasks[0], day(1))).toEqual({ done: 1, pending: 1 });
    expect(countByDate(tasks[0], day(2)).pending).toBe(0);
  });

  it('compressed dates stay strictly increasing', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('Maths', '', '')).tasks;
    const id = tasks[0].id;
    await app.AddSession(id, day(0));
    await app.AddSession(id, day(1));
    tasks = (await app.AddSession(id, day(2))).tasks;
    const gradedID = tasks[0].sessions[0].id;

    // hard: round(1×0.7)=1 and round(2×0.7)=1 would collide; the second
    // bumps to keep increasing.
    tasks = (await app.GradeSession(id, gradedID, 'hard')).tasks;
    expect(sessionDates(tasks[0])).toEqual([day(0), day(1), day(2)]);
  });

  it('rejects unknown grades and done sessions', async () => {
    const { app } = newTestApp();
    let tasks = (await app.AddTask('Maths', '', '')).tasks;
    const id = tasks[0].id;
    tasks = (await app.AddSession(id, day(0))).tasks;
    const sid = tasks[0].sessions[0].id;

    await expect(app.GradeSession(id, sid, 'amazing')).rejects.toBe(
      'grade must be one of: again, hard, good, easy'
    );
    await app.ToggleSession(id, sid);
    await expect(app.GradeSession(id, sid, 'good')).rejects.toBe('session is already done');
  });
});

// A session date that fails to parse (possible via an imported backup) must
// error out without marking the session done — GradeSession validates before
// mutating, and mutate() restores the backup on errors.
it('a bad stored date rolls back without half-grading', async () => {
  const kv = memKV(
    JSON.stringify({
      version: 1,
      state: {
        subjects: [],
        tasks: [
          {
            id: 'task1',
            name: 'Maths',
            description: '',
            color: 'blue',
            subjectId: '',
            tags: [],
            archived: false,
            adaptive: true,
            order: 0,
            createdAt: TEST_CLOCK.toISOString(),
            sessions: [{ id: 'bad', date: 'garbage', done: false }],
          },
        ],
        settings: { dailyGoalMinutes: 120 },
      },
      focusSessions: [],
    })
  );
  const { app } = newTestApp(kv);

  await expect(app.GradeSession('task1', 'bad', 'good')).rejects.toBe(
    'session has an invalid date'
  );
  const st = await app.GetState();
  expect(st.tasks[0].sessions[0].done).toBe(false);
  expect(st.tasks[0].sessions[0].completedAt).toBeUndefined();
});
