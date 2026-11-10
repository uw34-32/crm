<script>
  // Moves a deal without dragging: pick a stage and a place in it. When the stage is
  // a lost one, the reason is asked here too. A drop into a lost stage opens the same
  // dialog with the place already set, so only the reason is asked.
  import { tick, untrack } from 'svelte';
  import { needsLostReason } from '$lib/deals/board.js';
  import { DEAL_LIMITS } from '$lib/deals/fields.js';

  /** @typedef {import('$lib/deals/board.js').Column} Column */
  /** @typedef {import('$lib/deals/board.js').Deal} Deal */
  /**
   * @type {{
   *   deal: Deal,
   *   columns: Column[],
   *   preset?: { stageId: string, before: string | null } | null,
   *   onmove: (target: { stageId: string, before: string | null, lostReason: string }) => void,
   *   oncancel: () => void
   * }}
   */
  let { deal, columns, preset = null, onmove, oncancel } = $props();

  /** @type {HTMLDialogElement | null} */
  let dialog = $state(null);
  /** @type {HTMLElement | null} */
  let opener = null;

  let stageId = $state(untrack(() => preset?.stageId ?? deal.stage_id));
  const target = $derived(columns.find((column) => column.stage.id === stageId) ?? null);
  const others = $derived(target ? target.deals.filter((d) => d.id !== deal.id) : []);
  // The dense position among the other deals of the target, or at the end.
  let index = $state(untrack(() => (preset ? -1 : Math.max(0, (columns.find((c) => c.stage.id === deal.stage_id)?.deals.findIndex((d) => d.id === deal.id) ?? -1)))));
  let lostReason = $state('');
  let reasonError = $state('');
  const askReason = $derived(target ? needsLostReason(deal, target.stage) : false);

  $effect(() => {
    if (dialog && !dialog.open) {
      opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      dialog.showModal();
      tick().then(() => {
        const first = dialog?.querySelector('select, textarea');
        if (first instanceof HTMLElement) first.focus();
      });
    }
  });

  /** A change of stage puts the deal at the end of the new one. */
  function onStageChange() {
    index = stageId === deal.stage_id ? Math.max(0, others.length) : others.length;
    reasonError = '';
  }

  function cancel() {
    oncancel();
    opener?.focus();
  }

  /** @param {SubmitEvent} event */
  function submit(event) {
    event.preventDefault();
    const reason = lostReason.trim();
    if (askReason) {
      if (!reason) {
        reasonError = 'Say why the deal was lost.';
        return;
      }
      if ([...reason].length > DEAL_LIMITS.lost_reason) {
        reasonError = `Use at most ${DEAL_LIMITS.lost_reason} characters.`;
        return;
      }
    }
    const before = preset ? preset.before : (others[index]?.id ?? null);
    onmove({ stageId, before, lostReason: askReason ? reason : '' });
    opener?.focus();
  }
</script>

<dialog bind:this={dialog} class="modal" aria-labelledby="move-title" oncancel={(event) => { event.preventDefault(); cancel(); }}>
  <form method="dialog" novalidate onsubmit={submit}>
    <h2 class="modal-title" id="move-title">{preset && askReason ? `Mark ${deal.title} as lost` : `Move ${deal.title}`}</h2>
    {#if !preset}
      <div class="form-group">
        <label class="form-label" for="move-stage">Stage</label>
        <select id="move-stage" class="select" bind:value={stageId} onchange={onStageChange}>
          {#each columns as column (column.stage.id)}
            <option value={column.stage.id}>{column.stage.name}{column.stage.id === deal.stage_id ? ' (current)' : ''}</option>
          {/each}
        </select>
      </div>
      <div class="form-group">
        <label class="form-label" for="move-position">Place in {target?.stage.name ?? 'the stage'}</label>
        <select id="move-position" class="select" bind:value={index} disabled={target?.hasMore && index === others.length}>
          {#each { length: others.length + 1 } as _, i (i)}
            <option value={i}>{i + 1} of {others.length + 1}{i === others.length ? ' (last)' : ''}</option>
          {/each}
        </select>
        {#if target?.hasMore}
          <p class="form-hint">Not every deal of this stage is loaded; last means after all of them.</p>
        {/if}
      </div>
    {:else}
      <p class="modal-text">The deal moves to {target?.stage.name ?? 'the stage'}.</p>
    {/if}
    {#if askReason}
      <div class="form-group">
        <label class="form-label" for="move-reason">Why was it lost?</label>
        <textarea
          id="move-reason"
          class="input"
          rows="3"
          bind:value={lostReason}
          aria-required="true"
          aria-invalid={reasonError ? 'true' : undefined}
          aria-describedby={reasonError ? 'move-reason-error' : undefined}
        ></textarea>
        {#if reasonError}
          <p class="form-error" id="move-reason-error">{reasonError}</p>
        {/if}
      </div>
    {/if}
    <div class="modal-actions">
      <button type="button" class="btn btn-secondary" onclick={cancel}>Cancel</button>
      <button type="submit" class="btn btn-primary">{askReason ? 'Mark as lost' : 'Move deal'}</button>
    </div>
  </form>
</dialog>

<style>
  textarea.input {
    resize: vertical;
    min-height: 88px;
  }
</style>
