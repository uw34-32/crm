import { describe, expect, it } from 'vitest';
import { listHref, rememberList, searchFromUrl, withSearch } from './url.js';

describe('search in the URL', () => {
  it('reads q, or an empty search when there is none', () => {
    expect(searchFromUrl(new URL('http://app.test/contacts?q=ada'))).toBe('ada');
    expect(searchFromUrl(new URL('http://app.test/contacts'))).toBe('');
  });

  it('caps a search at the API limit of 200 characters', () => {
    const long = 'é'.repeat(250);
    expect([...searchFromUrl(new URL(`http://app.test/contacts?q=${encodeURIComponent(long)}`))]).toHaveLength(200);
  });

  it('writes the search into the path, and drops q when it is blank', () => {
    const url = new URL('http://app.test/contacts?q=old');

    expect(withSearch(url, 'ada & co')).toBe('/contacts?q=ada+%26+co');
    expect(withSearch(url, '   ')).toBe('/contacts');
  });

  it('keeps other query parameters', () => {
    expect(withSearch(new URL('http://app.test/companies?view=all'), 'acme')).toBe('/companies?view=all&q=acme');
  });
});

describe('the list to go back to', () => {
  it('is the plain list until one is remembered', () => {
    expect(listHref('companies')).toBe('/companies');
  });

  it('is the last list URL, search included', () => {
    rememberList('contacts', new URL('http://app.test/contacts?q=ada'));

    expect(listHref('contacts')).toBe('/contacts?q=ada');
  });
});
