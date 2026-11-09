// Where the tokens live. The access token stays in memory, so script injected into the
// page cannot read it from storage, and a reload trades the refresh token for a new
// one. The refresh token sits in localStorage, so a reload stays signed in: the API
// takes bearer tokens from three clients, so there is no httpOnly cookie to hold it.
// The README records this tradeoff.

export const REFRESH_TOKEN_KEY = 'crm.refresh_token';

/**
 * @typedef {Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>} KeyValueStorage
 */

/**
 * Storage can throw, in a private window or with site data blocked. Then the refresh
 * token lives in memory for this tab only.
 * @param {KeyValueStorage} storage
 */
export function createTokenStore(storage) {
  let access = '';
  /** @type {string | null} */
  let fallbackRefresh = null;

  return {
    getAccess: () => access,
    getRefresh() {
      try {
        return storage.getItem(REFRESH_TOKEN_KEY);
      } catch {
        return fallbackRefresh;
      }
    },
    /** @param {{ token: string, refresh_token: string }} session */
    save(session) {
      access = session.token;
      fallbackRefresh = session.refresh_token;
      try {
        storage.setItem(REFRESH_TOKEN_KEY, session.refresh_token);
      } catch {
        // Kept in memory above.
      }
    },
    clear() {
      access = '';
      fallbackRefresh = null;
      try {
        storage.removeItem(REFRESH_TOKEN_KEY);
      } catch {
        // Nothing stored to remove.
      }
    }
  };
}

/**
 * Calls onSignOut when another tab removes the refresh token: it signed out, or its
 * token was refused. Browsers send storage events only to the other tabs, so this tab's
 * own sign-out does not call it. Returns a function that stops listening.
 * @param {Pick<EventTarget, 'addEventListener' | 'removeEventListener'>} target the window
 * @param {() => void} onSignOut
 */
export function watchSignOut(target, onSignOut) {
  /** @param {Event} event */
  const listener = (event) => {
    const { key, newValue } = /** @type {StorageEvent} */ (event);
    // A null key means another tab cleared all of storage.
    if ((key === REFRESH_TOKEN_KEY || key === null) && newValue === null) onSignOut();
  };
  target.addEventListener('storage', listener);
  return () => target.removeEventListener('storage', listener);
}
