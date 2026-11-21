// Pipelines, deals, activities, and members of one workspace; see the README's route
// table. Lists answer with items, limit, offset, and has_more.

import type { Client } from './client';
import type { Page } from './records';
import type { Deal, Stage } from '../lib/deals/board';
import type { Activity } from '../lib/activities/timeline';

/** The largest page the API serves. */
export const MAX_PAGE = 200;

export type Pipeline = { id: string; name: string; stages?: Stage[] };
export type Member = { user_id: string; email: string; name?: string; role: 'owner' | 'admin' | 'member' };

const base = (workspaceId: string, resource: string) => `/workspaces/${encodeURIComponent(workspaceId)}/${resource}`;
const one = (workspaceId: string, resource: string, id: string) => `${base(workspaceId, resource)}/${encodeURIComponent(id)}`;

/** limit and offset always go; a filter goes only when it has a value. */
function withQuery(path: string, { limit, offset, ...filters }: { limit: number; offset: number } & Record<string, string | number | undefined | null>) {
  const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== null && value !== '') query.set(key, String(value));
  }
  return `${path}?${query}`;
}

type Api = Pick<Client, 'request'>;

export function pipelinesApi(client: Api) {
  return {
    /** Every pipeline, without stages; a workspace has far fewer than a page. */
    list: (workspaceId: string) => client.request<Page<Pipeline>>(withQuery(base(workspaceId, 'pipelines'), { limit: MAX_PAGE, offset: 0 })),
    /** One pipeline with its stages in order. */
    get: (workspaceId: string, id: string) => client.request<Pipeline>(one(workspaceId, 'pipelines', id))
  };
}

export type DealQuery = { pipeline_id?: string; stage_id?: string; owner_id?: string; status?: string; q?: string; limit: number; offset: number };

export function dealsApi(client: Api) {
  return {
    list: (workspaceId: string, query: DealQuery) => client.request<Page<Deal>>(withQuery(base(workspaceId, 'deals'), query)),
    get: (workspaceId: string, id: string) => client.request<Deal>(one(workspaceId, 'deals', id)),
    create: (workspaceId: string, body: Record<string, unknown>) => client.request<Deal>(base(workspaceId, 'deals'), { method: 'POST', body }),
    update: (workspaceId: string, id: string, body: Record<string, unknown>) => client.request<Deal>(one(workspaceId, 'deals', id), { method: 'PATCH', body }),
    move: (workspaceId: string, id: string, body: { stage_id: string; position?: number; lost_reason?: string }) =>
      client.request<Deal>(`${one(workspaceId, 'deals', id)}/move`, { method: 'POST', body }),
    remove: (workspaceId: string, id: string) => client.request<null>(one(workspaceId, 'deals', id), { method: 'DELETE' })
  };
}

export type ActivityQuery = { deal_id?: string; contact_id?: string; company_id?: string; limit: number; offset: number };

export function activitiesApi(client: Api) {
  return {
    /** Newest first. */
    list: (workspaceId: string, query: ActivityQuery) => client.request<Page<Activity>>(withQuery(base(workspaceId, 'activities'), query)),
    create: (workspaceId: string, body: Record<string, unknown>) => client.request<Activity>(base(workspaceId, 'activities'), { method: 'POST', body }),
    update: (workspaceId: string, id: string, body: Record<string, unknown>) => client.request<Activity>(one(workspaceId, 'activities', id), { method: 'PATCH', body }),
    complete: (workspaceId: string, id: string) => client.request<Activity>(`${one(workspaceId, 'activities', id)}/complete`, { method: 'POST' }),
    reopen: (workspaceId: string, id: string) => client.request<Activity>(`${one(workspaceId, 'activities', id)}/reopen`, { method: 'POST' }),
    remove: (workspaceId: string, id: string) => client.request<null>(one(workspaceId, 'activities', id), { method: 'DELETE' })
  };
}

export function membersApi(client: Api) {
  return {
    /** Every member, as one list. */
    list: (workspaceId: string) => client.request<Member[]>(base(workspaceId, 'members'))
  };
}
