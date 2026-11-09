<script>
  import { goto } from '$app/navigation';
  import CompanyForm from '$lib/components/CompanyForm.svelte';
  import { recordsApi } from '$lib/api/records.js';
  import { toSentence } from '$lib/forms.js';
  import { companyCreateBody } from '$lib/records/fields.js';
  import { leaveOnWorkspaceSwitch } from '$lib/records/screens.svelte.js';
  import { listHref } from '$lib/records/url.js';
  import { api } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';

  const companies = recordsApi(api, 'companies');
  const workspaceId = leaveOnWorkspaceSwitch('companies');
  let busy = $state(false);
  let formError = $state('');

  /** @param {import('$lib/records/fields.js').CompanyDraft} draft */
  async function create(draft) {
    busy = true;
    formError = '';
    try {
      const company = /** @type {{ id: string, name: string }} */ (await companies.create(workspaceId, companyCreateBody(draft)));
      showToast(`Added ${company.name}`);
      await goto(`/companies/${company.id}`, { replaceState: true });
    } catch (err) {
      formError = toSentence(err instanceof Error ? err.message : String(err));
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>New company · CRM</title>
</svelte:head>

<div class="animate-in">
  <h1 class="page-title">New company</h1>
  <p class="page-lede">Only the name is required.</p>
  <CompanyForm submitLabel="Add company" busyLabel="Adding…" {busy} {formError} cancelHref={listHref('companies')} onsubmit={create} />
</div>
