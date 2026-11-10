// The state behind the pipeline board: the workspace's pipelines, the open one with
// its stages and deals, and moves that show at once and are undone if the API refuses
// them. Call it while a component initializes, since it sets up effects.

import { untrack } from 'svelte';
import { goto } from '$app/navigation';
import { page } from '$app/state';
import { MAX_PAGE, dealsApi, pipelinesApi } from '../api/crm.js';
import { toSentence } from '../forms.js';
import { isMissing } from '../records/load.js';
import { rememberList } from '../records/url.js';
import { api } from '../session.svelte.js';
import { showToast } from '../toast.svelte.js';
import { workspaces } from '../workspaces.svelte.js';
import { appendDeals, buildColumns, planMove, removeDeal, replaceDeal, undoMove } from './board.js';
import { pipelineFromUrl, withPipeline } from './url.js';

/** @typedef {import('./board.js').Column} Column */
/** @typedef {import('./board.js').Deal} Deal */
/** @typedef {import('./board.js').Stage} Stage */
/** @typedef {{ id: string, name: string, stages?: Stage[] }} Pipeline */
/** @typedef {'won' | 'lost'} ClosedKind */
/** @typedef {{ offset: number, hasMore: boolean, loading: boolean, error: string }} Paging */

/** Open deals are all loaded, a page at a time, up to this many pages. */
export const OPEN_PAGES_MAX = 25;
/** Won and lost deals come in pages this size, on request. */
export const CLOSED_PAGE = 50;

/** @param {unknown} err */
const messageOf = (err) => (err instanceof Error ? err.message : String(err));

/** @returns {Paging} */
const freshPaging = () => ({ offset: 0, hasMore: false, loading: false, error: '' });

