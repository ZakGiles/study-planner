// Alert sounds for the focus timer, shared between the Focus tab (playback when a
// block ends) and the Settings tab (upload/test/reset). Uploaded files are kept
// as Blobs in IndexedDB (no practical size limit, unlike localStorage) and played
// from object URLs; with none set a synthesised chime plays instead.
import { writable } from 'svelte/store';

export type SoundKind = 'study' | 'break';
type SoundRecord = { blob: Blob; name: string };
export type SoundInfo = { url: string; name: string };

const empty = (): Record<SoundKind, SoundInfo> => ({
  study: { url: '', name: '' },
  break: { url: '', name: '' },
});

// sounds exposes the current custom sound per kind for the UI; `current` is the
// synchronous mirror playback reads.
export const sounds = writable<Record<SoundKind, SoundInfo>>(empty());
let current = empty();

// Point a kind at a new object URL, revoking the one it replaces.
function setSoundUrl(kind: SoundKind, url: string, name: string) {
  if (current[kind].url) URL.revokeObjectURL(current[kind].url);
  current = { ...current, [kind]: { url, name } };
  sounds.set(current);
}

// ---- IndexedDB ----
const SOUND_DB = 'study-planner-sounds';
const SOUND_STORE = 'sounds';
function openSoundDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(SOUND_DB, 1);
    req.onupgradeneeded = () => req.result.createObjectStore(SOUND_STORE);
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}
async function soundDBPut(key: SoundKind, value: SoundRecord) {
  const db = await openSoundDB();
  try {
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(SOUND_STORE, 'readwrite');
      tx.objectStore(SOUND_STORE).put(value, key);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
  } finally {
    db.close();
  }
}
async function soundDBGet(key: SoundKind): Promise<SoundRecord | undefined> {
  const db = await openSoundDB();
  try {
    return await new Promise<SoundRecord | undefined>((resolve, reject) => {
      const req = db.transaction(SOUND_STORE, 'readonly').objectStore(SOUND_STORE).get(key);
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
  } finally {
    db.close();
  }
}
async function soundDBDelete(key: SoundKind) {
  const db = await openSoundDB();
  try {
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(SOUND_STORE, 'readwrite');
      tx.objectStore(SOUND_STORE).delete(key);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
  } finally {
    db.close();
  }
}

// ---- Playback ----
let audioCtx: AudioContext | undefined;
// Lazily create/resume the audio context (browsers require a user gesture to
// start it — callers invoke this on a gesture so a chime can fire later).
export function ensureAudio(): AudioContext | undefined {
  try {
    if (!audioCtx) audioCtx = new (window.AudioContext || (window as any).webkitAudioContext)();
    if (audioCtx.state === 'suspended') void audioCtx.resume();
  } catch {
    return undefined; // audio unavailable; play calls become no-ops
  }
  return audioCtx;
}
// A short two-note chime so there's always a sound with nothing uploaded:
// study rises, break falls.
function playChime(kind: SoundKind) {
  const ctx = ensureAudio();
  if (!ctx) return;
  const notes = kind === 'study' ? [660, 990] : [880, 587];
  notes.forEach((freq, i) => {
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = 'sine';
    osc.frequency.value = freq;
    const t = ctx.currentTime + i * 0.18;
    gain.gain.setValueAtTime(0.0001, t);
    gain.gain.exponentialRampToValueAtTime(0.32, t + 0.02);
    gain.gain.exponentialRampToValueAtTime(0.0001, t + 0.38);
    osc.connect(gain).connect(ctx.destination);
    osc.start(t);
    osc.stop(t + 0.42);
  });
}
export function playSound(kind: SoundKind) {
  const url = current[kind].url;
  if (url) {
    new Audio(url).play().catch(() => playChime(kind)); // fall back if blocked
  } else {
    playChime(kind);
  }
}

// ---- Management ----
// uploadSound persists a file and points the kind at it; it throws on failure so
// the caller can surface the error.
export async function uploadSound(kind: SoundKind, file: File): Promise<void> {
  await soundDBPut(kind, { blob: file, name: file.name });
  setSoundUrl(kind, URL.createObjectURL(file), file.name);
}
export async function clearSound(kind: SoundKind): Promise<void> {
  try {
    await soundDBDelete(kind);
  } catch {
    /* ignore */
  }
  setSoundUrl(kind, '', '');
}

// loadSounds hydrates the stores from IndexedDB once at startup, migrating any
// sound saved by the earlier localStorage version. Idempotent.
let loaded = false;
export async function loadSounds(): Promise<void> {
  if (loaded) return;
  loaded = true;
  for (const kind of ['study', 'break'] as const) {
    try {
      const legacy = localStorage.getItem(`focusSound:${kind}`);
      if (legacy) {
        const name = localStorage.getItem(`focusSoundName:${kind}`) ?? 'sound';
        try {
          await soundDBPut(kind, { blob: await (await fetch(legacy)).blob(), name });
        } catch {
          /* ignore */
        }
        localStorage.removeItem(`focusSound:${kind}`);
        localStorage.removeItem(`focusSoundName:${kind}`);
      }
      const rec = await soundDBGet(kind);
      if (rec) setSoundUrl(kind, URL.createObjectURL(rec.blob), rec.name);
    } catch {
      /* ignore load errors */
    }
  }
}
