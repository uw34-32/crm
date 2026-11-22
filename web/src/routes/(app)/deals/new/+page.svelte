<script>
  import { t } from '$lib/i18n.svelte.js';
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { dealsApi, pipelinesApi } from '$lib/api/crm.js';
  import DealForm from '$lib/components/DealForm.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import { dealCreateBody } from '$lib/deals/fields.js';
  import { markWon, wonStageId } from '$lib/deals/won.js';
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
      // A sale to be closed needs a won stage, so find it first and create nothing if there is none.
      const wonId = draft.status === 'won' ? await wonStageId(pipelinesApi(api), workspaceId, draft.pipeline_id ?? '') : '';
      const deal = /** @type {{ id: string, title: string }} */ (await deals.create(workspaceId, dealCreateBody(draft)));
      if (wonId) {
        // The API starts a deal in an open stage; the sale is closed by moving it.
        try {
          await markWon(deals, workspaceId, deal.id, wonId);
          showToast(t('Added {name} as won', { name: deal.title }));
        } catch (err) {
          showToast(`${t('Added {name}, but it could not be marked as won. Move it to Won from its page.', { name: deal.title })} ${toSentence(err instanceof Error ? err.message : String(err))}`, 6000);
        }
      } else {
        showToast(t('Added {name}', { name: deal.title }));
      }
      await goto(`/deals/${deal.id}`, { replaceState: true });
    } catch (err) {
      formError = isMissing(err)
        ? t('The pipeline, stage, company, contact, or owner no longer exists. Choose another one.')
        : toSentence(err instanceof Error ? err.message : String(err));
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>{t('Record a sale · CRM')}</title>
</svelte:head>

<div class="animate-in">
  <h1 class="page-title">{t('Record a sale')}</h1>
  <p class="page-lede">{t('A title and a value are enough. It is recorded as a sale already made; choose In progress to follow it on the board.')}</p>
  {#await pipelines}
    <div aria-busy="true">
      <p class="sr-only" role="status">{t('Loading the pipelines')}</p>
      <span class="skeleton" style="display:block;height:520px;border-radius:var(--radius)"></span>
    </div>
  {:then list}
    <DealForm creating {workspaceId} pipelines={list} submitLabel={t('Record sale')} busyLabel={t('Adding…')} {busy} {formError} {cancelHref} onsubmit={create} />
  {:catch err}
    <ErrorNotice message={err instanceof Error ? err.message : String(err)} onretry={() => location.reload()} />
  {/await}
</div>