export function boardScreen() {
  const pipelines = pipelinesApi(api);
  const deals = dealsApi(api);

  const board = $state({
    /** @type {'loading' | 'ready' | 'error'} */
    status: 'loading',
    error: '',
    /** @type {Pipeline[]} */
    pipelines: [],
    /** @type {Pipeline | null} */
    pipeline: null,
    /** @type {Column[]} */
    columns: [],
    /** @type {Record<ClosedKind, Paging>} */
    closed: { won: freshPaging(), lost: freshPaging() },
    /** Deals with a move in flight, which cannot be moved again until it answers. */
    /** @type {Record<string, boolean>} */
    busy: {},
    /** Whether open deals beyond OPEN_PAGES_MAX pages were left out. */
    truncated: false
  });

  let workspaceId = '';
  let generation = 0;
  /** What the last load was asked for, so the URL echo of a chosen pipeline does not load twice. */
  let requested = { workspaceId: '', pipelineId: '' };

  /** Every open deal of a pipeline, in board order, up to the page cap. @param {string} pipelineId */
  async function loadOpen(pipelineId) {
    /** @type {Deal[]} */
    const items = [];
    for (let pageIndex = 0; pageIndex < OPEN_PAGES_MAX; pageIndex++) {
      const result = await deals.list(workspaceId, { pipeline_id: pipelineId, status: 'open', limit: MAX_PAGE, offset: items.length });
      items.push(...result.items);
      if (!result.has_more) return { items, truncated: false };
    }
    return { items, truncated: true };
  }

  /** @param {string} pipelineId @param {ClosedKind} kind @param {number} offset */
  const loadClosed = (pipelineId, kind, offset) => deals.list(workspaceId, { pipeline_id: pipelineId, status: kind, limit: CLOSED_PAGE, offset });

  /** @param {Stage[]} stages @param {Partial<Record<ClosedKind, boolean>>} more */
  function hasMoreByStage(stages, more) {
    /** @type {Record<string, boolean>} */
    const flags = {};
    for (const stage of stages) {
      if (stage.kind !== 'open' && stage.kind in more) flags[stage.id] = Boolean(more[stage.kind]);
    }
    return flags;
  }

  /**
   * Loads the pipelines and opens the one asked for, or the first when the URL names
   * none or one this workspace does not have, writing the choice back to the URL.
   * @param {string} wanted
   */
  async function load(wanted) {
    const mine = ++generation;
    requested = { workspaceId, pipelineId: wanted };
    board.status = 'loading';
    board.error = '';
    try {
      const list = /** @type {Pipeline[]} */ ((await pipelines.list(workspaceId)).items);
      if (mine !== generation) return;
      board.pipelines = list;
      const chosen = list.find((pipeline) => pipeline.id === wanted) ?? list[0];
      if (!chosen) {
        board.pipeline = null;
        board.columns = [];
        board.status = 'ready';
        return;
      }
      if (chosen.id !== wanted) {
        requested = { workspaceId, pipelineId: chosen.id };
        goto(withPipeline(page.url, chosen.id), { replaceState: true, keepFocus: true, noScroll: true });
      }
      const [pipeline, open, won, lost] = await Promise.all([
        /** @type {Promise<Pipeline>} */ (pipelines.get(workspaceId, chosen.id)),
        loadOpen(chosen.id),
        loadClosed(chosen.id, 'won', 0),
        loadClosed(chosen.id, 'lost', 0)
      ]);
      if (mine !== generation) return;
      const stages = pipeline.stages ?? [];
      board.pipeline = pipeline;
      board.closed = {
        won: { ...freshPaging(), offset: won.items.length, hasMore: won.has_more },
        lost: { ...freshPaging(), offset: lost.items.length, hasMore: lost.has_more }
      };
      board.columns = buildColumns(stages, [...open.items, ...won.items, ...lost.items], {
        hasMore: hasMoreByStage(stages, { won: won.has_more, lost: lost.has_more })
      });
      board.truncated = open.truncated;
      board.busy = {};
      board.status = 'ready';
    } catch (err) {
      if (mine !== generation) return;
      board.status = 'error';
      board.error = messageOf(err);
    }
  }

  $effect(() => {
    const id = workspaces.current?.id;
    const wanted = pipelineFromUrl(page.url);
    rememberList('pipeline', page.url);
    if (!id) return;
    untrack(() => {
      if (requested.workspaceId === id && requested.pipelineId === wanted && board.status !== 'error') return;
      workspaceId = id;
      load(wanted);
    });
  });

  $effect(() => () => {
    generation++;
  });

  /**
   * Moves a deal to a stage, before another deal or at the end. The board shows the
   * move at once; if the API refuses it, the deal goes back and a toast says why. A
   * 404 means the stage or the deal is gone, so the board reloads.
   * @param {string} dealId
   * @param {string} stageId
   * @param {string | null} before
   * @param {string} [lostReason]
   */
  async function move(dealId, stageId, before, lostReason = '') {
    if (board.busy[dealId]) return;
    const plan = planMove(board.columns, dealId, stageId, before);
    if (!plan) return;
    const mine = generation;
    board.columns = plan.columns;
    board.busy[dealId] = true;
    const body = lostReason ? { ...plan.body, lost_reason: lostReason } : plan.body;
    try {
      const moved = /** @type {Deal} */ (await deals.move(workspaceId, dealId, body));
      if (mine !== generation) return;
      board.columns = replaceDeal(board.columns, moved);
    } catch (err) {
      if (mine !== generation) return;
      board.columns = undoMove(board.columns, dealId, plan.from);
      showToast(`The move was undone. ${toSentence(messageOf(err))}`, 5000);
      if (isMissing(err) && board.pipeline) load(board.pipeline.id);
    } finally {
      if (mine === generation) delete board.busy[dealId];
    }
  }

  /** Loads the next page of won or lost deals into their columns. @param {ClosedKind} kind */
  async function loadMore(kind) {
    const paging = board.closed[kind];
    const pipeline = board.pipeline;
    if (!pipeline || paging.loading || !paging.hasMore) return;
    const mine = generation;
    paging.loading = true;
    paging.error = '';
    try {
      const result = await loadClosed(pipeline.id, kind, paging.offset);
      if (mine !== generation) return;
      paging.offset += result.items.length;
      paging.hasMore = result.has_more;
      board.columns = appendDeals(board.columns, result.items, hasMoreByStage(pipeline.stages ?? [], { [kind]: result.has_more }));
    } catch (err) {
      if (mine !== generation) return;
      paging.error = messageOf(err);
    } finally {
      if (mine === generation) paging.loading = false;
    }
  }

  return {
    board,
    /** Opens another pipeline, as a new history entry so the back button returns. @param {string} id */
    choose: (id) => goto(withPipeline(page.url, id), { keepFocus: true, noScroll: true }),
    reload: () => load(board.pipeline?.id ?? pipelineFromUrl(page.url)),
    move,
    loadMore,
    /** Takes a deleted deal off the board. @param {string} dealId */
    forget: (dealId) => {
      board.columns = removeDeal(board.columns, dealId);
    }
  };
}
