// A counter that moves whenever a screen creates, edits, or deletes a record, so the
// lists and detail screens that are still open load again when they next render.

import { createStore } from '../store';

export const changes = createStore({ version: 0 });

export function recordsChanged() {
  changes.set({ version: changes.get().version + 1 });
}
