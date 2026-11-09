<script>
  import { goto } from '$app/navigation';
  import { onMount, tick } from 'svelte';
  import ErrorNotice from '$lib/components/ErrorNotice.svelte';
  import Field from '$lib/components/Field.svelte';
  import { checkName, toSentence } from '$lib/forms.js';
  import { showToast } from '$lib/toast.svelte.js';
  import { createWorkspace, workspaces } from '$lib/workspaces.svelte.js';

  let name = $state('');
  let error = $state('');
  let formError = $state('');
  let busy = $state(false);
  /** @type {HTMLElement | null} */
  let notice = $state(null);

  // Read once: creating the first workspace must not turn this into the other page.
  const first = workspaces.list.length === 0;

  onMount(() => document.getElementById('workspace-name')?.focus());

  /** @param {SubmitEvent} event */
  async function submit(event) {
    event.preventDefault();
    formError = '';
    error = checkName(name, 'Enter a name for the workspace.');
    if (error) {
      document.getElementById('workspace-name')?.focus();
      return;
    }
    busy = true;
    try {
      const workspace = await createWorkspace(name.trim());
      showToast(`Created ${workspace.name}`);
      await goto('/dashboard');
    } catch (err) {
      formError = toSentence(err instanceof Error ? err.message : String(err));
      await tick();
      notice?.focus();
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>{first ? 'Create your first workspace' : 'New workspace'} · CRM</title>
</svelte:head>

<div class="new-workspace animate-in">
  <h1 class="page-title">{first ? 'Create your first workspace' : 'New workspace'}</h1>
  <p class="page-lede">
    {first
      ? 'A workspace holds the contacts, companies, and deals your team shares. You will be its owner.'
      : 'You will be its owner, and you can switch between workspaces from the top bar.'}
  </p>

  {#if formError}
    <ErrorNotice message={formError} bind:element={notice} />
  {/if}

  <form class="section-card section-card-body" novalidate onsubmit={submit}>
    <Field
      id="workspace-name"
      label="Workspace name"
      autocomplete="organization"
      required
      hint="Usually your company or team name."
      bind:value={name}
      {error}
    />
    <div class="actions">
      {#if !first}
        <a class="btn btn-secondary" href="/dashboard">Cancel</a>
      {/if}
      <button class="btn btn-primary" type="submit" disabled={busy} aria-busy={busy}>
        {busy ? 'Creating…' : 'Create workspace'}
      </button>
    </div>
  </form>
</div>

<style>
  .new-workspace {
    max-width: 480px;
  }

  .page-title {
    font-size: 1.5rem;
    font-weight: 700;
    line-height: 1.25;
    letter-spacing: -0.3px;
  }

  .page-lede {
    margin: 4px 0 20px;
    color: var(--text2);
    font-size: 0.9375rem;
    max-width: 65ch;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    margin-top: 8px;
  }

  @media (max-width: 400px) {
    .actions > :global(*) {
      flex: 1;
    }
  }
</style>
