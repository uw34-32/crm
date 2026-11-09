// The page's localStorage, or one stand-in where there is none. Storage can be missing
// (no window, as in tests) or refuse access (a private window, or site data blocked),
// and every caller copes with a storage that throws: the token store keeps the refresh
// token in memory, and the workspace choice lasts until the page closes.

/** @typedef {Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>} KeyValueStorage */

const refuse = () => {
  throw new Error('Storage is not available.');
};

/** @type {KeyValueStorage} */
const refusingStorage = { getItem: refuse, setItem: refuse, removeItem: refuse };

/**
 * @param {() => KeyValueStorage | null | undefined} [read] how to reach storage; the window's by default
 * @returns {KeyValueStorage}
 */
export function browserStorage(read = () => window.localStorage) {
  try {
    return read() ?? refusingStorage;
  } catch {
    return refusingStorage;
  }
}
