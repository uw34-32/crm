import { describe, expect, it, vi } from 'vitest';
import { createTokenStore, REFRESH_TOKEN_KEY, watchSignOut } from './tokens.js';

function memoryStorage() {
  const map = new Map();
  return {
    map,
    getItem: (/** @type {string} */ key) => (map.has(key) ? map.get(key) : null),
    setItem: (/** @type {string} */ key, /** @type {string} */ value) => void map.set(key, String(value)),
    removeItem: (/** @type {string} */ key) => void map.delete(key)
  };
}

describe('token store', () => {
  it('keeps the access token in memory and only the refresh token in storage', () => {
    const storage = memoryStorage();
    const tokens = createTokenStore(storage);

    tokens.save({ token: 'access-1', refresh_token: 'refresh-1' });

    expect(tokens.getAccess()).toBe('access-1');
    expect(tokens.getRefresh()).toBe('refresh-1');
    expect([...storage.map.entries()]).toEqual([[REFRESH_TOKEN_KEY, 'refresh-1']]);
  });

  it('reads the refresh token from storage each time, so another tab’s rotation is seen', () => {
    const storage = memoryStorage();
    const tokens = createTokenStore(storage);
    tokens.save({ token: 'access-1', refresh_token: 'refresh-1' });

    storage.setItem(REFRESH_TOKEN_KEY, 'refresh-from-another-tab');

    expect(tokens.getRefresh()).toBe('refresh-from-another-tab');
  });

  it('starts a reload with no access token and the stored refresh token', () => {
    const storage = memoryStorage();
    storage.setItem(REFRESH_TOKEN_KEY, 'refresh-1');

    const tokens = createTokenStore(storage);

    expect(tokens.getAccess()).toBe('');
    expect(tokens.getRefresh()).toBe('refresh-1');
  });

  it('clears both tokens', () => {
    const storage = memoryStorage();
    const tokens = createTokenStore(storage);
    tokens.save({ token: 'access-1', refresh_token: 'refresh-1' });

    tokens.clear();

    expect(tokens.getAccess()).toBe('');
    expect(tokens.getRefresh()).toBeNull();
    expect(storage.map.size).toBe(0);
  });

  it('works for this tab when storage refuses to', () => {
    const tokens = createTokenStore({
      getItem() {
        throw new Error('denied');
      },
      setItem() {
        throw new Error('denied');
      },
      removeItem() {
        throw new Error('denied');
      }
    });

    tokens.save({ token: 'access-1', refresh_token: 'refresh-1' });

    expect(tokens.getAccess()).toBe('access-1');
    expect(tokens.getRefresh()).toBe('refresh-1');
    expect(() => tokens.clear()).not.toThrow();
    expect(tokens.getRefresh()).toBeNull();
  });
});

describe('sign-out in another tab', () => {
  /** @param {Partial<StorageEvent>} init */
  function storageEvent(init) {
    return Object.assign(new Event('storage'), init);
  }

  it('reports a refresh token removed by another tab', () => {
    const target = new EventTarget();
    const signedOut = vi.fn();
    watchSignOut(target, signedOut);

    target.dispatchEvent(storageEvent({ key: REFRESH_TOKEN_KEY, oldValue: 'refresh-1', newValue: null }));

    expect(signedOut).toHaveBeenCalledTimes(1);
  });

  it('reports storage cleared by another tab', () => {
    const target = new EventTarget();
    const signedOut = vi.fn();
    watchSignOut(target, signedOut);

    target.dispatchEvent(storageEvent({ key: null, newValue: null }));

    expect(signedOut).toHaveBeenCalledTimes(1);
  });

  it('ignores a rotation and other keys', () => {
    const target = new EventTarget();
    const signedOut = vi.fn();
    watchSignOut(target, signedOut);

    target.dispatchEvent(storageEvent({ key: REFRESH_TOKEN_KEY, oldValue: 'refresh-1', newValue: 'refresh-2' }));
    target.dispatchEvent(storageEvent({ key: 'crm.workspace.u1', newValue: null }));

    expect(signedOut).not.toHaveBeenCalled();
  });

  it('stops listening when asked', () => {
    const target = new EventTarget();
    const signedOut = vi.fn();
    const stop = watchSignOut(target, signedOut);

    stop();
    target.dispatchEvent(storageEvent({ key: REFRESH_TOKEN_KEY, newValue: null }));

    expect(signedOut).not.toHaveBeenCalled();
  });
});
