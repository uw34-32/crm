<script>
  // One deal on the board: title, value, who it is with, and its owner. The card drags
  // with the mouse, and its Move button opens a dialog for the keyboard and for touch,
  // where there is no drag.
  import { ArrowRightLeft, GripVertical } from '@lucide/svelte';
  import { formatMoney } from '$lib/deals/money.js';

  /**
   * @type {{
   *   deal: import('$lib/deals/board.js').Deal,
   *   ownerName: string,
   *   busy?: boolean,
   *   dragging?: boolean,
   *   onmove: () => void,
   *   ondragstart: (event: DragEvent) => void,
   *   ondragend: () => void
   * }}
   */
  let { deal, ownerName, busy = false, dragging = false, onmove, ondragstart, ondragend } = $props();

  const party = $derived(deal.company_name ?? deal.contact_name ?? '');
  const value = $derived(deal.value_minor > 0 ? formatMoney(deal.value_minor, deal.currency) : '');
</script>

<article
  class="deal-card"
  class:dragging
  class:busy
  draggable="true"
  aria-busy={busy}
  aria-labelledby="deal-{deal.id}-title"
  data-deal-id={deal.id}
  {ondragstart}
  {ondragend}
>
  <span class="grip" aria-hidden="true"><GripVertical size={16} /></span>
  <div class="deal-body">
    <h3 class="deal-title" id="deal-{deal.id}-title"><a href="/deals/{deal.id}">{deal.title}</a></h3>
    {#if value || party}
      <p class="deal-line">
        {#if value}<span class="deal-value">{value}</span>{/if}
        {#if value && party}<span aria-hidden="true"> · </span>{/if}
        {#if party}<span class="deal-party">{party}</span>{/if}
      </p>
    {/if}
    {#if ownerName}
      <p class="deal-owner">{ownerName}</p>
    {/if}
  </div>
  <button type="button" class="icon-btn move" onclick={onmove} disabled={busy} aria-label="Move {deal.title}" title="Move">
    <ArrowRightLeft size={16} aria-hidden="true" />
  </button>
</article>

<style>
  .deal-card {
    position: relative;
    display: flex;
    align-items: flex-start;
    gap: 4px;
    padding: 10px 6px 10px 4px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 10px;
    cursor: grab;
    transition: opacity 0.15s, box-shadow 0.15s;
  }

  .deal-card:hover {
    box-shadow: 0 1px 6px color-mix(in srgb, var(--text) 10%, transparent);
  }

  .deal-card.dragging {
    opacity: 0.4;
  }

  .deal-card.busy {
    cursor: progress;
  }

  .deal-card.busy .deal-body {
    opacity: 0.6;
  }

  .grip {
    flex-shrink: 0;
    display: inline-flex;
    padding-top: 2px;
    color: var(--text2);
    opacity: 0.6;
  }

  .deal-body {
    flex: 1;
    min-width: 0;
  }

  .deal-title {
    font-size: 0.9375rem;
    font-weight: 500;
    line-height: 1.3;
    overflow-wrap: anywhere;
  }

  .deal-title a {
    color: var(--text);
  }

  .deal-title a::after {
    /* The whole card opens the deal, while the title stays the link for the keyboard. */
    content: '';
    position: absolute;
    inset: 0;
    border-radius: 10px;
  }

  .deal-line,
  .deal-owner {
    margin-top: 3px;
    font-size: 0.8125rem;
    color: var(--text2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .deal-value {
    font-weight: 600;
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }

  .move {
    position: relative;
    z-index: 1;
    flex-shrink: 0;
    min-width: 36px;
    min-height: 36px;
    margin: -4px -2px -4px 0;
  }

  @media (hover: hover) {
    .move {
      opacity: 0;
    }

    .deal-card:hover .move,
    .deal-card:focus-within .move {
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .deal-card {
      transition: none;
    }
  }
</style>
