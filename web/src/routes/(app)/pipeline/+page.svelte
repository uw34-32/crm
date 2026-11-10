<script>
  import { Plus } from '@lucide/svelte';
  import { tick, untrack } from 'svelte';
  import DealCard from '$lib/components/DealCard.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import MoveDealDialog from '$lib/components/MoveDealDialog.svelte';
  import { stageTotals } from '$lib/deals/board.js';
  import { boardScreen } from '$lib/deals/board.svelte.js';
  import { formatMoney } from '$lib/deals/money.js';
  import { loadMembers, members } from '$lib/members.svelte.js';
  import { memberName } from '$lib/activities/timeline.js';
  import { workspaces } from '$lib/workspaces.svelte.js';

  /** @typedef {import('$lib/deals/board.js').Column} Column */
  /** @typedef {import('$lib/deals/board.js').Deal} Deal */

  const screen = boardScreen();
  const board = screen.board;

  $effect(() => {
    const id = workspaces.current?.id;
    if (id) untrack(() => loadMembers(id));
  });

  const empty = $derived(board.status === 'ready' && board.columns.every((column) => column.deals.length === 0));

  /** @param {Deal} deal */
  const ownerOf = (deal) => (members.status === 'ready' && deal.owner_id ? memberName(members.list, deal.owner_id) : '');

  /** @param {Column} column */
  const totalOf = (column) =>
    stageTotals(column.deals)
      .map((total) => formatMoney(total.minor, total.currency, undefined, { compact: true }))
      .join(' · ');

  // Drag and drop with the browser's own drag events. The drop target is the deal the
  // dragged one would land before, or null for the end of a column.
  /** @type {string | null} */
  let dragging = $state(null);
  /** @type {{ stageId: string, before: string | null } | null} */
  let over = $state(null);

  /** @param {DragEvent} event @param {Deal} deal */
  function dragStart(event, deal) {
    if (board.busy[deal.id]) {
      event.preventDefault();
      return;
    }
    dragging = deal.id;
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', deal.id);
    }
  }

  function dragEnd() {
    dragging = null;
    over = null;
  }

  /** @param {DragEvent} event @param {Column} column */
  function dragOverColumn(event, column) {
    if (!dragging) return;
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
    const card = event.target instanceof Element ? event.target.closest('[data-deal-id]') : null;
    if (!card) {
      over = { stageId: column.stage.id, before: null };
      return;
    }
    const id = card.getAttribute('data-deal-id') ?? '';
    if (id === dragging) {
      over = null;
      return;
    }
    const rect = card.getBoundingClientRect();
    const upperHalf = event.clientY < rect.top + rect.height / 2;
    const index = column.deals.findIndex((deal) => deal.id === id);
    const before = upperHalf ? id : (column.deals[index + 1]?.id ?? null);
    over = { stageId: column.stage.id, before: before === dragging ? null : before };
  }

  /** @param {DragEvent} event @param {Column} column */
  function drop(event, column) {
    event.preventDefault();
    const id = dragging;
    const target = over;
    dragEnd();
    if (!id || !target || target.stageId !== column.stage.id) return;
    const deal = board.columns.flatMap((c) => c.deals).find((d) => d.id === id);
    if (!deal) return;
    if (column.stage.kind === 'lost' && deal.stage_id !== column.stage.id) {
      // The reason is required, so the move waits for the dialog.
      moving = { deal, preset: target };
      return;
    }
    screen.move(id, target.stageId, target.before);
  }

  /** @type {{ deal: Deal, preset: { stageId: string, before: string | null } | null } | null} */
  let moving = $state(null);

  /** @param {{ stageId: string, before: string | null, lostReason: string }} target */
  function moveFromDialog({ stageId, before, lostReason }) {
    const deal = moving?.deal;
    moving = null;
    if (!deal) return;
    screen.move(deal.id, stageId, before, lostReason);
    // The card may have been rebuilt in another column; keyboard users stay on it.
    tick().then(() => {
      const button = document.querySelector(`[data-deal-id="${deal.id}"] button`);
      if (button instanceof HTMLElement) button.focus();
    });
  }

  /** @param {Column} column */
  const closedKind = (column) => (column.stage.kind === 'won' || column.stage.kind === 'lost' ? column.stage.kind : null);
