import { describe, expect, it, vi } from 'vitest';
import { createPager, emptyList } from './list.js';

/** A list API over an array: substring search and offset paging, as the API pages. */
function fakeList(/** @type {{ id: string, name: string }[]} */ rows) {
  return vi.fn(async (/** @type {{ q: string, limit: number, offset: number }} */ { q, limit, offset }) => {
    const hits = rows.filter((r) => r.name.toLowerCase().includes(q.trim().toLowerCase()));
    const items = hits.slice(offset, offset + limit);
    return { items, limit, offset, has_more: offset + limit < hits.length };
  });
}

const rows = Array.from({ length: 5 }, (_, i) => ({ id: `r${i}`, name: i % 2 ? `Odd ${i}` : `Even ${i}` }));

describe('list paging', () => {
  it('loads the first page and says whether more remain', async () => {
    const state = emptyList();
    const fetchPage = fakeList(rows);
    const pager = createPager(state, fetchPage, { limit: 2 });

    await pager.load('');

    expect(state.status).toBe('ready');
    expect(state.items.map((r) => r.id)).toEqual(['r0', 'r1']);
    expect(state.hasMore).toBe(true);
    expect(fetchPage).toHaveBeenCalledWith({ q: '', limit: 2, offset: 0 });
  });

  it('appends the next page from where the last one ended', async () => {
    const state = emptyList();
    const pager = createPager(state, fakeList(rows), { limit: 2 });
    await pager.load('');

    await pager.more();
    await pager.more();

    expect(state.items.map((r) => r.id)).toEqual(['r0', 'r1', 'r2', 'r3', 'r4']);
    expect(state.hasMore).toBe(false);
  });

  it('starts over for a new search, and sends the search trimmed', async () => {
    const state = emptyList();
    const fetchPage = fakeList(rows);
    const pager = createPager(state, fetchPage, { limit: 2 });
    await pager.load('');
    await pager.more();

    await pager.load('  odd ');

    expect(state.q).toBe('odd');
    expect(state.items.map((r) => r.id)).toEqual(['r1', 'r3']);
    expect(fetchPage).toHaveBeenLastCalledWith({ q: 'odd', limit: 2, offset: 0 });
  });

  it('drops an answer that arrives after a newer search', async () => {
    const state = emptyList();
    /** @type {((value: unknown) => void)[]} */
    const answers = [];
    const fetchPage = vi.fn(() => new Promise((resolve) => answers.push(resolve)));
    const pager = createPager(state, /** @type {any} */ (fetchPage));

    const first = pager.load('a');
    const second = pager.load('ab');
    answers[1]({ items: [{ id: 'new' }], has_more: false });
    answers[0]({ items: [{ id: 'stale' }], has_more: true });
    await Promise.all([first, second]);

    expect(state.items).toEqual([{ id: 'new' }]);
    expect(state.q).toBe('ab');
    expect(state.hasMore).toBe(false);
  });

  it('reports a failed first page as an error, and keeps loaded rows when a later page fails', async () => {
    const state = emptyList();
    const fetchPage = vi.fn().mockRejectedValueOnce(new Error('Cannot reach the server.'));
    const pager = createPager(state, fetchPage, { limit: 2 });

    await pager.load('');
    expect(state.status).toBe('error');
    expect(state.error).toBe('Cannot reach the server.');

    fetchPage.mockResolvedValueOnce({ items: [{ id: 'r0' }, { id: 'r1' }], has_more: true });
    await pager.load('');
    fetchPage.mockRejectedValueOnce(new Error('The server is busy.'));
    await pager.more();

    expect(state.status).toBe('ready');
    expect(state.items).toHaveLength(2);
    expect(state.moreError).toBe('The server is busy.');
    expect(state.hasMore).toBe(true);
  });

  it('asks for one more page at a time', async () => {
    const state = emptyList();
    const fetchPage = fakeList(rows);
    const pager = createPager(state, fetchPage, { limit: 2 });
    await pager.load('');

    await Promise.all([pager.more(), pager.more()]);

    expect(fetchPage).toHaveBeenCalledTimes(2);
    expect(state.items).toHaveLength(4);
  });

  it('keeps only matching rows, while paging by what the API returned', async () => {
    const state = emptyList();
    const fetchPage = fakeList(rows);
    const pager = createPager(state, fetchPage, { limit: 2, keep: (row) => row.name.startsWith('Even') });
    await pager.load('');

    await pager.more();

    expect(state.items.map((r) => r.id)).toEqual(['r0', 'r2']);
    expect(fetchPage).toHaveBeenLastCalledWith({ q: '', limit: 2, offset: 2 });
  });
});
