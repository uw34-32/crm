// The state behind the pipeline board: the workspace's pipelines, the open one with its
// stages and deals, and moves that show at once and are undone if the API refuses them.

import { MAX_PAGE, type dealsApi, type Pipeline, type pipelinesApi } from '../../api/crm';
import { toSentence } from '../forms';
import { createStore } from '../store';
import { isMissing } from '../records/load';
import { appendDeals, buildColumns, planMove, removeDeal, replaceDeal, undoMove, type Column, type Deal, type Stage } from './board';

export type ClosedKind = 'won' | 'lost';
type Paging = { offset: number; hasMore: boolean; loading: boolean; error: string };

/** Open deals are all loaded, a page at a time, up to this many pages. */
export const OPEN_PAGES_MAX = 25;
/** Won and lost deals come in pages this size, on request. */
export const CLOSED_PAGE = 50;

export type BoardState = {
  status: 'loading' | 'ready' | 'error';
  error: string;
  pipelines: Pipeline[];
  pipeline: Pipeline | null;
  columns: Column[];
  closed: Record<ClosedKind, Paging>;
  /** Deals with a move in flight, which cannot be moved again until it answers. */
  busy: Record<string, boolean>;
  /** Whether open deals beyond OPEN_PAGES_MAX pages were left out. */
  truncated: boolean;
  /** The last thing a move did that the person should hear about. */
  notice: string;
};

const messageOf = (err: unknown) => (err instanceof Error ? err.message : String(err));
const freshPaging = (): Paging => ({ offset: 0, hasMore: false, loading: false, error: '' });

type Deps = {
  pipelines: Pick<ReturnType<typeof pipelinesApi>, 'list' | 'get'>;
  deals: Pick<ReturnType<typeof dealsApi>, 'list' | 'move'>;
};

