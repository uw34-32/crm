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
