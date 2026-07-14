// Ports of ical_test.go — the RFC 5545 output spec, including the UTC DTSTAMP
// conversion and the 75-octet folding limit.
import { describe, expect, it } from 'vitest';
import { buildICS, ICAL_PROD_ID } from './ical';
import type { Session, Task } from './models';

// Go's icalNow is 12:00 in UTC+2, i.e. 10:00Z — buildICS must stamp in UTC.
const ICAL_NOW = new Date(Date.UTC(2026, 5, 12, 10, 0, 0));

function mkSession(id: string, date: string, done = false): Session {
  return { id, date, done };
}

function mkTask(partial: Partial<Task> & { name: string }): Task {
  return {
    id: partial.name,
    description: '',
    color: '',
    subjectId: '',
    tags: [],
    archived: false,
    adaptive: false,
    order: 0,
    createdAt: new Date(0).toISOString(),
    sessions: [],
    ...partial,
  };
}

// lines splits an ICS document into its logical CRLF-separated lines, asserting
// CRLF endings were used throughout.
function lines(ics: string): string[] {
  expect(ics.replace(/\r\n/g, ''), 'found a bare LF not part of a CRLF pair').not.toContain('\n');
  return ics.replace(/\r\n$/, '').split('\r\n');
}

const countLine = (ls: string[], want: string) => ls.filter((l) => l === want).length;
const hasLine = (ls: string[], want: string) => countLine(ls, want) > 0;

describe('buildICS', () => {
  it('emits one all-day VEVENT per pending session of active tasks', () => {
    const tasks = [
      mkTask({
        name: 'Linear Algebra',
        description: 'Eigenvalues',
        sessions: [
          mkSession('s1', '2026-06-14'),
          mkSession('s2', '2026-06-20', true), // done: excluded
        ],
      }),
      mkTask({
        name: 'Archived',
        archived: true,
        sessions: [mkSession('s3', '2026-06-15')], // archived: excluded
      }),
      mkTask({
        name: 'History',
        sessions: [mkSession('s4', '2026-06-13')], // no description
      }),
    ];

    const ics = buildICS(tasks, ICAL_NOW);
    const ls = lines(ics);

    // Calendar wrapper.
    expect(ls[0]).toBe('BEGIN:VCALENDAR');
    expect(ls[ls.length - 1]).toBe('END:VCALENDAR');
    for (const want of [
      'VERSION:2.0',
      'PRODID:' + ICAL_PROD_ID,
      'CALSCALE:GREGORIAN',
      'X-WR-CALNAME:Study Planner',
    ]) {
      expect(hasLine(ls, want), want).toBe(true);
    }

    // Exactly two events: the pending sessions s1 and s4.
    expect(countLine(ls, 'BEGIN:VEVENT')).toBe(2);
    for (const id of ['s1', 's4']) expect(hasLine(ls, `UID:${id}@study-planner`), id).toBe(true);
    for (const id of ['s2', 's3']) expect(hasLine(ls, `UID:${id}@study-planner`), id).toBe(false);

    // DTSTAMP is the supplied instant in UTC.
    expect(hasLine(ls, 'DTSTAMP:20260612T100000Z')).toBe(true);

    // All-day event: DTEND is the day after DTSTART.
    expect(hasLine(ls, 'DTSTART;VALUE=DATE:20260614')).toBe(true);
    expect(hasLine(ls, 'DTEND;VALUE=DATE:20260615')).toBe(true);
    expect(hasLine(ls, 'SUMMARY:Study: Linear Algebra')).toBe(true);
    expect(hasLine(ls, 'DESCRIPTION:Eigenvalues')).toBe(true);
  });

  it('sorts events by date', () => {
    const tasks = [
      mkTask({
        name: 'T',
        sessions: [mkSession('late', '2026-07-01'), mkSession('early', '2026-06-01')],
      }),
    ];
    const order = lines(buildICS(tasks, ICAL_NOW)).filter((l) => l.startsWith('UID:'));
    expect(order).toHaveLength(2);
    expect(order[0]).toBe('UID:early@study-planner');
  });

  it('escapes TEXT specials and newlines', () => {
    const tasks = [
      mkTask({
        name: 'Maths, Physics; Chemistry\\Bio',
        description: 'line one\nline two',
        sessions: [mkSession('s1', '2026-06-14')],
      }),
    ];
    const ics = buildICS(tasks, ICAL_NOW);
    expect(ics).toContain('SUMMARY:Study: Maths\\, Physics\\; Chemistry\\\\Bio');
    expect(ics).toContain('DESCRIPTION:line one\\nline two');
  });

  it('folds long lines to 75 octets', () => {
    const encoder = new TextEncoder();
    const tasks = [mkTask({ name: 'A'.repeat(200), sessions: [mkSession('s1', '2026-06-14')] })];
    for (const l of lines(buildICS(tasks, ICAL_NOW))) {
      expect(encoder.encode(l).length, l).toBeLessThanOrEqual(75);
    }
  });

  it('skips sessions with malformed dates', () => {
    const tasks = [
      mkTask({
        name: 'T',
        sessions: [mkSession('bad', 'not-a-date'), mkSession('ok', '2026-06-14')],
      }),
    ];
    const ls = lines(buildICS(tasks, ICAL_NOW));
    expect(countLine(ls, 'BEGIN:VEVENT')).toBe(1);
    expect(hasLine(ls, 'UID:ok@study-planner')).toBe(true);
  });

  it('renders an empty calendar with the wrapper intact', () => {
    const ls = lines(buildICS([], ICAL_NOW));
    expect(countLine(ls, 'BEGIN:VEVENT')).toBe(0);
    expect(ls[0]).toBe('BEGIN:VCALENDAR');
    expect(ls[ls.length - 1]).toBe('END:VCALENDAR');
  });
});
