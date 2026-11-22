<script>
  import { nameOf, t } from '$lib/i18n.svelte.js';
  // Creates or edits a deal. Creating also picks the pipeline and whether the deal is in
  // progress or already won; an existing deal changes stage only by moving, so editing
  // leaves them out.
  // Checks mirror the API's limits and run on submit.
  import { tick, untrack } from 'svelte';
  import CompanyPicker from '$lib/components/CompanyPicker.svelte';
  import ContactPicker from '$lib/components/ContactPicker.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import Field from '$lib/components/Field.svelte';
  import { dayKey } from '$lib/activities/timeline.js';
  import { validateDeal } from '$lib/deals/fields.js';
  import { majorText } from '$lib/deals/money.js';
  import { members } from '$lib/members.svelte.js';

  /** @typedef {import('$lib/deals/fields.js').DealDraft} DealDraft */
  /**
   * @type {{
   *   initial?: Record<string, any>,
   *   workspaceId: string,
   *   pipelines?: { id: string, name: string }[],
   *   creating?: boolean,
   *   submitLabel: string,
   *   busyLabel: string,
   *   busy?: boolean,
   *   formError?: string,
   *   cancelHref: string,
   *   onsubmit: (draft: DealDraft) => void
   * }}
   */
  let { initial = {}, workspaceId, pipelines = [], creating = false, submitLabel, busyLabel, busy = false, formError = '', cancelHref, onsubmit } = $props();

  const order = /** @type {const} */ (['title', 'value', 'currency', 'pipeline_id', 'expected_close_date']);
  const draft = $state(
    untrack(() => ({
      title: initial.title ?? '',
      currency: initial.currency ?? 'USD',
      value: initial.value_minor ? majorText(initial.value_minor, initial.currency ?? 'USD') : '',
      pipeline_id: initial.pipeline_id ?? pipelines[0]?.id ?? '',
      status: 'won',
      // A new deal records a sale made today unless the date is changed; an edited one keeps its own.
      expected_close_date: initial.expected_close_date ?? (creating ? dayKey(new Date()) : ''),
      owner_id: initial.owner_id ?? ''
    }))
  );
  let company = $state(untrack(() => (initial.company_id ? { id: initial.company_id, name: initial.company_name ?? t('Linked company') } : null)));
  let contact = $state(untrack(() => (initial.contact_id ? { id: initial.contact_id, name: initial.contact_name ?? t('Linked contact') } : null)));
  /** @type {Record<string, string>} */
  let errors = $state({});
  /** @type {HTMLElement | null} */
  let notice = $state(null);
  // A new deal asks only what recording a sale needs; the rest opens on request, or when it holds an error.
  let dateTouched = false;
  let details = $state(false);
  const showDetails = $derived(!creating || details);

  $effect(() => {
    if (formError) tick().then(() => notice?.focus());
  });

  // The date is the day the sale was made, so it only suits a won deal: an open one has no date until one is typed.
  function changeStatus() {
    if (!dateTouched) draft.expected_close_date = draft.status === 'won' ? dayKey(new Date()) : '';
  }

  /** @param {SubmitEvent} event */
  async function submit(event) {
    event.preventDefault();
    errors = validateDeal(draft);
    const first = order.find((field) => errors[field]);
    if (first) {
      if (first === 'pipeline_id') details = true;
      await tick();
      document.getElementById(`deal-${first}`)?.focus();
      return;
    }
    onsubmit({ ...draft, company_id: company?.id ?? null, contact_id: contact?.id ?? null });
  }
</script>

{#if formError}
  <ErrorNotice message={formError} bind:element={notice} />
{/if}

<form class="section-card section-card-body" novalidate onsubmit={submit}>
  <Field id="deal-title" label={t('Title')} required bind:value={draft.title} error={errors.title} />
  <div class="row money">
    <Field id="deal-value" label={t('Value')} hint={creating && !errors.value ? undefined : t('In the currency\'s main unit, such as 1250.50.')} bind:value={draft.value} error={errors.value} />
    <Field id="deal-currency" label={t('Currency')} autocomplete="off" hint={creating && !errors.currency ? undefined : t('Three letters, such as EUR.')} bind:value={draft.currency} error={errors.currency} />
  </div>
  {#if creating}
    <div class="row">
      <div class="form-group">
        <label class="form-label" for="deal-status">{t('Status')}</label>
        <select id="deal-status" class="select" bind:value={draft.status} onchange={changeStatus} aria-describedby="deal-status-hint">
          <option value="open">{t('In progress')}</option>
          <option value="won">{nameOf('Won')}</option>
        </select>
        <p class="form-hint" id="deal-status-hint">{t('Won records a sale that is already closed. In progress keeps the deal open on the board.')}</p>
      </div>
    </div>
  {/if}
  <div class="form-group">
    <label class="form-label" for="deal-expected_close_date">{t('Expected close date')}</label>
    <input
      id="deal-expected_close_date"
      class="input"
      type="date"
      bind:value={draft.expected_close_date}
      oninput={() => (dateTouched = true)}
      min="1900-01-01"
      max="2200-12-31"
      aria-invalid={errors.expected_close_date ? 'true' : undefined}
      aria-describedby={errors.expected_close_date ? 'deal-expected_close_date-error' : undefined}
    />
    {#if errors.expected_close_date}<p class="form-error" id="deal-expected_close_date-error">{errors.expected_close_date}</p>{/if}
  </div>
  {#if creating}
    <button type="button" class="btn btn-ghost details-toggle" aria-expanded={details} aria-controls="deal-details" onclick={() => (details = !details)}>
      {details ? t('Fewer details') : t('More details')}
    </button>
  {/if}
  {#if showDetails}
    <div id="deal-details">
      {#if creating}
        <div class="form-group">
          <label class="form-label" for="deal-pipeline_id">{t('Pipeline')}</label>
          <select id="deal-pipeline_id" class="select" bind:value={draft.pipeline_id} aria-invalid={errors.pipeline_id ? 'true' : undefined}>
            {#each pipelines as pipeline (pipeline.id)}
              <option value={pipeline.id}>{nameOf(pipeline.name)}</option>
            {/each}
          </select>
          {#if errors.pipeline_id}<p class="form-error">{errors.pipeline_id}</p>{/if}
        </div>
      {/if}
      <CompanyPicker id="deal-company" {workspaceId} bind:value={company} />
      <ContactPicker id="deal-contact" {workspaceId} bind:value={contact} />
      <div class="form-group">
        <label class="form-label" for="deal-owner_id">{t('Owner')}</label>
        <select id="deal-owner_id" class="select" bind:value={draft.owner_id}>
          <option value="">{creating ? t('Me') : t('No owner')}</option>
          {#each members.list as member (member.user_id)}
            <option value={member.user_id}>{member.name || member.email}</option>
          {/each}
        </select>
      </div>
    </div>
  {/if}
  <div class="form-actions">
    <a class="btn btn-secondary" href={cancelHref}>{t('Cancel')}</a>
    <button class="btn btn-primary" type="submit" disabled={busy} aria-busy={busy}>{busy ? busyLabel : submitLabel}</button>
  </div>
</form>

<style>
  /* The amount and its currency share a line, so recording a sale fits a phone screen. */
  .row.money {
    grid-template-columns: 2fr 1fr;
  }
  .details-toggle {
    margin-bottom: 16px;
  }
  .row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
    column-gap: 12px;
  }
</style>
