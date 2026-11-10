// Pipelines, deals, activities, and members of one workspace; see the README's route
// table. Lists answer with items, limit, offset, and has_more.

/**
 * @typedef {{ request: (path: string, options?: import('./client.js').RequestOptions) => Promise<unknown> }} Client
 * @typedef {{ items: any[], limit: number, offset: number, has_more: boolean }} Page
 */

/** The largest page the API serves. */
export const MAX_PAGE = 200;

/** @param {string} workspaceId @param {string} resource */
const base = (workspaceId, resource) => `/workspaces/${encodeURIComponent(workspaceId)}/${resource}`;

/** @param {string} workspaceId @param {string} resource @param {string} id */
const one = (workspaceId, resource, id) => `${base(workspaceId, resource)}/${encodeURIComponent(id)}`;

/**
 * limit and offset always go; a filter goes only when it has a value.
 * @param {string} path
 * @param {Record<string, string | number | undefined | null>} params
 */
function withQuery(path, { limit, offset, ...filters }) {
  const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== null && value !== '') query.set(key, String(value));
  }
  return `${path}?${query}`;
}

/** @param {Client} client */
export function pipelinesApi(client) {
  return {
    /** Every pipeline, without stages; a workspace has far fewer than a page. @param {string} workspaceId @returns {Promise<Page>} */
    list: (workspaceId) => /** @type {Promise<Page>} */ (client.request(withQuery(base(workspaceId, 'pipelines'), { limit: MAX_PAGE, offset: 0 }))),
    /** One pipeline with its stages in order. @param {string} workspaceId @param {string} id */
    get: (workspaceId, id) => client.request(one(workspaceId, 'pipelines', id))
  };
}

/**
 * @typedef {{ pipeline_id?: string, stage_id?: string, owner_id?: string, status?: string, q?: string, limit: number, offset: number }} DealQuery
 */

/** @param {Client} client */
export function dealsApi(client) {
  return {
    /** @param {string} workspaceId @param {DealQuery} query @returns {Promise<Page>} */
    list: (workspaceId, query) => /** @type {Promise<Page>} */ (client.request(withQuery(base(workspaceId, 'deals'), query))),
    /** @param {string} workspaceId @param {string} id */
    get: (workspaceId, id) => client.request(one(workspaceId, 'deals', id)),
    /** @param {string} workspaceId @param {Record<string, unknown>} body */
    create: (workspaceId, body) => client.request(base(workspaceId, 'deals'), { method: 'POST', body }),
    /** @param {string} workspaceId @param {string} id @param {Record<string, unknown>} body */
    update: (workspaceId, id, body) => client.request(one(workspaceId, 'deals', id), { method: 'PATCH', body }),
    /** @param {string} workspaceId @param {string} id @param {{ stage_id: string, position?: number, lost_reason?: string }} body */
    move: (workspaceId, id, body) => client.request(`${one(workspaceId, 'deals', id)}/move`, { method: 'POST', body }),
    /** @param {string} workspaceId @param {string} id */
    remove: (workspaceId, id) => client.request(one(workspaceId, 'deals', id), { method: 'DELETE' })
  };
}

/**
 * @typedef {{ deal_id?: string, contact_id?: string, company_id?: string, limit: number, offset: number }} ActivityQuery
 */

/** @param {Client} client */
export function activitiesApi(client) {
  return {
    /** Newest first. @param {string} workspaceId @param {ActivityQuery} query @returns {Promise<Page>} */
    list: (workspaceId, query) => /** @type {Promise<Page>} */ (client.request(withQuery(base(workspaceId, 'activities'), query))),
    /** @param {string} workspaceId @param {Record<string, unknown>} body */
    create: (workspaceId, body) => client.request(base(workspaceId, 'activities'), { method: 'POST', body }),
    /** @param {string} workspaceId @param {string} id @param {Record<string, unknown>} body */
    update: (workspaceId, id, body) => client.request(one(workspaceId, 'activities', id), { method: 'PATCH', body }),
    /** @param {string} workspaceId @param {string} id */
    complete: (workspaceId, id) => client.request(`${one(workspaceId, 'activities', id)}/complete`, { method: 'POST' }),
    /** @param {string} workspaceId @param {string} id */
    reopen: (workspaceId, id) => client.request(`${one(workspaceId, 'activities', id)}/reopen`, { method: 'POST' }),
    /** @param {string} workspaceId @param {string} id */
    remove: (workspaceId, id) => client.request(one(workspaceId, 'activities', id), { method: 'DELETE' })
  };
}

/** @typedef {{ user_id: string, email: string, name?: string, role: 'owner' | 'admin' | 'member' }} Member */

/** @param {Client} client */
export function membersApi(client) {
  return {
    /** Every member, as one list. @param {string} workspaceId @returns {Promise<Member[]>} */
    list: (workspaceId) => /** @type {Promise<Member[]>} */ (client.request(base(workspaceId, 'members')))
  };
}
