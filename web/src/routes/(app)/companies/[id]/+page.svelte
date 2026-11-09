<script>
  import { ArrowLeft, Pencil, Plus, Trash2 } from '@lucide/svelte';
  import { untrack } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import NotFound from '$lib/components/NotFound.svelte';
  import RecordAvatar from '$lib/components/RecordAvatar.svelte';
  import { recordsApi } from '$lib/api/records.js';
  import { toSentence } from '$lib/forms.js';
  import { contactName, initials } from '$lib/records/format.js';
  import { createPager, emptyList } from '$lib/records/list.js';
  import { isMissing } from '$lib/records/load.js';
  import { recordScreen } from '$lib/records/screens.svelte.js';
  import { api } from '$lib/session.svelte.js';
  import { showToast } from '$lib/toast.svelte.js';

  const companies = recordsApi(api, 'companies');
  const contactsApi = recordsApi(api, 'contacts');
  const id = $derived(page.params.id ?? '');
  const screen = recordScreen('companies', companies, () => id);
  const company = $derived(screen.state.status === 'ready' ? screen.state.record : null);
  const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' });

  // The contacts API has no company filter, so this searches for the company's name,
  // which every linked contact matches, and keeps the ones linked to this company.
  const people = $state(/** @type {import('$lib/records/list.js').ListState<any>} */ (emptyList()));
  const pager = createPager(people, (params) => contactsApi.list(screen.workspaceId, params), {
    keep: (contact) => contact.company_id === company?.id
  });

  // A common name matches many people at other companies, so look a few pages ahead
  // for this company's people before asking the reader to look further.
  const LOOKAHEAD_PAGES = 4;
  const ENOUGH_PEOPLE = 10;

  /** @param {string} name */
  async function loadPeople(name) {
    await pager.load(name);
    for (let round = 1; round < LOOKAHEAD_PAGES; round++) {
      if (company?.name !== name || people.q !== name) return;
      if (people.status !== 'ready' || !people.hasMore || people.moreError || people.items.length >= ENOUGH_PEOPLE) return;
      await pager.more();
    }
  }

  $effect(() => {
    const name = company?.name;
    if (name) untrack(() => loadPeople(name));
  });

  $effect(() => () => pager.stop());

  let confirming = $state(false);
  let deleting = $state(false);
  let deleteError = $state('');

  async function remove() {
    if (!company) return;
    deleting = true;
    deleteError = '';
    try {
      await companies.remove(screen.workspaceId, company.id);
    } catch (err) {
      if (!isMissing(err)) {
        deleteError = toSentence(err instanceof Error ? err.message : String(err));
        deleting = false;
        return;
      }
    }
    confirming = false;
    showToast(`Deleted ${company.name}`);
    await goto(screen.backHref(), { replaceState: true });
  }

  /** @param {string} domain */
  const siteOf = (domain) => `https://${domain}`;
</script>

<svelte:head>
  <title>{company?.name ?? 'Company'} · CRM</title>
</svelte:head>

