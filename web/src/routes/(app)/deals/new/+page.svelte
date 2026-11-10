<script>
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { dealsApi, pipelinesApi } from '$lib/api/crm.js';
  import DealForm from '$lib/components/DealForm.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import { dealCreateBody } from '$lib/deals/fields.js';
  import { toSentence } from '$lib/forms.js';
  import { loadMembers } from '$lib/members.svelte.js';
  import { isMissing } from '$lib/records/load.js';
  import { leaveOnWorkspaceSwitch } from '$lib/records/screens.svelte.js';
  import { api } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';

  const deals = dealsApi(api);
  const workspaceId = leaveOnWorkspaceSwitch('pipeline');
  untrack(() => loadMembers(workspaceId));

  let busy = $state(false);
  let formError = $state('');

  // The board links here with the pipeline it showed, so the deal starts there.
  const wanted = page.url.searchParams.get('pipeline') ?? '';
  const cancelHref = wanted ? `/pipeline?pipeline=${encodeURIComponent(wanted)}` : '/pipeline';

  /** @type {Promise<{ id: string, name: string }[]>} */
  const pipelines = pipelinesApi(api)
    .list(workspaceId)
    .then((result) => {
      const list = /** @type {{ id: string, name: string }[]} */ (result.items);
      return [...list].sort((a, b) => Number(b.id === wanted) - Number(a.id === wanted));
    });

  /** @param {import('$lib/deals/fields.js').DealDraft} draft */
  async function create(draft) {
    busy = true;
    formError = '';
    try {
      const deal = /** @type {{ id: string, title: string }} */ (await deals.create(workspaceId, dealCreateBody(draft)));
      showToast(`Added ${deal.title}`);
      await goto(`/deals/${deal.id}`, { replaceState: true });
    } catch (err) {
      formError = isMissing(err)
        ? 'The pipeline, stage, company, contact, or owner no longer exists. Choose another one.'
        : toSentence(err instanceof Error ? err.message : String(err));
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>New deal · CRM</title>
</svelte:head>

<div class="animate-in">
  <h1 class="page-title">New deal</h1>
  <p class="page-lede">A title and a currency are enough to start. The deal goes to the end of its first stage.</p>
  {#await pipelines}
    <div aria-busy="true">
      <p class="sr-only" role="status">Loading the pipelines</p>
      <span class="skeleton" style="display:block;height:520px;border-radius:var(--radius)"></span>
    </div>
  {:then list}
    <DealForm creating {workspaceId} pipelines={list} submitLabel="Add deal" busyLabel="Adding…" {busy} {formError} {cancelHref} onsubmit={create} />
  {:catch err}
    <ErrorNotice message={err instanceof Error ? err.message : String(err)} onretry={() => location.reload()} />
  {/await}
</div>
