// The members of the open workspace, for owner and assignee names and pickers. Loaded
// once per workspace and shared by every screen that shows a member's name.

import { membersApi } from './api/crm.js';
import { api } from './session.svelte.js';

/** @typedef {import('./api/crm.js').Member} Member */

export const members = $state({
  /** The workspace the list belongs to. */
  workspaceId: '',
  /** @type {Member[]} */
  list: [],
  /** @type {'idle' | 'loading' | 'ready' | 'error'} */
  status: 'idle',
  error: ''
});

const membersOf = membersApi(api);

/**
 * Loads the members of a workspace unless they are already here. A failure leaves
 * names blank rather than blocking the screen; the next call tries again.
 * @param {string} workspaceId
 */
export async function loadMembers(workspaceId) {
  if (!workspaceId) return;
  if (members.workspaceId === workspaceId && (members.status === 'ready' || members.status === 'loading')) return;
  members.workspaceId = workspaceId;
  members.status = 'loading';
  members.error = '';
  members.list = [];
  try {
    const list = await membersOf.list(workspaceId);
    if (members.workspaceId !== workspaceId) return;
    members.list = [...list].sort((a, b) => (a.name || a.email).localeCompare(b.name || b.email));
    members.status = 'ready';
  } catch (err) {
    if (members.workspaceId !== workspaceId) return;
    members.status = 'error';
    members.error = err instanceof Error ? err.message : String(err);
  }
}
