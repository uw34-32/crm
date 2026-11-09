// One screen's list: the first page for a search, further pages on request, and the
// states in between. It writes into a state object the screen passes in, a $state
// proxy in a component and a plain object in tests. Answers that arrive after a newer
// search are dropped, so fast typing never shows an older search's rows.

export const PAGE_SIZE = 50;

/**
 * @template T
 * @typedef {{
 *   items: T[],
 *   q: string,
 *   status: 'idle' | 'loading' | 'ready' | 'error',
 *   error: string,
 *   hasMore: boolean,
 *   loadingMore: boolean,
 *   moreError: string
 * }} ListState
 */

/**
 * @template T
 * @typedef {(params: { q: string, limit: number, offset: number }) => Promise<{ items: T[], has_more: boolean }>} FetchPage
 */

/** @template T @returns {ListState<T>} */
export function emptyList() {
  return { items: [], q: '', status: 'idle', error: '', hasMore: false, loadingMore: false, moreError: '' };
}

/** @param {unknown} err */
const messageOf = (err) => (err instanceof Error ? err.message : String(err));

/**
 * @template T
 * @param {ListState<T>} state
 * @param {FetchPage<T>} fetchPage
 * @param {{ limit?: number, keep?: (item: T) => boolean }} [options]
 *   keep filters rows on this side; paging still counts every row the API returned.
 */
export function createPager(state, fetchPage, { limit = PAGE_SIZE, keep } = {}) {
  let generation = 0;
  // Where the next page starts: every row the API has returned for this search.
  let offset = 0;

  /** @param {T[]} items */
  const kept = (items) => (keep ? items.filter(keep) : items);

  /** Loads the first page of a search, replacing what is shown. @param {string} search */
  async function load(search) {
    const q = search.trim();
    const mine = ++generation;
    state.q = q;
    state.status = 'loading';
    state.error = '';
    state.moreError = '';
    state.loadingMore = false;
    try {
      const page = await fetchPage({ q, limit, offset: 0 });
      if (mine !== generation) return;
      offset = page.items.length;
      state.items = kept(page.items);
      state.hasMore = page.has_more;
      state.status = 'ready';
    } catch (err) {
      if (mine !== generation) return;
      state.items = [];
      state.hasMore = false;
      state.error = messageOf(err);
      state.status = 'error';
    }
  }

  /** Appends the next page; a second call while one is loading does nothing. */
  async function more() {
    if (state.status !== 'ready' || !state.hasMore || state.loadingMore) return;
    const mine = generation;
    state.loadingMore = true;
    state.moreError = '';
    try {
      const page = await fetchPage({ q: state.q, limit, offset });
      if (mine !== generation) return;
      offset += page.items.length;
      state.items = [...state.items, ...kept(page.items)];
      state.hasMore = page.has_more;
    } catch (err) {
      if (mine !== generation) return;
      state.moreError = messageOf(err);
    } finally {
      if (mine === generation) state.loadingMore = false;
    }
  }

  /** Forgets answers still on the way, as when the screen closes. */
  function stop() {
    generation++;
  }

  return { load, more, stop };
}
