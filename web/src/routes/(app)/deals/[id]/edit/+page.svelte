<script>
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { dealsApi } from '$lib/api/crm.js';
  import DealForm from '$lib/components/DealForm.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import NotFound from '$lib/components/NotFound.svelte';
  import { dealPatchBody } from '$lib/deals/fields.js';
  import { toSentence } from '$lib/forms.js';
  import { loadMembers } from '$lib/members.svelte.js';
  import { isMissing } from '$lib/records/load.js';
  import { recordScreen } from '$lib/records/screens.svelte.js';
  import { api } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';

  const deals = dealsApi(api);
  const id = $derived(page.params.id ?? '');
  const screen = recordScreen('pipeline', deals, () => id);
  untrack(() => loadMembers(screen.workspaceId));
  let busy = $state(false);
  let formError = $state('');

  /** @param {import('$lib/deals/fields.js').DealDraft} draft */
  async function save(draft) {
    const body = dealPatchBody(screen.state.record, draft);
    if (Object.keys(body).length === 0) {
      await goto(`/deals/${id}`);
      return;
    }
    busy = true;
    formError = '';
    try {
      const deal = /** @type {{ title: string }} */ (await deals.update(screen.workspaceId, id, body));
      showToast(`Saved ${deal.title}`);
      await goto(`/deals/${id}`, { replaceState: true });
    } catch (err) {
      formError = isMissing(err)
        ? 'This deal or the chosen company, contact, or owner no longer exists. Someone may have deleted it.'
        : toSentence(err instanceof Error ? err.message : String(err));
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>Edit {screen.state.record?.title ?? 'deal'} · CRM</title>
</svelte:head>

{#if screen.state.status === 'missing'}
  <NotFound title="Deal not found" backHref={screen.backHref()} backLabel="Back to the pipeline" />
{:else if screen.state.status === 'error'}
  <h1 class="page-title">Edit deal</h1>
  <ErrorNotice message={screen.state.error} onretry={screen.reload} />
{:else if screen.state.status === 'ready'}
  <div class="animate-in">
    <h1 class="page-title">Edit {screen.state.record.title}</h1>
    <p class="page-lede">The stage changes by moving the deal on the board or from its page.</p>
    {#key screen.state.record.id}
      <DealForm
        initial={screen.state.record}
        workspaceId={screen.workspaceId}
        submitLabel="Save changes"
        busyLabel="Saving…"
        {busy}
        {formError}
        cancelHref="/deals/{id}"
        onsubmit={save}
      />
    {/key}
  </div>
{:else}
  <div aria-busy="true">
    <p class="sr-only" role="status">Loading the deal</p>
    <span class="skeleton" style="display:block;width:50%;height:28px;margin-bottom:20px"></span>
    <span class="skeleton" style="display:block;height:480px;border-radius:var(--radius)"></span>
  </div>
{/if}
