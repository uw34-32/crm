import { createListStore, createPager, type FetchPage } from '../src/lib/records/list';

const page = (items: number[], has_more = false) => ({ items, has_more });

function setup(fetchPage: FetchPage<number>, options?: { limit?: number; keep?: (n: number) => boolean }) {
  const store = createListStore<number>();
  return { store, pager: createPager(store, fetchPage, options) };
}

describe('pager', () => {
  it('loads the first page of a trimmed search', async () => {
    const fetchPage = jest.fn(async () => page([1, 2], true));
    const { store, pager } = setup(fetchPage);
    await pager.load('  ada ');
    expect(fetchPage).toHaveBeenCalledWith({ q: 'ada', limit: 50, offset: 0 });
    expect(store.get()).toMatchObject({ items: [1, 2], q: 'ada', status: 'ready', hasMore: true });
  });

  it('appends the next page from where the last one ended', async () => {
    const fetchPage = jest.fn().mockResolvedValueOnce(page([1, 2], true)).mockResolvedValueOnce(page([3], false));
    const { store, pager } = setup(fetchPage);
    await pager.load('');
    await pager.more();
    expect(fetchPage).toHaveBeenLastCalledWith({ q: '', limit: 50, offset: 2 });
    expect(store.get()).toMatchObject({ items: [1, 2, 3], hasMore: false });
  });

  it('ignores a second more() while one is loading', async () => {
    let finish: (p: ReturnType<typeof page>) => void = () => {};
    const fetchPage = jest.fn().mockResolvedValueOnce(page([1], true)).mockImplementationOnce(() => new Promise((resolve) => (finish = resolve)));
    const { pager } = setup(fetchPage);
    await pager.load('');
    const first = pager.more();
    await pager.more();
    finish(page([2]));
    await first;
    expect(fetchPage).toHaveBeenCalledTimes(2);
  });

  it('drops the answer of an older search', async () => {
    const resolvers: ((p: ReturnType<typeof page>) => void)[] = [];
    const fetchPage = jest.fn(() => new Promise<ReturnType<typeof page>>((resolve) => resolvers.push(resolve)));
    const { store, pager } = setup(fetchPage);
    const slow = pager.load('a');
    const fast = pager.load('ab');
    resolvers[1](page([2]));
    await fast;
    resolvers[0](page([1]));
    await slow;
    expect(store.get()).toMatchObject({ items: [2], q: 'ab' });
  });

  it('reports a failed load and a failed next page apart', async () => {
    const fetchPage = jest.fn().mockRejectedValueOnce(new Error('down')).mockResolvedValueOnce(page([1], true)).mockRejectedValueOnce(new Error('slow'));
    const { store, pager } = setup(fetchPage);
    await pager.load('');
    expect(store.get()).toMatchObject({ status: 'error', error: 'down', items: [] });
    await pager.load('');
    await pager.more();
    expect(store.get()).toMatchObject({ status: 'ready', items: [1], moreError: 'slow', loadingMore: false });
  });

  it('keeps only matching rows but pages by what the API returned', async () => {
    const fetchPage = jest.fn().mockResolvedValueOnce(page([1, 2, 3, 4], true)).mockResolvedValueOnce(page([5, 6]));
    const { store, pager } = setup(fetchPage, { keep: (n) => n % 2 === 0 });
    await pager.load('');
    await pager.more();
    expect(fetchPage).toHaveBeenLastCalledWith({ q: '', limit: 50, offset: 4 });
    expect(store.get().items).toEqual([2, 4, 6]);
  });

  it('forgets answers still on the way after stop', async () => {
    let finish: (p: ReturnType<typeof page>) => void = () => {};
    const { store, pager } = setup(() => new Promise((resolve) => (finish = resolve)));
    const loading = pager.load('');
    pager.stop();
    finish(page([1]));
    await loading;
    expect(store.get().status).toBe('loading');
  });
});
