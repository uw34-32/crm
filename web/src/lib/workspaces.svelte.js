// The user's workspaces and the one that is open.

import { api, session } from './session.svelte.js';
import { pickWorkspace, readSelection, saveSelection } from './workspaces/selection.js';

/**
 * @typedef {'owner' | 'admin' | 'member'} Role
 * @typedef {{ id: string, name: string, role: Role, created_at: string, updated_at: string }} Workspace
 */

/** Storage where there is one; selection.js copes with one that throws. */
function storage() {
  try {
    return window.localStorage;
  } catch {
    return /** @type {Storage} */ (/** @type {unknown} */ ({}));
  }
}

export const workspaces = $state({
  /** @type {Workspace[]} */
  list: [],
  /** @type {Workspace | null} */
  current: null,
  /** @type {'idle' | 'loading' | 'ready' | 'error'} */
  status: 'idle',
  error: '',
  /** The user the list belongs to, so another user's sign-in starts over. */
  /** @type {string | null} */
  userId: null
});

/** @param {Workspace | null} workspace */
function open(workspace) {
  workspaces.current = workspace;
  if (workspaces.userId) saveSelection(storage(), workspaces.userId, workspace?.id ?? null);
}

/** Loads the user's workspaces and opens the saved one, or the first. */
export async function loadWorkspaces() {
  const userId = session.user?.id ?? null;
  if (!userId) return;
  workspaces.userId = userId;
  workspaces.status = 'loading';
  workspaces.error = '';
  try {
    const list = /** @type {Workspace[]} */ (await api.request('/workspaces'));
    if (workspaces.userId !== userId) return;
    workspaces.list = list;
    open(pickWorkspace(list, readSelection(storage(), userId)));
    workspaces.status = 'ready';
  } catch (err) {
    if (workspaces.userId !== userId) return;
    workspaces.status = 'error';
    workspaces.error = err instanceof Error ? err.message : String(err);
  }
}

/** @param {string} id */
export function selectWorkspace(id) {
  const workspace = workspaces.list.find((w) => w.id === id);
  if (workspace) open(workspace);
}

/** Creates a workspace, with the caller as its owner, and opens it. @param {string} name */
export async function createWorkspace(name) {
  const workspace = /** @type {Workspace} */ (await api.request('/workspaces', { method: 'POST', body: { name } }));
  workspaces.list = [...workspaces.list, workspace];
  open(workspace);
  return workspace;
}

/** Forgets the list on sign-out; the saved choice stays for the user's next visit. */
export function resetWorkspaces() {
  workspaces.list = [];
  workspaces.current = null;
  workspaces.status = 'idle';
  workspaces.error = '';
  workspaces.userId = null;
}
