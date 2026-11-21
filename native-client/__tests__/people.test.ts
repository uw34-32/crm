import type { Contact } from '../src/api/records';
import { createListStore, createPager } from '../src/lib/records/list';
import { isPersonOf, loadPeople } from '../src/lib/records/people';

const person = (n: number, companyId: string): Contact => ({ id: `c${n}`, first_name: `P${n}`, company_id: companyId, created_at: '', updated_at: '' });

function setup(pages: Contact[][]) {
  let call = 0;
  const fetchPage = jest.fn(async () => {
    const items = pages[call++] ?? [];
    return { items, has_more: call < pages.length };
  });
  const store = createListStore<Contact>();
  const pager = createPager(store, fetchPage, { keep: isPersonOf('acme') });
  return { store, pager, fetchPage };
}

describe('company people', () => {
  it('looks ahead past other companies\' people until it has some', async () => {
    const { store, pager, fetchPage } = setup([[person(1, 'other')], [person(2, 'other')], [person(3, 'acme')]]);
    await loadPeople(pager, store, 'Acme');
    expect(fetchPage).toHaveBeenCalledTimes(3);
    expect(store.get().items.map((c) => c.id)).toEqual(['c3']);
  });

  it('stops looking after a few pages', async () => {
    const { store, pager, fetchPage } = setup(Array.from({ length: 8 }, (_, i) => [person(i, 'other')]));
    await loadPeople(pager, store, 'Acme');
    expect(fetchPage).toHaveBeenCalledTimes(4);
    expect(store.get().hasMore).toBe(true);
  });

  it('stops once it has enough people', async () => {
    const many = Array.from({ length: 10 }, (_, i) => person(i, 'acme'));
    const { store, pager, fetchPage } = setup([many, [person(99, 'acme')]]);
    await loadPeople(pager, store, 'Acme');
    expect(fetchPage).toHaveBeenCalledTimes(1);
  });

  it('stops when the company changed meanwhile', async () => {
    const { store, pager, fetchPage } = setup([[person(1, 'other')], [person(2, 'acme')]]);
    await loadPeople(pager, store, 'Acme', () => false);
    expect(fetchPage).toHaveBeenCalledTimes(1);
  });
});
