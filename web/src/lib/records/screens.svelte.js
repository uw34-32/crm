// The state behind list, detail, and form screens, shared by contacts and companies.
// Each function sets up effects, so call it while a component initializes.

import { tick, untrack } from 'svelte';
import { goto } from '$app/navigation';
import { page } from '$app/state';
import { debounce } from '../debounce.js';
import { focusMainHeading } from '../focus.js';
import { workspaces } from '../workspaces.svelte.js';
import { createPager, emptyList } from './list.js';
import { loadRecord } from './load.js';
import { listHref, rememberList, searchFromUrl, withSearch } from './url.js';

/** @typedef {import('./url.js').Kind} Kind */
/** @typedef {ReturnType<typeof import('../api/records.js').recordsApi>} RecordsApi */

const SEARCH_DELAY_MS = 300;

/**
 * A list with its search in the URL. Typing updates the field at once and the URL
 * after a pause, replacing the history entry; the URL then drives the request, so a
 * reload, a shared link, and the back button all show the same search.
 * @param {Kind} kind
 * @param {RecordsApi} api
 */
export function listScreen(kind, api) {
  const list = $state(/** @type {import('./list.js').ListState<any>} */ (emptyList()));
  const view = $state({ search: searchFromUrl(page.url) });
  let workspaceId = '';
  /** The search this screen last wrote to the URL, so its echo does not overwrite typing. */
  /** @type {string | null} */
  let written = null;
  const pager = createPager(list, (params) => api.list(workspaceId, params));

  /** @param {string} q */
  function writeUrl(q) {
    written = q;
    goto(withSearch(page.url, q), { replaceState: true, keepFocus: true, noScroll: true });
  }
  const writeLater = debounce(writeUrl, SEARCH_DELAY_MS);

  $effect(() => {
    const id = workspaces.current?.id;
    const q = searchFromUrl(page.url);
    rememberList(kind, page.url);
    if (!id) return;
    untrack(() => {
      if (q !== written) view.search = q;
      written = null;
      workspaceId = id;
      pager.load(q);
    });
  });

  $effect(() => () => {
    writeLater.cancel();
    pager.stop();
  });

  return {
    list,
    view,
    /** @param {string} q */
    setSearch(q) {
      view.search = q;
      writeLater(q);
    },
    clearSearch() {
      writeLater.cancel();
      view.search = '';
      writeUrl('');
    },
    more: pager.more,
    retry: () => pager.load(list.q)
  };
}

/**
 * Leaves a screen that belongs to one workspace when another is opened: a record of
 * the old workspace does not exist in the new one.
 * @param {Kind} kind
 */
export function leaveOnWorkspaceSwitch(kind) {
  const workspaceId = workspaces.current?.id ?? '';
  $effect(() => {
    const now = workspaces.current?.id;
    if (now && now !== workspaceId) untrack(() => goto(`/${kind}`, { replaceState: true }));
  });
  return workspaceId;
}

/**
 * One record, loaded for the workspace that is open, and reloaded when the ID in the
 * route changes.
 * @param {Kind} kind
 * @param {RecordsApi} api
 * @param {() => string} getId
 */
export function recordScreen(kind, api, getId) {
  const workspaceId = leaveOnWorkspaceSwitch(kind);
  const state = $state(/** @type {import('./load.js').Loaded<any> | { status: 'loading', record: null, error: '' }} */ ({ status: 'loading', record: null, error: '' }));
  let generation = 0;

  async function load() {
    const id = getId();
    const mine = ++generation;
    state.status = 'loading';
    state.error = '';
    const got = await loadRecord(() => api.get(workspaceId, id));
    if (mine !== generation) return;
    Object.assign(state, got);
    // The heading appears only now; navigation left focus on main while it loaded.
    await tick();
    const main = document.getElementById('main');
    if (main && document.activeElement === main) focusMainHeading(main);
  }

  $effect(() => {
    getId();
    untrack(load);
  });

  $effect(() => () => {
    generation++;
  });

  return { state, workspaceId, reload: load, backHref: () => listHref(kind) };
}
