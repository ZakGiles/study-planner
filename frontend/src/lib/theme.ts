// Theme (dark/light) as a shared store driven from the Settings tab. The
// preference is a pure UI choice, persisted locally in localStorage rather than
// in the data store.
import { writable } from 'svelte/store';

export type Theme = 'dark' | 'light';

const initial: Theme = localStorage.getItem('theme') === 'light' ? 'light' : 'dark';

// applyTheme re-points every colour variable. Without suspending transitions for
// the swap, every element with a hover transition would fade, animating the
// whole UI — so disable transitions for the one frame the change lands in, then
// restore them so hovers still animate.
function applyTheme(t: Theme) {
  const root = document.documentElement;
  root.classList.add('no-transition');
  root.dataset.theme = t;
  localStorage.setItem('theme', t);
  void root.offsetWidth; // force a reflow so the swap paints instantly
  requestAnimationFrame(() => root.classList.remove('no-transition'));
}

export const theme = writable<Theme>(initial);
// Apply on every change (including the initial value on first import).
theme.subscribe(applyTheme);
