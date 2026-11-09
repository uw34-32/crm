// Which workspace is open. The choice is saved per user, so two people sharing a
// browser each come back to their own.

/** @typedef {Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>} KeyValueStorage */

/** @param {string} userId */
const keyFor = (userId) => `crm.workspace.${userId}`;

/**
 * The saved workspace while the user still belongs to it, else the first one, else
 * none.
 * @template {{ id: string }} W
 * @param {W[]} workspaces
 * @param {string | null} savedId
 * @returns {W | null}
 */
export function pickWorkspace(workspaces, savedId) {
  return workspaces.find((w) => w.id === savedId) ?? workspaces[0] ?? null;
}

/** @param {KeyValueStorage} storage @param {string} userId */
export function readSelection(storage, userId) {
  try {
    return storage.getItem(keyFor(userId));
  } catch {
    return null;
  }
}

/** @param {KeyValueStorage} storage @param {string} userId @param {string | null} workspaceId */
export function saveSelection(storage, userId, workspaceId) {
  try {
    if (workspaceId) storage.setItem(keyFor(userId), workspaceId);
    else storage.removeItem(keyFor(userId));
  } catch {
    // Without storage the choice lasts until the page closes.
  }
}
