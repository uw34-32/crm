<script>
  import { t } from '$lib/i18n.svelte.js';
  import { Building2, Plus } from '@lucide/svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import ListSkeleton from '$lib/components/ListSkeleton.svelte';
  import RecordAvatar from '$lib/components/RecordAvatar.svelte';
  import SearchField from '$lib/components/SearchField.svelte';
  import { recordsApi } from '$lib/api/records.js';
  import { listScreen } from '$lib/records/screens.svelte.js';
  import { api } from '$lib/session.svelte.js';
  import { workspaces } from '$lib/workspaces.svelte.js';

  const screen = listScreen('companies', recordsApi(api, 'companies'));
  const list = screen.list;

  /** @param {{ domain: string, industry: string }} company */
  const detailOf = (company) => [company.domain, company.industry].filter(Boolean).join(' · ');
</script>

<svelte:head>
  <title>{t('Companies')} · {workspaces.current?.name ?? 'CRM'}</title>
</svelte:head>

<div class="animate-in">
  <div class="page-header">
    <h1 class="page-title">{t('Companies')}</h1>
    <a class="btn btn-primary" href="/companies/new"><Plus size={18} aria-hidden="true" /> {t('New company')}</a>
  </div>

  <SearchField
    id="company-search"
    label={t('Search companies')}
    placeholder={t('Search by name or domain')}
    value={screen.view.search}
    oninput={screen.setSearch}
  />

  {#if list.status === 'error'}
    <ErrorNotice message={list.error} onretry={screen.retry} />
  {:else if list.status !== 'ready' && list.items.length === 0}
    <ListSkeleton label={t('Loading companies')} shape="organization" />
  {:else if list.items.length === 0 && list.q}
    <div class="empty-state">
      <p class="empty-state-title" role="status">{t('No companies match “{q}”', { q: list.q })}</p>
      <p class="empty-state-text">{t('Search looks at company names and domains.')}</p>
      <button type="button" class="btn btn-secondary" onclick={screen.clearSearch}>{t('Clear search')}</button>
    </div>
  {:else if list.items.length === 0}
    <div class="empty-state">
      <p class="empty-state-title">{t('No companies yet')}</p>
      <p class="empty-state-text">{t('Add the organizations your team works with, then link people to them.')}</p>
      <a class="btn btn-primary" href="/companies/new"><Building2 size={18} aria-hidden="true" /> {t('Add the first company')}</a>
    </div>
  {:else}
    <p class="sr-only" role="status">
      {list.items.length}
      {list.items.length === 1 ? 'company' : 'companies'}{list.hasMore ? ', more to load' : ''}{list.q ? ` matching ${list.q}` : ''}
    </p>
    <ul class="record-list" aria-label={t('Companies')} aria-busy={list.status === 'loading'}>
      {#each list.items as company (company.id)}
        <li>
          <a class="list-item" href="/companies/{company.id}">
            <RecordAvatar name={company.name} shape="organization" />
            <span class="list-item-content">
              <span class="list-item-name">{company.name}</span>
              {#if detailOf(company)}
                <span class="list-item-detail">{detailOf(company)}</span>
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
          {list.loadingMore ? 'Loading…' : 'Load more companies'}
        </button>
      {/if}
      <p class="list-count">{list.items.length === 1 ? t('Showing 1 company') : t('Showing {n} companies', { n: list.items.length })}</p>
    </div>
  {/if}
</div>
