// The signed-in user and the API client every screen shares.

import { logout } from './api/auth';
import { createClient, type Session } from './api/client';
import { createTokenStore, type AsyncStorage } from './api/tokens';
import { createStore } from './lib/store';

export type User = { id: string; email: string; name: string };
export type AuthResponse = { token: string; refresh_token: string; user: User };

export type SessionState = {
  user: User | null;
  /** Whether the session from a previous run has been restored, or found absent. */
  ready: boolean;
  /** Why restoring failed, when it failed for a reason other than the token. */
  restoreError: string;
  /** Set when the API ended the session, so the sign-in screen can say why. */
  ended: boolean;
};

export function createSession({ baseUrl, fetch, storage }: { baseUrl: string; fetch: typeof globalThis.fetch; storage: AsyncStorage }) {
  const state = createStore<SessionState>({ user: null, ready: false, restoreError: '', ended: false });
  const stored = createTokenStore(storage);

  // Every new token pair carries the user, so saving it also says who is signed in.
  const tokens = {
    ...stored,
    save(pair: Session) {
      stored.save(pair);
      if (pair.user) state.set({ user: pair.user as User });
    }
  };

  const api = createClient({
    baseUrl,
    fetch,
    tokens,
    onSessionEnd: () => state.set({ user: null, restoreError: '', ended: true })
  });

  /**
   * Restores the session of a previous run by trading the stored refresh token for a new
   * pair. A refused token leaves the user signed out; a lost connection keeps the token
   * and reports the error, so a retry can still succeed.
   */
  async function restore() {
    state.set({ restoreError: '' });
    try {
      await stored.load();
      if (tokens.getRefresh()) await api.refresh();
    } catch (err) {
      if (tokens.getRefresh()) state.set({ restoreError: err instanceof Error ? err.message : String(err) });
    } finally {
      state.set({ ready: true });
    }
  }

  async function start(request: Promise<AuthResponse>) {
    const pair = await request;
    tokens.save(pair);
    state.set({ ended: false });
  }

  const signIn = (credentials: { email: string; password: string }) =>
    start(api.request<AuthResponse>('/auth/login', { method: 'POST', body: credentials, auth: false }));

  const register = (account: { name: string; email: string; password: string }) =>
    start(api.request<AuthResponse>('/auth/register', { method: 'POST', body: account, auth: false }));

  /** Signs out on purpose, including from the screen of a restore that failed. */
  async function signOut() {
    await logout(api, tokens);
    state.set({ user: null, restoreError: '', ended: false });
  }

  return { state, api, tokens: stored, restore, signIn, register, signOut };
}

export type SessionService = ReturnType<typeof createSession>;
