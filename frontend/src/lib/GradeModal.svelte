<script lang="ts">
  // The adaptive-grading modal, shared by every place a session of an adaptive
  // topic can be checked off (topic card, agenda, calendar). Wraps ConfirmModal
  // with the grade actions baked in and emits a clean `grade` / `cancel`.
  import { createEventDispatcher } from 'svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import { GRADE_ACTIONS, GRADE_VALUES } from './grades';

  export let topicName: string;

  const dispatch = createEventDispatcher<{ grade: string; cancel: void }>();

  function onChoose(e: CustomEvent<string>) {
    if (GRADE_VALUES.includes(e.detail)) dispatch('grade', e.detail);
    else dispatch('cancel');
  }
</script>

<ConfirmModal
  title="How did “{topicName}” go?"
  message="Your grade re-spaces the remaining reviews, starting from today."
  actions={GRADE_ACTIONS}
  on:choose={onChoose}
/>
