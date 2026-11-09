import { describe, expect, it } from 'vitest';
import { movedToAnotherPage } from './focus.js';

const at = (/** @type {string} */ href) => ({ url: new URL(href, 'http://app.test') });

describe('focus after navigation', () => {
  it('moves for a new page', () => {
    expect(movedToAnotherPage(at('/contacts'), at('/contacts/c1'))).toBe(true);
  });

  it('stays put when only the search changed, so typing keeps its place', () => {
    expect(movedToAnotherPage(at('/contacts?q=a'), at('/contacts?q=ab'))).toBe(false);
  });

  it('stays put on the first page load, where the browser starts at the top', () => {
    expect(movedToAnotherPage(null, at('/contacts'))).toBe(false);
  });
});
