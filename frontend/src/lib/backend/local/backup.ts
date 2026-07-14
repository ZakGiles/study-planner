// Whole-store backup for the web app, where the only copy of the data lives in
// the browser's localStorage — an explicit export/import is the user's
// insurance against cleared site data (and a migration path between machines).
// The file is the storage blob plus an exportedAt stamp, so a raw storage blob
// also imports cleanly.
import type { LocalStore } from './store';
import { normalizeFocus, normalizeState, SCHEMA_VERSION } from './store';

export function buildBackupJSON(store: LocalStore, now: Date): string {
  return JSON.stringify(
    {
      version: SCHEMA_VERSION,
      exportedAt: now.toISOString(),
      state: store.state,
      focusSessions: store.focus,
    },
    null,
    2
  );
}

// applyBackup validates and normalizes a backup file's text, then replaces the
// store's contents and persists. Nothing is touched unless the payload
// validates, and a failed persist rolls the memory swap back — an import never
// half-applies.
export function applyBackup(store: LocalStore, text: string): void {
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(text);
  } catch {
    parsed = null;
  }
  const blob = parsed && typeof parsed === 'object' ? (parsed as Record<string, unknown>) : null;
  const versionOK = blob !== null && (blob.version === undefined || blob.version === SCHEMA_VERSION);
  const state = versionOK ? normalizeState(blob.state) : null;
  if (!state) throw new Error("that file doesn't look like a Study Planner backup");

  const prevState = store.state;
  const prevFocus = store.focus;
  store.state = state;
  store.focus = normalizeFocus(blob!.focusSessions);
  try {
    store.save();
  } catch (e) {
    store.state = prevState;
    store.focus = prevFocus;
    throw e;
  }
}
