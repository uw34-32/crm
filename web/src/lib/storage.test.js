import { describe, expect, it } from 'vitest';
import { createTokenStore } from './api/tokens.js';
import { browserStorage } from './storage.js';
import { readSelection, saveSelection } from './workspaces/selection.js';

describe('browser storage', () => {
  it('answers the page’s storage when it can be read', () => {
    const storage = /** @type {Storage} */ (/** @type {unknown} */ ({ getItem: () => 'x', setItem() {}, removeItem() {} }));

    expect(browserStorage(() => storage)).toBe(storage);
  });

  it('answers a stand-in that refuses everything when reading storage throws', () => {
    const refusing = browserStorage(() => {
      throw new DOMException('denied', 'SecurityError');
    });

    expect(() => refusing.getItem('k')).toThrow();
    expect(() => refusing.setItem('k', 'v')).toThrow();
    expect(() => refusing.removeItem('k')).toThrow();
  });

  it('answers the same stand-in where there is no window, as in tests and prerendering', () => {
    const refusing = browserStorage();

    expect(() => refusing.getItem('k')).toThrow();
  });

  it('lets the token store and the workspace choice fall back the same way', () => {
    const refusing = browserStorage(() => {
      throw new Error('blocked');
    });
    const tokens = createTokenStore(refusing);

    tokens.save({ token: 'access-1', refresh_token: 'refresh-1' });
    saveSelection(refusing, 'u1', 'w1');

    expect(tokens.getRefresh()).toBe('refresh-1');
    expect(readSelection(refusing, 'u1')).toBeNull();
  });
});
