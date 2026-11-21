// Loads one record for a detail or edit screen. The API answers the same 404 for a
// record that was deleted, never existed, or belongs to another workspace, so all of
// them read as missing.

import { ApiError } from '../../api/client';

export type Loaded<T> =
  | { status: 'ready'; record: T; error: '' }
  | { status: 'missing'; record: null; error: '' }
  | { status: 'error'; record: null; error: string };

export const isMissing = (err: unknown) => err instanceof ApiError && err.status === 404;

export async function loadRecord<T>(fetchOne: () => Promise<T>): Promise<Loaded<T>> {
  try {
    return { status: 'ready', record: await fetchOne(), error: '' };
  } catch (err) {
    if (isMissing(err)) return { status: 'missing', record: null, error: '' };
    return { status: 'error', record: null, error: err instanceof Error ? err.message : String(err) };
  }
}
