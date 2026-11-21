// Contacts and companies of one workspace. Both resources share their routes' shape,
// paging, and rules; see the README.

import type { Client } from './client';

export type Contact = {
  id: string;
  first_name: string;
  last_name?: string | null;
  email?: string | null;
  phone?: string | null;
  title?: string | null;
  company_id?: string | null;
  company_name?: string | null;
  created_at: string;
  updated_at: string;
};

export type Company = {
  id: string;
  name: string;
  domain?: string | null;
  phone?: string | null;
  industry?: string | null;
  address?: string | null;
  created_at: string;
  updated_at: string;
};

export type Page<T> = { items: T[]; limit: number; offset: number; has_more: boolean };
export type Kind = 'contacts' | 'companies';
type Records<K extends Kind> = K extends 'contacts' ? Contact : Company;

export function recordsApi<K extends Kind>(client: Pick<Client, 'request'>, kind: K) {
  type T = Records<K>;
  const base = (workspaceId: string) => `/workspaces/${encodeURIComponent(workspaceId)}/${kind}`;
  const one = (workspaceId: string, id: string) => `${base(workspaceId)}/${encodeURIComponent(id)}`;

  return {
    list(workspaceId: string, { q, limit, offset }: { q: string; limit: number; offset: number }) {
      const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
      if (q) params.set('q', q);
      return client.request<Page<T>>(`${base(workspaceId)}?${params}`);
    },
    get: (workspaceId: string, id: string) => client.request<T>(one(workspaceId, id)),
    create: (workspaceId: string, body: Record<string, unknown>) => client.request<T>(base(workspaceId), { method: 'POST', body }),
    update: (workspaceId: string, id: string, body: Record<string, unknown>) => client.request<T>(one(workspaceId, id), { method: 'PATCH', body }),
    remove: (workspaceId: string, id: string) => client.request<null>(one(workspaceId, id), { method: 'DELETE' })
  };
}

export type RecordsApi<K extends Kind> = ReturnType<typeof recordsApi<K>>;
