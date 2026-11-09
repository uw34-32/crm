<script>
  import { session } from '$lib/session.svelte.js';
  import { workspaces } from '$lib/workspaces.svelte.js';

  const roles = { owner: 'Owner', admin: 'Admin', member: 'Member' };
  const date = new Intl.DateTimeFormat(undefined, { dateStyle: 'long' });

  let workspace = $derived(workspaces.current);
</script>

<svelte:head>
  <title>{workspace ? `${workspace.name} · ` : ''}CRM</title>
</svelte:head>

{#if workspace}
  <div class="animate-in">
    <h1 class="page-title">{workspace.name}</h1>
    <p class="page-lede">Welcome, {session.user?.name}.</p>

    <section class="section-card" aria-labelledby="workspace-heading">
      <div class="section-card-header">
        <h2 id="workspace-heading" class="section-card-title">Workspace</h2>
      </div>
      <dl class="section-card-body">
        <div class="field-row">
          <dt class="field-label">Name</dt>
          <dd class="field-value">{workspace.name}</dd>
        </div>
        <div class="field-row">
          <dt class="field-label">Your role</dt>
          <dd class="field-value">{roles[workspace.role] ?? workspace.role}</dd>
        </div>
        <div class="field-row">
          <dt class="field-label">Created</dt>
          <dd class="field-value">{date.format(new Date(workspace.created_at))}</dd>
        </div>
      </dl>
    </section>

    <section class="section-card" aria-labelledby="next-heading">
      <div class="section-card-header">
        <h2 id="next-heading" class="section-card-title">Coming next</h2>
      </div>
      <p class="section-card-body upcoming">
        Contacts, companies, the deal pipeline, and activities for {workspace.name} will open from here. Use the
        workspace menu at the top to switch between teams.
      </p>
    </section>
  </div>
{/if}

<style>
  .page-title {
    font-size: 1.5rem;
    font-weight: 700;
    line-height: 1.25;
    letter-spacing: -0.3px;
    overflow-wrap: anywhere;
  }

  .page-lede {
    margin: 4px 0 20px;
    color: var(--text2);
    font-size: 0.9375rem;
  }

  .upcoming {
    max-width: 65ch;
    color: var(--text2);
    font-size: 0.9375rem;
  }
</style>
