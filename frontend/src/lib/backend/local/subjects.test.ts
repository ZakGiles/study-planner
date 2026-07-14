// Ports of TestSubjects, TestReorderSubjects and TestSnapshotIsCopy from
// app_test.go.
import { describe, expect, it } from 'vitest';
import { newTestApp } from './testUtil';

describe('subjects', () => {
  it('covers the lifecycle: create, assign, ungroup-on-delete, validation', async () => {
    const { app } = newTestApp();

    // A new subject shows up in State.subjects.
    let st = await app.AddSubject('Mathematics');
    expect(st.subjects).toHaveLength(1);
    expect(st.subjects[0].name).toBe('Mathematics');
    const subjID = st.subjects[0].id;

    // A task can be created directly into a subject.
    st = await app.AddTask('Linear Algebra', '', subjID);
    expect(st.tasks[0].subjectId).toBe(subjID);
    const taskID = st.tasks[0].id;

    // An ungrouped task can be moved into the subject later.
    st = await app.AddTask('Calculus', '', '');
    const calcID = st.tasks[1].id;
    st = await app.SetTaskSubject(calcID, subjID);
    expect(st.tasks.find((t) => t.id === calcID)!.subjectId).toBe(subjID);

    // Unknown-subject assignment is rejected, for both add and move.
    await expect(app.AddTask('Orphan', '', 'nope')).rejects.toBe('subject not found');
    await expect(app.SetTaskSubject(taskID, 'nope')).rejects.toBe('subject not found');

    // Deleting the subject ungroups its tasks but keeps them.
    st = await app.DeleteSubject(subjID);
    expect(st.subjects).toHaveLength(0);
    expect(st.tasks).toHaveLength(2);
    for (const t of st.tasks) expect(t.subjectId).toBe('');
  });

  it('reorders subjects, unlisted ones keeping relative order', async () => {
    const { app } = newTestApp();
    await app.AddSubject('A');
    await app.AddSubject('B');
    let st = await app.AddSubject('C');
    const byName: Record<string, string> = {};
    for (const s of st.subjects) byName[s.name] = s.id;

    st = await app.ReorderSubjects([byName['C'], byName['A'], byName['B']]);
    expect(st.subjects.map((s) => s.name)).toEqual(['C', 'A', 'B']);
    st.subjects.forEach((s, i) => expect(s.order).toBe(i));
  });
});

it('snapshots are copies: mutating a returned State never leaks into the store', async () => {
  const { app } = newTestApp();
  let st = await app.AddTask('Physics', '', '');
  await app.AddSession(st.tasks[0].id, '2026-06-05');

  const got = await app.GetState();
  got.tasks[0].name = 'mutated';
  got.tasks[0].sessions[0].done = true;
  got.tasks[0].tags.push('sneaky');

  const fresh = await app.GetState();
  expect(fresh.tasks[0].name).toBe('Physics');
  expect(fresh.tasks[0].sessions[0].done).toBe(false);
  expect(fresh.tasks[0].tags).toEqual([]);
});
