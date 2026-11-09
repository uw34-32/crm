// What a list screen keeps in its URL: the search, as ?q=, so a reload, a shared link,
// and the back button all return to it. A record's own URL is its path.

export const MAX_SEARCH_CHARS = 200;

/** @param {URL} url */
export function searchFromUrl(url) {
  return [...(url.searchParams.get('q') ?? '')].slice(0, MAX_SEARCH_CHARS).join('');
}

/** The path and query of url with its search set to q, or removed when q is blank. @param {URL} url @param {string} q */
export function withSearch(url, q) {
  const params = new URLSearchParams(url.searchParams);
  if (q.trim()) params.set('q', q);
  else params.delete('q');
  const query = params.toString();
  return url.pathname + (query ? `?${query}` : '');
}

/** @typedef {'contacts' | 'companies'} Kind */

/** @type {Record<Kind, string>} */
const lastList = { contacts: '/contacts', companies: '/companies' };

/** Remembers the list a person last saw, so going back keeps their search. @param {Kind} kind @param {URL} url */
export function rememberList(kind, url) {
  lastList[kind] = url.pathname + url.search;
}

/** @param {Kind} kind */
export function listHref(kind) {
  return lastList[kind];
}
