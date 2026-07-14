// Shared helpers for the local-backend tests, mirroring app_test.go: a frozen
// clock (local noon, well clear of any midnight edge), a day(n) helper anchored
// to it, and an in-memory KV so tests never touch real localStorage and can
// inject write failures.
import type { main } from '../../../../wailsjs/go/models';
import { toISO } from '../../dates';
import { createLocalBackend, type LocalBackend } from './app';
import type { KV } from './store';

// Mirrors Go's testClock: 2026-06-12 12:00 local.
export const TEST_CLOCK = new Date(2026, 5, 12, 12, 0, 0);

// day returns TEST_CLOCK + n days as YYYY-MM-DD, matching the app's local-date
// convention (Go's day helper).
export function day(n: number): string {
  return toISO(new Date(2026, 5, 12 + n));
}

export interface MemKV extends KV {
  raw(): string | null;
  failWrites(on: boolean): void;
  backups: string[];
}

export function memKV(seed: string | null = null): MemKV {
  let value = seed;
  let fail = false;
  const backups: string[] = [];
  return {
    get: () => value,
    set: (v) => {
      if (fail) throw new Error('kv write failed');
      value = v;
    },
    backup: (v) => {
      backups.push(v);
    },
    raw: () => value,
    failWrites: (on) => {
      fail = on;
    },
    backups,
  };
}

export function newTestApp(kv: MemKV = memKV()): { app: LocalBackend; kv: MemKV } {
  return { app: createLocalBackend(kv, () => TEST_CLOCK), kv };
}

export function sessionDates(t: main.Task): string[] {
  return t.sessions.map((s) => s.date);
}

// countByDate returns how many sessions a task has on date, split done/pending.
export function countByDate(t: main.Task, date: string): { done: number; pending: number } {
  let done = 0;
  let pending = 0;
  for (const s of t.sessions) {
    if (s.date !== date) continue;
    if (s.done) done++;
    else pending++;
  }
  return { done, pending };
}
