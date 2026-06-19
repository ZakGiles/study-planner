import type { main } from '../../wailsjs/go/models';

// Every backend mutation resolves to the full new State (subjects + tasks), and
// every view handles it the same way: hand the State to the owner, surface
// failures as a message, optionally hold a busy flag while in flight. makeMutator
// builds that wrapper once per component. The returned function resolves to
// whether the call succeeded, so callers can run follow-up state changes (clear a
// form, revert an optimistic reorder) only on success.
export function makeMutator(handlers: {
  state: (state: main.State) => void;
  error: (msg: string) => void;
  busy?: (busy: boolean) => void;
}): (p: Promise<main.State>) => Promise<boolean> {
  return async (p) => {
    handlers.busy?.(true);
    try {
      handlers.state(await p);
      return true;
    } catch (e) {
      handlers.error(String(e));
      return false;
    } finally {
      handlers.busy?.(false);
    }
  };
}
