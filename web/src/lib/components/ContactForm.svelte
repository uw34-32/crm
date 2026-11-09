<script>
  // Creates or edits a contact. Checks mirror the API's limits and run on submit; the
  // first field with a problem takes focus, and each message is tied to its field.
  import { tick, untrack } from 'svelte';
  import CompanyPicker from '$lib/components/CompanyPicker.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import Field from '$lib/components/Field.svelte';
  import { validateContact } from '$lib/records/fields.js';

  /** @typedef {import('$lib/records/fields.js').ContactDraft} ContactDraft */
  /**
   * @type {{
   *   initial?: ContactDraft & { company_name?: string | null },
   *   workspaceId: string,
   *   submitLabel: string,
   *   busyLabel: string,
   *   busy?: boolean,
   *   formError?: string,
   *   cancelHref: string,
   *   onsubmit: (draft: ContactDraft) => void
   * }}
   */
  let { initial = {}, workspaceId, submitLabel, busyLabel, busy = false, formError = '', cancelHref, onsubmit } = $props();

  const order = /** @type {const} */ (['first_name', 'last_name', 'title', 'email', 'phone']);
  const draft = $state(untrack(() => Object.fromEntries(order.map((field) => [field, initial[field] ?? '']))));
  let company = $state(
    untrack(() => (initial.company_id ? { id: initial.company_id, name: initial.company_name ?? 'Linked company' } : null))
  );
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
    errors = validateContact(draft);
    const first = order.find((field) => errors[field]);
    if (first) {
      await tick();
      document.getElementById(`contact-${first}`)?.focus();
      return;
    }
    onsubmit({ ...draft, company_id: company?.id ?? null });
  }
</script>

{#if formError}
  <ErrorNotice message={formError} bind:element={notice} />
{/if}

<form class="section-card section-card-body" novalidate onsubmit={submit}>
  <div class="name-row">
    <Field id="contact-first_name" label="First name" autocomplete="given-name" required bind:value={draft.first_name} error={errors.first_name} />
    <Field id="contact-last_name" label="Last name" autocomplete="family-name" bind:value={draft.last_name} error={errors.last_name} />
  </div>
  <Field id="contact-title" label="Job title" autocomplete="organization-title" bind:value={draft.title} error={errors.title} />
  <CompanyPicker id="contact-company" {workspaceId} bind:value={company} />
  <Field id="contact-email" label="Email" type="email" autocomplete="email" bind:value={draft.email} error={errors.email} />
  <Field id="contact-phone" label="Phone" type="tel" autocomplete="tel" bind:value={draft.phone} error={errors.phone} />
  <div class="form-actions">
    <a class="btn btn-secondary" href={cancelHref}>Cancel</a>
    <button class="btn btn-primary" type="submit" disabled={busy} aria-busy={busy}>{busy ? busyLabel : submitLabel}</button>
  </div>
</form>

<style>
  .name-row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
    column-gap: 12px;
  }
</style>
