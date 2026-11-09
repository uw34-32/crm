// The signed-in user and the API client every screen shares.

import { logout } from './api/auth.js';
import { createClient } from './api/client.js';
import { createTokenStore, watchSignOut } from './api/tokens.js';
import { browserStorage } from './storage.js';

/**
 * @typedef {{ id: string, email: string, name: string }} User
 * @typedef {{ token: string, refresh_token: string, user: User }} AuthResponse
 */

export const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8085';

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

function endSession() {
  session.user = null;
  session.restoreError = '';
  session.ended = true;
}

export const api = createClient({
  baseUrl: API_URL,
  fetch: (input, init) => fetch(input, init),
  tokens,
  onSessionEnd: endSession
});

// Signing out in one tab, or a refused token there, signs out every tab: they share the
// refresh token, so this one could not refresh anyway.
if (typeof window !== 'undefined') {
  watchSignOut(window, () => {
    stored.clear();
    if (session.user || session.restoreError) endSession();
  });
}

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

/** Signs out on purpose, including from the screen of a restore that failed. */
export async function signOut() {
  await logout(api, tokens);
  session.user = null;
  session.restoreError = '';
  session.ended = false;
}
