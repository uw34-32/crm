<script>
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import CompanyForm from '$lib/components/CompanyForm.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import NotFound from '$lib/components/NotFound.svelte';
  import { recordsApi } from '$lib/api/records.js';
  import { toSentence } from '$lib/forms.js';
  import { companyPatchBody } from '$lib/records/fields.js';
  import { isMissing } from '$lib/records/load.js';
  import { recordScreen } from '$lib/records/screens.svelte.js';
  import { api } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';

  const companies = recordsApi(api, 'companies');
  const id = $derived(page.params.id ?? '');
  const screen = recordScreen('companies', companies, () => id);
  let busy = $state(false);
  let formError = $state('');

  /** @param {import('$lib/records/fields.js').CompanyDraft} draft */
  async function save(draft) {
    const body = companyPatchBody(screen.state.record, draft);
    if (Object.keys(body).length === 0) {
      await goto(`/companies/${id}`);
      return;
    }
    busy = true;
    formError = '';
    try {
      const company = /** @type {{ name: string }} */ (await companies.update(screen.workspaceId, id, body));
      showToast(`Saved ${company.name}`);
      await goto(`/companies/${id}`, { replaceState: true });
    } catch (err) {
      formError = isMissing(err)
        ? 'This company no longer exists. It may have been deleted by someone else.'
        : toSentence(err instanceof Error ? err.message : String(err));
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>Edit {screen.state.record?.name ?? 'company'} · CRM</title>
</svelte:head>

{#if screen.state.status === 'missing'}
  <NotFound title="Company not found" backHref={screen.backHref()} backLabel="Back to companies" />
{:else if screen.state.status === 'error'}
  <h1 class="page-title">Edit company</h1>
  <ErrorNotice message={screen.state.error} onretry={screen.reload} />
{:else if screen.state.status === 'ready'}
  <div class="animate-in">
    <h1 class="page-title">Edit {screen.state.record.name}</h1>
    <p class="page-lede">Clear a field to remove it.</p>
    {#key screen.state.record.id}
      <CompanyForm
        initial={screen.state.record}
        submitLabel="Save changes"
        busyLabel="Saving…"
        {busy}
        {formError}
        cancelHref="/companies/{id}"
        onsubmit={save}
      />
    {/key}
  </div>
{:else}
  <div aria-busy="true">
    <p class="sr-only" role="status">Loading the company</p>
    <span class="skeleton" style="display:block;width:50%;height:28px;margin-bottom:20px"></span>
    <span class="skeleton" style="display:block;height:320px;border-radius:var(--radius)"></span>
  </div>
{/if}
