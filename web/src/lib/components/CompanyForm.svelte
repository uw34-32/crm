<script>
  // Creates or edits a company. Checks mirror the API's limits and run on submit; the
  // first field with a problem takes focus, and each message is tied to its field.
  import { tick, untrack } from 'svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import Field from '$lib/components/Field.svelte';
  import { validateCompany } from '$lib/records/fields.js';

  /**
   * @type {{
   *   initial?: import('$lib/records/fields.js').CompanyDraft,
   *   submitLabel: string,
   *   busyLabel: string,
   *   busy?: boolean,
   *   formError?: string,
   *   cancelHref: string,
   *   onsubmit: (draft: Required<import('$lib/records/fields.js').CompanyDraft>) => void
   * }}
   */
  let { initial = {}, submitLabel, busyLabel, busy = false, formError = '', cancelHref, onsubmit } = $props();

  const order = /** @type {const} */ (['name', 'domain', 'industry', 'phone', 'address']);
  const draft = $state(untrack(() => Object.fromEntries(order.map((field) => [field, initial[field] ?? '']))));
  /** @type {Record<string, string>} */
  let errors = $state({});
  /** @type {HTMLElement | null} */
  let notice = $state(null);

  $effect(() => {
    if (formError) tick().then(() => notice?.focus());
  });

  /** @param {SubmitEvent} event */
  async function submit(event) {
    event.preventDefault();
    errors = validateCompany(draft);
    const first = order.find((field) => errors[field]);
    if (first) {
      await tick();
      document.getElementById(`company-${first}`)?.focus();
      return;
    }
    onsubmit(/** @type {Required<import('$lib/records/fields.js').CompanyDraft>} */ ({ ...draft }));
  }
</script>

{#if formError}
  <ErrorNotice message={formError} bind:element={notice} />
{/if}

<form class="section-card section-card-body" novalidate onsubmit={submit}>
  <Field id="company-name" label="Name" autocomplete="organization" required bind:value={draft.name} error={errors.name} />
  <Field
    id="company-domain"
    label="Domain"
    autocomplete="off"
    hint="Such as acme.example, without https://."
    bind:value={draft.domain}
    error={errors.domain}
  />
  <Field id="company-industry" label="Industry" autocomplete="off" bind:value={draft.industry} error={errors.industry} />
  <Field id="company-phone" label="Phone" type="tel" autocomplete="tel" bind:value={draft.phone} error={errors.phone} />
  <Field id="company-address" label="Address" autocomplete="street-address" bind:value={draft.address} error={errors.address} />
  <div class="form-actions">
    <a class="btn btn-secondary" href={cancelHref}>Cancel</a>
    <button class="btn btn-primary" type="submit" disabled={busy} aria-busy={busy}>{busy ? busyLabel : submitLabel}</button>
  </div>
</form>
