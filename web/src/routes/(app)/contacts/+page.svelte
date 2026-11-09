<script>
  import { Plus, UserPlus } from '@lucide/svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import ListSkeleton from '$lib/components/ListSkeleton.svelte';
  import RecordAvatar from '$lib/components/RecordAvatar.svelte';
  import SearchField from '$lib/components/SearchField.svelte';
  import { recordsApi } from '$lib/api/records.js';
  import { contactName, contactSubtitle } from '$lib/records/format.js';
  import { listScreen } from '$lib/records/screens.svelte.js';
  import { api } from '$lib/session.svelte.js';
  import { workspaces } from '$lib/workspaces.svelte.js';

  const screen = listScreen('contacts', recordsApi(api, 'contacts'));
  const list = screen.list;

  /** @param {any} contact */
  const detailOf = (contact) => contactSubtitle(contact) || contact.email || contact.phone;
</script>

<svelte:head>
  <title>Contacts · {workspaces.current?.name ?? 'CRM'}</title>
</svelte:head>

<div class="animate-in">
  <div class="page-header">
    <h1 class="page-title">Contacts</h1>
    <a class="btn btn-primary" href="/contacts/new"><Plus size={18} aria-hidden="true" /> New contact</a>
  </div>

  <SearchField
    id="contact-search"
    label="Search contacts"
    placeholder="Search by name, email, or company"
    value={screen.view.search}
    oninput={screen.setSearch}
  />

  {#if list.status === 'error'}
    <ErrorNotice message={list.error} onretry={screen.retry} />
  {:else if list.status !== 'ready' && list.items.length === 0}
    <ListSkeleton label="Loading contacts" />
  {:else if list.items.length === 0 && list.q}
    <div class="empty-state">
      <p class="empty-state-title" role="status">No contacts match “{list.q}”</p>
      <p class="empty-state-text">Search looks at names, email addresses, and the company each person works at.</p>
      <button type="button" class="btn btn-secondary" onclick={screen.clearSearch}>Clear search</button>
    </div>
  {:else if list.items.length === 0}
    <div class="empty-state">
      <p class="empty-state-title">No contacts yet</p>
      <p class="empty-state-text">Add the people your team talks to. Everyone in {workspaces.current?.name ?? 'the workspace'} will see them.</p>
      <a class="btn btn-primary" href="/contacts/new"><UserPlus size={18} aria-hidden="true" /> Add the first contact</a>
    </div>
  {:else}
    <p class="sr-only" role="status">
      {list.items.length}
      {list.items.length === 1 ? 'contact' : 'contacts'}{list.hasMore ? ', more to load' : ''}{list.q ? ` matching ${list.q}` : ''}
    </p>
    <ul class="record-list" aria-label="Contacts" aria-busy={list.status === 'loading'}>
      {#each list.items as contact (contact.id)}
        <li>
          <a class="list-item" href="/contacts/{contact.id}">
            <RecordAvatar name={contactName(contact)} />
            <span class="list-item-content">
              <span class="list-item-name">{contactName(contact)}</span>
              {#if detailOf(contact)}
                <span class="list-item-detail">{detailOf(contact)}</span>
              {/if}
            </span>
          </a>
        </li>
      {/each}
    </ul>
    <div class="list-footer">
      {#if list.moreError}
        <ErrorNotice message={list.moreError} />
      {/if}
      {#if list.hasMore}
        <button type="button" class="btn btn-secondary" onclick={screen.more} disabled={list.loadingMore} aria-busy={list.loadingMore}>
          {list.loadingMore ? 'Loading…' : 'Load more contacts'}
        </button>
      {/if}
      <p class="list-count">Showing {list.items.length} {list.items.length === 1 ? 'contact' : 'contacts'}</p>
    </div>
  {/if}
</div>
