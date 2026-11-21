// One screen's list: the first page for a search, further pages on request, and the
// states in between. Answers that arrive after a newer search are dropped, so fast
// typing never shows an older search's rows.

import type { Store } from '../store';
import { createStore } from '../store';

export const PAGE_SIZE = 50;
/** The API's largest page. */
const MAX_PAGE = 200;

export type ListState<T> = {
  items: T[];
  q: string;
  status: 'idle' | 'loading' | 'ready' | 'error';
  error: string;
  hasMore: boolean;
  loadingMore: boolean;
  moreError: string;
};

export type FetchPage<T> = (params: { q: string; limit: number; offset: number }) => Promise<{ items: T[]; has_more: boolean }>;

export function emptyList<T>(): ListState<T> {
  return { items: [], q: '', status: 'idle', error: '', hasMore: false, loadingMore: false, moreError: '' };
}

export function createListStore<T>(): Store<ListState<T>> {
  return createStore<ListState<T>>(emptyList<T>());
}

const messageOf = (err: unknown) => (err instanceof Error ? err.message : String(err));

/**
 * keep filters rows on this side; paging still counts every row the API returned.
 */
export function createPager<T>(store: Store<ListState<T>>, fetchPage: FetchPage<T>, { limit = PAGE_SIZE, keep }: { limit?: number; keep?: (item: T) => boolean } = {}) {
  let generation = 0;
  // Where the next page starts: every row the API has returned for this search.
  let offset = 0;

  const kept = (items: T[]) => (keep ? items.filter(keep) : items);

  /**
   * Loads the first page of a search, replacing what is shown. A quiet load is for
   * a refresh behind the rows already on screen: it keeps them, asks for as many as
   * were loaded so far, and swaps them in when the answer arrives.
   */
  async function load(search: string, { quiet = false }: { quiet?: boolean } = {}) {
    const q = search.trim();
    const mine = ++generation;
    const size = quiet ? Math.min(MAX_PAGE, Math.max(limit, offset)) : limit;
    if (!quiet) store.set({ q, status: 'loading', error: '', moreError: '', loadingMore: false });
    try {
      const page = await fetchPage({ q, limit: size, offset: 0 });
      if (mine !== generation) return;
      offset = page.items.length;
      store.set({ q, items: kept(page.items), hasMore: page.has_more, status: 'ready', error: '', moreError: '', loadingMore: false });
    } catch (err) {
      if (mine !== generation) return;
      // Behind the rows on screen, a failed refresh changes nothing; the next one tries again.
      if (quiet) return;
      store.set({ items: [], hasMore: false, error: messageOf(err), status: 'error' });
    }
  }

  /** Appends the next page; a second call while one is loading does nothing. */
  async function more() {
    const state = store.get();
    if (state.status !== 'ready' || !state.hasMore || state.loadingMore) return;
    const mine = generation;
    store.set({ loadingMore: true, moreError: '' });
    try {
      const page = await fetchPage({ q: state.q, limit, offset });
      if (mine !== generation) return;
      offset += page.items.length;
      store.set({ items: [...store.get().items, ...kept(page.items)], hasMore: page.has_more });
    } catch (err) {
      if (mine !== generation) return;
      store.set({ moreError: messageOf(err) });
    } finally {
      if (mine === generation) store.set({ loadingMore: false });
    }
  }

  /** Forgets answers still on the way, as when the screen closes. */
  function stop() {
    generation++;
  }

  return { load, more, stop };
}
