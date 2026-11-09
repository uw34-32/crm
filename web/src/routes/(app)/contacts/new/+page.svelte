<script>
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import ContactForm from '$lib/components/ContactForm.svelte';
  import { recordsApi } from '$lib/api/records.js';
  import { toSentence } from '$lib/forms.js';
  import { contactCreateBody } from '$lib/records/fields.js';
  import { contactName } from '$lib/records/format.js';
  import { isMissing } from '$lib/records/load.js';
  import { leaveOnWorkspaceSwitch } from '$lib/records/screens.svelte.js';
  import { listHref } from '$lib/records/url.js';
  import { api } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';

  const contacts = recordsApi(api, 'contacts');
  const companies = recordsApi(api, 'companies');
  const workspaceId = leaveOnWorkspaceSwitch('contacts');
  let busy = $state(false);
  let formError = $state('');

  // Arriving from a company's page links the new contact to it.
  const companyId = page.url.searchParams.get('company');
  /** @type {Promise<{ id: string, name: string } | null>} */
  const preset = companyId
    ? /** @type {Promise<{ id: string, name: string }>} */ (companies.get(workspaceId, companyId)).catch(() => null)
    : Promise.resolve(null);
  const cancelHref = companyId ? `/companies/${encodeURIComponent(companyId)}` : listHref('contacts');

  /** @param {import('$lib/records/fields.js').ContactDraft} draft */
  async function create(draft) {
    busy = true;
    formError = '';
    try {
      const contact = /** @type {any} */ (await contacts.create(workspaceId, contactCreateBody(draft)));
      showToast(`Added ${contactName(contact)}`);
      await goto(`/contacts/${contact.id}`, { replaceState: true });
    } catch (err) {
      formError = isMissing(err)
        ? 'The chosen company no longer exists. Choose another one, or remove the link.'
        : toSentence(err instanceof Error ? err.message : String(err));
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>New contact · CRM</title>
</svelte:head>

<div class="animate-in">
  <h1 class="page-title">New contact</h1>
  <p class="page-lede">Only the first name is required. Someone with one name goes there.</p>
  {#await preset}
    <div aria-busy="true">
      <p class="sr-only" role="status">Loading the company</p>
      <span class="skeleton" style="display:block;height:420px;border-radius:var(--radius)"></span>
    </div>
  {:then company}
    <ContactForm
      initial={company ? { company_id: company.id, company_name: company.name } : {}}
      {workspaceId}
      submitLabel="Add contact"
      busyLabel="Adding…"
      {busy}
      {formError}
      {cancelHref}
      onsubmit={create}
    />
  {/await}
</div>
