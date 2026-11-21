// Where the tokens live. The access token stays in memory, so a reload trades the refresh
// token for a new one. The refresh token sits in the platform's secure storage (the
// Keychain on iOS, the Keystore on Android), read once at start and then kept in memory,
// because the API client reads it synchronously.

import type { Session, Tokens } from './client';

export const REFRESH_TOKEN_KEY = 'crm.refresh_token';

/** Storage that can be slow and can fail. */
export type AsyncStorage = {
  get: (key: string) => Promise<string | null>;
  set: (key: string, value: string) => Promise<void>;
  remove: (key: string) => Promise<void>;
};

export type TokenStore = Tokens & {
  /** Reads the saved refresh token. Call once before the first refresh. */
  load: () => Promise<void>;
  /** Resolves when every write so far has reached storage, or failed. */
  settled: () => Promise<void>;
};

/**
 * Storage can throw, for instance with the device locked. Then the refresh token lives
 * in memory for this run only, and the next start is signed out.
 */
export function createTokenStore(storage: AsyncStorage): TokenStore {
  let access = '';
  let refresh: string | null = null;
  // Writes run one after another, so a clear is never overtaken by an earlier save.
  let writes: Promise<void> = Promise.resolve();

  const enqueue = (write: () => Promise<void>) => {
    writes = writes.then(write).catch(() => undefined);
  };

  return {
    getAccess: () => access,
    getRefresh: () => refresh,
    async load() {
      try {
        refresh = (await storage.get(REFRESH_TOKEN_KEY)) ?? refresh;
      } catch {
        // Nothing saved that this run can read.
      }
    },
    save(session: Session) {
      access = session.token;
      refresh = session.refresh_token;
      const token = session.refresh_token;
      enqueue(() => storage.set(REFRESH_TOKEN_KEY, token));
    },
    clear() {
      access = '';
      refresh = null;
      enqueue(() => storage.remove(REFRESH_TOKEN_KEY));
    },
    settled: () => writes
  };
}
