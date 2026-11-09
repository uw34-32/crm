import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { REFRESH_TOKEN_KEY } from './api/tokens.js';

// The session module reads the window when it loads, so each test loads a fresh copy
// against a stand-in window with its own storage.

function fakeWindow() {
  const map = new Map();
  const win = Object.assign(new EventTarget(), {
    localStorage: {
      getItem: (/** @type {string} */ key) => (map.has(key) ? map.get(key) : null),
      setItem: (/** @type {string} */ key, /** @type {string} */ value) => void map.set(key, String(value)),
      removeItem: (/** @type {string} */ key) => void map.delete(key)
    }
  });
  return { win, map };
}

/** @param {number} status @param {unknown} [body] */
function reply(status, body) {
  return new Response(body === undefined ? null : JSON.stringify(body), { status });
}

beforeEach(() => {
  vi.resetModules();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('session', () => {
  it('leaves the restore error behind when signing out after a failed restore', async () => {
    const { win, map } = fakeWindow();
    map.set(REFRESH_TOKEN_KEY, 'refresh-1');
    vi.stubGlobal('window', win);
    vi.stubGlobal('fetch', vi.fn(async () => Promise.reject(new TypeError('Failed to fetch'))));
    const { restoreSession, session, signOut } = await import('./session.svelte.js');

    await restoreSession();
    expect(session.restoreError).toMatch(/cannot reach the server/i);

    await signOut();

    expect(session.restoreError).toBe('');
    expect(map.has(REFRESH_TOKEN_KEY)).toBe(false);
  });

  it('signs out here when another tab signs out', async () => {
    const { win, map } = fakeWindow();
    map.set(REFRESH_TOKEN_KEY, 'refresh-1');
    vi.stubGlobal('window', win);
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => reply(200, { data: { token: 'access-2', refresh_token: 'refresh-2', user: { id: 'u1', email: 'a@b.co', name: 'Ada' } } }))
    );
    const { restoreSession, session } = await import('./session.svelte.js');
    await restoreSession();
    expect(session.user?.id).toBe('u1');

    map.delete(REFRESH_TOKEN_KEY);
    win.dispatchEvent(Object.assign(new Event('storage'), { key: REFRESH_TOKEN_KEY, oldValue: 'refresh-2', newValue: null }));

    expect(session.user).toBeNull();
    expect(session.ended).toBe(true);
  });
});
