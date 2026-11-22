<script>
  import { dateLocale, t } from '$lib/i18n.svelte.js';
  import { session } from '$lib/session.svelte.js';
  import { workspaces } from '$lib/workspaces.svelte.js';

  const roleLabel = (/** @type {string} */ role) => ({ owner: t('Owner'), admin: t('Admin'), member: t('Member') })[/** @type {'owner'} */ (role)] ?? role;
  const date = { format: (/** @type {Date} */ date) => new Intl.DateTimeFormat(dateLocale(), { dateStyle: 'long' }).format(date) };

  let workspace = $derived(workspaces.current);
</script>

<svelte:head>
  <title>{workspace ? `${workspace.name} · ` : ''}CRM</title>
</svelte:head>

{#if workspace}
  <div class="animate-in">
    <h1 class="page-title">{workspace.name}</h1>
    <p class="page-lede">{t('Welcome, {name}.', { name: session.user?.name ?? '' })}</p>

    <section class="section-card" aria-labelledby="workspace-heading">
      <div class="section-card-header">
        <h2 id="workspace-heading" class="section-card-title">{t('Workspace')}</h2>
      </div>
      <dl class="section-card-body">
        <div class="field-row">
          <dt class="field-label">{t('Name')}</dt>
          <dd class="field-value">{workspace.name}</dd>
        </div>
        <div class="field-row">
          <dt class="field-label">{t('Your role')}</dt>
          <dd class="field-value">{roleLabel(workspace.role)}</dd>
        </div>
        <div class="field-row">
          <dt class="field-label">{t('Created')}</dt>
          <dd class="field-value">{date.format(new Date(workspace.created_at))}</dd>
        </div>
      </dl>
    </section>

    <section class="section-card" aria-labelledby="next-heading">
      <div class="section-card-header">
        <h2 id="next-heading" class="section-card-title">{t('Coming next')}</h2>
      </div>
      <p class="section-card-body upcoming">
        {t('Find the people and organizations {name} works with under', { name: workspace.name })} <a href="/contacts">{t('Contacts')}</a> {t('and')}
        <a href="/companies">{t('Companies')}</a>. {t('The deal pipeline and activities will open from here. Use the workspace menu at the top to switch between teams.')}
      </p>
    </section>
  </div>
{/if}

<style>
  .upcoming {
    max-width: 65ch;
    color: var(--text2);
    font-size: 0.9375rem;
  }
</style>
