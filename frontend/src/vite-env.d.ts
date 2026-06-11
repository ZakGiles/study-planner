/// <reference types="svelte" />
/// <reference types="vite/client" />

// svelte-dnd-action dispatches consider/finalize as custom DOM events; the
// installed svelte-check (2.x) predates action-attribute typings, so declare
// them here for on:consider/on:finalize to typecheck.
declare namespace svelte.JSX {
  interface HTMLAttributes<T> {
    onconsider?: (e: CustomEvent<import('svelte-dnd-action').DndEvent<any>>) => void;
    onfinalize?: (e: CustomEvent<import('svelte-dnd-action').DndEvent<any>>) => void;
  }
}
