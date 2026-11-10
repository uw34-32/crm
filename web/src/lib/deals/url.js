// The board keeps the open pipeline in its URL as ?pipeline=, so a reload, a shared
// link, and the back button all return to it.

/** @param {URL} url */
export function pipelineFromUrl(url) {
  return url.searchParams.get('pipeline') ?? '';
}

/** The path and query of url with its pipeline set, or removed when blank. @param {URL} url @param {string} id */
export function withPipeline(url, id) {
  const params = new URLSearchParams(url.searchParams);
  if (id) params.set('pipeline', id);
  else params.delete('pipeline');
  const query = params.toString();
  return url.pathname + (query ? `?${query}` : '');
}
