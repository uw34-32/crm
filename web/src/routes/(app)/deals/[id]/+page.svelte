<script>
  import { dateLocale, nameOf, t } from '$lib/i18n.svelte.js';
  import { ArrowLeft, Pencil, Trash2 } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { dealsApi, pipelinesApi } from '$lib/api/crm.js';
  import ActivityTimeline from '$lib/components/ActivityTimeline.svelte';
  import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import MoveDealDialog from '$lib/components/MoveDealDialog.svelte';
  import NotFound from '$lib/components/NotFound.svelte';
  import { buildColumns } from '$lib/deals/board.js';
  import { formatMoney } from '$lib/deals/money.js';
  import { memberName } from '$lib/activities/timeline.js';
  import { toSentence } from '$lib/forms.js';
  import { loadMembers, members } from '$lib/members.svelte.js';
  import { isMissing } from '$lib/records/load.js';
  import { recordScreen } from '$lib/records/screens.svelte.js';
  import { api } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';

  const deals = dealsApi(api);
  const id = $derived(page.params.id ?? '');
  const screen = recordScreen('pipeline', deals, () => id);
  const deal = $derived(screen.state.status === 'ready' ? screen.state.record : null);
  const date = { format: (/** @type {Date} */ date) => new Intl.DateTimeFormat(dateLocale(), { dateStyle: 'medium' }).format(date) };
  const day = { format: (/** @type {Date} */ date) => new Intl.DateTimeFormat(dateLocale(), { dateStyle: 'medium', timeZone: 'UTC' }).format(date) };

  untrack(() => loadMembers(screen.workspaceId));

  /** The stages of the deal's pipeline, for the stage control. @type {import('$lib/deals/board.js').Stage[]} */
  let stages = $state([]);
  let pipelineName = $state('');
  let stagesError = $state('');
  let stagesFor = '';

  $effect(() => {
    const pipelineId = deal?.pipeline_id;
    if (!pipelineId || pipelineId === stagesFor) return;
    stagesFor = pipelineId;
    untrack(async () => {
      stagesError = '';
      try {
        const pipeline = /** @type {{ name: string, stages?: import('$lib/deals/board.js').Stage[] }} */ (await pipelinesApi(api).get(screen.workspaceId, pipelineId));
        if (stagesFor !== pipelineId) return;
        stages = pipeline.stages ?? [];
        pipelineName = pipeline.name;
      } catch (err) {
        if (stagesFor !== pipelineId) return;
        stagesError = err instanceof Error ? err.message : String(err);
      }
    });
  });

  const stage = $derived(stages.find((s) => s.id === deal?.stage_id) ?? null);
  const owner = $derived(deal?.owner_id && members.status === 'ready' ? memberName(members.list, deal.owner_id) : '');

  let moving = $state(false);
  let moveBusy = $state(false);
  /** @type {string} */
  let pendingStage = $state('');

  /** @param {string} stageId @param {string} [lostReason] */
  async function moveTo(stageId, lostReason = '') {
    if (!deal) return;
    moveBusy = true;
    try {
      const body = lostReason ? { stage_id: stageId, lost_reason: lostReason } : { stage_id: stageId };
      const moved = /** @type {Record<string, any>} */ (await deals.move(screen.workspaceId, deal.id, body));
      Object.assign(screen.state, { record: { ...deal, ...moved } });
      showToast(t('Moved to {stage}', { stage: stages.find((s) => s.id === stageId)?.name ?? t('the new stage') }));
    } catch (err) {
      showToast(`${t('The move failed.')} ${toSentence(err instanceof Error ? err.message : String(err))}`, 5000);
      if (isMissing(err)) screen.reload();
    } finally {
      moveBusy = false;
    }
  }

  /** @param {Event & { currentTarget: HTMLSelectElement }} event */
  function onStage(event) {
    const stageId = event.currentTarget.value;
    event.currentTarget.value = deal?.stage_id ?? '';
    const target = stages.find((s) => s.id === stageId);
    if (!target || !deal || target.id === deal.stage_id) return;
    if (target.kind === 'lost') {
      pendingStage = stageId;
      moving = true;
    } else {
      moveTo(stageId);
    }
  }

  let confirming = $state(false);
  let deleting = $state(false);
  let deleteError = $state('');

  async function remove() {
    if (!deal) return;
    deleting = true;
    deleteError = '';
    try {
      await deals.remove(screen.workspaceId, deal.id);
    } catch (err) {
      if (!isMissing(err)) {
        deleteError = toSentence(err instanceof Error ? err.message : String(err));
        deleting = false;
        return;
      }
    }
    confirming = false;
    showToast(t('Deleted {name}', { name: deal.title }));
    await goto(screen.backHref(), { replaceState: true });
  }
</script>

<svelte:head>
  <title>{deal?.title ?? t('Deal')} · CRM</title>
</svelte:head>

