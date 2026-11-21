// The people linked to a company. The contacts API has no company filter, so this
// searches for the company's name, which every linked contact matches, and keeps the
// ones linked to this company.

import type { Contact } from '../../api/records';
import type { Store } from '../store';
import type { ListState } from './list';

// A common name matches many people at other companies, so look a few pages ahead for
// this company's people before asking the reader to look further.
export const LOOKAHEAD_PAGES = 4;
export const ENOUGH_PEOPLE = 10;

export const isPersonOf = (companyId: string) => (contact: Contact) => contact.company_id === companyId;

type Pager = { load: (search: string) => Promise<void>; more: () => Promise<void> };

/** Loads the first page, then more while few people were found and more pages exist. */
export async function loadPeople(pager: Pager, store: Store<ListState<Contact>>, name: string, isCurrent: () => boolean = () => true) {
  await pager.load(name);
  for (let round = 1; round < LOOKAHEAD_PAGES; round++) {
    const state = store.get();
    if (!isCurrent() || state.q !== name.trim()) return;
    if (state.status !== 'ready' || !state.hasMore || state.moreError || state.items.length >= ENOUGH_PEOPLE) return;
    await pager.more();
  }
}
