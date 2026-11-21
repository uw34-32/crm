// The open workspace's members, for owners, assignees, and author names. Loaded once per
// workspace and kept; a failed load can be retried.

import { useEffect } from 'react';
import { membersApi, type Member } from '../api/crm';
import { session } from '../services';
import { createStore, useStore } from './store';

type State = { workspaceId: string | null; list: Member[]; status: 'idle' | 'loading' | 'ready' | 'error'; error: string };

const store = createStore<State>({ workspaceId: null, list: [], status: 'idle', error: '' });
const api = membersApi(session.api);

export async function loadMembers(workspaceId: string) {
  store.set({ workspaceId, list: [], status: 'loading', error: '' });
  try {
    const list = await api.list(workspaceId);
    if (store.get().workspaceId === workspaceId) store.set({ list, status: 'ready' });
  } catch (err) {
    if (store.get().workspaceId === workspaceId) store.set({ status: 'error', error: err instanceof Error ? err.message : String(err) });
  }
}

/** The members of a workspace, loading them when this is the first screen to ask. */
export function useMembers(workspaceId: string | null) {
  const state = useStore(store);
  useEffect(() => {
    if (workspaceId && store.get().workspaceId !== workspaceId) void loadMembers(workspaceId);
  }, [workspaceId]);
  const mine = state.workspaceId === workspaceId;
  return { members: mine ? state.list : [], status: mine ? state.status : 'loading', retry: () => workspaceId && loadMembers(workspaceId) };
}