</script>

<svelte:head>
  <title>{board.pipeline?.name ?? 'Pipeline'} · {workspaces.current?.name ?? 'CRM'}</title>
</svelte:head>

<div class="animate-in">
  <div class="page-header">
    <div class="heading">
      <h1 class="page-title">Pipeline</h1>
      {#if board.pipelines.length > 1}
        <label class="sr-only" for="pipeline-select">Pipeline</label>
        <select
          id="pipeline-select"
          class="select pipeline-select"
          value={board.pipeline?.id ?? ''}
          onchange={(event) => screen.choose(event.currentTarget.value)}
        >
          {#each board.pipelines as pipeline (pipeline.id)}
            <option value={pipeline.id}>{pipeline.name}</option>
          {/each}
        </select>
      {:else if board.pipeline}
        <p class="pipeline-name">{board.pipeline.name}</p>
      {/if}
    </div>
    <a class="btn btn-primary" href="/deals/new{board.pipeline ? `?pipeline=${encodeURIComponent(board.pipeline.id)}` : ''}">
      <Plus size={18} aria-hidden="true" /> New deal
    </a>
  </div>

  {#if board.status === 'error'}
    <ErrorNotice message={board.error} onretry={screen.reload} />
  {:else if board.status === 'loading'}
    <div class="board" aria-busy="true">
      <p class="sr-only" role="status">Loading the pipeline</p>
      {#each { length: 4 } as _, i (i)}
        <div class="column">
          <span class="skeleton" style="display:block;height:20px;width:60%;margin:8px 8px 12px"></span>
          <span class="skeleton" style="display:block;height:72px;border-radius:10px;margin-bottom:8px"></span>
          <span class="skeleton" style="display:block;height:72px;border-radius:10px"></span>
        </div>
      {/each}
    </div>
  {:else if !board.pipeline}
    <div class="empty-state">
      <p class="empty-state-title">No pipeline yet</p>
      <p class="empty-state-text">Every workspace starts with one, so this one should return after a reload.</p>
      <button type="button" class="btn btn-secondary" onclick={screen.reload}>Reload</button>
    </div>
  {:else}
    {#if empty}
      <div class="empty-state board-empty">
        <p class="empty-state-title">No deals in {board.pipeline.name} yet</p>
        <p class="empty-state-text">A deal starts in the first stage and moves across the board as it progresses. Drag a card, or use its Move button.</p>
      </div>
    {/if}
    {#if board.truncated}
      <p class="load-error" role="status">This pipeline has more open deals than the board shows. Close or move some to see the rest.</p>
    {/if}
    <div class="board" role="list" aria-label="Stages of {board.pipeline.name}">
      {#each board.columns as column (column.stage.id)}
        {@const kind = closedKind(column)}
        {@const total = totalOf(column)}
        <section
          class="column"
          class:over={over?.stageId === column.stage.id}
          class:closed={kind !== null}
          role="listitem"
          aria-labelledby="stage-{column.stage.id}"
          ondragover={(event) => dragOverColumn(event, column)}
          ondragleave={(event) => {
            if (event.currentTarget === event.target) over = null;
          }}
          ondrop={(event) => drop(event, column)}
        >
          <header class="column-header">
            <span class="stage-dot" style:background={column.stage.color} aria-hidden="true"></span>
            <h2 class="stage-name" id="stage-{column.stage.id}">{column.stage.name}</h2>
            <span class="stage-count">{column.deals.length}{column.hasMore ? '+' : ''}<span class="sr-only"> {column.deals.length === 1 ? 'deal' : 'deals'}</span></span>
            {#if total}
              <span class="stage-total">{total}</span>
            {/if}
          </header>
          <div class="cards">
            {#each column.deals as deal (deal.id)}
              {#if over?.stageId === column.stage.id && over.before === deal.id}
                <div class="drop-mark" aria-hidden="true"></div>
              {/if}
              <DealCard
                {deal}
                ownerName={ownerOf(deal)}
                busy={Boolean(board.busy[deal.id])}
                dragging={dragging === deal.id}
                onmove={() => (moving = { deal, preset: null })}
                ondragstart={(event) => dragStart(event, deal)}
                ondragend={dragEnd}
              />
            {/each}
            {#if over?.stageId === column.stage.id && over.before === null && dragging && !column.deals.some((d) => d.id === dragging && column.deals.at(-1)?.id === dragging)}
              <div class="drop-mark" aria-hidden="true"></div>
            {/if}
            {#if column.deals.length === 0 && !column.hasMore}
              <p class="column-empty">No deals</p>
            {/if}
            {#if kind && column.hasMore}
              <div class="column-more">
                {#if board.closed[kind].error}
                  <p class="form-error">{board.closed[kind].error}</p>
                {/if}
                <button type="button" class="btn btn-secondary" onclick={() => screen.loadMore(kind)} disabled={board.closed[kind].loading} aria-busy={board.closed[kind].loading}>
                  {board.closed[kind].loading ? 'Loading…' : `Load more ${kind} deals`}
                </button>
              </div>
            {/if}
          </div>
        </section>
      {/each}
    </div>
  {/if}
</div>

{#if moving}
  {#key moving.deal.id}
    <MoveDealDialog deal={moving.deal} columns={board.columns} preset={moving.preset} onmove={moveFromDialog} oncancel={() => (moving = null)} />
  {/key}
{/if}

<style>
  .heading {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px 14px;
    min-width: 0;
  }

  .pipeline-select {
    width: auto;
    min-height: 44px;
    padding: 8px 36px 8px 12px;
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .pipeline-name {
    color: var(--text2);
    font-size: 0.9375rem;
  }

  .board-empty {
    padding: 24px 20px 8px;
  }

  /* Columns scroll sideways inside the page, which itself never does. */
  .board {
    /* The containing block of the screen-reader-only text inside, so it cannot widen the page. */
    position: relative;
    display: flex;
    gap: 12px;
    align-items: flex-start;
    margin: 0 -16px;
    padding: 4px 16px 16px;
    overflow-x: auto;
    scroll-snap-type: x proximity;
    scroll-padding: 16px;
    -webkit-overflow-scrolling: touch;
  }

  .column {
    flex: 0 0 272px;
    display: flex;
    flex-direction: column;
    max-height: calc(100vh - 220px);
    min-height: 160px;
    background: var(--surface2);
    border: 1px solid transparent;
    border-radius: var(--radius);
    scroll-snap-align: start;
    transition: border-color 0.15s;
  }

  .column.over {
    border-color: var(--accent);
  }

  .column-header {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px 8px;
    padding: 10px 12px 8px;
  }

  .stage-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .stage-name {
    flex: 1;
    min-width: 0;
    font-size: 0.875rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .stage-count {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text2);
    padding: 1px 7px;
    border-radius: 999px;
    background: var(--surface);
    font-variant-numeric: tabular-nums;
  }

  .stage-total {
    flex-basis: 100%;
    font-size: 0.8125rem;
    color: var(--text2);
    font-variant-numeric: tabular-nums;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cards {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 0 8px 8px;
    overflow-y: auto;
    min-height: 56px;
  }

  .drop-mark {
    height: 3px;
    margin: -5px 4px;
    border-radius: 2px;
    background: var(--accent);
  }

  .column-empty {
    padding: 16px 8px;
    text-align: center;
    font-size: 0.8125rem;
    color: var(--text2);
  }

  .column-more {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding-top: 4px;
  }

  @media (max-width: 600px) {
    .column {
      flex-basis: min(300px, calc(100vw - 48px));
      max-height: calc(100vh - 260px);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .column {
      transition: none;
    }
  }
</style>
