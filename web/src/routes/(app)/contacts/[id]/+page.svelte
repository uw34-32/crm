<script>
  import { ArrowLeft, Pencil, Trash2 } from '@lucide/svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import NotFound from '$lib/components/NotFound.svelte';
  import RecordAvatar from '$lib/components/RecordAvatar.svelte';
  import { recordsApi } from '$lib/api/records.js';
  import { toSentence } from '$lib/forms.js';
  import { contactName } from '$lib/records/format.js';
  import { isMissing } from '$lib/records/load.js';
  import { recordScreen } from '$lib/records/screens.svelte.js';
  import { api } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';

  const contacts = recordsApi(api, 'contacts');
  const id = $derived(page.params.id ?? '');
  const screen = recordScreen('contacts', contacts, () => id);
  const contact = $derived(screen.state.status === 'ready' ? screen.state.record : null);
  const name = $derived(contact ? contactName(contact) : '');
  const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' });

  let confirming = $state(false);
  let deleting = $state(false);
  let deleteError = $state('');

  async function remove() {
    if (!contact) return;
    deleting = true;
    deleteError = '';
    try {
      await contacts.remove(screen.workspaceId, contact.id);
    } catch (err) {
      // Already gone is what was asked for.
      if (!isMissing(err)) {
        deleteError = toSentence(err instanceof Error ? err.message : String(err));
        deleting = false;
        return;
      }
    }
    confirming = false;
    showToast(`Deleted ${name}`);
    await goto(screen.backHref(), { replaceState: true });
  }
</script>

<svelte:head>
  <title>{name || 'Contact'} · CRM</title>
</svelte:head>

{#if screen.state.status === 'missing'}
  <NotFound title="Contact not found" backHref={screen.backHref()} backLabel="Back to contacts" />
{:else}
  <div class="detail-header">
    <a class="btn-ghost" href={screen.backHref()}><ArrowLeft size={18} aria-hidden="true" /> Contacts</a>
    {#if contact}
      <div class="detail-actions">
        <a class="btn-ghost" href="/contacts/{contact.id}/edit"><Pencil size={18} aria-hidden="true" /> Edit</a>
        <button type="button" class="btn-ghost danger" onclick={() => (confirming = true)}>
          <Trash2 size={18} aria-hidden="true" /> Delete
        </button>
      </div>
    {/if}
  </div>

  {#if screen.state.status === 'error'}
    <h1 class="page-title">Contact</h1>
    <ErrorNotice message={screen.state.error} onretry={screen.reload} />
  {:else if contact}
    <div class="animate-in">
      <div class="detail-hero">
        <RecordAvatar {name} size={64} />
        <div class="detail-hero-text">
          <h1 class="page-title">{name}</h1>
          {#if contact.title || contact.company_name}
            <p class="detail-subtitle">
              {contact.title}{#if contact.title && contact.company_name}{' at '}{/if}{#if contact.company_name}<a
                  href="/companies/{contact.company_id}">{contact.company_name}</a
                >{/if}
            </p>
          {/if}
        </div>
      </div>

      <section class="section-card" aria-labelledby="contact-details">
        <div class="section-card-header">
          <h2 id="contact-details" class="section-card-title">Details</h2>
        </div>
        {#if contact.email || contact.phone || contact.company_name}
          <dl class="section-card-body">
            {#if contact.email}
              <div class="field-row">
                <dt class="field-label">Email</dt>
                <dd class="field-value"><a href="mailto:{contact.email}">{contact.email}</a></dd>
              </div>
            {/if}
            {#if contact.phone}
              <div class="field-row">
                <dt class="field-label">Phone</dt>
                <dd class="field-value"><a href="tel:{contact.phone}">{contact.phone}</a></dd>
              </div>
            {/if}
            {#if contact.company_name}
              <div class="field-row">
                <dt class="field-label">Company</dt>
                <dd class="field-value"><a href="/companies/{contact.company_id}">{contact.company_name}</a></dd>
              </div>
            {/if}
          </dl>
        {:else}
          <p class="section-card-body muted">No email, phone, or company yet. <a href="/contacts/{contact.id}/edit">Add details</a></p>
        {/if}
      </section>

      <p class="detail-meta">Added {date.format(new Date(contact.created_at))} · Updated {date.format(new Date(contact.updated_at))}</p>
    </div>
  {:else}
    <div aria-busy="true">
      <p class="sr-only" role="status">Loading the contact</p>
      <div class="detail-hero">
        <span class="skeleton skeleton-circle" style="width:64px;height:64px"></span>
        <span class="skeleton" style="width:200px;height:28px"></span>
      </div>
      <span class="skeleton" style="display:block;height:140px;border-radius:var(--radius)"></span>
    </div>
  {/if}
{/if}

<ConfirmDialog
  open={confirming}
  title="Delete {name || 'this contact'}?"
  message="They leave the contact list for everyone in the workspace. Deals keep their history without the person."
  confirmLabel="Delete contact"
  busyLabel="Deleting…"
  busy={deleting}
  error={deleteError}
  onconfirm={remove}
  oncancel={() => {
    confirming = false;
    deleteError = '';
  }}
/>

<style>
  .muted {
    color: var(--text2);
    font-size: 0.9375rem;
  }
</style>
