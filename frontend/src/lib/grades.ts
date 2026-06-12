import type { ModalAction } from './ConfirmModal.svelte';

// Grade choices for adaptive topics, mirroring the backend's gradeFactors.
// Shown wherever a session of an adaptive topic is checked off. Colours use the
// theme CSS variables (not literal hexes) so the dots track the active theme.
export const GRADE_ACTIONS: ModalAction[] = [
  { value: 'easy', label: 'Easy', color: 'var(--green)', detail: 'Effortless recall — remaining reviews stretch out (×1.4).' },
  { value: 'good', label: 'Good', color: 'var(--accent)', detail: 'Recalled with some effort — spacing kept as planned, from today.' },
  { value: 'hard', label: 'Hard', color: 'var(--amber)', detail: 'Struggled — remaining reviews come sooner (×0.7).' },
  { value: 'again', label: 'Again', color: 'var(--red)', detail: 'Could not recall — review again tomorrow, rest compressed.' },
  { value: 'cancel', label: 'Cancel', kind: 'ghost' },
];

export const GRADE_VALUES = ['again', 'hard', 'good', 'easy'];
