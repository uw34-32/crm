// The signed-in user and the API client every screen shares.

import { logout } from './api/auth.js';
import { createClient } from './api/client.js';
import { createTokenStore } from './api/tokens.js';

/**
 * @typedef {{ id: string, email: string, name: string }} User
 * @typedef {{ token: string, refresh_token: string, user: User }} AuthResponse
 */

export const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8085';

/** Storage, or a stand-in that refuses everything where there is none. */
function browserStorage() {
  try {
    return window.localStorage;
  } catch {
    const refuse = () => {
      throw new Error('no storage');
    };
    return { getItem: refuse, setItem: refuse, removeItem: refuse };
  }
}

export const session = $state({
  /** @type {User | null} */
  user: null,
  /** Whether the session from a previous visit has been restored, or found absent. */
  ready: false,
  /** Why restoring the session failed, when it failed for a reason other than the token. */
  restoreError: '',
  /** Set when the API ended the session, so the sign-in page can say why. */
  ended: false
});

const stored = createTokenStore(browserStorage());

// Every new token pair carries the user, so saving it also says who is signed in.
const tokens = {
  ...stored,
  /** @param {import('./api/client.js').Session} pair */
  save(pair) {
    stored.save(pair);
    if (pair.user) session.user = /** @type {User} */ (pair.user);
  }
};

export const api = createClient({
  baseUrl: API_URL,
  fetch: (input, init) => fetch(input, init),
  tokens,
  onSessionEnd() {
    session.user = null;
    session.ended = true;
  }
});

/**
 * Restores the session of a previous visit by trading the stored refresh token for a
 * new pair. A refused token leaves the user signed out; a lost connection keeps the
 * token and reports the error, so a retry can still succeed.
 */
export async function restoreSession() {
  session.restoreError = '';
  try {
    if (tokens.getRefresh()) await api.refresh();
  } catch (err) {
    if (tokens.getRefresh()) session.restoreError = err instanceof Error ? err.message : String(err);
  } finally {
    session.ready = true;
  }
}

/** @param {AuthResponse} pair */
function start(pair) {
  tokens.save(pair);
  session.ended = false;
}

/** @param {{ email: string, password: string }} credentials */
export async function signIn(credentials) {
  start(/** @type {AuthResponse} */ (await api.request('/auth/login', { method: 'POST', body: credentials, auth: false })));
}

/** @param {{ name: string, email: string, password: string }} account */
export async function register(account) {
  start(/** @type {AuthResponse} */ (await api.request('/auth/register', { method: 'POST', body: account, auth: false })));
}

export async function signOut() {
  await logout(api, tokens);
  session.user = null;
  session.ended = false;
}