export function createBoard({ pipelines, deals }: Deps) {
  const store = createStore<BoardState>({
    status: 'loading',
    error: '',
    pipelines: [],
    pipeline: null,
    columns: [],
    closed: { won: freshPaging(), lost: freshPaging() },
    busy: {},
    truncated: false,
    notice: ''
  });
  let workspaceId = '';
  /** Moves and page loads belong to one generation; a full load or stop ends it. */
  let generation = 0;
  /** Refreshes, quiet or not, supersede one another through this. */
  let loadSeq = 0;
  /** Counts the moves begun, so a refresh can tell whether one began while it was loading. */
  let moveEpoch = 0;
  /** A quiet refresh that waited for moves to settle. */
  let pending: { workspaceId: string; wanted: string } | null = null;

  async function loadOpen(pipelineId: string) {
    const items: Deal[] = [];
    for (let pageIndex = 0; pageIndex < OPEN_PAGES_MAX; pageIndex++) {
      const result = await deals.list(workspaceId, { pipeline_id: pipelineId, status: 'open', limit: MAX_PAGE, offset: items.length });
      items.push(...result.items);
      if (!result.has_more) return { items, truncated: false };
    }
    return { items, truncated: true };
  }

  const loadClosed = (pipelineId: string, kind: ClosedKind, offset: number) =>
    deals.list(workspaceId, { pipeline_id: pipelineId, status: kind, limit: CLOSED_PAGE, offset });

  function hasMoreByStage(stages: Stage[], more: Partial<Record<ClosedKind, boolean>>) {
    const flags: Record<string, boolean> = {};
    for (const stage of stages) {
      if (stage.kind !== 'open' && stage.kind in more) flags[stage.id] = Boolean(more[stage.kind as ClosedKind]);
    }
    return flags;
  }

  const moving = () => Object.keys(store.get().busy).length > 0;

  /** Runs the refresh that waited for the last move to settle. */
  function runPending() {
    if (!pending || moving()) return;
    const { workspaceId: forWorkspace, wanted } = pending;
    pending = null;
    void load(forWorkspace, wanted, { quiet: true });
  }

  /**
   * Loads the pipelines and opens the one asked for, or the first. A quiet load is a
   * refresh behind the board on screen: it keeps the columns, a failure changes
   * nothing, it leaves moves in flight alone, and it waits for them rather than show a
   * board older than the move.
   */
  async function load(forWorkspace: string, wanted = '', { quiet = false }: { quiet?: boolean } = {}) {
    workspaceId = forWorkspace;
    if (quiet && moving()) {
      pending = { workspaceId: forWorkspace, wanted };
      return;
    }
    // A full load starts over, so it ends the moves and page loads of the board it replaces.
    const mine = quiet ? generation : ++generation;
    const seq = ++loadSeq;
    const epoch = moveEpoch;
    const stale = () => mine !== generation || seq !== loadSeq;
    if (!quiet) store.set({ status: 'loading', error: '', notice: '' });
    try {
      const list = (await pipelines.list(workspaceId)).items;
      if (stale()) return;
      const chosen = list.find((pipeline) => pipeline.id === wanted) ?? list[0];
      if (!chosen) {
        store.set({ pipelines: list, pipeline: null, columns: [], status: 'ready' });
        return;
      }
      const [pipeline, open, won, lost] = await Promise.all([pipelines.get(workspaceId, chosen.id), loadOpen(chosen.id), loadClosed(chosen.id, 'won', 0), loadClosed(chosen.id, 'lost', 0)]);
      if (stale()) return;
      // A move began while this was loading, even one that has finished: the deals read
      // may predate it. Read again once the moves settle.
      if (quiet && (moving() || moveEpoch !== epoch)) {
        pending = { workspaceId, wanted };
        if (!moving()) runPending();
        return;
      }
      const stages = pipeline.stages ?? [];
      store.set({
        pipelines: list,
        pipeline,
        closed: {
          won: { ...freshPaging(), offset: won.items.length, hasMore: won.has_more },
          lost: { ...freshPaging(), offset: lost.items.length, hasMore: lost.has_more }
        },
        columns: buildColumns(stages, [...open.items, ...won.items, ...lost.items], { hasMore: hasMoreByStage(stages, { won: won.has_more, lost: lost.has_more }) }),
        truncated: open.truncated,
        busy: {},
        status: 'ready'
      });
    } catch (err) {
      if (stale() || quiet) return;
      store.set({ status: 'error', error: messageOf(err) });
    }
  }

  /**
   * Moves a deal to a stage, before another deal or at the end. The board shows the
   * move at once; if the API refuses it, the deal goes back and a notice says why. A
   * 404 means the stage or the deal is gone, so the board reloads.
   */
  async function move(dealId: string, stageId: string, before: string | null, lostReason = '') {
    const state = store.get();
    if (state.busy[dealId]) return;
    const plan = planMove(state.columns, dealId, stageId, before);
    if (!plan) return;
    moveEpoch++;
    const mine = generation;
    const kindOf = (id: string) => state.columns.find((c) => c.stage.id === id)?.stage.kind;
    store.set({ columns: plan.columns, busy: { ...state.busy, [dealId]: true }, notice: '' });
    const body = lostReason ? { ...plan.body, lost_reason: lostReason } : plan.body;
    try {
      const moved = await deals.move(workspaceId, dealId, body);
      if (mine !== generation) return;
      store.set({ columns: replaceDeal(store.get().columns, moved) });
      // A deal entering or leaving a closed stage changes what the next page starts after.
      const from = kindOf(plan.from.stageId);
      const to = kindOf(stageId);
      if (from !== to) {
        const closed = { ...store.get().closed };
        if (from && from !== 'open') closed[from] = { ...closed[from], offset: Math.max(0, closed[from].offset - 1) };
        if (to && to !== 'open') closed[to] = { ...closed[to], offset: closed[to].offset + 1 };
        store.set({ closed });
      }
    } catch (err) {
      if (mine !== generation) return;
      store.set({ columns: undoMove(store.get().columns, dealId, plan.from), notice: `The move was undone. ${toSentence(messageOf(err))}` });
      const pipeline = store.get().pipeline;
      if (isMissing(err) && pipeline) void load(workspaceId, pipeline.id);
    } finally {
      if (mine === generation) {
        const { [dealId]: _done, ...rest } = store.get().busy;
        store.set({ busy: rest });
        runPending();
      }
    }
  }

  /** Loads the next page of won or lost deals into their columns. */
  async function loadMore(kind: ClosedKind) {
    const { closed, pipeline } = store.get();
    const paging = closed[kind];
    if (!pipeline || paging.loading || !paging.hasMore) return;
    const mine = generation;
    const setPaging = (patch: Partial<Paging>) => store.set({ closed: { ...store.get().closed, [kind]: { ...store.get().closed[kind], ...patch } } });
    setPaging({ loading: true, error: '' });
    try {
      const result = await loadClosed(pipeline.id, kind, paging.offset);
      if (mine !== generation) return;
      setPaging({ offset: paging.offset + result.items.length, hasMore: result.has_more });
      store.set({ columns: appendDeals(store.get().columns, result.items, hasMoreByStage(pipeline.stages ?? [], { [kind]: result.has_more })) });
    } catch (err) {
      if (mine !== generation) return;
      setPaging({ error: messageOf(err) });
    } finally {
      if (mine === generation) setPaging({ loading: false });
    }
  }

  return {
    store,
    load,
    move,
    loadMore,
    /** Takes a deleted deal off the board. */
    forget: (dealId: string) => store.set({ columns: removeDeal(store.get().columns, dealId) }),
    stop: () => {
      generation++;
    }
  };
}

export type Board = ReturnType<typeof createBoard>;
