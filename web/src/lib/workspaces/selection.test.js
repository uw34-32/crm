import { describe, expect, it } from 'vitest';
import { pickWorkspace, readSelection, saveSelection } from './selection.js';

const acme = { id: 'w-acme', name: 'Acme' };
const globex = { id: 'w-globex', name: 'Globex' };

/** A Storage stand-in backed by a Map. */
function memoryStorage() {
  const map = new Map();
  return {
    map,
    getItem: (/** @type {string} */ key) => (map.has(key) ? map.get(key) : null),
    setItem: (/** @type {string} */ key, /** @type {string} */ value) => void map.set(key, String(value)),
    removeItem: (/** @type {string} */ key) => void map.delete(key)
  };
}

const brokenStorage = {
  getItem() {
    throw new Error('storage disabled');
  },
  setItem() {
    throw new Error('storage disabled');
  },
  removeItem() {
    throw new Error('storage disabled');
  }
};

describe('pickWorkspace', () => {
  it('keeps the saved workspace while the user still belongs to it', () => {
    expect(pickWorkspace([acme, globex], 'w-globex')).toBe(globex);
  });

  it('falls back to the first workspace when the saved one is gone', () => {
    expect(pickWorkspace([acme, globex], 'w-deleted')).toBe(acme);
  });

  it('falls back to the first workspace when nothing was saved', () => {
    expect(pickWorkspace([globex, acme], null)).toBe(globex);
  });

  it('picks nothing when the user has no workspace', () => {
    expect(pickWorkspace([], 'w-acme')).toBeNull();
  });
});

describe('saved selection', () => {
  it('is kept per user', () => {
    const storage = memoryStorage();
    saveSelection(storage, 'u1', 'w-acme');
    saveSelection(storage, 'u2', 'w-globex');

    expect(readSelection(storage, 'u1')).toBe('w-acme');
    expect(readSelection(storage, 'u2')).toBe('w-globex');
    expect(readSelection(storage, 'u3')).toBeNull();
  });

  it('is cleared by saving none', () => {
    const storage = memoryStorage();
    saveSelection(storage, 'u1', 'w-acme');
    saveSelection(storage, 'u1', null);

    expect(readSelection(storage, 'u1')).toBeNull();
    expect(storage.map.size).toBe(0);
  });

  it('survives storage that refuses to work, such as a private window', () => {
    expect(() => saveSelection(brokenStorage, 'u1', 'w-acme')).not.toThrow();
    expect(readSelection(brokenStorage, 'u1')).toBeNull();
    expect(pickWorkspace([acme], readSelection(brokenStorage, 'u1'))).toBe(acme);
  });
});