{#if screen.state.status === 'missing'}
  <NotFound title={t('Deal not found')} backHref={screen.backHref()} backLabel="Back to the pipeline" />
{:else}
  <div class="detail-header">
    <a class="btn-ghost" href={screen.backHref()}><ArrowLeft size={18} aria-hidden="true" /> {t('Pipeline')}</a>
    {#if deal}
      <div class="detail-actions">
        <a class="btn-ghost" href="/deals/{deal.id}/edit"><Pencil size={18} aria-hidden="true" /> {t('Edit')}</a>
        <button type="button" class="btn-ghost danger" onclick={() => (confirming = true)}>
          <Trash2 size={18} aria-hidden="true" /> {t('Delete')}
        </button>
      </div>
    {/if}
  </div>

  {#if screen.state.status === 'error'}
    <h1 class="page-title">{t('Deal')}</h1>
    <ErrorNotice message={screen.state.error} onretry={screen.reload} />
  {:else if deal}
    <div class="animate-in">
      <div class="detail-hero">
        <div class="detail-hero-text">
          <h1 class="page-title">{deal.title}</h1>
          <p class="detail-subtitle">
            {deal.value_minor > 0 ? formatMoney(deal.value_minor, deal.currency) : 'No value yet'}
            {#if pipelineName} · {nameOf(pipelineName)}{/if}
          </p>
        </div>
      </div>

      <section class="section-card" aria-labelledby="deal-details">
        <div class="section-card-header">
          <h2 id="deal-details" class="section-card-title">{t('Details')}</h2>
        </div>
        <dl class="section-card-body">
          <div class="field-row">
            <dt class="field-label"><label for="deal-stage">{t('Stage')}</label></dt>
            <dd class="field-value stage-control">
              {#if stages.length > 0}
                <select id="deal-stage" class="select" value={deal.stage_id} onchange={onStage} disabled={moveBusy} aria-busy={moveBusy}>
                  {#each stages as s (s.id)}
                    <option value={s.id}>{nameOf(s.name)}</option>
                  {/each}
                </select>
              {:else if stagesError}
                <span class="form-error">{stagesError}</span>
              {:else}
                <span>{deal.status}</span>
              {/if}
            </dd>
          </div>
          <div class="field-row">
            <dt class="field-label">{t('Status')}</dt>
            <dd class="field-value">{stage ? (deal.status === 'open' ? t('Open') : deal.status === 'won' ? t('Won') : t('Lost')) : deal.status}</dd>
          </div>
          {#if deal.status === 'lost' && deal.lost_reason}
            <div class="field-row">
              <dt class="field-label">{t('Lost because')}</dt>
              <dd class="field-value reason">{deal.lost_reason}</dd>
            </div>
          {/if}
          {#if deal.expected_close_date}
            <div class="field-row">
              <dt class="field-label">{t('Expected close')}</dt>
              <dd class="field-value">{day.format(new Date(`${deal.expected_close_date}T00:00:00Z`))}</dd>
            </div>
          {/if}
          {#if deal.closed_at}
            <div class="field-row">
              <dt class="field-label">{t('Closed')}</dt>
              <dd class="field-value">{date.format(new Date(deal.closed_at))}</dd>
            </div>
          {/if}
          {#if deal.company_id}
            <div class="field-row">
              <dt class="field-label">{t('Company')}</dt>
              <dd class="field-value"><a href="/companies/{deal.company_id}">{deal.company_name}</a></dd>
            </div>
          {/if}
          {#if deal.contact_id}
            <div class="field-row">
              <dt class="field-label">{t('Contact')}</dt>
              <dd class="field-value"><a href="/contacts/{deal.contact_id}">{deal.contact_name}</a></dd>
            </div>
          {/if}
          <div class="field-row">
            <dt class="field-label">{t('Owner')}</dt>
            <dd class="field-value">{owner || (deal.owner_id ? '' : 'No owner')}</dd>
          </div>
        </dl>
      </section>

      <ActivityTimeline workspaceId={screen.workspaceId} link={{ deal_id: deal.id }} subjectName={deal.title} />

      <p class="detail-meta">{t('Added {date}', { date: date.format(new Date(deal.created_at)) })} · {t('Updated {date}', { date: date.format(new Date(deal.updated_at)) })}</p>
    </div>
  {:else}
    <div aria-busy="true">
      <p class="sr-only" role="status">{t('Loading the deal')}</p>
      <span class="skeleton" style="display:block;width:50%;height:28px;margin-bottom:20px"></span>
      <span class="skeleton" style="display:block;height:240px;border-radius:var(--radius)"></span>
    </div>
  {/if}
{/if}

{#if moving && deal}
  <MoveDealDialog
    {deal}
    columns={buildColumns(stages, [deal])}
    preset={{ stageId: pendingStage, before: null }}
    onmove={({ stageId, lostReason }) => {
      moving = false;
      moveTo(stageId, lostReason);
    }}
    oncancel={() => (moving = false)}
  />
{/if}

<ConfirmDialog
  open={confirming}
  title={t('Delete {name}?', { name: deal?.title ?? t('this deal') })}
  message={t('It leaves the pipeline for everyone in the workspace, and its activities that are linked only to it go too.')}
  confirmLabel={t('Delete deal')}
  busyLabel={t('Deleting…')}
  busy={deleting}
  error={deleteError}
  onconfirm={remove}
  oncancel={() => {
    confirming = false;
    deleteError = '';
  }}
/>

<style>
  .stage-control {
    flex: 1;
    max-width: 240px;
  }

  .stage-control :global(.select) {
    min-height: 44px;
  }

  .reason {
    white-space: pre-line;
  }
</style>
