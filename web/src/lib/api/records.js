// Contacts and companies of one workspace. Both resources share their routes' shape,
// paging, and rules; see the README.

/**
 * @typedef {{ request: (path: string, options?: import('./client.js').RequestOptions) => Promise<unknown> }} Client
 * @typedef {{ items: any[], limit: number, offset: number, has_more: boolean }} Page
 */

/**
 * @param {Client} client
 * @param {'contacts' | 'companies'} kind
 */
export function recordsApi(client, kind) {
  const base = (/** @type {string} */ workspaceId) => `/workspaces/${encodeURIComponent(workspaceId)}/${kind}`;
  const one = (/** @type {string} */ workspaceId, /** @type {string} */ id) => `${base(workspaceId)}/${encodeURIComponent(id)}`;

  return {
    /** @param {string} workspaceId @param {{ q: string, limit: number, offset: number }} params @returns {Promise<Page>} */
    list(workspaceId, { q, limit, offset }) {
      const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      if (q) params.set('q', q);
      return /** @type {Promise<Page>} */ (client.request(`${base(workspaceId)}?${params}`));
    },
    /** @param {string} workspaceId @param {string} id */
    get: (workspaceId, id) => client.request(one(workspaceId, id)),
    /** @param {string} workspaceId @param {Record<string, unknown>} body */
    create: (workspaceId, body) => client.request(base(workspaceId), { method: 'POST', body }),
    /** @param {string} workspaceId @param {string} id @param {Record<string, unknown>} body */
    update: (workspaceId, id, body) => client.request(one(workspaceId, id), { method: 'PATCH', body }),
    /** @param {string} workspaceId @param {string} id */
    remove: (workspaceId, id) => client.request(one(workspaceId, id), { method: 'DELETE' })
  };
}
