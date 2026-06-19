import type { ModalAction } from './ConfirmModal.svelte';

// Grade choices for adaptive tasks, mirroring the backend's gradeFactors.
// Shown wherever a session of an adaptive task is checked off. Colours use the
// theme CSS variables (not literal hexes) so the dots track the active theme.
export const GRADE_ACTIONS: ModalAction[] = [
  { value: 'easy', label: 'Easy', color: 'var(--green)', detail: 'Effortless recall — remaining reviews stretch out (×1.4).' },
  { value: 'good', label: 'Good', color: 'var(--accent)', detail: 'Recalled with some effort — spacing kept as planned, from today.' },
  { value: 'hard', label: 'Hard', color: 'var(--amber)', detail: 'Struggled — remaining reviews come sooner (×0.7).' },
  { value: 'again', label: 'Again', color: 'var(--red)', detail: 'Could not recall — review again tomorrow, rest compressed.' },
  { value: 'cancel', label: 'Cancel', kind: 'ghost' },
];

// Derived from GRADE_ACTIONS so the two can never drift: everything except the
// dismiss action is a grade the backend accepts.
export const GRADE_VALUES = GRADE_ACTIONS.filter((a) => a.value !== 'cancel').map((a) => a.value);
