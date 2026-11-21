// The state behind list, detail, and form screens, shared by contacts and companies.

import { useEffect, useRef, useState } from 'react';
import { router } from 'expo-router';
import type { Page } from '../../api/records';
import { useStore } from '../store';
import { workspaces } from '../../services';
import { changes } from './changes';
import { createListStore, createPager, type FetchPage } from './list';
import { loadRecord, type Loaded } from './load';

const SEARCH_DELAY_MS = 300;

/** The open workspace's ID, or null while there is none. */
export function useWorkspaceId() {
  return useStore(workspaces.state).current?.id ?? null;
}

/**
 * A searchable, paged list of the open workspace. Typing updates the field at once and
 * the search after a pause. A change to any record anywhere reloads it.
 */
export function useRecordList<T>(fetchPage: (workspaceId: string, params: Parameters<FetchPage<T>>[0]) => Promise<Page<T>>) {
  const workspaceId = useWorkspaceId();
  const { version } = useStore(changes);
  const latest = useRef({ fetchPage, workspaceId });
  latest.current = { fetchPage, workspaceId };
  const [{ store, pager }] = useState(() => {
    const listStore = createListStore<T>();
    const listPager = createPager(listStore, (params) => latest.current.fetchPage(latest.current.workspaceId ?? '', params));
    return { store: listStore, pager: listPager };
  });
  const state = useStore(store);
  const [search, setSearch] = useState('');
  const [q, setQ] = useState('');

  useEffect(() => {
    const timer = setTimeout(() => setQ(search), SEARCH_DELAY_MS);
    return () => clearTimeout(timer);
  }, [search]);

  // A new search or workspace starts over; a change to some record refreshes behind the rows.
  const loadedVersion = useRef(version);
  useEffect(() => {
    const quiet = loadedVersion.current !== version;
    loadedVersion.current = version;
    if (workspaceId) void pager.load(q, { quiet });
  }, [workspaceId, q, version, pager]);

  useEffect(() => () => pager.stop(), [pager]);

  return {
    state,
    search,
    setSearch,
    more: pager.more,
    retry: () => pager.load(q)
  };
}

type RecordState<T> = Loaded<T> | { status: 'loading'; record: null; error: '' };

/** One record of the open workspace, loaded again when its ID or any record changes. */
export function useRecord<T>(fetchOne: (workspaceId: string, id: string) => Promise<T>, id: string, { paused = false }: { paused?: boolean } = {}) {
  const workspaceId = useWorkspaceId();
  const { version } = useStore(changes);
  const latest = useRef(fetchOne);
  latest.current = fetchOne;
  const [state, setState] = useState<RecordState<T>>({ status: 'loading', record: null, error: '' });
  const generation = useRef(0);
  const shown = useRef('');

  async function load() {
    if (!workspaceId || paused) return;
    const mine = ++generation.current;
    // Keep what is on screen while it loads again after an edit, but never another record's.
    const key = `${workspaceId}/${id}`;
    const same = shown.current === key;
    shown.current = key;
    setState((now) => (same && now.status === 'ready' ? now : { status: 'loading', record: null, error: '' }));
    const got = await loadRecord(() => latest.current(workspaceId, id));
    if (mine === generation.current) setState(got);
  }

  useEffect(() => {
    void load();
    return () => {
      generation.current++;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId, id, version, paused]);

  return { state, workspaceId, reload: load };
}

/**
 * Leaves a screen that belongs to one workspace when another is opened: a record of the
 * old workspace does not exist in the new one.
 */
export function useLeaveOnWorkspaceSwitch(listHref: '/contacts' | '/companies') {
  const workspaceId = useWorkspaceId();
  // The workspace this screen first saw, which may arrive after it mounts.
  const first = useRef(workspaceId);
  useEffect(() => {
    if (!workspaceId) return;
    if (!first.current) first.current = workspaceId;
    else if (workspaceId !== first.current) router.dismissTo(listHref);
  }, [workspaceId, listHref]);
  return workspaceId;
}