{#if screen.state.status === 'missing'}
  <NotFound title="Company not found" backHref={screen.backHref()} backLabel="Back to companies" />
{:else}
  <div class="detail-header">
    <a class="btn-ghost" href={screen.backHref()}><ArrowLeft size={18} aria-hidden="true" /> Companies</a>
    {#if company}
      <div class="detail-actions">
        <a class="btn-ghost" href="/companies/{company.id}/edit"><Pencil size={18} aria-hidden="true" /> Edit</a>
        <button type="button" class="btn-ghost danger" onclick={() => (confirming = true)}>
          <Trash2 size={18} aria-hidden="true" /> Delete
        </button>
      </div>
    {/if}
  </div>

  {#if screen.state.status === 'error'}
    <h1 class="page-title">Company</h1>
    <ErrorNotice message={screen.state.error} onretry={screen.reload} />
  {:else if company}
    <div class="animate-in">
      <div class="detail-hero">
        <RecordAvatar name={company.name} size={64} shape="organization" />
        <div class="detail-hero-text">
          <h1 class="page-title">{company.name}</h1>
          {#if company.industry}<p class="detail-subtitle">{company.industry}</p>{/if}
        </div>
      </div>

      <section class="section-card" aria-labelledby="company-details">
        <div class="section-card-header">
          <h2 id="company-details" class="section-card-title">Details</h2>
        </div>
        {#if company.domain || company.phone || company.address}
          <dl class="section-card-body">
            {#if company.domain}
              <div class="field-row">
                <dt class="field-label">Website</dt>
                <dd class="field-value"><a href={siteOf(company.domain)} target="_blank" rel="noopener noreferrer">{company.domain}</a></dd>
              </div>
            {/if}
            {#if company.phone}
              <div class="field-row">
                <dt class="field-label">Phone</dt>
                <dd class="field-value"><a href="tel:{company.phone}">{company.phone}</a></dd>
              </div>
            {/if}
            {#if company.address}
              <div class="field-row">
                <dt class="field-label">Address</dt>
                <dd class="field-value address">{company.address}</dd>
              </div>
            {/if}
          </dl>
        {:else}
          <p class="section-card-body muted">No website, phone, or address yet. <a href="/companies/{company.id}/edit">Add details</a></p>
        {/if}
      </section>

      <section class="section-card" aria-labelledby="company-people">
        <div class="section-card-header">
          <h2 id="company-people" class="section-card-title">People</h2>
          <a class="card-action" href="/contacts/new?company={company.id}"><Plus size={16} aria-hidden="true" /> Add a person</a>
        </div>
        {#if people.status === 'error'}
          <div class="section-card-body"><ErrorNotice message={people.error} onretry={() => loadPeople(company.name)} /></div>
        {:else if people.status !== 'ready'}
          <div class="section-card-body" aria-busy="true">
            <p class="sr-only" role="status">Loading the people at {company.name}</p>
            <span class="skeleton skeleton-line" style="display:block;width:50%;margin-bottom:10px"></span>
            <span class="skeleton skeleton-line" style="display:block;width:35%"></span>
          </div>
        {:else if people.items.length === 0 && !people.hasMore}
          <p class="section-card-body muted">Nobody is linked to {company.name} yet.</p>
        {:else}
          {#if people.items.length === 0}
            <p class="section-card-body muted">Nobody linked to {company.name} has turned up yet.</p>
          {/if}
          <ul class="people">
            {#each people.items as person (person.id)}
              <li>
                <a class="person" href="/contacts/{person.id}">
                  <span class="person-initials" aria-hidden="true">{initials(contactName(person))}</span>
                  <span class="person-text">
                    <span class="person-name">{contactName(person)}</span>
                    {#if person.title}<span class="person-title">{person.title}</span>{/if}
                  </span>
                </a>
              </li>
            {/each}
          </ul>
          {#if people.moreError}
            <div class="section-card-body"><ErrorNotice message={people.moreError} /></div>
          {/if}
          {#if people.hasMore}
            <div class="section-card-body more">
              <button type="button" class="btn btn-secondary" onclick={pager.more} disabled={people.loadingMore} aria-busy={people.loadingMore}>
                {people.loadingMore ? 'Loading…' : 'Look for more people'}
              </button>
            </div>
          {/if}
        {/if}
      </section>

      <p class="detail-meta">Added {date.format(new Date(company.created_at))} · Updated {date.format(new Date(company.updated_at))}</p>
    </div>
  {:else}
    <div aria-busy="true">
      <p class="sr-only" role="status">Loading the company</p>
      <div class="detail-hero">
        <span class="skeleton" style="width:64px;height:64px;border-radius:16px"></span>
        <span class="skeleton" style="width:200px;height:28px"></span>
      </div>
      <span class="skeleton" style="display:block;height:140px;border-radius:var(--radius)"></span>
    </div>
  {/if}
{/if}

<ConfirmDialog
  open={confirming}
  title="Delete {company?.name ?? 'this company'}?"
  message="The people linked to it stay in your contacts and lose the link. Deals keep their history without the company."
  confirmLabel="Delete company"
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

  .address {
    white-space: pre-line;
  }

  .card-action {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-height: 44px;
    padding: 8px 12px;
    margin: -8px -4px;
    border-radius: 9px;
    color: var(--accent-text);
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .card-action:hover {
    background: var(--surface2);
    text-decoration: none;
  }

  .people {
    list-style: none;
  }

  .people li + li {
    border-top: 1px solid var(--border);
  }

  .person {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 56px;
    padding: 8px 16px;
    color: inherit;
  }

  .person:hover {
    background: var(--surface2);
    text-decoration: none;
  }

  .person-initials {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    background: var(--surface2);
    color: var(--accent-text);
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .person-text {
    min-width: 0;
  }

  .person-name,
  .person-title {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .person-name {
    font-weight: 500;
  }

  .person-title {
    font-size: 0.875rem;
    color: var(--text2);
  }

  .more {
    border-top: 1px solid var(--border);
    text-align: center;
  }
</style>
