// The backend adapter — the single seam between the UI and its data layer.
// On desktop the Wails-generated bindings talk to the Go backend; in a plain
// browser (the GitHub Pages build) a TypeScript port of the same 27-method API
// runs against localStorage. Detection happens once at module load: the Wails
// shell injects its runtime (window.go) as a head script before any module
// script executes, so the flag is reliable in `wails dev` and packaged builds.
//
// Statically importing both sides is safe: the wailsjs functions dereference
// window.go only when called, and the local modules are side-effect-free at
// import time — on desktop they sit inert in the bundle, never constructed.
import type { main } from '../../../wailsjs/go/models';
import * as desktop from '../../../wailsjs/go/main/App.js';
import { createLocalBackend, type LocalBackend } from './local/app';

export const isDesktop = typeof window !== 'undefined' && !!(window as any).go;

// What the current host can do; components gate host-specific UI on these
// rather than sniffing window.go themselves. autoStart on desktop is further
// gated by the Go side's `available` flag (false in unbundled dev builds).
export const capabilities = {
  autoStart: isDesktop,
  backup: !isDesktop,
  notifications: !isDesktop && typeof window !== 'undefined' && 'Notification' in window,
} as const;

// On the web the local backend is constructed once at module load, which also
// loads the stored data.
const local: LocalBackend | null = isDesktop ? null : createLocalBackend();
const impl: typeof desktop = local ?? desktop;

export const {
  GetState,
  GetFocusSessions,
  AddTask,
  UpdateTask,
  SetTaskColor,
  SetTaskArchived,
  SetTaskSubject,
  ReorderTasks,
  DeleteTask,
  AddSubject,
  UpdateSubject,
  SetSubjectColor,
  ReorderSubjects,
  DeleteSubject,
  AddSession,
  AddSpacedSessions,
  DeleteSession,
  ToggleSession,
  RecordFocusSession,
  SetTaskAdaptive,
  RescheduleSession,
  RescheduleOverdueSessions,
  GradeSession,
  SetDailyGoalMinutes,
  ExportCalendar,
  GetAutoStart,
  SetAutoStart,
} = impl;

// Web-only extras (Settings gates its Backup card on capabilities.backup):
// whole-store export/import, the user's insurance against cleared site data.
export function exportBackup(): Promise<string> {
  if (!local) return Promise.reject('backups are only available in the web app');
  return local.ExportBackup();
}

export function importBackup(text: string): Promise<{ state: main.State; focusSessions: main.FocusSession[] }> {
  if (!local) return Promise.reject('backups are only available in the web app');
  return local.ImportBackup(text);
}
