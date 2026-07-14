// Ports of the pure-model tests in app_test.go (TestSpacedDates,
// TestNormalizeTags, TestPickColor, TestNormalizeOrder), plus coverage of the
// date validation that Go gets for free from time.Parse.
import { describe, expect, it } from 'vitest';
import {
  addDaysISO,
  isValidDate,
  normalizeOrder,
  normalizeTags,
  pickColor,
  spacedDates,
  type Task,
} from './models';

function mkTask(id: string, order: number, createdAt: string): Task {
  return {
    id,
    name: id,
    description: '',
    color: '',
    subjectId: '',
    tags: [],
    archived: false,
    adaptive: false,
    order,
    createdAt,
    sessions: [],
  };
}

describe('spacedDates', () => {
  it('sorts, dedupes and drops negative offsets', () => {
    expect(spacedDates('2026-06-01', [7, 0, 0, 3, -2])).toEqual([
      '2026-06-01',
      '2026-06-04',
      '2026-06-08',
    ]);
  });
});

describe('normalizeTags', () => {
  it('trims, drops empties and dedupes case-insensitively keeping first casing', () => {
    expect(normalizeTags([' Go ', 'go', '', 'GO', 'rust'])).toEqual(['Go', 'rust']);
  });

  it('caps the tag count at 12', () => {
    const long = Array.from({ length: 20 }, (_, i) => String.fromCharCode(97 + i) + '-tag');
    expect(normalizeTags(long)).toHaveLength(12);
  });

  it('caps each tag at 30 code points', () => {
    const got = normalizeTags(['abcdefghijklmnopqrstuvwxyz0123456789']);
    expect([...got[0]]).toHaveLength(30);
  });
});

describe('pickColor', () => {
  it('reproduces the round-robin cycle for sequential adds', () => {
    const used: string[] = [];
    for (const want of ['blue', 'violet', 'emerald']) {
      expect(pickColor(used)).toBe(want);
      used.push(pickColor(used));
    }
  });

  it('reuses the least-used token after a delete', () => {
    expect(pickColor(['violet', 'emerald'])).toBe('blue');
  });

  it("doesn't count reset ('') colours against any token", () => {
    expect(pickColor(['', ''])).toBe('blue');
  });
});

describe('normalizeOrder', () => {
  it('sorts by order with createdAt tiebreak and reassigns contiguously', () => {
    const base = Date.UTC(2026, 0, 1);
    const tasks = [
      mkTask('b', 5, new Date(base + 2 * 3600_000).toISOString()),
      mkTask('a', 0, new Date(base + 3600_000).toISOString()),
      mkTask('c', 0, new Date(base).toISOString()),
    ];
    normalizeOrder(tasks);
    expect(tasks.map((t) => t.id)).toEqual(['c', 'a', 'b']);
    tasks.forEach((t, i) => expect(t.order).toBe(i));
  });
});

describe('isValidDate', () => {
  it('accepts real zero-padded calendar dates', () => {
    expect(isValidDate('2026-06-12')).toBe(true);
    expect(isValidDate('2024-02-29')).toBe(true); // leap day
  });

  it('rejects what Go time.Parse rejects', () => {
    for (const bad of ['2026-02-30', '2026-6-1', '12-06-2026', 'garbage', '2026-13-01', '']) {
      expect(isValidDate(bad), bad).toBe(false);
    }
  });
});

describe('addDaysISO', () => {
  it('rolls over months and years', () => {
    expect(addDaysISO('2026-12-31', 1)).toBe('2027-01-01');
    expect(addDaysISO('2026-06-01', 30)).toBe('2026-07-01');
    expect(addDaysISO('2026-06-12', 0)).toBe('2026-06-12');
  });
});
