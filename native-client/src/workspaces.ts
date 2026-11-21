// The user's workspaces and the one that is open. The choice is saved per user, so two
// people sharing a device each come back to their own.

import type { Client } from './api/client';
import type { AsyncStorage } from './api/tokens';
import { createStore } from './lib/store';

export type Role = 'owner' | 'admin' | 'member';
export type Workspace = { id: string; name: string; role: Role; created_at: string; updated_at: string };

export type WorkspacesState = {
  list: Workspace[];
  current: Workspace | null;
  status: 'idle' | 'loading' | 'ready' | 'error';
  error: string;
  /** The user the list belongs to, so another user's sign-in starts over. */
  userId: string | null;
};

const keyFor = (userId: string) => `crm.workspace.${userId}`;

/** The saved workspace while the user still belongs to it, else the first one, else none. */
export function pickWorkspace<W extends { id: string }>(workspaces: W[], savedId: string | null): W | null {
  return workspaces.find((w) => w.id === savedId) ?? workspaces[0] ?? null;
}

export function createWorkspaces({ api, storage }: { api: Pick<Client, 'request'>; storage: AsyncStorage }) {
  const state = createStore<WorkspacesState>({ list: [], current: null, status: 'idle', error: '', userId: null });

  async function readSelection(userId: string) {
    try {
      return await storage.get(keyFor(userId));
    } catch {
      return null;
    }
  }

  async function saveSelection(userId: string, workspaceId: string | null) {
    try {
      if (workspaceId) await storage.set(keyFor(userId), workspaceId);
      else await storage.remove(keyFor(userId));
    } catch {
      // Without storage the choice lasts until the app closes.
    }
  }

  function open(workspace: Workspace | null) {
    state.set({ current: workspace });
    const { userId } = state.get();
    if (userId) void saveSelection(userId, workspace?.id ?? null);
  }

  /** Loads the user's workspaces and opens the saved one, or the first. */
  async function load(userId: string) {
    state.set({ userId, status: 'loading', error: '' });
    try {
      const list = await api.request<Workspace[]>('/workspaces');
      const saved = await readSelection(userId);
      if (state.get().userId !== userId) return;
      state.set({ list });
      open(pickWorkspace(list, saved));
      state.set({ status: 'ready' });
    } catch (err) {
      if (state.get().userId !== userId) return;
      state.set({ status: 'error', error: err instanceof Error ? err.message : String(err) });
    }
  }

  function select(id: string) {
    const workspace = state.get().list.find((w) => w.id === id);
    if (workspace) open(workspace);
  }

  /** Creates a workspace, with the caller as its owner, and opens it. */
  async function create(name: string) {
    const userId = state.get().userId;
    const workspace = await api.request<Workspace>('/workspaces', { method: 'POST', body: { name } });
    // The user may have signed out while this was in flight; the workspace exists on the
    // server, and the next load of its owner will find it.
    if (state.get().userId !== userId) return workspace;
    state.set({ list: [...state.get().list, workspace] });
    open(workspace);
    return workspace;
  }

  /** Forgets the list on sign-out; the saved choice stays for the user's next visit. */
  function reset() {
    state.set({ list: [], current: null, status: 'idle', error: '', userId: null });
  }

  return { state, load, select, create, reset };
}

export type WorkspacesService = ReturnType<typeof createWorkspaces>;
