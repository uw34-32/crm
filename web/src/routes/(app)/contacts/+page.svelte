<script>
  import { t } from '$lib/i18n.svelte.js';
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
  <title>{t('Contacts')} · {workspaces.current?.name ?? 'CRM'}</title>
</svelte:head>

<div class="animate-in">
  <div class="page-header">
    <h1 class="page-title">{t('Contacts')}</h1>
    <a class="btn btn-primary" href="/contacts/new"><Plus size={18} aria-hidden="true" /> {t('New contact')}</a>
  </div>

  <SearchField
    id="contact-search"
    label={t('Search contacts')}
    placeholder={t('Search by name, email, or company')}
    value={screen.view.search}
    oninput={screen.setSearch}
  />

  {#if list.status === 'error'}
    <ErrorNotice message={list.error} onretry={screen.retry} />
  {:else if list.status !== 'ready' && list.items.length === 0}
    <ListSkeleton label={t('Loading contacts')} />
  {:else if list.items.length === 0 && list.q}
    <div class="empty-state">
      <p class="empty-state-title" role="status">{t('No contacts match “{q}”', { q: list.q })}</p>
      <p class="empty-state-text">{t('Search looks at names, email addresses, and the company each person works at.')}</p>
      <button type="button" class="btn btn-secondary" onclick={screen.clearSearch}>{t('Clear search')}</button>
    </div>
  {:else if list.items.length === 0}
    <div class="empty-state">
      <p class="empty-state-title">{t('No contacts yet')}</p>
      <p class="empty-state-text">{t('Add the people your team talks to. Everyone in {name} will see them.', { name: workspaces.current?.name ?? t('the workspace') })}</p>
      <a class="btn btn-primary" href="/contacts/new"><UserPlus size={18} aria-hidden="true" /> {t('Add the first contact')}</a>
    </div>
  {:else}
    <p class="sr-only" role="status">
      {list.items.length}
      {list.items.length === 1 ? 'contact' : 'contacts'}{list.hasMore ? ', more to load' : ''}{list.q ? ` matching ${list.q}` : ''}
    </p>
    <ul class="record-list" aria-label={t('Contacts')} aria-busy={list.status === 'loading'}>
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
      <p class="list-count">{list.items.length === 1 ? t('Showing 1 contact') : t('Showing {n} contacts', { n: list.items.length })}</p>
    </div>
  {/if}
</div>
