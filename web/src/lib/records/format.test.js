import { describe, expect, it } from 'vitest';
import { contactName, contactSubtitle, initials } from './format.js';

describe('names', () => {
  it('joins first and last name, skipping a blank last name', () => {
    expect(contactName({ first_name: 'Ada', last_name: 'Lovelace' })).toBe('Ada Lovelace');
    expect(contactName({ first_name: 'Plato', last_name: '' })).toBe('Plato');
  });

  it('takes initials from the first two words, as capitals', () => {
    expect(initials('ada lovelace')).toBe('AL');
    expect(initials('Engines Ltd of London')).toBe('EL');
    expect(initials('  ')).toBe('?');
    expect(initials('Émile')).toBe('É');
  });

  it('describes a contact by title and company, whichever it has', () => {
    expect(contactSubtitle({ title: 'Analyst', company_name: 'Engines Ltd' })).toBe('Analyst · Engines Ltd');
    expect(contactSubtitle({ title: '', company_name: 'Engines Ltd' })).toBe('Engines Ltd');
    expect(contactSubtitle({ title: '', company_name: null })).toBe('');
  });
});
