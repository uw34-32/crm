// Loads one record for a detail or edit screen. The API answers the same 404 for a
// record that was deleted, never existed, or belongs to another workspace, so all of
// them read as missing.

import { ApiError } from '../api/client.js';

/**
 * @template T
 * @typedef {{ status: 'ready', record: T, error: '' } | { status: 'missing', record: null, error: '' } | { status: 'error', record: null, error: string }} Loaded
 */

/** @param {unknown} err */
export const isMissing = (err) => err instanceof ApiError && err.status === 404;

/**
 * @template T
 * @param {() => Promise<T>} fetchOne
 * @returns {Promise<Loaded<T>>}
 */
export async function loadRecord(fetchOne) {
  try {
    return { status: 'ready', record: await fetchOne(), error: '' };
  } catch (err) {
    if (isMissing(err)) return { status: 'missing', record: null, error: '' };
    return { status: 'error', record: null, error: err instanceof Error ? err.message : String(err) };
  }
}
