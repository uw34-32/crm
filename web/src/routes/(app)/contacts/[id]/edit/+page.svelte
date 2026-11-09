<script>
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import ContactForm from '$lib/components/ContactForm.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import NotFound from '$lib/components/NotFound.svelte';
  import { recordsApi } from '$lib/api/records.js';
  import { toSentence } from '$lib/forms.js';
  import { contactPatchBody } from '$lib/records/fields.js';
  import { contactName } from '$lib/records/format.js';
  import { isMissing } from '$lib/records/load.js';
  import { recordScreen } from '$lib/records/screens.svelte.js';
  import { api } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';

  const contacts = recordsApi(api, 'contacts');
  const id = $derived(page.params.id ?? '');
  const screen = recordScreen('contacts', contacts, () => id);
  let busy = $state(false);
  let formError = $state('');

  /** @param {import('$lib/records/fields.js').ContactDraft} draft */
  async function save(draft) {
    const body = contactPatchBody(screen.state.record, draft);
    if (Object.keys(body).length === 0) {
      await goto(`/contacts/${id}`);
      return;
    }
    busy = true;
    formError = '';
    try {
      const contact = /** @type {any} */ (await contacts.update(screen.workspaceId, id, body));
      showToast(`Saved ${contactName(contact)}`);
      await goto(`/contacts/${id}`, { replaceState: true });
    } catch (err) {
      formError = isMissing(err)
        ? 'This contact or the chosen company no longer exists. Someone may have deleted it.'
        : toSentence(err instanceof Error ? err.message : String(err));
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>Edit {screen.state.record ? contactName(screen.state.record) : 'contact'} · CRM</title>
</svelte:head>

{#if screen.state.status === 'missing'}
  <NotFound title="Contact not found" backHref={screen.backHref()} backLabel="Back to contacts" />
{:else if screen.state.status === 'error'}
  <h1 class="page-title">Edit contact</h1>
  <ErrorNotice message={screen.state.error} onretry={screen.reload} />
{:else if screen.state.status === 'ready'}
  <div class="animate-in">
    <h1 class="page-title">Edit {contactName(screen.state.record)}</h1>
    <p class="page-lede">Clear a field to remove it.</p>
    {#key screen.state.record.id}
      <ContactForm
        initial={screen.state.record}
        workspaceId={screen.workspaceId}
        submitLabel="Save changes"
        busyLabel="Saving…"
        {busy}
        {formError}
        cancelHref="/contacts/{id}"
        onsubmit={save}
      />
    {/key}
  </div>
{:else}
  <div aria-busy="true">
    <p class="sr-only" role="status">Loading the contact</p>
    <span class="skeleton" style="display:block;width:50%;height:28px;margin-bottom:20px"></span>
    <span class="skeleton" style="display:block;height:420px;border-radius:var(--radius)"></span>
  </div>
{/if}
